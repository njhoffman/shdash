package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func panel(title string, prio, lines int) Panel {
	p := Panel{Title: title, Priority: prio, Note: "1m"}
	for i := range lines {
		p.Lines = append(p.Lines, fmt.Sprintf("%s line %d", title, i))
	}
	return p
}

func TestColumns(t *testing.T) {
	for w, want := range map[int]int{40: 1, 79: 1, 80: 2, 139: 2, 140: 3, 300: 3} {
		if got := Columns(w); got != want {
			t.Errorf("Columns(%d) = %d, want %d", w, got, want)
		}
	}
}

func TestLayoutFitsWidth(t *testing.T) {
	long := Panel{Title: "Long", Priority: 1, Lines: []string{strings.Repeat("x", 500)}}
	for _, w := range []int{20, 60, 100, 160} {
		out := Layout([]Panel{long, panel("B", 2, 3), panel("C", 3, 2)}, w, 0)
		for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			if lipgloss.Width(l) > w {
				t.Errorf("width %d: line is %d wide: %q", w, lipgloss.Width(l), l)
			}
		}
		if !strings.Contains(out, "…") {
			t.Errorf("width %d: long line not cut with an ellipsis", w)
		}
	}
}

func TestLayoutPlacesInShortestColumn(t *testing.T) {
	out := Layout([]Panel{panel("A", 1, 10), panel("B", 2, 2), panel("C", 3, 2)}, 100, 0)
	rows := strings.Split(out, "\n")
	// A fills the left column; B and then C stack in the right one.
	if !strings.HasPrefix(rows[0], "A") || !strings.Contains(rows[0], "B") {
		t.Fatalf("first row: %q", rows[0])
	}
	if !strings.Contains(rows[4], "C") {
		t.Errorf("C is not under B (row 4: %q)\n%s", rows[4], out)
	}
}

func TestLayoutOrdersByPriority(t *testing.T) {
	out := Layout([]Panel{panel("Low", 9, 1), panel("High", 1, 1)}, 60, 0)
	if strings.Index(out, "High") > strings.Index(out, "Low") {
		t.Errorf("higher priority not first:\n%s", out)
	}
}

func TestLayoutDropsLowPriorityToFitHeight(t *testing.T) {
	ps := []Panel{panel("A", 1, 5), panel("B", 2, 5), panel("C", 3, 5)}
	out := Layout(ps, 60, 13)
	if !strings.Contains(out, "A line") || !strings.Contains(out, "B line") || strings.Contains(out, "C line") {
		t.Errorf("want A and B but not C:\n%s", out)
	}
	if n := strings.Count(out, "\n"); n > 13 {
		t.Errorf("%d lines, want at most 13", n)
	}
	// No height limit keeps everything.
	if out := Layout(ps, 60, 0); !strings.Contains(out, "C line") {
		t.Errorf("height 0 dropped a panel:\n%s", out)
	}
}

func TestLayoutCutsALonePanel(t *testing.T) {
	out := Layout([]Panel{panel("A", 1, 30)}, 60, 5)
	if n := strings.Count(out, "\n"); n != 5 {
		t.Errorf("%d lines, want 5:\n%s", n, out)
	}
}

func TestLayoutNoteRightAligned(t *testing.T) {
	out := Layout([]Panel{panel("A", 1, 1)}, 40, 0)
	first := strings.Split(out, "\n")[0]
	if !strings.HasSuffix(first, "1m") || lipgloss.Width(first) != 40 {
		t.Errorf("title row %q (%d wide)", first, lipgloss.Width(first))
	}
}

func TestLayoutEmpty(t *testing.T) {
	if out := Layout(nil, 80, 10); out != "" {
		t.Errorf("want nothing, got %q", out)
	}
}

func TestLayoutIconTitle(t *testing.T) {
	p := panel("Git", 1, 1)
	p.Icon = "\uE702"
	out := Layout([]Panel{p}, 40, 0)
	first, _, _ := strings.Cut(out, "\n")
	if !strings.HasPrefix(first, "\uE702 Git") || !strings.HasSuffix(strings.TrimRight(first, " "), "1m") {
		t.Errorf("title line: %q", first)
	}
	if w := lipgloss.Width(first); w > 40 {
		t.Errorf("title line is %d wide, over 40", w)
	}
	p.Icon = ""
	if first, _, _ := strings.Cut(Layout([]Panel{p}, 40, 0), "\n"); !strings.HasPrefix(first, "Git") {
		t.Errorf("title without icon: %q", first)
	}
}

func TestArrangeMatchesLayoutAndSpots(t *testing.T) {
	// Given out of priority order: spots are by the given index.
	ps := []Panel{panel("C", 3, 2), panel("A", 1, 4), panel("B", 2, 1)}
	a := Arrange(ps, 100)
	if got, want := strings.Join(a.Lines, "\n")+"\n", Layout(ps, 100, 0); got != want {
		t.Errorf("Arrange lines differ from Layout:\n%s\nvs\n%s", got, want)
	}
	if a.Columns != 2 {
		t.Fatalf("columns: %d", a.Columns)
	}
	// A goes first to column 0; B to the empty column 1; C under B (shorter).
	want := []Spot{{1, 3}, {0, 0}, {1, 0}}
	for i, s := range a.Spots {
		if s != want[i] {
			t.Errorf("spot %d (%s): %+v, want %+v", i, ps[i].Title, s, want[i])
		}
		if !strings.HasPrefix(strings.TrimSpace(colText(a, s)), ps[i].Title) {
			t.Errorf("row %d col %d is %q, not %s's title", s.Row, s.Col, colText(a, s), ps[i].Title)
		}
	}
}

// colText is the start of the cell at a spot (enough to find a title).
func colText(a Arrangement, s Spot) string {
	line := a.Lines[s.Row]
	if s.Col == 0 {
		return line
	}
	colWidth := (100 - gap*(a.Columns-1)) / a.Columns
	return line[min(len(line), s.Col*(colWidth+gap)):]
}

func TestArrangeEmpty(t *testing.T) {
	if a := Arrange(nil, 80); len(a.Lines) != 0 || len(a.Spots) != 0 {
		t.Errorf("empty: %+v", a)
	}
}

func TestFocusTitleIsReversed(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	p := panel("Git", 1, 0)
	plain := Block(p, 30)[0]
	p.Focus = true
	focused := Block(p, 30)[0]
	if plain == focused || !strings.Contains(focused, "\x1b[") || !strings.Contains(focused, "7") {
		t.Errorf("focused title not reversed: %q vs %q", focused, plain)
	}
}
