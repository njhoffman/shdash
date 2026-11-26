package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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
