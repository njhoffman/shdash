package collect

import (
	"context"
	"strings"
	"testing"
)

func TestReposLocal(t *testing.T) {
	l := lines(t, Repos(), fakeEnv(gitOutputs(), nil, nil))
	assertHas(t, l, "local   5 own · 1 patched · 2 dirty",
		"keytrace         main   · 11 unpushed · 1 stash", "NyaomiDEV/Sunamu master · 3 unpushed",
		"~/.config/nvim   main   · 2 changed · 2 untracked", "1 more (repo-triage)")
	assertLacks(t, l, "~/ghq/github.com/njhoffman/keytrace", "audit commit")
	// Unpushed before dirty.
	all := strings.Join(l, "\n")
	if strings.Index(all, "keytrace") > strings.Index(all, "~/.config/nvim") {
		t.Error("repos with unpushed commits do not come first")
	}
}

func TestReposNoUpstream(t *testing.T) {
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

func TestReposFailing(t *testing.T) {
	out := gitOutputs()
	out["repo-triage --tsv"] = "ERR:repo-triage: exit status 1"
	if data, err := Repos().Collect(context.Background(), fakeEnv(out, nil, nil)); data != nil || err == nil {
		t.Fatalf("want an error and no data, got %v, %v", data, err)
	}
}

func TestReposUnparsable(t *testing.T) {
	out := gitOutputs()
	out["repo-triage --tsv"] = "not a tsv\n"
	if _, err := Repos().Collect(context.Background(), fakeEnv(out, nil, nil)); err == nil {
		t.Fatal("want an error for output with no rows")
	}
}

func TestReposTTLLongerThanGit(t *testing.T) {
	if Repos().TTL <= Git().TTL {
		t.Errorf("Repos TTL %v is not longer than Git's %v", Repos().TTL, Git().TTL)
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
