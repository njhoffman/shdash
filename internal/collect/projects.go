package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/njhoffman/shdash/internal/format"
)

type ProjectsData struct {
	Projects []Project     `json:"projects"`
	Tmux     []TmuxSession `json:"tmux"`
	Dirs     []RecentDir   `json:"dirs"`
}

// RecentDir is a directory from z.lua's history and its last visit.
type RecentDir struct {
	Path string    `json:"path"`
	When time.Time `json:"when"`
}

type Project struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Branch      string `json:"branch"`
	Dirty       int    `json:"dirty"`
	Ahead       int    `json:"ahead"`
	Behind      int    `json:"behind"`
	Open        int    `json:"open"`
	P1          int    `json:"p1"`
	Overdue     int    `json:"overdue"`
	LastSession int64  `json:"last_session"`
}

type TmuxSession struct {
	Name     string `json:"name"`
	Windows  int    `json:"windows"`
	Attached bool   `json:"attached"`
}

const (
	projectsShown = 5
	dirsShown     = 5
)

// Projects is the most recent Claude Code projects (claude-projects), the
// tmux sessions, and the most recently visited directories from z.lua's
// data file.
func Projects() Collector {
	return Collector{
		Name: "projects", Title: "Projects", Priority: 2,
		TTL: 5 * time.Minute, Timeout: 20 * time.Second, Schema: 2,
		Collect: collectProjects,
		Lines:   linesOf(projectLines),
	}
}

func collectProjects(ctx context.Context, env *Env) (any, error) {
	d := &ProjectsData{Dirs: recentDirs(env)}
	projects, err := claudeProjects(ctx, env)
	d.Projects = projects
	// No tmux server is not an error: the panel just lists no sessions.
	if out, terr := env.Run(ctx, "tmux", "list-sessions", "-F",
		"#{session_name}\t#{session_windows}\t#{session_attached}"); terr == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			f := strings.Split(line, "\t")
			if len(f) != 3 {
				continue
			}
			w, _ := strconv.Atoi(f[1])
			a, _ := strconv.Atoi(f[2])
			d.Tmux = append(d.Tmux, TmuxSession{Name: f[0], Windows: w, Attached: a > 0})
		}
	}
	if err != nil && len(d.Dirs) == 0 && len(d.Tmux) == 0 {
		return nil, err
	}
	return d, err
}

func claudeProjects(ctx context.Context, env *Env) ([]Project, error) {
	out, err := env.Run(ctx, "claude-projects", "--json", "--sort", "recent")
	if err != nil {
		return nil, err
	}
	var ps []Project
	if err := json.Unmarshal(out, &ps); err != nil {
		return nil, fmt.Errorf("claude-projects: %w", err)
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].LastSession > ps[j].LastSession })
	if len(ps) > projectsShown {
		ps = ps[:projectsShown]
	}
	return ps, nil
}

// recentDirs reads z.lua's data file ($_ZL_DATA, else ~/.zlua: one
// path|rank|unix-time line per directory) and returns the most recently
// visited directories, newest first. $HOME, /, directories that no longer
// exist and those inside a more recent one already listed are left out. A
// missing or unreadable file is no directories.
func recentDirs(env *Env) []RecentDir {
	name := env.Getenv("_ZL_DATA")
	if name == "" {
		name = filepath.Join(env.Home, ".zlua")
	}
	b, err := env.ReadFile(name)
	if err != nil {
		return nil
	}
	var all []RecentDir
	for _, line := range strings.Split(string(b), "\n") {
		rest, ts, ok := cutLast(line, "|")
		if !ok {
			continue
		}
		p, _, ok := cutLast(rest, "|")
		n, err := strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
		if !ok || err != nil || !filepath.IsAbs(p) {
			continue
		}
		all = append(all, RecentDir{Path: filepath.Clean(p), When: time.Unix(n, 0)})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].When.After(all[j].When) })
	var out []RecentDir
	for _, d := range all {
		if len(out) == dirsShown {
			break
		}
		if d.Path == "/" || d.Path == filepath.Clean(env.Home) || insideAny(d.Path, out) {
			continue
		}
		if env.IsDir != nil && !env.IsDir(d.Path) {
			continue
		}
		out = append(out, d)
	}
	return out
}

func cutLast(s, sep string) (before, after string, ok bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

// insideAny is true when p is one of dirs or below one of them.
func insideAny(p string, dirs []RecentDir) bool {
	for _, d := range dirs {
		if p == d.Path || strings.HasPrefix(p, d.Path+"/") {
			return true
		}
	}
	return false
}

func projectLines(d *ProjectsData, env *Env) []string {
	var l []string
	width := 0
	for _, p := range d.Projects {
		width = max(width, len(p.Name))
	}
	for _, p := range d.Projects {
		branch := p.Branch
		if p.Ahead > 0 {
			branch += fmt.Sprintf(" +%d", p.Ahead)
		}
		if p.Behind > 0 {
			branch += fmt.Sprintf(" -%d", p.Behind)
		}
		todo := ""
		if p.Open > 0 {
			todo = format.Count(p.Open, "todo", "todos", false)
			if p.P1 > 0 {
				todo += fmt.Sprintf(" (%d p1)", p.P1)
			}
		}
		last := ""
		if p.LastSession > 0 {
			last = format.Dim.Render(format.Age(env.Now().Sub(time.Unix(p.LastSession, 0))))
		}
		l = append(l, format.Accent.Render(format.Pad(p.Name, width))+" "+format.Join(
			p.Version, branch,
			format.Count(p.Dirty, "changed", "changed", true),
			todo,
			highlight(p.Overdue > 0, format.Count(p.Overdue, "overdue", "overdue", true)),
			last))
	}
	if len(d.Tmux) > 0 {
		names := make([]string, len(d.Tmux))
		for i, s := range d.Tmux {
			names[i] = s.Name
			if s.Attached {
				names[i] = format.Strong.Render(s.Name + "*")
			}
		}
		l = append(l, format.KV("tmux", strings.Join(names, " ")))
	}
	if len(d.Dirs) > 0 {
		l = append(l, format.Dim.Render("dirs"))
		for _, dir := range d.Dirs {
			l = append(l, "  "+format.Dim.Render(format.Pad(format.Age(env.Now().Sub(dir.When)), 4))+" "+env.Tilde(dir.Path))
		}
	}
	return l
}
