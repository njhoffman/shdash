// Package format holds the dashboard's styles and the small helpers that
// turn numbers, sizes and durations into panel text.
package format

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// One accent color; everything else is plain, faint or bold.
var (
	Accent = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "75"})
	Title  = Accent.Bold(true)
	Dim    = lipgloss.NewStyle().Faint(true)
	Strong = lipgloss.NewStyle().Bold(true)
)

// Age is a duration in its largest whole unit: 45s, 12m, 3h, 4d.
func Age(d time.Duration) string {
	switch {
	case d < 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}

// GiB formats a byte count in GiB with one decimal below 100.
func GiB(n uint64) string {
	g := float64(n) / (1 << 30)
	if g < 100 {
		return fmt.Sprintf("%.1f", g)
	}
	return fmt.Sprintf("%.0f", g)
}

// Pct is part as a whole percentage of total, 0 when total is 0.
func Pct(part, total uint64) int {
	if total == 0 {
		return 0
	}
	return int(float64(part)*100/float64(total) + 0.5)
}

// KV is a row: the key faint and padded, then the value.
func KV(key, value string) string {
	return Dim.Render(fmt.Sprintf("%-7s", key)) + " " + value
}

// Join joins the non-empty parts with a faint separator.
func Join(parts ...string) string {
	kept := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, Dim.Render(" · "))
}

// Count is "n word", with plural when n != 1; empty when n is 0 and
// omitZero is set.
func Count(n int, singular, plural string, omitZero bool) string {
	if n == 0 && omitZero {
		return ""
	}
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// Pad pads s with spaces to width w (by display width).
func Pad(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}
