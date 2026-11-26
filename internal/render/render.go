// Package render lays panels out in columns that fit the terminal.
package render

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/njhoffman/shdash/internal/format"
)

// Panel is a titled block of lines. Note sits right of the title (the
// data's age).
type Panel struct {
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
	columns := make([][]string, cols)
	for _, p := range ps {
		block := block(p, colWidth)
		best := 0
		for i := range columns {
			if len(columns[i]) < len(columns[best]) {
				best = i
			}
		}
		if len(columns[best]) > 0 {
			columns[best] = append(columns[best], "")
		}
		columns[best] = append(columns[best], block...)
	}
	return columns
}

func block(p Panel, w int) []string {
	title := format.Title.Render(p.Title)
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
