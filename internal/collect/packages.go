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
	// ScannedAt is when audit's last packages scan finished, zero when the
	// export does not say (then GeneratedAt stands in).
	ScannedAt time.Time `json:"scanned_at,omitzero"`
	// CheckedAt is the oldest registry lookup among the managers, zero when
	// none has a registry.
	CheckedAt time.Time        `json:"checked_at,omitzero"`
	Managers  []ManagerSummary `json:"managers"`
}

// ManagerSummary counts one manager's packages. Updates are those audit
// found behind (a patch, minor or major gap, or an update a system manager
// offers); Major is how many of those are major; Unchecked are the ones with
// a registry that audit never looked up. Gaps and CarriedFrom are for the
// full-screen panel.
type ManagerSummary struct {
	Name      string `json:"name"`
	Installed int    `json:"installed"`
	Updates   int    `json:"updates"`
	Major     int    `json:"major"`
	Unchecked int    `json:"unchecked"`
	Gaps      Gaps   `json:"gaps"`
	// CarriedFrom is set when audit's last scan could not read this manager
	// and kept the one from that time.
	CarriedFrom time.Time `json:"carried_from,omitzero"`
}

// Gaps counts a manager's packages by how far they trail their latest
// release. Unknown are those whose versions do not compare (a commit, a
// local build) or that have no latest version.
type Gaps struct {
	Current int `json:"current"`
	Patch   int `json:"patch"`
	Minor   int `json:"minor"`
	Major   int `json:"major"`
	Update  int `json:"update"`
	Ahead   int `json:"ahead"`
	Unknown int `json:"unknown"`
}

// count adds one package with audit's gap g.
func (g *Gaps) count(gap string) {
	switch gap {
	case "current":
		g.Current++
	case "patch":
		g.Patch++
	case "minor":
		g.Minor++
	case "major":
		g.Major++
	case "update":
		g.Update++
	case "ahead":
		g.Ahead++
	default:
		g.Unknown++
	}
}

func (g Gaps) behind() int { return g.Patch + g.Minor + g.Major + g.Update }

// auditStale is the age of the registry data past which the panel
// highlights it.
const auditStale = 7 * 24 * time.Hour

// Packages reads the per-manager counts in audit's summary.json, or, from
// an audit that does not write them yet, counts packages.json; it never
// runs a package manager.
func Packages() Collector {
	lines, detail := capped(packagesLines)
	return Collector{
		Name: "packages", Title: "Packages", Icon: "\U000F03D6", Priority: 5,
		TTL: 10 * time.Minute, Timeout: 10 * time.Second, Inline: true, Schema: 4,
		Source:  auditSummary,
		Collect: collectPackages,
		Lines:   lines,
		Detail:  detail,
	}
}

// managerCounts is a manager's entry in summary.json's packages block.
type managerCounts struct {
	Available   bool      `json:"available"`
	Installed   int       `json:"installed"`
	Outdated    int       `json:"outdated"`
	Gaps        Gaps      `json:"gaps"`
	Unchecked   int       `json:"unchecked"`
	CheckedAt   time.Time `json:"checked_at"`
	CarriedFrom time.Time `json:"carried_from"`
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
		GeneratedAt time.Time `json:"generated_at"`
		Commands    []struct {
			Command string `json:"command"`
			LastRun *struct {
				FinishedAt time.Time `json:"finished_at"`
			} `json:"last_run"`
		} `json:"commands"`
		Packages map[string]managerCounts `json:"packages"`
	}
	if err := json.Unmarshal(b, &x); err != nil {
		return nil, fmt.Errorf("audit summary.json: %w", err)
	}
	if len(x.Packages) == 0 {
		return nil, nil
	}
	d := &PackagesData{GeneratedAt: x.GeneratedAt}
	for _, c := range x.Commands {
		if c.Command == "packages" && c.LastRun != nil {
			d.ScannedAt = c.LastRun.FinishedAt
		}
	}
	for name, m := range x.Packages {
		if !m.Available || m.Installed == 0 {
			continue
		}
		d.add(ManagerSummary{Name: name, Installed: m.Installed, Updates: m.Outdated,
			Major: m.Gaps.Major, Unchecked: m.Unchecked, Gaps: m.Gaps, CarriedFrom: m.CarriedFrom}, m.CheckedAt)
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
		Run         struct {
			FinishedAt time.Time `json:"finished_at"`
		} `json:"run"`
		Packages []struct {
			Manager     string    `json:"manager"`
			Available   bool      `json:"available"`
			Unchecked   int       `json:"unchecked"`
			CheckedAt   time.Time `json:"checked_at"`
			CarriedFrom time.Time `json:"carried_from"`
			Items       []struct {
				Gap string `json:"gap"`
			} `json:"items"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(b, &x); err != nil {
		return nil, fmt.Errorf("audit packages.json: %w", err)
	}
	d := &PackagesData{GeneratedAt: x.GeneratedAt, ScannedAt: x.Run.FinishedAt}
	for _, m := range x.Packages {
		if !m.Available || len(m.Items) == 0 {
			continue
		}
		s := ManagerSummary{Name: m.Manager, Installed: len(m.Items), Unchecked: m.Unchecked,
			CarriedFrom: m.CarriedFrom}
		for _, it := range m.Items {
			s.Gaps.count(it.Gap)
		}
		s.Updates, s.Major = s.Gaps.behind(), s.Gaps.Major
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

// packagesLines is one row per manager and the footer; full adds, under
// each manager, its packages by gap, its unchecked count and whether audit
// kept an earlier scan of it.
func packagesLines(d *PackagesData, env *Env, full bool) []string {
	nameW, countW, unchecked := 0, 0, 0
	for _, m := range d.Managers {
		nameW = max(nameW, len(m.Name))
		countW = max(countW, len(fmt.Sprint(m.Installed)))
		unchecked += m.Unchecked
	}
	var l []string
	for _, m := range d.Managers {
		updates := format.Count(m.Updates, "update", "updates", false)
		if m.Major > 0 {
			updates += " " + format.Strong.Render(fmt.Sprintf("(%d major)", m.Major))
		}
		l = append(l, format.Dim.Render(format.Pad(m.Name, nameW))+" "+
			fmt.Sprintf("%*d", countW, m.Installed)+format.Dim.Render(" · ")+updates)
		if full {
			l = append(l, gapLines(m, env)...)
		}
	}
	if len(d.Managers) == 0 {
		l = append(l, format.Dim.Render("no package managers in audit's export"))
	}
	scanned := d.ScannedAt
	if scanned.IsZero() {
		scanned = d.GeneratedAt
	}
	l = append(l, format.Join(
		format.Dim.Render("scanned "+format.Age(env.Now().Sub(scanned))+" ago"),
		dimmed(format.Count(unchecked, "unchecked", "unchecked", true))))
	// The registry lookups are what go stale, and only `audit packages
	// outdated` redoes them; system managers have none.
	if !d.CheckedAt.IsZero() {
		age := env.Now().Sub(d.CheckedAt)
		checked := "registry checked " + format.Age(age) + " ago"
		if age > auditStale {
			l = append(l, format.Strong.Render(checked), format.Dim.Render("  audit packages outdated"))
		} else {
			l = append(l, format.Dim.Render(checked))
		}
	}
	return l
}

// gapLines are a manager's rows under it in the full-screen panel.
func gapLines(m ManagerSummary, env *Env) []string {
	g := m.Gaps
	by := format.Join(
		format.Count(g.Current, "current", "current", true),
		format.Count(g.Patch, "patch", "patch", true),
		format.Count(g.Minor, "minor", "minor", true),
		highlight(g.Major > 0, format.Count(g.Major, "major", "major", true)),
		format.Count(g.Update, "update", "updates", true),
		format.Count(g.Ahead, "ahead", "ahead", true),
		format.Count(g.Unknown, "unknown", "unknown", true),
		format.Count(m.Unchecked, "unchecked", "unchecked", true))
	var l []string
	if by != "" {
		l = append(l, "  "+by)
	}
	if !m.CarriedFrom.IsZero() {
		l = append(l, "  "+format.Strong.Render("not read; kept the scan from "+
			format.Age(env.Now().Sub(m.CarriedFrom))+" ago"))
	}
	return l
}
