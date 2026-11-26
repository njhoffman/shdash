// Package render lays panels out in columns that fit the terminal.
package render

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/njhoffman/shdash/internal/format"
)

// Panel is a titled block of lines. Icon, when set, goes before the title;
// Note sits right of the title (the
// data's age).
type Panel struct {
	// Focus draws the title in reverse video (interactive mode).
	Focus    bool
	Icon     string
	Title    string
	Note     string
	Priority int
	Lines    []string
}

const gap = 3

// Columns is how many columns a width gets: one under 80, two under 140,
// three from 140.
func Columns(width int) int {
	switch {
	case width < 80:
		return 1
	case width < 140:
		return 2
	default:
		return 3
	}
}

// Layout renders panels into Columns(width) columns, each panel going to the
// shortest column in priority order. With height > 0 the lowest-priority
// panels are dropped until every column fits; a lone panel still too tall
// is cut. No line is wider than width.
func Layout(panels []Panel, width, height int) string {
	if width < 20 {
		width = 20
	}
	ps := append([]Panel(nil), panels...)
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Priority < ps[j].Priority })
	cols := min(Columns(width), max(len(ps), 1))
	colWidth := (width - gap*(cols-1)) / cols
	var columns [][]string
	for {
		columns = place(ps, cols, colWidth)
		if height <= 0 || tallest(columns) <= height || len(ps) <= 1 {
			break
		}
		ps = ps[:len(ps)-1]
		cols = min(Columns(width), len(ps))
		colWidth = (width - gap*(cols-1)) / cols
	}
	if height > 0 {
		for i := range columns {
			if len(columns[i]) > height {
				columns[i] = columns[i][:height]
			}
		}
	}
	return join(columns, colWidth)
}

func place(ps []Panel, cols, colWidth int) [][]string {
	columns, _ := placed(ps, cols, colWidth)
	return columns
}

// Spot is where Arrange put a panel: its column and first row.
type Spot struct{ Col, Row int }

// placed is place, also returning each panel's spot (by index in ps).
func placed(ps []Panel, cols, colWidth int) ([][]string, []Spot) {
	columns := make([][]string, cols)
	spots := make([]Spot, len(ps))
	for i, p := range ps {
		block := block(p, colWidth)
		best := 0
		for c := range columns {
			if len(columns[c]) < len(columns[best]) {
				best = c
			}
		}
		if len(columns[best]) > 0 {
			columns[best] = append(columns[best], "")
		}
		spots[i] = Spot{best, len(columns[best])}
		columns[best] = append(columns[best], block...)
	}
	return columns, spots
}

// Arrangement is a layout with no height limit: its lines, and where each
// panel went, by index in the panels given to Arrange.
type Arrangement struct {
	Lines   []string
	Spots   []Spot
	Columns int
}

// Arrange lays every panel out like Layout with no height limit, and says
// where each one went, so interactive mode can move focus and scroll.
func Arrange(panels []Panel, width int) Arrangement {
	if width < 20 {
		width = 20
	}
	order := make([]int, len(panels))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return panels[order[a]].Priority < panels[order[b]].Priority })
	ps := make([]Panel, len(order))
	for i, o := range order {
		ps[i] = panels[o]
	}
	cols := min(Columns(width), max(len(ps), 1))
	colWidth := (width - gap*(cols-1)) / cols
	columns, sorted := placed(ps, cols, colWidth)
	spots := make([]Spot, len(panels))
	for i, o := range order {
		spots[o] = sorted[i]
	}
	text := strings.TrimSuffix(join(columns, colWidth), "\n")
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	return Arrangement{Lines: lines, Spots: spots, Columns: cols}
}

// Block renders one panel at width w: its title line, then its lines.
func Block(p Panel, w int) []string { return block(p, w) }

func block(p Panel, w int) []string {
	label := p.Title
	if p.Icon != "" {
		label = p.Icon + " " + p.Title
	}
	style := format.Title
	if p.Focus {
		style = style.Reverse(true)
	}
	title := style.Render(label)
	if p.Note != "" {
		pad := w - lipgloss.Width(title) - lipgloss.Width(p.Note)
		if pad >= 1 {
			title += strings.Repeat(" ", pad) + p.Note
		}
	}
	out := []string{fit(title, w)}
	for _, l := range p.Lines {
		out = append(out, fit(l, w))
	}
	return out
}

func fit(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

func tallest(columns [][]string) int {
	n := 0
	for _, c := range columns {
		n = max(n, len(c))
	}
	return n
}

func join(columns [][]string, colWidth int) string {
	rows := tallest(columns)
	var b strings.Builder
	for r := range rows {
		var line strings.Builder
		for i, c := range columns {
			cell := ""
			if r < len(c) {
				cell = c[r]
			}
			if i < len(columns)-1 {
				line.WriteString(format.Pad(cell, colWidth))
				line.WriteString(strings.Repeat(" ", gap))
			} else {
				line.WriteString(cell)
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
	return b.String()
}

// Fit cuts s to width w, ending it with "…" when cut.
func Fit(s string, w int) string { return fit(s, w) }
