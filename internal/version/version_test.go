package version

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	installed := &debug.BuildInfo{Main: debug.Module{Version: "v0.3.1"}}
	stamped := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "db3e35d0123456789"},
			{Key: "vcs.time", Value: "2026-11-26T06:10:00Z"},
		},
	}
	for name, c := range map[string]struct {
		version, commit, date string
		info                  *debug.BuildInfo
		want                  string
	}{
		"no build info":        {"dev", unset, unset, nil, "dev unknown unknown"},
		"go install @version":  {"dev", unset, unset, installed, "v0.3.1 unknown unknown"},
		"go build in checkout": {"dev", unset, unset, stamped, "dev db3e35d 2026-11-26T06:10:00Z"},
		"ldflags win":          {"v0.3.0", "abc1234", "2026-01-01T00:00:00Z", stamped, "v0.3.0 abc1234 2026-01-01T00:00:00Z"},
		"short revision":       {"dev", unset, unset, &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}}}, "dev abc unknown"},
	} {
		v, cm, d := resolve(c.version, c.commit, c.date, c.info)
		if got := strings.Join([]string{v, cm, d}, " "); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}
