package collect

import (
	"context"
	"strings"
	"testing"
)

const auditDir = "/home/u/.local/share/audit/export"

func auditFiles(t *testing.T) map[string]string {
	return map[string]string{
		auditDir + "/summary.json": fixture(t, "audit-summary.json"),
		auditDir + "/tasks.json":   fixture(t, "audit-tasks.json"),
	}
}

func TestAuditSummaryAndTopTasks(t *testing.T) {
	env := fakeEnv(nil, auditFiles(t), nil)
	env.Home = "/home/u"
	l := lines(t, Audit(), env)
	assertHas(t, l, "3d old", "charon", "high", "stale   packages",
		"~/.local/bin/x runs /sbin/missing", "second high", "a medium one")
	// Three tasks: both open highs, then the first medium; nothing done, low or
	// a suggestion.
	assertLacks(t, l, "second medium", "a low one", "a finished high one", "a suggestion", "/home/u/")
}

func TestAuditOpenTotals(t *testing.T) {
	summary := `{"generated_at": "2026-11-25T11:00:00Z", "host": "h", "commands": [
	  {"command": "a", "freshness": "fresh", "open_tasks": {"high": 1, "medium": 2, "low": 0, "suggestion": 4}},
	  {"command": "b", "freshness": "fresh", "open_tasks": {"high": 2, "medium": 0, "low": 0, "suggestion": 1}}]}`
	env := fakeEnv(nil, map[string]string{auditDir + "/summary.json": summary}, nil)
	l := lines(t, Audit(), env)
	assertHas(t, l, "1h old", "3 high", "2 medium", "5 suggestions")
	assertLacks(t, l, "low", "stale")
}

func TestAuditExportDirOverride(t *testing.T) {
	env := fakeEnv(nil, map[string]string{"/elsewhere/summary.json": fixture(t, "audit-summary.json")},
		map[string]string{"SHDASH_AUDIT_EXPORT": "/elsewhere"})
	assertHas(t, lines(t, Audit(), env), "charon")
}

func TestAuditMissingExport(t *testing.T) {
	_, err := Audit().Collect(context.Background(), fakeEnv(nil, nil, nil))
	if err == nil || !strings.Contains(err.Error(), "~/.local/share/audit/export") {
		t.Fatalf("want a missing-export error naming the directory, got %v", err)
	}
}

func TestAuditBrokenTasksKeepsSummary(t *testing.T) {
	files := auditFiles(t)
	files[auditDir+"/tasks.json"] = "{"
	data, err := Audit().Collect(context.Background(), fakeEnv(nil, files, nil))
	if data == nil || err == nil {
		t.Fatalf("want the summary with a tasks.json error, got %v, %v", data, err)
	}
}
