package collect

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/njhoffman/shdash/internal/format"
)

// ReposData counts repo-triage's repos with local-only work by group (own,
// patched, dirty); Local lists the own ones.
type ReposData struct {
	Groups map[string]int `json:"groups"`
	Local  []LocalRepo    `json:"local"`
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

const reposShown = 4

// Repos is repo-triage's repos with local-only work. repo-triage reads every
// repo (~35 s over ~300), so this panel has a longer TTL than Git's and is
// never collected in the foreground.
func Repos() Collector {
	lines, detail := capped(reposLines)
	return Collector{
		Name: "repos", Title: "Repos", Icon: "\uF401", Priority: 4,
		TTL: time.Hour, Timeout: 3 * time.Minute, Schema: 1,
		Collect: collectRepos,
		Lines:   lines,
		Detail:  detail,
	}
}

func collectRepos(ctx context.Context, env *Env) (any, error) {
	groups, local, err := triage(ctx, env)
	if err != nil {
		return nil, err
	}
	return &ReposData{Groups: groups, Local: local}, nil
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

func reposLines(d *ReposData, env *Env, full bool) []string {
	l := []string{format.KV("local", format.Join(
		format.Count(d.Groups["own"], "own", "own", false),
		format.Count(d.Groups["patched"], "patched", "patched", true),
		format.Count(d.Groups["dirty"], "dirty", "dirty", true)))}
	shown := upTo(d.Local, reposShown, full)
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
