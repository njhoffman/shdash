package collect

import (
	"context"
	"strings"
	"testing"
	"time"
)

const tmuxArgs = "tmux list-sessions -F #{session_name}\t#{session_windows}\t#{session_attached}"

func projectsJSON() string {
	rows := []string{}
	for i, name := range []string{"old", "fzx", "audit", "notes", "musicman", "shell", "polybar"} {
		ts := testNow.Unix() - int64((7-i)*3600)
		if name == "old" {
			ts = testNow.Unix() - 30*86400
		}
		extra := `"branch": "main", "ahead": 0, "behind": 0, "open": 0, "p1": 0, "overdue": 0`
		switch name {
		case "shell":
			extra = `"branch": "main", "ahead": 1, "behind": 2, "open": 55, "p1": 2, "overdue": 1, "dirty": 3`
		case "polybar":
			extra = `"branch": "", "open": 2`
		}
		rows = append(rows, `{"name": "`+name+`", "version": "1.0.0", "last_session": `+itoa(ts)+`, `+extra+`}`)
	}
	return "[" + strings.Join(rows, ",") + "]"
}

func TestProjectsRecentAndTmux(t *testing.T) {
	env := fakeEnv(map[string]string{
		"claude-projects --json --sort recent": projectsJSON(),
		tmuxArgs:                               "main-shell\t3\t1\nmain-harmonic\t1\t0\n",
	}, nil, nil)
	l := lines(t, Projects(), env)
	assertHas(t, l, "polybar ", "main +1 -2", "3 changed", "55 todos (2 p1)", "1 overdue",
		"tmux    2 sessions · main-shell*")
	assertLacks(t, l, "main-harmonic")
	// The five newest sessions; the oldest two are left out.
	assertLacks(t, l, "old ", "fzx")
	if len(l) != 6 {
		t.Errorf("want 5 projects and the tmux row, got %d lines:\n%s", len(l), strings.Join(l, "\n"))
	}
	if !strings.HasPrefix(l[0], "polybar") {
		t.Errorf("newest project is not first: %q", l[0])
	}
}

func TestProjectsWithoutTmux(t *testing.T) {
	env := fakeEnv(map[string]string{"claude-projects --json --sort recent": projectsJSON()}, nil, nil)
	l := lines(t, Projects(), env)
	assertLacks(t, l, "tmux")
}

func TestProjectsErrors(t *testing.T) {
	for name, out := range map[string]map[string]string{
		"missing":  nil,
		"bad json": {"claude-projects --json --sort recent": "{"},
	} {
		if _, err := Projects().Collect(context.Background(), fakeEnv(out, nil, nil)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// zlua is a z.lua data file: path|rank|unix-time, one visit time per row
// given as an age before testNow.
func zlua(rows map[string]time.Duration) string {
	var b strings.Builder
	for p, age := range rows {
		b.WriteString(p + "|1.5|" + itoa(testNow.Add(-age).Unix()) + "\n")
	}
	return b.String()
}

func TestRecentDirs(t *testing.T) {
	data := zlua(map[string]time.Duration{
		"/home/u":               0,                // $HOME: left out
		"/":                     time.Second,      // /: left out
		"/home/u/ghq/a":         time.Minute,      // newest kept
		"/home/u/ghq/a/sub":     2 * time.Minute,  // inside a: collapsed
		"/home/u/gone":          3 * time.Minute,  // no longer exists
		"/home/u/.shell/deploy": 20 * time.Minute, // kept
		"/home/u/weird|name":    25 * time.Minute, // a | in the path survives
		"/home/u/Downloads":     2 * time.Hour,
		"/home/u/.shell":        3 * time.Hour, // parent of a shown dir: kept
		"/home/u/old":           30 * 24 * time.Hour,
		"relative/path":         time.Minute, // not absolute: ignored
	}) + "garbage line\n/home/u/x|1|notanumber\n"
	env := fakeEnv(nil, map[string]string{"/home/u/.zlua": data}, nil)
	env.IsDir = func(p string) bool { return p != "/home/u/gone" }
	got := recentDirs(context.Background(), env)
	var paths []string
	for _, d := range got {
		paths = append(paths, d.Path)
	}
	want := "/home/u/ghq/a /home/u/.shell/deploy /home/u/weird|name /home/u/Downloads /home/u/.shell /home/u/old"
	if strings.Join(paths, " ") != want {
		t.Errorf("dirs:\n got %s\nwant %s", strings.Join(paths, " "), want)
	}
}

func TestRecentDirsDataVariable(t *testing.T) {
	env := fakeEnv(nil, map[string]string{"/data/z": zlua(map[string]time.Duration{"/srv/x": time.Minute})},
		map[string]string{"_ZL_DATA": "/data/z"})
	if got := recentDirs(context.Background(), env); len(got) != 1 || got[0].Path != "/srv/x" {
		t.Errorf("dirs from $_ZL_DATA: %+v", got)
	}
}

func TestProjectsShowsDirs(t *testing.T) {
	env := fakeEnv(map[string]string{"claude-projects --json --sort recent": projectsJSON()},
		map[string]string{"/home/u/.zlua": zlua(map[string]time.Duration{
			"/home/u/ghq/a": time.Minute, "/home/u/Downloads": 2 * time.Hour})}, nil)
	l := lines(t, Projects(), env)
	assertHas(t, l, "dirs", "  1m   ~/ghq/a", "  2h   ~/Downloads")
	all := strings.Join(l, "\n")
	if strings.Index(all, "~/ghq/a") > strings.Index(all, "~/Downloads") {
		t.Error("dirs are not newest first")
	}
}

func TestProjectsNoZluaIsNoDirs(t *testing.T) {
	env := fakeEnv(map[string]string{"claude-projects --json --sort recent": projectsJSON()}, nil, nil)
	assertLacks(t, lines(t, Projects(), env), "dirs")
}

func TestProjectsKeepsDirsWhenClaudeProjectsFails(t *testing.T) {
	env := fakeEnv(nil, map[string]string{"/home/u/.zlua": zlua(map[string]time.Duration{"/home/u/ghq/a": time.Minute})}, nil)
	data, err := Projects().Collect(context.Background(), env)
	if data == nil || err == nil {
		t.Fatalf("want dirs with the claude-projects error, got %v, %v", data, err)
	}
	assertHas(t, render(t, Projects(), env, data), "~/ghq/a")
}

func TestRecentDirsDoNotWaitForAHungMount(t *testing.T) {
	old := dirCheckBudget
	dirCheckBudget = 50 * time.Millisecond
	defer func() { dirCheckBudget = old }()
	data := zlua(map[string]time.Duration{
		"/home/u/a":       time.Minute,
		"/mnt/nfs/movies": 2 * time.Minute, // the stat hangs
		"/home/u/gone":    3 * time.Minute, // unchecked once the budget is spent
	})
	env := fakeEnv(nil, map[string]string{"/home/u/.zlua": data}, nil)
	hang := make(chan struct{})
	defer close(hang)
	env.IsDir = func(p string) bool {
		switch p {
		case "/mnt/nfs/movies":
			<-hang
		case "/home/u/gone":
			return false
		}
		return true
	}
	start := time.Now()
	got := recentDirs(context.Background(), env)
	if took := time.Since(start); took > time.Second {
		t.Fatalf("recentDirs waited %v for a hung stat", took)
	}
	var paths []string
	for _, d := range got {
		paths = append(paths, d.Path)
	}
	if strings.Join(paths, " ") != "/home/u/a /mnt/nfs/movies /home/u/gone" {
		t.Errorf("dirs: %v", paths)
	}
}

func TestRecentDirsStopAtTheCollectorsDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env := fakeEnv(nil, map[string]string{"/home/u/.zlua": zlua(map[string]time.Duration{"/home/u/gone": time.Minute})}, nil)
	env.IsDir = func(string) bool { t.Error("stat after the deadline"); return false }
	if got := recentDirs(ctx, env); len(got) != 1 {
		t.Errorf("dirs: %+v", got)
	}
}

func TestProjectsTmuxDetailListsEverySession(t *testing.T) {
	env := fakeEnv(map[string]string{
		"claude-projects --json --sort recent": projectsJSON(),
		tmuxArgs:                               "main-shell\t3\t1\nmain-harmonic\t1\t0\nmain-rofi\t2\t0\n",
	}, nil, nil)
	data, err := Projects().Collect(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	assertHas(t, detail(t, Projects(), env, data), "tmux    main-shell* main-harmonic main-rofi")
	assertHas(t, render(t, Projects(), env, data), "tmux    3 sessions · main-shell*")
}
