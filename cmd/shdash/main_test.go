package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/njhoffman/shdash/internal/cache"
	"github.com/njhoffman/shdash/internal/collect"
	"github.com/njhoffman/shdash/internal/tui"
)

var now = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

type fake struct {
	mu    sync.Mutex
	calls map[string]int
	fail  map[string]bool
}

func (f *fake) collector(name string, prio int, inline bool) collect.Collector {
	return collect.Collector{
		Name: name, Title: strings.ToUpper(name[:1]) + name[1:], Priority: prio,
		TTL: 10 * time.Minute, Timeout: time.Second, Inline: inline, Schema: 1,
		Collect: func(context.Context, *collect.Env) (any, error) {
			f.mu.Lock()
			f.calls[name]++
			failing := f.fail[name]
			f.mu.Unlock()
			if failing {
				return nil, errors.New(name + " failed")
			}
			return map[string]string{"v": name + " data"}, nil
		},
		Lines: func(raw json.RawMessage, _ *collect.Env) ([]string, error) {
			var m map[string]string
			if err := json.Unmarshal(raw, &m); err != nil {
				return nil, err
			}
			return []string{m["v"]}, nil
		},
	}
}

type harness struct {
	f       *fake
	d       deps
	spawned [][]string
	vars    map[string]string
}

func newHarness(t *testing.T) *harness {
	h := &harness{f: &fake{calls: map[string]int{}, fail: map[string]bool{}}, vars: map[string]string{}}
	h.d = deps{
		env: &collect.Env{
			Now:    func() time.Time { return now },
			Getenv: func(k string) string { return h.vars[k] },
		},
		collectors: []collect.Collector{
			h.f.collector("cheap", 1, true),
			h.f.collector("slow", 2, false),
		},
		cacheDir: t.TempDir(),
		spawn:    func(names []string) error { h.spawned = append(h.spawned, names); return nil },
	}
	return h
}

func (h *harness) run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, h.d)
	return code, out.String(), errb.String()
}

func TestFirstRunCollectsInlineAndSpawnsTheRest(t *testing.T) {
	h := newHarness(t)
	code, out, _ := h.run()
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "cheap data") || !strings.Contains(out, "collecting") {
		t.Errorf("output:\n%s", out)
	}
	if h.f.calls["cheap"] != 1 || h.f.calls["slow"] != 0 {
		t.Errorf("calls: %v", h.f.calls)
	}
	if len(h.spawned) != 1 || strings.Join(h.spawned[0], ",") != "slow" {
		t.Errorf("spawned: %v", h.spawned)
	}
}

func TestFreshCacheRunsNothing(t *testing.T) {
	h := newHarness(t)
	for _, n := range []string{"cheap", "slow"} {
		cache.Save(h.d.cacheDir, n, &cache.Entry{Schema: 1, FetchedAt: now.Add(-time.Minute), Data: json.RawMessage(`{"v":"` + n + ` cached"}`)})
	}
	_, out, _ := h.run()
	if !strings.Contains(out, "cheap cached") || !strings.Contains(out, "slow cached") {
		t.Errorf("output:\n%s", out)
	}
	if len(h.f.calls) != 0 || len(h.spawned) != 0 {
		t.Errorf("calls %v, spawned %v", h.f.calls, h.spawned)
	}
}

func TestStaleCacheShowsOldDataAndSpawns(t *testing.T) {
	h := newHarness(t)
	cache.Save(h.d.cacheDir, "cheap", &cache.Entry{Schema: 1, FetchedAt: now.Add(-time.Hour), Data: json.RawMessage(`{"v":"cheap old"}`)})
	cache.Save(h.d.cacheDir, "slow", &cache.Entry{Schema: 1, FetchedAt: now, Data: json.RawMessage(`{"v":"slow new"}`)})
	_, out, _ := h.run()
	if !strings.Contains(out, "cheap old") || h.f.calls["cheap"] != 0 {
		t.Errorf("stale inline panel was collected in the foreground: %v\n%s", h.f.calls, out)
	}
	if len(h.spawned) != 1 || strings.Join(h.spawned[0], ",") != "cheap" {
		t.Errorf("spawned: %v", h.spawned)
	}
}

func TestRefreshCollectsEverything(t *testing.T) {
	h := newHarness(t)
	cache.Save(h.d.cacheDir, "cheap", &cache.Entry{Schema: 1, FetchedAt: now, Data: json.RawMessage(`{"v":"x"}`)})
	_, out, _ := h.run("--refresh")
	if h.f.calls["cheap"] != 1 || h.f.calls["slow"] != 1 || len(h.spawned) != 0 {
		t.Errorf("calls %v, spawned %v", h.f.calls, h.spawned)
	}
	if !strings.Contains(out, "slow data") {
		t.Errorf("output:\n%s", out)
	}
}

func TestFailingPanelShowsItsError(t *testing.T) {
	h := newHarness(t)
	h.f.fail["cheap"] = true
	_, out, _ := h.run("--refresh")
	if !strings.Contains(out, "Cheap") || !strings.Contains(out, "failed: cheap failed") || !strings.Contains(out, "slow data") {
		t.Errorf("output:\n%s", out)
	}
}

func TestNoSpawnWhileARefreshRuns(t *testing.T) {
	h := newHarness(t)
	unlock, ok, err := cache.Lock(h.d.cacheDir, false)
	if err != nil || !ok {
		t.Fatalf("lock: %v %v", ok, err)
	}
	h.run()
	unlock()
	if len(h.spawned) != 0 {
		t.Errorf("spawned under a held lock: %v", h.spawned)
	}
	// The lock is free again: the next run spawns.
	h.run()
	if len(h.spawned) != 1 {
		t.Errorf("spawned: %v", h.spawned)
	}
}

func TestBackgroundRefreshLogsRunAndErrors(t *testing.T) {
	h := newHarness(t)
	h.f.fail["slow"] = true
	code, _, errs := h.run("--background-refresh", "cheap,slow")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	want := now.Format(time.RFC3339) + " refresh cheap,slow\n  slow: slow failed\n"
	if errs != want {
		t.Errorf("log:\n%q\nwant\n%q", errs, want)
	}
	// Nothing due: nothing logged.
	h.f.fail["slow"] = false
	cache.Save(h.d.cacheDir, "slow", &cache.Entry{Schema: 1, FetchedAt: now, Data: json.RawMessage(`{"v":"x"}`)})
	if _, _, errs := h.run("--background-refresh", "cheap,slow"); errs != "" {
		t.Errorf("logged with nothing due: %q", errs)
	}
}

func TestOpenLogAppendsAndRotates(t *testing.T) {
	dir := t.TempDir()
	write := func(s string) {
		t.Helper()
		f, err := openLog(dir, 8)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(s)
		f.Close()
	}
	read := func(name string) string {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		return string(b)
	}
	write("run1\n")
	write("run2\n")
	if got := read("refresh.log"); got != "run1\nrun2\n" {
		t.Errorf("appended log: %q", got)
	}
	// Past the limit (8 bytes): moved aside, a new log is started.
	write("run3\n")
	if read("refresh.log.1") != "run1\nrun2\n" || read("refresh.log") != "run3\n" {
		t.Errorf("after rotation: %q / %q", read("refresh.log.1"), read("refresh.log"))
	}
}

func TestBackgroundRefresh(t *testing.T) {
	h := newHarness(t)
	cache.Save(h.d.cacheDir, "cheap", &cache.Entry{Schema: 1, FetchedAt: now, Data: json.RawMessage(`{"v":"x"}`)})
	// cheap is fresh, so only slow is collected.
	if code, _, _ := h.run("--background-refresh", "cheap,slow"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if h.f.calls["cheap"] != 0 || h.f.calls["slow"] != 1 {
		t.Errorf("calls: %v", h.f.calls)
	}
	// Another refresh holding the lock: nothing runs.
	unlock, _, _ := cache.Lock(h.d.cacheDir, false)
	defer unlock()
	os.Remove(filepath.Join(h.d.cacheDir, "slow.json"))
	h.run("--background-refresh", "slow")
	if h.f.calls["slow"] != 1 {
		t.Errorf("refresh ran under a held lock: %v", h.f.calls)
	}
}

func TestIcons(t *testing.T) {
	h := newHarness(t)
	for _, n := range []string{"cheap", "slow"} {
		cache.Save(h.d.cacheDir, n, &cache.Entry{Schema: 1, FetchedAt: now, Data: json.RawMessage(`{"v":"x"}`)})
	}
	h.d.collectors[0].Icon = "@"
	for name, c := range map[string]struct {
		env  string
		args []string
		want bool
	}{
		"default off":              {"", nil, false},
		"SHDASH_ICONS=1":           {"1", nil, true},
		"SHDASH_ICONS=true":        {"True", nil, true},
		"SHDASH_ICONS=0":           {"0", nil, false},
		"--icons":                  {"", []string{"--icons"}, true},
		"--no-icons beats env":     {"1", []string{"--no-icons"}, false},
		"--no-icons beats --icons": {"", []string{"--icons", "--no-icons"}, false},
	} {
		h.vars["SHDASH_ICONS"] = c.env
		_, out, _ := h.run(append([]string{"--no-color", "--width", "60"}, c.args...)...)
		if got := strings.HasPrefix(out, "@ Cheap"); got != c.want {
			t.Errorf("%s: icon shown = %v, want %v:\n%s", name, got, c.want, out)
		}
	}
}

func TestInteractive(t *testing.T) {
	h := newHarness(t)
	var got *tui.Options
	h.d.interactive = func(o tui.Options) error { got = &o; return nil }
	// Piped: refused, nothing run.
	if code, _, errs := h.run("-i"); code != 2 || got != nil || !strings.Contains(errs, "needs a terminal") {
		t.Fatalf("-i without a terminal: %d %q %v", code, errs, got)
	}
	h.d.termSize = func() (int, int, bool) { return 100, 30, true }
	h.vars["SHDASH_ICONS"] = "1"
	if code, out, _ := h.run("-i"); code != 0 || out != "" {
		t.Fatalf("-i: exit %d, printed %q", code, out)
	}
	if got == nil || got.CacheDir != h.d.cacheDir || !got.Icons || !got.AutoRefresh || len(got.Collectors) != 2 {
		t.Errorf("options: %+v", got)
	}
	// Interactive mode spawns no background refresh and collects nothing itself.
	if len(h.spawned) != 0 || len(h.f.calls) != 0 {
		t.Errorf("spawned %v, calls %v", h.spawned, h.f.calls)
	}
	h.d.interactive = func(tui.Options) error { return errors.New("no tty") }
	if code, _, errs := h.run("-i"); code != 1 || !strings.Contains(errs, "no tty") {
		t.Errorf("failing -i: %d %q", code, errs)
	}
}

func TestLayoutSize(t *testing.T) {
	h := newHarness(t)
	tty := func(w, ht int) func() (int, int, bool) { return func() (int, int, bool) { return w, ht, true } }
	pipe := func() (int, int, bool) { return 0, 0, false }
	for name, c := range map[string]struct {
		size          func() (int, int, bool)
		cols          string
		width, height int
		wantW, wantH  int
	}{
		"tty":           {tty(120, 40), "", 0, -1, 120, 39},
		"tty flags":     {tty(120, 40), "", 90, 0, 90, 0},
		"pipe":          {pipe, "", 0, -1, 80, 0},
		"pipe COLUMNS":  {pipe, "150", 0, -1, 150, 0},
		"bad COLUMNS":   {pipe, "wide", 0, -1, 80, 0},
		"pipe --height": {pipe, "", 0, 12, 80, 12},
	} {
		h.d.termSize = c.size
		h.vars["COLUMNS"] = c.cols
		if w, ht := layoutSize(c.width, c.height, h.d); w != c.wantW || ht != c.wantH {
			t.Errorf("%s: got %dx%d, want %dx%d", name, w, ht, c.wantW, c.wantH)
		}
	}
}

func TestArguments(t *testing.T) {
	h := newHarness(t)
	if code, _, errs := h.run("extra"); code != 2 || !strings.Contains(errs, "unexpected argument") {
		t.Errorf("extra argument: %d %q", code, errs)
	}
	if code, _, _ := h.run("-x"); code != 2 {
		t.Errorf("-x: exit %d", code)
	}
	if code, _, errs := h.run("--help"); code != 0 || !strings.Contains(errs, "usage: shdash") {
		t.Errorf("--help: %d %q", code, errs)
	}
	if code, out, _ := h.run("--version"); code != 0 || !strings.HasPrefix(out, "shdash dev") {
		t.Errorf("--version: %d %q", code, out)
	}
}

// TestBinaryRefreshesInBackground runs the built binary against stub tools:
// the first run prints at once and leaves a detached refresh behind, which
// writes the slow panels' caches.
func TestBinaryRefreshesInBackground(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "shdash")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	stubs := filepath.Join(tmp, "stubs")
	os.Mkdir(stubs, 0o755)
	for name, body := range map[string]string{
		"fastfetch":       `echo '[{"type":"OS","result":{"prettyName":"Stub OS"}}]'`,
		"shell-stats":     `echo '{"repo":{"aliases":7}}'`,
		"claude-projects": `sleep 0.3; echo '[{"name":"stubproj","last_session":1}]'`,
		"tmux":            `exit 1`,
		"ghq":             `echo "$HOME/ghq/github.com/njhoffman/r"`,
		"git":             `printf '1795600000\tstub commit\n'`,
		"repo-triage":     `printf 'own\t~/r\tmain\t1\t0\t0\t-\t0\t-\n'`,
	} {
		os.WriteFile(filepath.Join(stubs, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755)
	}
	cacheDir := filepath.Join(tmp, "cache")
	cmd := exec.Command(bin, "--no-color", "--width", "100")
	cmd.Env = []string{"PATH=" + stubs + ":/usr/bin:/bin", "HOME=" + tmp, "SHDASH_CACHE_DIR=" + cacheDir}
	start := time.Now()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if time.Since(start) > 250*time.Millisecond {
		t.Errorf("first run waited for the background refresh (%v)", time.Since(start))
	}
	for _, want := range []string{"Stub OS", "7 aliases", "collecting"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("first run lacks %q:\n%s", want, out)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, e1 := os.Stat(filepath.Join(cacheDir, "projects.json"))
		_, e2 := os.Stat(filepath.Join(cacheDir, "git.json"))
		if e1 == nil && e2 == nil {
			break
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(filepath.Join(cacheDir, "refresh.log"))
			t.Fatalf("background refresh wrote no caches; refresh.log:\n%s", log)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Wait for the lock to be released before the second run.
	unlock, _, _ := cache.Lock(cacheDir, true)
	unlock()
	cmd = exec.Command(bin, "--no-color", "--width", "100")
	cmd.Env = []string{"PATH=" + stubs + ":/usr/bin:/bin", "HOME=" + tmp, "SHDASH_CACHE_DIR=" + cacheDir}
	out, _ = cmd.Output()
	for _, want := range []string{"stubproj", "stub commit", "local   1 own"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("second run lacks %q:\n%s", want, out)
		}
	}
}
