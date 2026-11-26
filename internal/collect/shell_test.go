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

const startupFile = "/home/u/.cache/shell/startup-times"

// startupTimes is a startup-times file of the given seconds, oldest first.
func startupTimes(secs ...string) string {
	var b strings.Builder
	for i, s := range secs {
		b.WriteString(itoa(testNow.Unix()-int64(len(secs)-i)*60) + "\t" + s + "\n")
	}
	return b.String()
}

func TestParseStartupTimes(t *testing.T) {
	for name, c := range map[string]struct {
		in           string
		last, median float64
		n            int
	}{
		"odd":       {startupTimes("0.3", "0.1", "0.2"), 0.2, 0.2, 3},
		"even":      {startupTimes("0.1", "0.4", "0.2", "0.3"), 0.3, 0.25, 4},
		"bad lines": {"garbage\n123\tnot\n-1\t0.5\n" + startupTimes("0.21") + "456\t-0.1\n", 0.21, 0.21, 1},
		"zsh %.3g":  {startupTimes("1.2", "0.213"), 0.213, 0.7065, 2},
	} {
		got := parseStartupTimes(c.in)
		if got == nil || got.Last != c.last || got.Median != c.median || got.N != c.n {
			t.Errorf("%s: %+v, want last %v median %v n %d", name, got, c.last, c.median, c.n)
		}
	}
	if got := parseStartupTimes("nothing here\n"); got != nil {
		t.Errorf("no valid lines: %+v", got)
	}
}

func TestStartupTimesKeepTheLastWindow(t *testing.T) {
	var secs []string
	for range 100 {
		secs = append(secs, "9.0") // older than the window: ignored
	}
	for range startupWindow {
		secs = append(secs, "0.2")
	}
	if got := parseStartupTimes(startupTimes(secs...)); got.N != startupWindow || got.Median != 0.2 {
		t.Errorf("window: %+v", got)
	}
}

func TestShellStartupRow(t *testing.T) {
	env := fakeEnv(map[string]string{"shell-stats --json": shellStats},
		map[string]string{startupFile: startupTimes("0.3", "0.25", "0.21")}, nil)
	l := lines(t, Shell(), env)
	assertHas(t, l, "startup 0.21s · median 0.25s (3) · 259 files · 15393 lines")
}

func TestShellStartupWithoutShellStats(t *testing.T) {
	env := fakeEnv(nil, map[string]string{startupFile: startupTimes("0.2")}, nil)
	data, err := Shell().Collect(context.Background(), env)
	if data == nil || err == nil {
		t.Fatalf("want startup times with the shell-stats error, got %v, %v", data, err)
	}
	l := render(t, Shell(), env, data)
	assertHas(t, l, "startup 0.20s · median 0.20s (1)")
	assertLacks(t, l, "files")
}

func TestShellNoStartupTimes(t *testing.T) {
	env := fakeEnv(map[string]string{"shell-stats --json": shellStats}, nil, nil)
	l := lines(t, Shell(), env)
	assertHas(t, l, "startup 259 files")
	assertLacks(t, l, "median")
}

func TestShellSlowStartupIsHighlighted(t *testing.T) {
	d := &ShellData{Startup: &StartupTimes{Last: 0.4, Median: 0.6, N: 10}}
	slow := strings.Join(shellLines(d, fakeEnv(nil, nil, nil)), "\n")
	d.Startup.Median = 0.3
	fast := strings.Join(shellLines(d, fakeEnv(nil, nil, nil)), "\n")
	if !strings.Contains(slow, highlight(true, "median 0.60s (10)")) {
		t.Errorf("slow median not highlighted: %q", slow)
	}
	if !strings.Contains(fast, "median 0.30s (10)") {
		t.Errorf("fast median: %q", fast)
	}
}

func TestShellStartupIsReadWhenDrawn(t *testing.T) {
	files := map[string]string{startupFile: startupTimes("0.3")}
	env := fakeEnv(map[string]string{"shell-stats --json": shellStats}, files, nil)
	data, err := Shell().Collect(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	// A new shell started after the collection.
	files[startupFile] = startupTimes("0.3", "0.25")
	assertHas(t, render(t, Shell(), env, data), "startup 0.25s · median 0.28s (2)")
	// The file is gone: the collected copy is shown.
	delete(files, startupFile)
	assertHas(t, render(t, Shell(), env, data), "startup 0.30s · median 0.30s (1)")
}
