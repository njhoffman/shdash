package collect

import (
	"context"
	"strings"
	"testing"
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
		"tmux    main-shell* main-harmonic")
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
