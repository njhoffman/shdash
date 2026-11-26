package collect

import (
	"context"
	"strings"
	"testing"
)

func gitOutputs() map[string]string {
	return map[string]string{
		"ghq list -p": strings.Join([]string{
			"/home/u/ghq/github.com/njhoffman/audit",
			"/home/u/ghq/github.com/njhoffman/fzx",
			"/home/u/ghq/github.com/njhoffman/nocommits",
			"/home/u/ghq/github.com/someone/tool",
		}, "\n") + "\n",
		"git -C /home/u/.shell log -1 --format=%ct%x09%s":                         itoa(testNow.Add(-2*3600e9).Unix()) + "\tshell commit\n",
		"git -C /home/u/ghq/github.com/njhoffman/audit log -1 --format=%ct%x09%s": itoa(testNow.Add(-30*60e9).Unix()) + "\taudit commit\n",
		"git -C /home/u/ghq/github.com/njhoffman/fzx log -1 --format=%ct%x09%s":   itoa(testNow.Add(-50*3600e9).Unix()) + "\tfzx commit\n",
		"git -C /home/u/ghq/github.com/someone/tool log -1 --format=%ct%x09%s":    itoa(testNow.Unix()) + "\tnot mine\n",
		"repo-triage --tsv": strings.Join([]string{
			"own\t~/.config/nvim\tmain\t2\t2\t0\t-\t0\tgit@github.com:njhoffman/nvim.git",
			"own\t~/ghq/github.com/njhoffman/keytrace\tmain\t0\t0\t11\t-\t1\tgit@github.com:njhoffman/keytrace",
			"own\t~/ghq/github.com/NyaomiDEV/Sunamu\tmaster\t0\t0\t3\t-\t0\tgit@github.com:NyaomiDEV/Sunamu",
			"own\t~/git/scratch\tdev\t0\t1\t0\tdev\t0\t-",
			"own\t~/git/more\tmain\t1\t0\t0\t-\t0\t-",
			"patched\t~/ghq/github.com/x/y\tmaster\t0\t0\t1\t-\t0\tgit@github.com:x/y",
			"dirty\t~/ghq/github.com/a/b\tmaster\t3\t0\t0\t-\t0\tgit@github.com:a/b",
			"dirty\t~/ghq/github.com/a/c\tmaster\t1\t0\t0\t-\t0\tgit@github.com:a/c",
		}, "\n") + "\n",
	}
}

func TestGitRecentAndLocal(t *testing.T) {
	l := lines(t, Git(), fakeEnv(gitOutputs(), nil, nil))
	assertHas(t, l, "30m  audit", "audit commit", "2h   shell", "2d   fzx",
		"local   5 own · 1 patched · 2 dirty",
		"keytrace main  11 unpushed · 1 stash", "NyaomiDEV/Sunamu master  3 unpushed",
		"~/.config/nvim main  2 changed · 2 untracked", "1 more (repo-triage)")
	assertLacks(t, l, "not mine", "~/ghq/github.com/njhoffman/keytrace")
	// Newest first, unpushed before dirty.
	all := strings.Join(l, "\n")
	if strings.Index(all, "audit commit") > strings.Index(all, "shell commit") {
		t.Error("recent commits are not newest first")
	}
	if strings.Index(all, "keytrace") > strings.Index(all, "~/.config/nvim") {
		t.Error("repos with unpushed commits do not come first")
	}
}

func TestGitNoUpstream(t *testing.T) {
	_, local, err := triage(context.Background(), fakeEnv(gitOutputs(), nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range local {
		if want := r.Path == "~/git/scratch"; r.NoUpstream != want {
			t.Errorf("%s: NoUpstream = %v", r.Path, r.NoUpstream)
		}
	}
}

func TestGitOwnerOverride(t *testing.T) {
	env := fakeEnv(gitOutputs(), nil, map[string]string{"SHDASH_OWNER": "someone"})
	l := lines(t, Git(), env)
	assertHas(t, l, "not mine")
	assertLacks(t, l, "audit commit")
}

func TestGitTriageFailingKeepsRecent(t *testing.T) {
	out := gitOutputs()
	out["repo-triage --tsv"] = "ERR:repo-triage: exit status 1"
	env := fakeEnv(out, nil, nil)
	data, err := Git().Collect(context.Background(), env)
	if data == nil || err == nil {
		t.Fatalf("want recent commits with a triage error, got %v, %v", data, err)
	}
	l := render(t, Git(), env, data)
	assertHas(t, l, "audit commit")
	assertLacks(t, l, "local")
}

func TestGitBothFailing(t *testing.T) {
	if data, err := Git().Collect(context.Background(), fakeEnv(nil, nil, nil)); data != nil || err == nil {
		t.Fatalf("want an error and no data, got %v, %v", data, err)
	}
}

func TestShortRepo(t *testing.T) {
	for in, want := range map[string]string{
		"~/ghq/github.com/njhoffman/audit": "audit",
		"~/ghq/github.com/someone/tool":    "someone/tool",
		"~/ghq/bitbucket.org/team/repo":    "team/repo",
		"~/.config/nvim":                   "~/.config/nvim",
		"/srv/repo":                        "/srv/repo",
	} {
		if got := shortRepo(in, "njhoffman"); got != want {
			t.Errorf("shortRepo(%q) = %q, want %q", in, got, want)
		}
	}
}
