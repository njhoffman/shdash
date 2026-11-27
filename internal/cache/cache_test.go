package cache

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSaveLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	if e, err := Load(dir, "x"); e != nil || err != nil {
		t.Fatalf("missing entry: got %v, %v", e, err)
	}
	at := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	in := &Entry{Schema: 2, FetchedAt: at, AttemptedAt: at, Error: "boom", Data: json.RawMessage(`{"a":1}`)}
	if err := Save(dir, "x", in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(dir, "x")
	if err != nil || out.Schema != 2 || !out.FetchedAt.Equal(at) || out.Error != "boom" || string(out.Data) != `{"a":1}` {
		t.Fatalf("round trip: got %+v, %v", out, err)
	}
	left, _ := filepath.Glob(filepath.Join(dir, ".*.tmp"))
	if len(left) > 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

func TestLoadCorrupt(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.json"), []byte("{"), 0o644)
	if _, err := Load(dir, "x"); err == nil {
		t.Fatal("want an error for a corrupt file")
	}
}

func TestLock(t *testing.T) {
	dir := t.TempDir()
	unlock, ok, err := Lock(dir, false)
	if err != nil || !ok {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	if _, ok, err := Lock(dir, false); err != nil || ok {
		t.Fatalf("second lock should be refused: %v %v", ok, err)
	}
	unlock()
	unlock2, ok, err := Lock(dir, false)
	if err != nil || !ok {
		t.Fatalf("lock after unlock: %v %v", ok, err)
	}
	unlock2()
}

func TestDir(t *testing.T) {
	env := map[string]string{}
	get := func(k string) string { return env[k] }
	if got := Dir(get, "/h"); got != "/h/.cache/shdash" {
		t.Errorf("default: %s", got)
	}
	env["XDG_CACHE_HOME"] = "/c"
	if got := Dir(get, "/h"); got != "/c/shdash" {
		t.Errorf("XDG: %s", got)
	}
	env["SHDASH_CACHE_DIR"] = "/d"
	if got := Dir(get, "/h"); got != "/d" {
		t.Errorf("override: %s", got)
	}
}

func TestClean(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	files := map[string]time.Duration{
		"git.json":         0,
		"old-panel.json":   8 * 24 * time.Hour,
		"other-build.json": 6 * 24 * time.Hour, // another build's panel, still written
		".git-123.tmp":     25 * time.Hour,
		".git-456.tmp":     time.Minute, // a Save under way, perhaps
		"refresh.log":      0,
		"refresh.log.1":    0,
		".refresh.lock":    0,
		".hidden.json":     0,
		"notes.txt":        48 * time.Hour,
		".no-suffix-12345": 48 * time.Hour,
	}
	for name, age := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := Clean(dir, []string{"git", "shell"}, now)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(removed)
	if got := strings.Join(removed, " "); got != ".git-123.tmp old-panel.json" {
		t.Errorf("removed %q", got)
	}
	for name := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		gone := errors.Is(err, fs.ErrNotExist)
		if want := name == ".git-123.tmp" || name == "old-panel.json"; gone != want {
			t.Errorf("%s: gone %v, want %v", name, gone, want)
		}
	}
	if removed, err := Clean(filepath.Join(dir, "missing"), nil, now); err != nil || removed != nil {
		t.Errorf("missing dir: %v %v", removed, err)
	}
}
