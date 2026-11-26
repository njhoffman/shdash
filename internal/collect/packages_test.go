package collect

import (
	"context"
	"strings"
	"testing"
	"time"
)

const packagesFile = "/home/u/.local/share/audit/export/packages.json"

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
		"uv":  {Name: "uv", Installed: 3, Updates: 1, Unchecked: 1},
	} {
		if got[name] != want {
			t.Errorf("%s: %+v, want %+v", name, got[name], want)
		}
	}
}

func TestPackagesLines(t *testing.T) {
	env := packagesEnv(t)
	l := lines(t, Packages(), env)
	assertHas(t, l, "apt 3 · 1 update", "gem 3 · 2 updates (1 major)", "uv  3 · 1 update · 1 unchecked",
		"checked 3d ago")
	assertLacks(t, l, "flatpak", "snap", "0 unchecked")
}

func TestPackagesStaleExportIsHighlighted(t *testing.T) {
	d := &PackagesData{GeneratedAt: testNow.Add(-8 * 24 * time.Hour)}
	l := packagesLines(d, fakeEnv(nil, nil, nil))
	assertHas(t, l, "no package managers", "checked 8d ago")
}

func TestPackagesErrors(t *testing.T) {
	if _, err := Packages().Collect(context.Background(), fakeEnv(nil, nil, nil)); err == nil ||
		!strings.Contains(err.Error(), "audit packages") {
		t.Errorf("missing export: %v", err)
	}
	bad := fakeEnv(nil, map[string]string{packagesFile: "{"}, nil)
	if _, err := Packages().Collect(context.Background(), bad); err == nil {
		t.Error("bad json: want an error")
	}
}

func TestPackagesFollowsAuditExportVariable(t *testing.T) {
	env := fakeEnv(nil, map[string]string{"/x/packages.json": fixture(t, "audit-packages.json")},
		map[string]string{"SHDASH_AUDIT_EXPORT": "/x"})
	if _, err := Packages().Collect(context.Background(), env); err != nil {
		t.Errorf("SHDASH_AUDIT_EXPORT: %v", err)
	}
}
