package collect

import (
	"context"
	"strings"
	"testing"
)

const liveFile = "/home/u/.cache/shell/env-stats-live"

var shellStats = `{"live": null, "repo": {"aliases": 547, "functions": 403, "scripts": 203,
  "startup_files": 259, "startup_lines": 15393, "widgets": 44,
  "name_violations": 0, "name_pending": 0, "name_kept": 180, "name_clashes": []}}`

func liveStats(epoch string) string {
	return "b6f17c7 " + epoch + " aliases=618 functions=8035 widgets=1413 zinit_plugins=31 zinit_snippets=8 commands_unique=15196 paths=18\n"
}

func TestShellLiveAndRepo(t *testing.T) {
	epoch := testNow.Add(-3 * 60 * 60 * 1e9).Unix()
	env := fakeEnv(map[string]string{"shell-stats --json": shellStats},
		map[string]string{liveFile: liveStats(itoa(epoch))}, nil)
	l := lines(t, Shell(), env)
	assertHas(t, l, "618 aliases", "8035 functions", "1413 widgets", "31 zinit", "8 snippets",
		"15196 commands", "18 dirs", "547 aliases", "203 scripts", "259 files", "15393 lines",
		"0 violations", "180 kept", "live counts 3h old")
}

func TestShellLiveFileFollowsXDG(t *testing.T) {
	env := fakeEnv(map[string]string{"shell-stats --json": shellStats},
		map[string]string{"/x/cache/shell/env-stats-live": liveStats("1795665667")},
		map[string]string{"XDG_CACHE_HOME": "/x/cache"})
	assertHas(t, lines(t, Shell(), env), "618 aliases")
}

func TestShellWithoutLiveCounts(t *testing.T) {
	env := fakeEnv(map[string]string{"shell-stats --json": shellStats}, nil, nil)
	l := lines(t, Shell(), env)
	assertHas(t, l, "no counts yet", "547 aliases")
	assertLacks(t, l, "live counts")
}

func TestShellStatsFailingKeepsLiveCounts(t *testing.T) {
	env := fakeEnv(nil, map[string]string{liveFile: liveStats("1795665667")}, nil)
	data, err := Shell().Collect(context.Background(), env)
	if data == nil || err == nil || !strings.Contains(err.Error(), "shell-stats") {
		t.Fatalf("want live data with a shell-stats error, got %v, %v", data, err)
	}
	assertHas(t, render(t, Shell(), env, data), "618 aliases")
}

func TestShellNothingAtAll(t *testing.T) {
	if data, err := Shell().Collect(context.Background(), fakeEnv(nil, nil, nil)); data != nil || err == nil {
		t.Fatalf("want an error and no data, got %v, %v", data, err)
	}
}

func TestShellMarksNameViolations(t *testing.T) {
	stats := strings.Replace(shellStats, `"name_violations": 0`, `"name_violations": 2`, 1)
	stats = strings.Replace(stats, `"name_clashes": []`, `"name_clashes": ["a", "b"]`, 1)
	l := lines(t, Shell(), fakeEnv(map[string]string{"shell-stats --json": stats}, nil, nil))
	assertHas(t, l, "2 violations", "2 clashes")
}

func TestParseLiveStats(t *testing.T) {
	for name, in := range map[string]string{
		"empty":     "",
		"no counts": "fp 1795665667",
		"only junk": "fp 1795665667 a=b c",
	} {
		if m, _ := parseLiveStats(in); m != nil {
			t.Errorf("%s: want nil, got %v", name, m)
		}
	}
	m, at := parseLiveStats("fp notanumber aliases=3 bad=x")
	if m["aliases"] != 3 || len(m) != 1 || !at.IsZero() {
		t.Errorf("got %v %v", m, at)
	}
}
