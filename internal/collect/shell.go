package collect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/njhoffman/shdash/internal/format"
)

type ShellData struct {
	// Live are the live shell's counts from shell-env-stats' cache (a login
	// zsh takes ~12 s to count, so this never runs one); nil when that cache
	// is missing.
	Live   map[string]int `json:"live,omitempty"`
	LiveAt time.Time      `json:"live_at,omitempty"`
	Repo   *ShellRepo     `json:"repo,omitempty"`
}

type ShellRepo struct {
	Aliases        int               `json:"aliases"`
	Functions      int               `json:"functions"`
	Scripts        int               `json:"scripts"`
	StartupFiles   int               `json:"startup_files"`
	StartupLines   int               `json:"startup_lines"`
	Widgets        int               `json:"widgets"`
	NameViolations int               `json:"name_violations"`
	NamePending    int               `json:"name_pending"`
	NameKept       int               `json:"name_kept"`
	NameClashes    []json.RawMessage `json:"name_clashes"`
}

// Shell is the shell environment: live counts from shell-env-stats' cache,
// the repo's static counts from shell-stats.
func Shell() Collector {
	return Collector{
		Name: "shell", Title: "Shell", Priority: 6,
		TTL: 30 * time.Minute, Timeout: 20 * time.Second, Inline: true, Schema: 1,
		Collect: collectShell,
		Lines:   linesOf(shellLines),
	}
}

func collectShell(ctx context.Context, env *Env) (any, error) {
	d := &ShellData{}
	var errs []error
	livePath := filepath.Join(env.XDG("XDG_CACHE_HOME", ".cache"), "shell", "env-stats-live")
	if b, err := env.ReadFile(livePath); err == nil {
		d.Live, d.LiveAt = parseLiveStats(string(b))
	}
	out, err := env.Run(ctx, "shell-stats", "--json")
	if err == nil {
		var r struct {
			Repo *ShellRepo `json:"repo"`
		}
		if err = json.Unmarshal(out, &r); err == nil {
			d.Repo = r.Repo
		}
	}
	if err != nil {
		errs = append(errs, fmt.Errorf("shell-stats: %w", err))
	}
	if d.Live == nil && d.Repo == nil {
		return nil, errors.Join(append(errs, errors.New("no live counts yet (run shell-env-stats live)"))...)
	}
	return d, errors.Join(errs...)
}

// parseLiveStats reads "FINGERPRINT EPOCH key=value ...".
func parseLiveStats(s string) (map[string]int, time.Time) {
	f := strings.Fields(s)
	if len(f) < 3 {
		return nil, time.Time{}
	}
	var at time.Time
	if ts, err := strconv.ParseInt(f[1], 10, 64); err == nil {
		at = time.Unix(ts, 0)
	}
	m := map[string]int{}
	for _, kv := range f[2:] {
		k, v, ok := strings.Cut(kv, "=")
		if n, err := strconv.Atoi(v); ok && err == nil {
			m[k] = n
		}
	}
	if len(m) == 0 {
		return nil, time.Time{}
	}
	return m, at
}

func shellLines(d *ShellData, env *Env) []string {
	var l []string
	if v := d.Live; v != nil {
		l = append(l,
			format.KV("live", format.Join(
				format.Count(v["aliases"], "alias", "aliases", true),
				format.Count(v["functions"], "function", "functions", true),
				format.Count(v["widgets"], "widget", "widgets", true))),
			format.KV("plugins", format.Join(
				format.Count(v["zinit_plugins"], "zinit", "zinit", false),
				format.Count(v["zinit_snippets"], "snippet", "snippets", true))),
			format.KV("path", format.Join(
				format.Count(v["commands_unique"], "command", "commands", false),
				format.Count(v["paths"], "dir", "dirs", true))))
	} else {
		l = append(l, format.KV("live", format.Dim.Render("no counts yet (shell-env-stats live)")))
	}
	if r := d.Repo; r != nil {
		l = append(l,
			format.KV("repo", format.Join(
				format.Count(r.Aliases, "alias", "aliases", false),
				format.Count(r.Functions, "function", "functions", false),
				format.Count(r.Scripts, "script", "scripts", false))),
			format.KV("startup", format.Join(
				format.Count(r.StartupFiles, "file", "files", false),
				format.Count(r.StartupLines, "line", "lines", false))))
		names := format.Join(
			format.Count(r.NameViolations, "violation", "violations", false),
			format.Count(r.NamePending, "pending", "pending", true),
			format.Count(len(r.NameClashes), "clash", "clashes", true),
			format.Count(r.NameKept, "kept", "kept", true))
		if r.NameViolations > 0 || len(r.NameClashes) > 0 {
			names = format.Strong.Render(names)
		}
		l = append(l, format.KV("names", names))
	}
	if d.Live != nil && !d.LiveAt.IsZero() {
		l = append(l, format.Dim.Render("live counts "+format.Age(env.Now().Sub(d.LiveAt))+" old"))
	}
	return l
}
