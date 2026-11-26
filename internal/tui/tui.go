// Package tui is shdash's interactive mode: the snapshot's panels in a
// resize-aware layout, with panel focus, scrolling, a full-screen view of one
// panel, and refreshes run in-process.
package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/njhoffman/shdash/internal/cache"
	"github.com/njhoffman/shdash/internal/collect"
	"github.com/njhoffman/shdash/internal/dash"
	"github.com/njhoffman/shdash/internal/format"
	"github.com/njhoffman/shdash/internal/render"
)

// tick is how often the cache is read again (a background refresh's results
// show up this way) and due panels are refreshed.
const tick = 2 * time.Second

// Options configure a Model.
type Options struct {
	CacheDir   string
	Collectors []collect.Collector
	Env        *collect.Env
	Icons      bool
	// AutoRefresh refreshes panels past their TTL on every tick.
	AutoRefresh bool
}

// Model is the bubbletea model.
type Model struct {
	opt        Options
	states     []dash.State
	width      int
	height     int
	focus      int
	offset     int // first grid line shown
	detail     bool
	detailOff  int // first detail line shown
	refreshing map[string]bool
	status     string
}

// New loads the cache and returns the model.
func New(opt Options) Model {
	return Model{
		opt:        opt,
		states:     dash.Load(opt.CacheDir, opt.Collectors),
		width:      80,
		height:     24,
		refreshing: map[string]bool{},
	}
}

// Run starts interactive mode on the terminal.
func Run(opt Options) error {
	_, err := tea.NewProgram(New(opt), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

type tickMsg struct{}

// refreshedMsg is the end of a refresh: the states it collected, or busy
// when another process held the lock.
type refreshedMsg struct {
	names  []string
	states []dash.State
	busy   bool
	err    error
}

func tickCmd() tea.Cmd { return tea.Tick(tick, func(time.Time) tea.Msg { return tickMsg{} }) }

// Init refreshes what is due at once, then ticks.
func (m Model) Init() tea.Cmd {
	if m.opt.AutoRefresh {
		if names := m.due(); len(names) > 0 {
			return tea.Batch(tickCmd(), m.refreshCmd(names))
		}
	}
	return tickCmd()
}

// due is the panels past their TTL that are not already refreshing.
func (m Model) due() []string {
	var names []string
	for _, s := range m.states {
		if !m.refreshing[s.C.Name] && s.Due(m.opt.Env.Now()) {
			names = append(names, s.C.Name)
		}
	}
	return names
}

// startRefresh marks the panels refreshing and returns the command that
// refreshes them.
func (m *Model) startRefresh(names []string) tea.Cmd {
	var todo []string
	for _, n := range names {
		if !m.refreshing[n] {
			m.refreshing[n] = true
			todo = append(todo, n)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	return m.refreshCmd(todo)
}

// refreshCmd collects the named panels under the refresh lock. When another
// refresh holds the lock it gives up; the next tick reads what that one
// writes.
func (m Model) refreshCmd(names []string) tea.Cmd {
	dir, env := m.opt.CacheDir, m.opt.Env
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var todo []dash.State
	for _, s := range m.states {
		if want[s.C.Name] {
			todo = append(todo, s)
		}
	}
	return func() tea.Msg {
		unlock, ok, err := cache.Lock(dir, false)
		if err != nil {
			return refreshedMsg{names: names, err: err}
		}
		if !ok {
			return refreshedMsg{names: names, busy: true}
		}
		defer unlock()
		done, err := dash.RefreshAll(context.Background(), dir, todo, env)
		return refreshedMsg{names: names, states: done, err: err}
	}
}

// Update handles keys, mouse wheel, resizes, ticks and finished refreshes.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.keepFocusVisible()
		return m, nil
	case tickMsg:
		m.reload()
		cmds := []tea.Cmd{tickCmd()}
		if m.opt.AutoRefresh {
			if names := m.due(); len(names) > 0 {
				cmds = append(cmds, m.startRefresh(names))
			}
		}
		return m, tea.Batch(cmds...)
	case refreshedMsg:
		for _, n := range msg.names {
			delete(m.refreshing, n)
		}
		switch {
		case msg.err != nil:
			m.status = "refresh failed: " + msg.err.Error()
		case msg.busy:
			m.status = "a background refresh is running; its results show when it ends"
		default:
			m.status = ""
		}
		for _, s := range msg.states {
			for i := range m.states {
				if m.states[i].C.Name == s.C.Name && s.E != nil {
					m.states[i] = s
				}
			}
		}
		return m, nil
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scroll(-3)
		case tea.MouseButtonWheelDown:
			m.scroll(3)
		}
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		if len(m.states) > 0 {
			return m, m.startRefresh([]string{m.states[m.focus].C.Name})
		}
	case "R":
		var names []string
		for _, s := range m.states {
			names = append(names, s.C.Name)
		}
		return m, m.startRefresh(names)
	}
	if m.detail {
		switch msg.String() {
		case "esc", "enter", "backspace":
			m.detail = false
			m.keepFocusVisible()
		case "j", "down":
			m.scroll(1)
		case "k", "up":
			m.scroll(-1)
		case "ctrl+d", "pgdown", " ":
			m.scroll(m.bodyHeight() / 2)
		case "ctrl+u", "pgup":
			m.scroll(-m.bodyHeight() / 2)
		case "g", "home":
			m.detailOff = 0
		case "G", "end":
			m.scroll(1 << 20)
		}
		return m, nil
	}
	switch msg.String() {
	case "enter":
		m.detail, m.detailOff = true, 0
	case "j", "down":
		m.moveVertical(1)
	case "k", "up":
		m.moveVertical(-1)
	case "h", "left":
		m.moveColumn(-1)
	case "l", "right":
		m.moveColumn(1)
	case "tab":
		m.cycle(1)
	case "shift+tab":
		m.cycle(-1)
	case "ctrl+d", "pgdown":
		m.scroll(m.bodyHeight() / 2)
	case "ctrl+u", "pgup":
		m.scroll(-m.bodyHeight() / 2)
	}
	return m, nil
}

// reload reads the cache again, keeping any state whose file is gone or
// unreadable.
func (m *Model) reload() {
	fresh := dash.Load(m.opt.CacheDir, m.opt.Collectors)
	for i := range fresh {
		if i < len(m.states) && fresh[i].E == nil {
			fresh[i].E = m.states[i].E
		}
	}
	m.states = fresh
}

// panels are the states as panels, the focused one marked and the
// refreshing ones noted.
func (m Model) panels() []render.Panel {
	ps := dash.Panels(m.states, m.opt.Env, m.opt.Icons)
	for i := range ps {
		ps[i].Focus = i == m.focus
		if i < len(m.states) && m.refreshing[m.states[i].C.Name] {
			ps[i].Note = format.Dim.Render("refreshing ") + ps[i].Note
		}
	}
	return ps
}

func (m Model) arrangement() render.Arrangement { return render.Arrange(m.panels(), m.width) }

// bodyHeight is the screen less the status line.
func (m Model) bodyHeight() int { return max(1, m.height-1) }

func (m *Model) scroll(n int) {
	if m.detail {
		lines := len(m.detailLines())
		m.detailOff = clamp(m.detailOff+n, 0, max(0, lines-m.bodyHeight()))
		return
	}
	lines := len(m.arrangement().Lines)
	m.offset = clamp(m.offset+n, 0, max(0, lines-m.bodyHeight()))
}

// keepFocusVisible scrolls the grid so the focused panel's title, and as
// much of the panel as fits, is on screen.
func (m *Model) keepFocusVisible() {
	if len(m.states) == 0 {
		return
	}
	m.focus = clamp(m.focus, 0, len(m.states)-1)
	a := m.arrangement()
	spot := a.Spots[m.focus]
	end := spot.Row + len(render.Block(m.panels()[m.focus], m.width))
	h := m.bodyHeight()
	if end > m.offset+h {
		m.offset = end - h
	}
	if spot.Row < m.offset || end-spot.Row > h {
		m.offset = spot.Row
	}
	m.offset = clamp(m.offset, 0, max(0, len(a.Lines)-h))
}

// moveVertical moves to the next panel above or below in the same column.
func (m *Model) moveVertical(dir int) {
	if len(m.states) == 0 {
		return
	}
	a := m.arrangement()
	cur := a.Spots[m.focus]
	best := -1
	for i, s := range a.Spots {
		if s.Col != cur.Col || (s.Row-cur.Row)*dir <= 0 {
			continue
		}
		if best < 0 || abs(s.Row-cur.Row) < abs(a.Spots[best].Row-cur.Row) {
			best = i
		}
	}
	if best >= 0 {
		m.focus = best
		m.keepFocusVisible()
	}
}

// moveColumn moves to the column on either side, to the panel whose top is
// nearest the current one's.
func (m *Model) moveColumn(dir int) {
	if len(m.states) == 0 {
		return
	}
	a := m.arrangement()
	cur := a.Spots[m.focus]
	best := -1
	for i, s := range a.Spots {
		if s.Col != cur.Col+dir {
			continue
		}
		if best < 0 || abs(s.Row-cur.Row) < abs(a.Spots[best].Row-cur.Row) {
			best = i
		}
	}
	if best >= 0 {
		m.focus = best
		m.keepFocusVisible()
	}
}

// cycle moves through the panels in priority order, wrapping.
func (m *Model) cycle(dir int) {
	n := len(m.states)
	if n == 0 {
		return
	}
	order := m.priorityOrder()
	pos := 0
	for i, idx := range order {
		if idx == m.focus {
			pos = i
		}
	}
	m.focus = order[(pos+dir+n)%n]
	m.keepFocusVisible()
}

func (m Model) priorityOrder() []int {
	order := make([]int, len(m.states))
	for i := range order {
		order[i] = i
	}
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && m.states[order[j]].C.Priority < m.states[order[j-1]].C.Priority; j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	return order
}

func (m Model) detailLines() []string {
	ps := m.panels()
	if len(ps) == 0 {
		return nil
	}
	p := ps[m.focus]
	p.Focus = false
	return render.Block(p, m.width)
}

// View draws the grid or the focused panel, and the status line.
func (m Model) View() string {
	var body []string
	var help string
	if m.detail {
		lines := m.detailLines()
		body = window(lines, m.detailOff, m.bodyHeight())
		help = "j/k scroll · esc back · r refresh · q quit"
	} else {
		body = window(m.arrangement().Lines, m.offset, m.bodyHeight())
		help = "hjkl/tab move · enter open · r refresh · R all · q quit"
	}
	for len(body) < m.bodyHeight() {
		body = append(body, "")
	}
	status := help
	if m.status != "" {
		status = m.status + " · " + help
	}
	body = append(body, format.Dim.Render(render.Fit(status, m.width)))
	return strings.Join(body, "\n")
}

func window(lines []string, off, h int) []string {
	off = clamp(off, 0, max(0, len(lines)-1))
	end := min(len(lines), off+h)
	if off >= end {
		return nil
	}
	return append([]string(nil), lines[off:end]...)
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
