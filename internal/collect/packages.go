package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/njhoffman/shdash/internal/format"
)

// PackagesData is, per package manager, how many packages are installed and
// how many can be updated, from audit's packages export.
type PackagesData struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Managers    []ManagerSummary `json:"managers"`
}

// ManagerSummary counts one manager's packages. Updates are those audit
// found behind (a patch, minor or major gap, or an update a system manager
// offers); Major is how many of those are major; Unchecked are the ones it
// could not compare.
type ManagerSummary struct {
	Name      string `json:"name"`
	Installed int    `json:"installed"`
	Updates   int    `json:"updates"`
	Major     int    `json:"major"`
	Unchecked int    `json:"unchecked"`
}

// auditStale is the export age past which the panel highlights it.
const auditStale = 7 * 24 * time.Hour

// Packages reads audit's packages.json export (audit packages outdated
// refreshes it); it never runs a package manager.
func Packages() Collector {
	return Collector{
		Name: "packages", Title: "Packages", Icon: "\U000F03D6", Priority: 5,
		TTL: 10 * time.Minute, Timeout: 10 * time.Second, Inline: true, Schema: 1,
		Collect: collectPackages,
		Lines:   linesOf(packagesLines),
	}
}

func collectPackages(_ context.Context, env *Env) (any, error) {
	dir := AuditDir(env)
	b, err := env.ReadFile(filepath.Join(dir, "packages.json"))
	if err != nil {
		return nil, fmt.Errorf("no audit packages export in %s (audit packages writes one)", env.Tilde(dir))
	}
	// Only the fields counted here; each item's registry metadata is skipped.
	var x struct {
		GeneratedAt time.Time `json:"generated_at"`
		Packages    []struct {
			Manager   string `json:"manager"`
			Available bool   `json:"available"`
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
		s := ManagerSummary{Name: m.Manager, Installed: len(m.Items)}
		for _, it := range m.Items {
			switch it.Gap {
			case "major":
				s.Major++
				s.Updates++
			case "minor", "patch", "update":
				s.Updates++
			case "":
				s.Unchecked++
			}
		}
		d.Managers = append(d.Managers, s)
	}
	sort.Slice(d.Managers, func(i, j int) bool { return d.Managers[i].Name < d.Managers[j].Name })
	return d, nil
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
	age := env.Now().Sub(d.GeneratedAt)
	checked := "checked " + format.Age(age) + " ago (audit packages outdated)"
	if age > auditStale {
		l = append(l, format.Strong.Render(checked))
	} else {
		l = append(l, format.Dim.Render(checked))
	}
	return l
}
