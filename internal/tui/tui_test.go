package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/njhoffman/shdash/internal/cache"
	"github.com/njhoffman/shdash/internal/collect"
)

var now = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

type fakes struct {
	mu    sync.Mutex
	calls map[string]int
}

// collector is a panel of n lines named "<name> line i" whose Collect
// returns "<name> new".
func (f *fakes) collector(name string, prio, n int) collect.Collector {
	return collect.Collector{
		Name: name, Title: strings.ToUpper(name), Icon: "@", Priority: prio,
		TTL: 10 * time.Minute, Timeout: time.Second, Schema: 1,
		Collect: func(context.Context, *collect.Env) (any, error) {
			f.mu.Lock()
			f.calls[name]++
			f.mu.Unlock()
			return []string{name + " new"}, nil
		},
		Lines: func(raw json.RawMessage, _ *collect.Env) ([]string, error) {
			var l []string
			if err := json.Unmarshal(raw, &l); err != nil {
				return nil, err
			}
			return l, nil
		},
	}
}

func lines(name string, n int) json.RawMessage {
	l := make([]string, n)
	for i := range l {
		l[i] = fmt.Sprintf("%s line %d", name, i)
	}
	b, _ := json.Marshal(l)
	return b
}

type setup struct {
	f   *fakes
	opt Options
}

// newSetup caches fresh data for: a (prio 1, 3 lines), b (2, 30 lines),
// c (3, 3 lines), d (4, 3 lines).
func newSetup(t *testing.T) *setup {
	s := &setup{f: &fakes{calls: map[string]int{}}}
	dir := t.TempDir()
	sizes := map[string]int{"a": 3, "b": 30, "c": 3, "d": 3}
	var cs []collect.Collector
	for i, n := range []string{"a", "b", "c", "d"} {
		cs = append(cs, s.f.collector(n, i+1, sizes[n]))
		cache.Save(dir, n, &cache.Entry{Schema: 1, FetchedAt: now, AttemptedAt: now, Data: lines(n, sizes[n])})
	}
	s.opt = Options{
		CacheDir: dir, Collectors: cs,
		Env: &collect.Env{Now: func() time.Time { return now }, Getenv: func(string) string { return "" }},
	}
	return s
}

func send(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		m = next.(Model)
		m = drain(t, m, cmd)
	}
	return m
}

// drain runs a command and feeds its messages back, skipping ticks (which
// would wait) and quit.
func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(50 * time.Millisecond):
		return m // a tick: not run in tests
	}
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = drain(t, m, c)
		}
		return m
	case tickMsg, tea.QuitMsg, nil:
		return m
	}
	next, cmd := m.Update(msg)
	return drain(t, next.(Model), cmd)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func size(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }

func focused(m Model) string { return m.states[m.focus].C.Name }

func TestViewFitsTheScreen(t *testing.T) {
	m := send(t, New(newSetup(t).opt), size(100, 20))
	v := strings.Split(m.View(), "\n")
	if len(v) != 20 {
		t.Fatalf("view has %d lines, want 20", len(v))
	}
	if !strings.Contains(v[19], "q quit") {
		t.Errorf("status line: %q", v[19])
	}
	if !strings.Contains(m.View(), "A") || !strings.Contains(m.View(), "a line 0") {
		t.Errorf("view:\n%s", m.View())
	}
}

func TestFocusMovesBetweenColumnsAndPanels(t *testing.T) {
	// Two columns at 100: a (col 0), b (col 1, tall), c under a, d under c.
	m := send(t, New(newSetup(t).opt), size(100, 50))
	steps := []struct{ key, want string }{
		{"j", "c"}, {"j", "d"}, {"j", "d"}, // bottom of column 0
		{"l", "b"}, {"l", "b"}, // no column 2
		{"h", "a"}, // b's top is nearest a's
		{"k", "a"},
		{"tab", "b"}, {"tab", "c"}, {"shift+tab", "b"},
	}
	for i, s := range steps {
		m = send(t, m, key(s.key))
		if got := focused(m); got != s.want {
			t.Fatalf("step %d (%s): focus %s, want %s", i, s.key, got, s.want)
		}
	}
}

func TestFocusScrollsIntoView(t *testing.T) {
	// One column at 60: a (4 lines), b (31), c, d; 10 rows of screen.
	m := send(t, New(newSetup(t).opt), size(60, 11))
	m = send(t, m, key("j"), key("j"))
	if focused(m) != "c" || m.offset == 0 {
		t.Fatalf("focus %s, offset %d", focused(m), m.offset)
	}
	if !strings.Contains(m.View(), "C") {
		t.Errorf("focused panel not on screen:\n%s", m.View())
	}
	m = send(t, m, key("k"), key("k"))
	if focused(m) != "a" || m.offset != 0 {
		t.Errorf("back to a: focus %s, offset %d", focused(m), m.offset)
	}
}

func TestDetailScrolls(t *testing.T) {
	m := send(t, New(newSetup(t).opt), size(60, 11), key("j"), key("enter"))
	if !m.detail || !strings.Contains(m.View(), "b line 0") || strings.Contains(m.View(), "b line 20") {
		t.Fatalf("detail:\n%s", m.View())
	}
	m = send(t, m, key("G"))
	if !strings.Contains(m.View(), "b line 29") {
		t.Errorf("end of detail:\n%s", m.View())
	}
	m = send(t, m, key("g"))
	if m.detailOff != 0 {
		t.Errorf("g: offset %d", m.detailOff)
	}
	m = send(t, m, key("esc"))
	if m.detail || focused(m) != "b" {
		t.Errorf("esc: detail %v focus %s", m.detail, focused(m))
	}
}

func TestRefreshFocusedAndAll(t *testing.T) {
	s := newSetup(t)
	m := send(t, New(s.opt), size(100, 40), key("r"))
	if s.f.calls["a"] != 1 || len(s.f.calls) != 1 {
		t.Fatalf("r: calls %v", s.f.calls)
	}
	if !strings.Contains(m.View(), "a new") || len(m.refreshing) != 0 {
		t.Errorf("after r:\n%s", m.View())
	}
	send(t, m, key("R"))
	for _, n := range []string{"a", "b", "c", "d"} {
		if want := map[string]int{"a": 2}[n]; s.f.calls[n] != max(want, 1) {
			t.Errorf("R: calls %v", s.f.calls)
			break
		}
	}
}

func TestRefreshWhileLocked(t *testing.T) {
	s := newSetup(t)
	unlock, ok, _ := cache.Lock(s.opt.CacheDir, false)
	if !ok {
		t.Fatal("lock")
	}
	defer unlock()
	m := send(t, New(s.opt), size(100, 40), key("r"))
	if len(s.f.calls) != 0 || !strings.Contains(m.View(), "background refresh is running") || len(m.refreshing) != 0 {
		t.Errorf("calls %v, refreshing %v:\n%s", s.f.calls, m.refreshing, m.View())
	}
}

func TestTickPicksUpOtherRefreshes(t *testing.T) {
	s := newSetup(t)
	m := send(t, New(s.opt), size(100, 40))
	cache.Save(s.opt.CacheDir, "c", &cache.Entry{Schema: 1, FetchedAt: now, AttemptedAt: now, Data: json.RawMessage(`["c from elsewhere"]`)})
	m = send(t, m, tickMsg{})
	if !strings.Contains(m.View(), "c from elsewhere") {
		t.Errorf("tick did not reload:\n%s", m.View())
	}
	if len(s.f.calls) != 0 {
		t.Errorf("fresh panels refreshed without AutoRefresh: %v", s.f.calls)
	}
}

func TestAutoRefreshDuePanels(t *testing.T) {
	s := newSetup(t)
	s.opt.AutoRefresh = true
	later := now.Add(time.Hour)
	s.opt.Env.Now = func() time.Time { return later }
	m := New(s.opt)
	m = drain(t, m, m.Init())
	if len(s.f.calls) != 4 {
		t.Fatalf("Init: calls %v", s.f.calls)
	}
	// Now fresh: a tick refreshes nothing.
	send(t, m, tickMsg{})
	if s.f.calls["a"] != 1 {
		t.Errorf("tick refreshed fresh panels: %v", s.f.calls)
	}
}

func TestRefreshingNoteAndNoDoubleRefresh(t *testing.T) {
	s := newSetup(t)
	m := send(t, New(s.opt), size(100, 40))
	m.refreshing["a"] = true
	if !strings.Contains(m.View(), "refreshing") {
		t.Errorf("no refreshing note:\n%s", m.View())
	}
	if cmd := m.startRefresh([]string{"a"}); cmd != nil {
		t.Error("a second refresh of a refreshing panel was started")
	}
}

func TestQuit(t *testing.T) {
	_, cmd := New(newSetup(t).opt).Update(key("q"))
	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q did not quit")
	}
}

func TestNoPanels(t *testing.T) {
	opt := newSetup(t).opt
	opt.Collectors = nil
	m := send(t, New(opt), size(80, 10), key("j"), key("tab"), key("enter"), key("r"))
	if len(strings.Split(m.View(), "\n")) != 10 {
		t.Errorf("view:\n%s", m.View())
	}
}

func TestDetailUsesTheFullRenderer(t *testing.T) {
	s := newSetup(t)
	s.opt.Collectors[0].Detail = func(json.RawMessage, *collect.Env) ([]string, error) {
		return []string{"every a line"}, nil
	}
	m := send(t, New(s.opt), size(100, 20))
	if strings.Contains(m.View(), "every a line") {
		t.Fatal("grid used Detail")
	}
	m = send(t, m, key("enter"))
	if !strings.Contains(m.View(), "every a line") {
		t.Errorf("detail did not use Detail:\n%s", m.View())
	}
}

func TestRefreshesInOneProcessTakeTurns(t *testing.T) {
	s := newSetup(t)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	slow := s.opt.Collectors[1] // b
	collectB := slow.Collect
	slow.Collect = func(ctx context.Context, env *collect.Env) (any, error) {
		started <- struct{}{}
		<-release
		return collectB(ctx, env)
	}
	s.opt.Collectors[1] = slow
	m := send(t, New(s.opt), size(100, 40))
	first := m.startRefresh([]string{"b"})
	second := m.startRefresh([]string{"a"})
	results := make(chan tea.Msg, 2)
	go func() { results <- first() }()
	<-started // b holds the lock
	go func() { results <- second() }()
	select {
	case msg := <-results:
		t.Fatalf("a finished while b held the lock: %+v", msg)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for range 2 {
		msg := (<-results).(refreshedMsg)
		if msg.busy || msg.err != nil {
			t.Errorf("refresh of %v: busy %v, err %v", msg.names, msg.busy, msg.err)
		}
	}
	if s.f.calls["a"] != 1 || s.f.calls["b"] != 1 {
		t.Errorf("calls: %v", s.f.calls)
	}
}

func TestTooNarrow(t *testing.T) {
	m := send(t, New(newSetup(t).opt), size(12, 5))
	v := strings.Split(m.View(), "\n")
	if len(v) != 5 || v[0] != "too narrow" {
		t.Errorf("narrow view: %q", v)
	}
	for _, l := range v {
		if len(l) > 12 {
			t.Errorf("line wider than the screen: %q", l)
		}
	}
	// Widening it again shows the panels.
	if m = send(t, m, size(80, 10)); !strings.Contains(m.View(), "a line 0") {
		t.Errorf("after widening:\n%s", m.View())
	}
}

func TestNeverCollectedSaysCollecting(t *testing.T) {
	s := newSetup(t)
	f := &fakes{calls: map[string]int{}}
	s.opt.Collectors = append(s.opt.Collectors, f.collector("e", 5, 1))
	m := send(t, New(s.opt), size(80, 60))
	view := m.View()
	if !strings.Contains(view, "collecting") || strings.Contains(view, "next run") {
		t.Errorf("view:\n%s", view)
	}
	m.focus = 4
	m = send(t, m, key("enter"))
	if v := m.View(); !strings.Contains(v, "collecting") || strings.Contains(v, "next run") {
		t.Errorf("detail:\n%s", v)
	}
}
