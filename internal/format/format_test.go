package format

import (
	"testing"
	"time"
)

func TestAge(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second:     "0s",
		45 * time.Second: "45s",
		12 * time.Minute: "12m",
		3 * time.Hour:    "3h",
		47 * time.Hour:   "47h",
		50 * time.Hour:   "2d",
	} {
		if got := Age(d); got != want {
			t.Errorf("Age(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestNumbers(t *testing.T) {
	if got := Pct(1, 3); got != 33 {
		t.Errorf("Pct(1,3) = %d", got)
	}
	if got := Pct(1, 0); got != 0 {
		t.Errorf("Pct(1,0) = %d", got)
	}
	if got := GiB(5 << 30); got != "5.0" {
		t.Errorf("GiB = %q", got)
	}
	if got := GiB(468 << 30); got != "468" {
		t.Errorf("GiB = %q", got)
	}
	for _, c := range []struct {
		n        int
		omitZero bool
		want     string
	}{{0, true, ""}, {0, false, "0 files"}, {1, false, "1 file"}, {2, true, "2 files"}} {
		if got := Count(c.n, "file", "files", c.omitZero); got != c.want {
			t.Errorf("Count(%d, %v) = %q, want %q", c.n, c.omitZero, got, c.want)
		}
	}
}

func TestJoinSkipsEmpty(t *testing.T) {
	if got := Join("a", "", "b"); got != "a · b" {
		t.Errorf("Join = %q", got)
	}
	if got := Join("", ""); got != "" {
		t.Errorf("Join of empties = %q", got)
	}
}

func TestTable(t *testing.T) {
	got := Table([][]string{
		{"shdash", "0.12.1", "main +3", "", "", "1 todo"},
		{"audit", "0.51.0", "main +16", "2 changed", "", "2 todos"},
		{"shell", "0.138.1", "main", "", "", "51 todos"},
		{"x"},
	})
	want := []string{
		"shdash 0.12.1  · main +3              · 1 todo",
		"audit  0.51.0  · main +16 · 2 changed · 2 todos",
		"shell  0.138.1 · main                 · 51 todos",
		"x",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q\n         want %q", i, got[i], want[i])
		}
	}
	if Table(nil) == nil || len(Table(nil)) != 0 {
		t.Errorf("Table(nil) = %v, want empty", Table(nil))
	}
}
