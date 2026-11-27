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
	// The cache keeps up to this many projects and directories, for the
	// full-screen panel.
	projectsStored = 50
	dirsStored     = 50
)

// Projects is the most recent Claude Code projects (claude-projects), the
// tmux sessions, and the most recently visited directories from z.lua's
// data file.
func Projects() Collector {
	lines, detail := capped(projectLines)
	return Collector{
		Name: "projects", Title: "Projects", Icon: "\U000F024B", Priority: 2,
		TTL: 5 * time.Minute, Timeout: 20 * time.Second, Schema: 3,
		Collect: collectProjects,
		Lines:   lines,
		Detail:  detail,
	}
}

func collectProjects(ctx context.Context, env *Env) (any, error) {
	d := &ProjectsData{Dirs: recentDirs(ctx, env)}
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
	if len(ps) > projectsStored {
		ps = ps[:projectsStored]
	}
	return ps, nil
}

// dirCheckBudget bounds the time spent checking that z.lua's directories
// still exist: a path on a network mount (an automount whose server is down)
// can block a stat for the mount's whole timeout.
var dirCheckBudget = time.Second

// recentDirs reads z.lua's data file ($_ZL_DATA, else ~/.zlua: one
// path|rank|unix-time line per directory) and returns the most recently
// visited directories, newest first. $HOME, /, directories that no longer
// exist and those inside a more recent one already listed are left out; a
// directory not checked within dirCheckBudget (or ctx) counts as existing.
// A missing or unreadable file is no directories.
func recentDirs(ctx context.Context, env *Env) []RecentDir {
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
	ctx, cancel := context.WithTimeout(ctx, dirCheckBudget)
	defer cancel()
	var out []RecentDir
	for _, d := range all {
		if len(out) == dirsStored {
			break
		}
		if d.Path == "/" || d.Path == filepath.Clean(env.Home) || insideAny(d.Path, out) {
			continue
		}
		if !isDirWithin(ctx, env, d.Path) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// isDirWithin is env.IsDir, or true when ctx ends first (the check is left
// running; it only reads).
func isDirWithin(ctx context.Context, env *Env, p string) bool {
	if env.IsDir == nil || ctx.Err() != nil {
		return true
	}
	res := make(chan bool, 1)
	go func() { res <- env.IsDir(p) }()
	select {
	case ok := <-res:
		return ok
	case <-ctx.Done():
		return true
	}
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

// tmuxRow is the session count and the attached sessions for the snapshot
// (every name is cut at 80 columns once there are a few), every session's
// name for the full-screen panel. Attached ones are bold, with a "*".
func tmuxRow(sessions []TmuxSession, full bool) string {
	var names []string
	for _, s := range sessions {
		switch {
		case s.Attached:
			names = append(names, format.Strong.Render(s.Name+"*"))
		case full:
			names = append(names, s.Name)
		}
	}
	if full {
		return strings.Join(names, " ")
	}
	return format.Join(format.Count(len(sessions), "session", "sessions", false), strings.Join(names, " "))
}

func projectLines(d *ProjectsData, env *Env, full bool) []string {
	var l []string
	shown := upTo(d.Projects, projectsShown, full)
	rows := make([][]string, 0, len(shown))
	for _, p := range shown {
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
		rows = append(rows, []string{
			// Dense columns first: at 38 columns a sparse one (changed)
			// would push every row's todos off the panel.
			format.Accent.Render(p.Name), p.Version, branch, todo,
			highlight(p.Overdue > 0, format.Count(p.Overdue, "overdue", "overdue", true)),
			format.Count(p.Dirty, "changed", "changed", true),
			last})
	}
	l = append(l, format.Table(rows)...)
	if len(d.Tmux) > 0 {
		l = append(l, format.KV("tmux", tmuxRow(d.Tmux, full)))
	}
	if len(d.Dirs) > 0 {
		l = append(l, format.Dim.Render("dirs"))
		for _, dir := range upTo(d.Dirs, dirsShown, full) {
			l = append(l, "  "+format.Dim.Render(format.Pad(format.Age(env.Now().Sub(dir.When)), 4))+" "+env.Tilde(dir.Path))
		}
	}
	return l
}
