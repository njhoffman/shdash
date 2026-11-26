package collect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"time"

	"github.com/njhoffman/shdash/internal/format"
)

// PackagesData is, per package manager, how many packages are installed and
// how many can be updated, from audit's export.
type PackagesData struct {
	GeneratedAt time.Time `json:"generated_at"`
	// CheckedAt is the oldest registry lookup among the managers, zero when
	// none has a registry.
	CheckedAt time.Time        `json:"checked_at,omitzero"`
	Managers  []ManagerSummary `json:"managers"`
}

// ManagerSummary counts one manager's packages. Updates are those audit
// found behind (a patch, minor or major gap, or an update a system manager
// offers); Major is how many of those are major; Unchecked are the ones with
// a registry that audit never looked up.
type ManagerSummary struct {
	Name      string `json:"name"`
	Installed int    `json:"installed"`
	Updates   int    `json:"updates"`
	Major     int    `json:"major"`
	Unchecked int    `json:"unchecked"`
}

// auditStale is the age of the registry data past which the panel
// highlights it.
const auditStale = 7 * 24 * time.Hour

// Packages reads the per-manager counts in audit's summary.json, or, from
// an audit that does not write them yet, counts packages.json; it never
// runs a package manager.
func Packages() Collector {
	return Collector{
		Name: "packages", Title: "Packages", Icon: "\U000F03D6", Priority: 5,
		TTL: 10 * time.Minute, Timeout: 10 * time.Second, Inline: true, Schema: 2,
		Collect: collectPackages,
		Lines:   linesOf(packagesLines),
	}
}

// managerCounts is a manager's entry in summary.json's packages block.
type managerCounts struct {
	Available bool `json:"available"`
	Installed int  `json:"installed"`
	Outdated  int  `json:"outdated"`
	Gaps      struct {
		Major int `json:"major"`
	} `json:"gaps"`
	Unchecked int       `json:"unchecked"`
	CheckedAt time.Time `json:"checked_at"`
}

func collectPackages(_ context.Context, env *Env) (any, error) {
	dir := AuditDir(env)
	d, err := packagesFromSummary(env, dir)
	if err != nil {
		return nil, err
	}
	if d == nil {
		if d, err = packagesFromExport(env, dir); err != nil {
			return nil, err
		}
	}
	sort.Slice(d.Managers, func(i, j int) bool { return d.Managers[i].Name < d.Managers[j].Name })
	return d, nil
}

// packagesFromSummary reads summary.json's packages block: nil, nil when
// the file or the block is missing.
func packagesFromSummary(env *Env, dir string) (*PackagesData, error) {
	b, err := env.ReadFile(filepath.Join(dir, "summary.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("audit summary.json: %w", err)
	}
	var x struct {
		GeneratedAt time.Time                `json:"generated_at"`
		Packages    map[string]managerCounts `json:"packages"`
	}
	if err := json.Unmarshal(b, &x); err != nil {
		return nil, fmt.Errorf("audit summary.json: %w", err)
	}
	if len(x.Packages) == 0 {
		return nil, nil
	}
	d := &PackagesData{GeneratedAt: x.GeneratedAt}
	for name, m := range x.Packages {
		if !m.Available || m.Installed == 0 {
			continue
		}
		d.add(ManagerSummary{Name: name, Installed: m.Installed, Updates: m.Outdated,
			Major: m.Gaps.Major, Unchecked: m.Unchecked}, m.CheckedAt)
	}
	return d, nil
}

// packagesFromExport counts packages.json's items, for an audit older than
// the summary's packages block.
func packagesFromExport(env *Env, dir string) (*PackagesData, error) {
	b, err := env.ReadFile(filepath.Join(dir, "packages.json"))
	if err != nil {
		return nil, fmt.Errorf("no audit packages export in %s (audit packages writes one)", env.Tilde(dir))
	}
	// Only the fields counted here; each item's registry metadata is skipped.
	var x struct {
		GeneratedAt time.Time `json:"generated_at"`
		Packages    []struct {
			Manager   string    `json:"manager"`
			Available bool      `json:"available"`
			Unchecked int       `json:"unchecked"`
			CheckedAt time.Time `json:"checked_at"`
			Items     []struct {
				Gap string `json:"gap"`
			} `json:"items"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(b, &x); err != nil {
		return nil, fmt.Errorf("audit packages.json: %w", err)
	}
	d := &PackagesData{GeneratedAt: x.GeneratedAt}
	for _, m := range x.Packages {
		if !m.Available || len(m.Items) == 0 {
			continue
		}
		s := ManagerSummary{Name: m.Manager, Installed: len(m.Items), Unchecked: m.Unchecked}
		for _, it := range m.Items {
			switch it.Gap {
			case "major":
				s.Major++
				s.Updates++
			case "minor", "patch", "update":
				s.Updates++
			}
		}
		d.add(s, m.CheckedAt)
	}
	return d, nil
}

// add appends s and keeps CheckedAt the oldest registry lookup.
func (d *PackagesData) add(s ManagerSummary, checked time.Time) {
	d.Managers = append(d.Managers, s)
	if !checked.IsZero() && (d.CheckedAt.IsZero() || checked.Before(d.CheckedAt)) {
		d.CheckedAt = checked
	}
}

// dimmed is s in the faint style, or "" for "" (so Join drops it).
func dimmed(s string) string {
	if s == "" {
		return ""
	}
	return format.Dim.Render(s)
}

func packagesLines(d *PackagesData, env *Env) []string {
	nameW, countW := 0, 0
	for _, m := range d.Managers {
		nameW = max(nameW, len(m.Name))
		countW = max(countW, len(fmt.Sprint(m.Installed)))
	}
	var l []string
	for _, m := range d.Managers {
		updates := format.Count(m.Updates, "update", "updates", false)
		if m.Major > 0 {
			updates += " " + format.Strong.Render(fmt.Sprintf("(%d major)", m.Major))
		}
		l = append(l, format.Dim.Render(format.Pad(m.Name, nameW))+" "+
			fmt.Sprintf("%*d", countW, m.Installed)+format.Dim.Render(" · ")+
			format.Join(updates, dimmed(format.Count(m.Unchecked, "unchecked", "unchecked", true))))
	}
	if len(d.Managers) == 0 {
		l = append(l, format.Dim.Render("no package managers in audit's export"))
	}
	// The registry lookups are what go stale; system managers have none,
	// so with only those the export's own time stands in.
	at := d.CheckedAt
	if at.IsZero() {
		at = d.GeneratedAt
	}
	age := env.Now().Sub(at)
	checked := "checked " + format.Age(age) + " ago (audit packages outdated)"
	if age > auditStale {
		l = append(l, format.Strong.Render(checked))
	} else {
		l = append(l, format.Dim.Render(checked))
	}
	return l
}
