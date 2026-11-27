package collect

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/njhoffman/shdash/internal/format"
)

type GitData struct {
	Recent []Commit `json:"recent"`
}

type Commit struct {
	Repo    string    `json:"repo"`
	Subject string    `json:"subject"`
	When    time.Time `json:"when"`
}

const (
	gitRecent = 5
	// gitStored is how many recent commits the cache keeps, for the
	// full-screen panel.
	gitStored  = 50
	gitWorkers = 8
)

// Git is the most recent commits across your own repos (ghq list -p under
// $SHDASH_OWNER, default njhoffman, plus the shell repo). It reads one
// commit per repo, so it is cheap next to Repos' repo-triage, but still
// collected in the background.
func Git() Collector {
	lines, detail := capped(gitLines)
	return Collector{
		Name: "git", Title: "Git", Icon: "\uE702", Priority: 3,
		TTL: 15 * time.Minute, Timeout: time.Minute, Schema: 3,
		Collect: collectGit,
		Lines:   lines,
		Detail:  detail,
	}
}

func collectGit(ctx context.Context, env *Env) (any, error) {
	recent, err := recentCommits(ctx, env)
	if recent == nil && err != nil {
		return nil, err
	}
	return &GitData{Recent: recent}, err
}

func owner(env *Env) string {
	if o := env.Getenv("SHDASH_OWNER"); o != "" {
		return o
	}
	return "njhoffman"
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
	// read counts the repos looked at before the deadline: git fails at once
	// on an ended context, and that is no answer about the repo.
	var read atomic.Int64
	for range gitWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				commits[i] = lastCommit(ctx, env, repos[i])
				if ctx.Err() == nil {
					read.Add(1)
				}
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
	if len(all) > gitStored {
		all = all[:gitStored]
	}
	if err := ctx.Err(); err != nil {
		return all, fmt.Errorf("git log: %w after %d of %d repos", err, read.Load(), len(repos))
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

func gitLines(d *GitData, env *Env, full bool) []string {
	var l []string
	shown := upTo(d.Recent, gitRecent, full)
	width := 0
	for _, c := range shown {
		width = max(width, len(c.Repo))
	}
	for _, c := range shown {
		l = append(l, format.Dim.Render(format.Pad(format.Age(env.Now().Sub(c.When)), 4))+" "+
			format.Muted.Render(format.Pad(c.Repo, width))+" "+c.Subject)
	}
	return l
}
