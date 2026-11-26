package dash

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/njhoffman/shdash/internal/cache"
	"github.com/njhoffman/shdash/internal/collect"
)

var now = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

type value struct{ N int }

// counter is a collector that returns value{N} or err, counting its calls.
func counter(name string, calls *int, n *int, err *error) collect.Collector {
	return collect.Collector{
		Name: name, Title: strings.ToUpper(name), Priority: 1, TTL: 10 * time.Minute,
		Timeout: time.Second, Schema: 1,
		Collect: func(context.Context, *collect.Env) (any, error) {
			*calls++
			if *err != nil {
				return nil, *err
			}
			return value{*n}, nil
		},
		Lines: func(raw json.RawMessage, _ *collect.Env) ([]string, error) {
			var v value
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, err
			}
			return []string{"n=" + string(rune('0'+v.N))}, nil
		},
	}
}

func env(at time.Time) *collect.Env {
	return &collect.Env{Now: func() time.Time { return at }, Getenv: func(string) string { return "" }}
}

func TestDue(t *testing.T) {
	c := collect.Collector{TTL: 10 * time.Minute}
	for name, tc := range map[string]struct {
		e    *cache.Entry
		want bool
	}{
		"missing":         {nil, true},
		"fresh":           {&cache.Entry{FetchedAt: now.Add(-time.Minute)}, false},
		"expired":         {&cache.Entry{FetchedAt: now.Add(-11 * time.Minute)}, true},
		"failed recently": {&cache.Entry{FetchedAt: now.Add(-time.Hour), AttemptedAt: now.Add(-time.Minute)}, false},
		"failed long ago": {&cache.Entry{FetchedAt: now.Add(-time.Hour), AttemptedAt: now.Add(-20 * time.Minute)}, true},
	} {
		if got := (State{c, tc.e}).Due(now); got != tc.want {
			t.Errorf("%s: Due = %v, want %v", name, got, tc.want)
		}
	}
}

func TestRefreshFailureKeepsData(t *testing.T) {
	dir := t.TempDir()
	calls, n := 0, 3
	var err error
	c := counter("x", &calls, &n, &err)
	states, rerr := RefreshAll(context.Background(), dir, Load(dir, []collect.Collector{c}), env(now))
	if rerr != nil || states[0].E.Error != "" {
		t.Fatalf("first refresh: %v %+v", rerr, states[0].E)
	}
	err = errors.New("tool broke")
	later := now.Add(time.Hour)
	states, _ = RefreshAll(context.Background(), dir, Load(dir, []collect.Collector{c}), env(later))
	e := states[0].E
	if e.Error != "tool broke" || string(e.Data) != `{"N":3}` || !e.FetchedAt.Equal(now) || !e.AttemptedAt.Equal(later) {
		t.Fatalf("after a failure: %+v", e)
	}
	// It is saved that way, too.
	saved, _ := cache.Load(dir, "x")
	if saved.Error != "tool broke" || string(saved.Data) != `{"N":3}` {
		t.Fatalf("saved: %+v", saved)
	}
	p := Panels(states, env(later), false)
	if len(p) != 1 || !strings.Contains(strings.Join(p[0].Lines, "\n"), "refresh failed: tool broke") {
		t.Fatalf("panel: %+v", p)
	}
}

func TestLoadIgnoresOtherSchema(t *testing.T) {
	dir := t.TempDir()
	cache.Save(dir, "x", &cache.Entry{Schema: 0, FetchedAt: now, Data: json.RawMessage(`{"old":true}`)})
	calls, n := 0, 1
	var err error
	if s := Load(dir, []collect.Collector{counter("x", &calls, &n, &err)}); s[0].E != nil {
		t.Fatalf("old-schema entry was loaded: %+v", s[0].E)
	}
}

func TestPanels(t *testing.T) {
	calls, n := 0, 1
	var err error
	c := counter("x", &calls, &n, &err)
	states := []State{
		{c, nil},
		{c, &cache.Entry{Schema: 1, AttemptedAt: now, Error: "never worked"}},
		{c, &cache.Entry{Schema: 1, FetchedAt: now.Add(-time.Minute), Data: json.RawMessage(`{"N":2}`)}},
		{c, &cache.Entry{Schema: 1, FetchedAt: now.Add(-time.Hour), Data: json.RawMessage(`{"N":4}`)}},
		{c, &cache.Entry{Schema: 1, FetchedAt: now, Data: json.RawMessage(`"not an object"`)}},
	}
	p := Panels(states, env(now), false)
	if len(p) != 5 {
		t.Fatalf("want a panel per state, got %d: %+v", len(p), p)
	}
	if !strings.Contains(p[0].Lines[0], "collecting") {
		t.Errorf("missing entry: %+v", p[0])
	}
	if len(p[1].Lines) != 1 || p[1].Lines[0] != "failed: never worked" || p[1].Note != "0s" {
		t.Errorf("error-only entry: %+v", p[1])
	}
	if p[2].Lines[0] != "n=2" || p[2].Note != "1m" {
		t.Errorf("fresh entry: %+v", p[2])
	}
	if p[3].Lines[0] != "n=4" || p[3].Note != "1h" {
		t.Errorf("stale entry: %+v", p[3])
	}
	if len(p[4].Lines) != 1 || !strings.HasPrefix(p[4].Lines[0], "cannot show cached data: ") {
		t.Errorf("undecodable entry: %+v", p[4])
	}
}

func TestPanelsIcons(t *testing.T) {
	calls, n := 0, 1
	var err error
	c := counter("x", &calls, &n, &err)
	c.Icon = "I"
	states := []State{{c, nil}}
	if p := Panels(states, env(now), false); p[0].Icon != "" {
		t.Errorf("icon without icons: %q", p[0].Icon)
	}
	if p := Panels(states, env(now), true); p[0].Icon != "I" {
		t.Errorf("icon with icons: %q", p[0].Icon)
	}
}

func TestDetailFallsBackToLines(t *testing.T) {
	calls, n := 0, 2
	var err error
	c := counter("x", &calls, &n, &err)
	st := State{c, &cache.Entry{Schema: 1, FetchedAt: now, Data: json.RawMessage(`{"N":2}`)}}
	if p := Detail(st, env(now), false); p.Lines[0] != "n=2" {
		t.Errorf("no Detail: %+v", p)
	}
	c.Detail = func(json.RawMessage, *collect.Env) ([]string, error) { return []string{"full"}, nil }
	st.C = c
	if p := Detail(st, env(now), false); p.Lines[0] != "full" {
		t.Errorf("with Detail: %+v", p)
	}
	if p := Panels([]State{st}, env(now), false); p[0].Lines[0] != "n=2" {
		t.Errorf("Panels used Detail: %+v", p[0])
	}
}
