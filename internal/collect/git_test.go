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

func TestGitRecent(t *testing.T) {
	l := lines(t, Git(), fakeEnv(gitOutputs(), nil, nil))
	assertHas(t, l, "30m  audit", "audit commit", "2h   shell", "2d   fzx")
	assertLacks(t, l, "not mine", "local", "keytrace")
	// Newest first.
	all := strings.Join(l, "\n")
	if strings.Index(all, "audit commit") > strings.Index(all, "shell commit") {
		t.Error("recent commits are not newest first")
	}
}

func TestGitDoesNotRunTriage(t *testing.T) {
	out := gitOutputs()
	out["repo-triage --tsv"] = "ERR:repo-triage must not run for Git"
	if _, err := Git().Collect(context.Background(), fakeEnv(out, nil, nil)); err != nil {
		t.Fatalf("collect: %v", err)
	}
}

func TestGitOwnerOverride(t *testing.T) {
	env := fakeEnv(gitOutputs(), nil, map[string]string{"SHDASH_OWNER": "someone"})
	l := lines(t, Git(), env)
	assertHas(t, l, "not mine")
	assertLacks(t, l, "audit commit")
}

func TestGitFailing(t *testing.T) {
	if data, err := Git().Collect(context.Background(), fakeEnv(nil, nil, nil)); data != nil || err == nil {
		t.Fatalf("want an error and no data, got %v, %v", data, err)
	}
}
