package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/njhoffman/shdash/internal/format"
)

type AuditData struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Host        string         `json:"host"`
	Commands    []AuditCommand `json:"commands"`
	Top         []AuditTask    `json:"top"`
}

type AuditCommand struct {
	Command   string         `json:"command"`
	Freshness string         `json:"freshness"`
	Open      map[string]int `json:"open_tasks"`
}

type AuditTask struct {
	Command  string `json:"command"`
	Priority string `json:"priority"`
	Title    string `json:"title"`
}

// auditTopTasks is how many open tasks the panel lists.
const auditTopTasks = 3

var auditPriorities = []string{"high", "medium", "low", "suggestion"}

// Audit reads audit's JSON export (summary.json, tasks.json); it never runs
// a scan.
func Audit() Collector {
	return Collector{
		Name: "audit", Title: "Audit", Priority: 4,
		TTL: 10 * time.Minute, Timeout: 10 * time.Second, Inline: true, Schema: 1,
		Collect: collectAudit,
		Lines:   linesOf(auditLines),
	}
}

// AuditDir is $SHDASH_AUDIT_EXPORT, else audit's default export directory.
func AuditDir(env *Env) string {
	if d := env.Getenv("SHDASH_AUDIT_EXPORT"); d != "" {
		return d
	}
	return filepath.Join(env.XDG("XDG_DATA_HOME", ".local/share"), "audit", "export")
}

func collectAudit(_ context.Context, env *Env) (any, error) {
	dir := AuditDir(env)
	b, err := env.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		return nil, fmt.Errorf("no audit export in %s (audit export writes one)", env.Tilde(dir))
	}
	d := &AuditData{}
	if err := json.Unmarshal(b, d); err != nil {
		return nil, fmt.Errorf("audit summary.json: %w", err)
	}
	b, err = env.ReadFile(filepath.Join(dir, "tasks.json"))
	if err != nil {
		return d, nil
	}
	var t struct {
		Tasks []struct {
			AuditTask
			Status string `json:"status"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return d, fmt.Errorf("audit tasks.json: %w", err)
	}
	rank := map[string]int{}
	for i, p := range auditPriorities {
		rank[p] = i
	}
	var open []AuditTask
	for _, x := range t.Tasks {
		if r, ok := rank[x.Priority]; x.Status == "open" && ok && r < 2 {
			open = append(open, x.AuditTask)
		}
	}
	sort.SliceStable(open, func(i, j int) bool { return rank[open[i].Priority] < rank[open[j].Priority] })
	if len(open) > auditTopTasks {
		open = open[:auditTopTasks]
	}
	d.Top = open
	return d, nil
}

func auditLines(d *AuditData, env *Env) []string {
	var l []string
	age := env.Now().Sub(d.GeneratedAt)
	report := format.Age(age) + " old"
	if age > 7*24*time.Hour {
		report = format.Strong.Render(report)
	}
	l = append(l, format.KV("report", format.Join(report, d.Host)))
	totals := map[string]int{}
	var stale []string
	for _, c := range d.Commands {
		for p, n := range c.Open {
			totals[p] += n
		}
		if c.Freshness != "" && c.Freshness != "fresh" {
			stale = append(stale, c.Command)
		}
	}
	l = append(l, format.KV("open", format.Join(
		highlight(totals["high"] > 0, format.Count(totals["high"], "high", "high", false)),
		format.Count(totals["medium"], "medium", "medium", true),
		format.Count(totals["low"], "low", "low", true),
		format.Count(totals["suggestion"], "suggestion", "suggestions", true))))
	if len(stale) > 0 {
		l = append(l, format.KV("stale", strings.Join(stale, ", ")))
	}
	for _, t := range d.Top {
		title := t.Title
		if env.Home != "" {
			title = strings.ReplaceAll(title, env.Home+"/", "~/")
		}
		l = append(l, format.KV(t.Priority, title))
	}
	return l
}

func highlight(on bool, s string) string {
	if on {
		return format.Strong.Render(s)
	}
	return s
}
