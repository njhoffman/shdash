package collect

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const (
	packagesFile = "/home/u/.local/share/audit/export/packages.json"
	summaryFile  = "/home/u/.local/share/audit/export/summary.json"
)

func packagesEnv(t *testing.T) *Env {
	return fakeEnv(nil, map[string]string{packagesFile: fixture(t, "audit-packages.json")}, nil)
}

func TestPackagesCounts(t *testing.T) {
	data, err := Packages().Collect(context.Background(), packagesEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	d := data.(*PackagesData)
	got := map[string]ManagerSummary{}
	var names []string
	for _, m := range d.Managers {
		got[m.Name] = m
		names = append(names, m.Name)
	}
	// Unavailable and empty managers are left out; the rest sorted by name.
	if strings.Join(names, " ") != "apt gem uv" {
		t.Fatalf("managers: %v", names)
	}
	for name, want := range map[string]ManagerSummary{
		"apt": {Name: "apt", Installed: 3, Updates: 1},
		"gem": {Name: "gem", Installed: 3, Updates: 2, Major: 1},
		// The item with no gap does not count: audit's own unchecked does.
		"uv": {Name: "uv", Installed: 3, Updates: 1, Unchecked: 1},
	} {
		if got[name] != want {
			t.Errorf("%s: %+v, want %+v", name, got[name], want)
		}
	}
	if want := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC); !d.CheckedAt.Equal(want) {
		t.Errorf("checked at %v, want the oldest lookup %v", d.CheckedAt, want)
	}
}

func TestPackagesPrefersSummaryBlock(t *testing.T) {
	// packages.json disagrees, so the counts show which file was read.
	env := fakeEnv(nil, map[string]string{
		summaryFile:  fixture(t, "audit-summary-packages.json"),
		packagesFile: fixture(t, "audit-packages.json"),
	}, nil)
	data, err := Packages().Collect(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	d := data.(*PackagesData)
	want := []ManagerSummary{
		{Name: "apt", Installed: 1506},
		{Name: "cargo", Installed: 91, Updates: 6, Unchecked: 1},
		{Name: "gem", Installed: 99, Updates: 60, Major: 6},
	}
	if len(d.Managers) != len(want) {
		t.Fatalf("managers: %+v", d.Managers)
	}
	for i := range want {
		if d.Managers[i] != want[i] {
			t.Errorf("%d: %+v, want %+v", i, d.Managers[i], want[i])
		}
	}
	l := lines(t, Packages(), env)
	assertHas(t, l, "apt   1506 · 0 updates", "cargo   91 · 6 updates",
		"gem     99 · 60 updates (6 major)", "scanned 5m ago · 1 unchecked", "registry checked 7d ago")
	assertLacks(t, l, "snap", "flatpak", "23")
}

func TestPackagesSummaryWithoutBlockFallsBack(t *testing.T) {
	env := fakeEnv(nil, map[string]string{
		summaryFile:  fixture(t, "audit-summary.json"),
		packagesFile: fixture(t, "audit-packages.json"),
	}, nil)
	data, err := Packages().Collect(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if d := data.(*PackagesData); len(d.Managers) != 3 || d.Managers[0].Name != "apt" {
		t.Errorf("fallback to packages.json: %+v", d.Managers)
	}
}

func TestPackagesWithoutRegistryLookups(t *testing.T) {
	// Only system managers: no registry line, and the export's time stands
	// in for a scan time it does not give.
	d := &PackagesData{GeneratedAt: testNow.Add(-2 * time.Hour),
		Managers: []ManagerSummary{{Name: "apt", Installed: 1}}}
	l := packagesLines(d, fakeEnv(nil, nil, nil))
	assertHas(t, l, "scanned 2h ago")
	assertLacks(t, l, "registry", "audit packages outdated")
}

func TestPackagesLines(t *testing.T) {
	env := packagesEnv(t)
	l := lines(t, Packages(), env)
	assertHas(t, l, "apt 3 · 1 update", "gem 3 · 2 updates (1 major)", "uv  3 · 1 update",
		"scanned 3d ago · 1 unchecked", "registry checked 5d ago")
	assertLacks(t, l, "flatpak", "snap", "0 unchecked", "audit packages outdated")
}

func TestPackagesStaleRegistryIsHighlighted(t *testing.T) {
	d := &PackagesData{GeneratedAt: testNow.Add(-time.Hour), CheckedAt: testNow.Add(-8 * 24 * time.Hour)}
	l := packagesLines(d, fakeEnv(nil, nil, nil))
	assertHas(t, l, "no package managers", "scanned 1h ago", "registry checked 8d ago", "audit packages outdated")
	assertLacks(t, l, "unchecked")
}

func TestPackagesRowsFitEightyColumns(t *testing.T) {
	// Two columns of 38 at 80; the widest names and counts must fit.
	d := &PackagesData{GeneratedAt: testNow, CheckedAt: testNow.Add(-20 * 24 * time.Hour), Managers: []ManagerSummary{
		{Name: "flatpak", Installed: 1506, Updates: 150, Major: 12, Unchecked: 99},
	}}
	for _, line := range packagesLines(d, fakeEnv(nil, nil, nil)) {
		if w := lipgloss.Width(line); w > 38 {
			t.Errorf("%d columns: %q", w, line)
		}
	}
}

func TestPackagesErrors(t *testing.T) {
	if _, err := Packages().Collect(context.Background(), fakeEnv(nil, nil, nil)); err == nil ||
		!strings.Contains(err.Error(), "audit packages") {
		t.Errorf("missing export: %v", err)
	}
	for name, files := range map[string]map[string]string{
		"bad packages.json": {packagesFile: "{"},
		"bad summary.json":  {summaryFile: "{", packagesFile: fixture(t, "audit-packages.json")},
	} {
		if _, err := Packages().Collect(context.Background(), fakeEnv(nil, files, nil)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestPackagesFollowsAuditExportVariable(t *testing.T) {
	env := fakeEnv(nil, map[string]string{"/x/packages.json": fixture(t, "audit-packages.json")},
		map[string]string{"SHDASH_AUDIT_EXPORT": "/x"})
	if _, err := Packages().Collect(context.Background(), env); err != nil {
		t.Errorf("SHDASH_AUDIT_EXPORT: %v", err)
	}
}
