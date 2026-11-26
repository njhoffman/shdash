package collect

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// detail renders data through c's Detail.
func detail(t *testing.T, c Collector, env *Env, data any) []string {
	t.Helper()
	if c.Detail == nil {
		t.Fatalf("%s has no Detail", c.Name)
	}
	raw, err := marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	l, err := c.Detail(raw, env)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestGitStoresMoreThanItShows(t *testing.T) {
	d := &GitData{}
	for i := range 12 {
		d.Recent = append(d.Recent, Commit{Repo: fmt.Sprintf("r%02d", i), Subject: "s", When: testNow})
	}
	env := fakeEnv(nil, nil, nil)
	snap := render(t, Git(), env, d)
	full := detail(t, Git(), env, d)
	if len(snap) != gitRecent || len(full) != 12 {
		t.Errorf("snapshot %d lines, full %d", len(snap), len(full))
	}
}

func TestReposDetailListsEveryRepo(t *testing.T) {
	env := fakeEnv(gitOutputs(), nil, nil)
	data, err := Repos().Collect(t.Context(), env)
	if err != nil {
		t.Fatal(err)
	}
	full := detail(t, Repos(), env, data)
	assertHas(t, full, "~/git/more")
	assertLacks(t, full, "more (repo-triage)")
	assertHas(t, render(t, Repos(), env, data), "1 more (repo-triage)")
}

func TestProjectsDetailListsAll(t *testing.T) {
	dirs := map[string]time.Duration{}
	for i := range 8 {
		dirs[fmt.Sprintf("/home/u/d%d", i)] = time.Duration(i+1) * time.Minute
	}
	env := fakeEnv(map[string]string{"claude-projects --json --sort recent": projectsJSON()},
		map[string]string{"/home/u/.zlua": zlua(dirs)}, nil)
	data, err := Projects().Collect(t.Context(), env)
	if err != nil {
		t.Fatal(err)
	}
	snap := strings.Join(render(t, Projects(), env, data), "\n")
	full := strings.Join(detail(t, Projects(), env, data), "\n")
	for _, name := range []string{"old ", "fzx ", "~/d7"} {
		if strings.Contains(snap, name) || !strings.Contains(full, name) {
			t.Errorf("%q: in snapshot %v, in detail %v", name, strings.Contains(snap, name), strings.Contains(full, name))
		}
	}
}

func TestAuditDetailListsMoreTasks(t *testing.T) {
	d := &AuditData{GeneratedAt: testNow}
	for i := range 7 {
		d.Top = append(d.Top, AuditTask{Command: "c", Priority: "high", Title: fmt.Sprintf("task %d", i)})
	}
	env := fakeEnv(nil, nil, nil)
	assertLacks(t, render(t, Audit(), env, d), "task 3")
	assertHas(t, detail(t, Audit(), env, d), "task 3", "task 6")
}
