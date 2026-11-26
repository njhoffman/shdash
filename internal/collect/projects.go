package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/njhoffman/shdash/internal/format"
)

type ProjectsData struct {
	Projects []Project     `json:"projects"`
	Tmux     []TmuxSession `json:"tmux"`
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

const projectsShown = 5

// Projects is the most recent Claude Code projects (claude-projects) and
// the tmux sessions.
func Projects() Collector {
	return Collector{
		Name: "projects", Title: "Projects", Priority: 2,
		TTL: 5 * time.Minute, Timeout: 20 * time.Second, Schema: 1,
		Collect: collectProjects,
		Lines:   linesOf(projectLines),
	}
}

func collectProjects(ctx context.Context, env *Env) (any, error) {
	out, err := env.Run(ctx, "claude-projects", "--json", "--sort", "recent")
	if err != nil {
		return nil, err
	}
	d := &ProjectsData{}
	if err := json.Unmarshal(out, &d.Projects); err != nil {
		return nil, fmt.Errorf("claude-projects: %w", err)
	}
	sort.SliceStable(d.Projects, func(i, j int) bool { return d.Projects[i].LastSession > d.Projects[j].LastSession })
	if len(d.Projects) > projectsShown {
		d.Projects = d.Projects[:projectsShown]
	}
	// No tmux server is not an error: the panel just lists no sessions.
	if out, err := env.Run(ctx, "tmux", "list-sessions", "-F",
		"#{session_name}\t#{session_windows}\t#{session_attached}"); err == nil {
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
	return d, nil
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
	return l
}
