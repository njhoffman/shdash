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
