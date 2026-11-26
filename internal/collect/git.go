package collect

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/njhoffman/shdash/internal/format"
)

type GitData struct {
	Recent []Commit `json:"recent"`
	// Groups counts repo-triage's repos with local-only work by group
	// (own, patched, dirty); Local lists the own ones.
	Groups map[string]int `json:"groups"`
	Local  []LocalRepo    `json:"local"`
}

type Commit struct {
	Repo    string    `json:"repo"`
	Subject string    `json:"subject"`
	When    time.Time `json:"when"`
}

type LocalRepo struct {
	Path       string `json:"path"`
	Branch     string `json:"branch"`
	Dirty      int    `json:"dirty"`
	Untracked  int    `json:"untracked"`
	Unpushed   int    `json:"unpushed"`
	NoUpstream bool   `json:"no_upstream"`
	Stashes    int    `json:"stashes"`
}

const (
	gitRecent  = 5
	gitLocal   = 4
	gitWorkers = 8
)

// Git is recent commits across your own repos (ghq list -p under
// $SHDASH_OWNER, default njhoffman, plus the shell repo) and repo-triage's
// repos with local-only work. repo-triage reads every repo (~30 s), so this
// panel is never collected in the foreground.
func Git() Collector {
	return Collector{
		Name: "git", Title: "Git", Priority: 3,
		TTL: 15 * time.Minute, Timeout: 3 * time.Minute, Schema: 1,
		Collect: collectGit,
		Lines:   linesOf(gitLines),
	}
}

func collectGit(ctx context.Context, env *Env) (any, error) {
	d := &GitData{}
	var recentErr, triageErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); d.Recent, recentErr = recentCommits(ctx, env) }()
	go func() { defer wg.Done(); d.Groups, d.Local, triageErr = triage(ctx, env) }()
	wg.Wait()
	if recentErr != nil && triageErr != nil {
		return nil, errors.Join(recentErr, triageErr)
	}
	return d, errors.Join(recentErr, triageErr)
}

func owner(env *Env) string {
	if o := env.Getenv("SHDASH_OWNER"); o != "" {
		return o
	}
	return "njhoffman"
}

// shortRepo drops the ghq root and host from a repo path, and the owner
// when it is you: ~/ghq/github.com/njhoffman/audit is "audit",
// ~/ghq/github.com/someone/tool "someone/tool"; other paths stay as they are.
func shortRepo(p, owner string) string {
	rest, ok := strings.CutPrefix(p, "~/ghq/")
	if !ok {
		return p
	}
	if _, after, ok := strings.Cut(rest, "/"); ok {
		rest = after
	}
	return strings.TrimPrefix(rest, owner+"/")
}

func ownRepos(ctx context.Context, env *Env) ([]string, error) {
	owner := owner(env)
	out, err := env.Run(ctx, "ghq", "list", "-p")
	if err != nil {
		return nil, err
	}
	shellRoot := env.Getenv("SHELL_ROOT")
	if shellRoot == "" {
		shellRoot = filepath.Join(env.Home, ".shell")
	}
	repos := []string{shellRoot}
	for _, p := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.Contains(p, "/"+owner+"/") && p != shellRoot {
			repos = append(repos, p)
		}
	}
	return repos, nil
}

func recentCommits(ctx context.Context, env *Env) ([]Commit, error) {
	repos, err := ownRepos(ctx, env)
	if err != nil {
		return nil, err
	}
	commits := make([]*Commit, len(repos))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range gitWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				commits[i] = lastCommit(ctx, env, repos[i])
			}
		}()
	}
	for i := range repos {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	var all []Commit
	for _, c := range commits {
		if c != nil {
			all = append(all, *c)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].When.After(all[j].When) })
	if len(all) > gitRecent {
		all = all[:gitRecent]
	}
	return all, nil
}

func lastCommit(ctx context.Context, env *Env, repo string) *Commit {
	out, err := env.Run(ctx, "git", "-C", repo, "log", "-1", "--format=%ct%x09%s")
	if err != nil {
		return nil
	}
	ts, subject, ok := strings.Cut(strings.TrimSpace(string(out)), "\t")
	n, err := strconv.ParseInt(ts, 10, 64)
	if !ok || err != nil {
		return nil
	}
	return &Commit{Repo: strings.TrimPrefix(filepath.Base(repo), "."), Subject: subject, When: time.Unix(n, 0)}
}

// triage parses repo-triage --tsv: group path branch dirty untracked
// unpushed no_upstream stashes remote.
func triage(ctx context.Context, env *Env) (map[string]int, []LocalRepo, error) {
	out, err := env.Run(ctx, "repo-triage", "--tsv")
	if err != nil {
		return nil, nil, err
	}
	groups := map[string]int{}
	var local []LocalRepo
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 8 {
			continue
		}
		groups[f[0]]++
		if f[0] != "own" {
			continue
		}
		local = append(local, LocalRepo{
			Path: f[1], Branch: f[2],
			Dirty: atoi(f[3]), Untracked: atoi(f[4]), Unpushed: atoi(f[5]),
			NoUpstream: f[6] != "-" && f[6] != "" && f[6] != "0",
			Stashes:    atoi(f[7]),
		})
	}
	if len(groups) == 0 && strings.TrimSpace(string(out)) != "" {
		return nil, nil, fmt.Errorf("repo-triage: no rows in its output")
	}
	sort.SliceStable(local, func(i, j int) bool {
		a, b := local[i], local[j]
		if a.Unpushed != b.Unpushed {
			return a.Unpushed > b.Unpushed
		}
		return a.Dirty+a.Untracked > b.Dirty+b.Untracked
	})
	return groups, local, nil
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func gitLines(d *GitData, env *Env) []string {
	var l []string
	width := 0
	for _, c := range d.Recent {
		width = max(width, len(c.Repo))
	}
	for _, c := range d.Recent {
		l = append(l, format.Dim.Render(format.Pad(format.Age(env.Now().Sub(c.When)), 4))+" "+
			format.Accent.Render(format.Pad(c.Repo, width))+" "+c.Subject)
	}
	if d.Groups != nil {
		l = append(l, format.KV("local", format.Join(
			format.Count(d.Groups["own"], "own", "own", false),
			format.Count(d.Groups["patched"], "patched", "patched", true),
			format.Count(d.Groups["dirty"], "dirty", "dirty", true))))
	}
	shown := d.Local
	if len(shown) > gitLocal {
		shown = shown[:gitLocal]
	}
	for _, r := range shown {
		state := format.Join(
			highlight(r.Unpushed > 0, format.Count(r.Unpushed, "unpushed", "unpushed", true)),
			format.Count(r.Dirty, "changed", "changed", true),
			format.Count(r.Untracked, "untracked", "untracked", true),
			format.Count(r.Stashes, "stash", "stashes", true),
			map[bool]string{true: "no upstream"}[r.NoUpstream])
		l = append(l, "  "+shortRepo(r.Path, owner(env))+" "+format.Dim.Render(r.Branch)+"  "+state)
	}
	if more := len(d.Local) - len(shown); more > 0 {
		l = append(l, format.Dim.Render(fmt.Sprintf("  %d more (repo-triage)", more)))
	}
	return l
}
