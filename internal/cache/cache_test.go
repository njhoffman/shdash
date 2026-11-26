package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
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
