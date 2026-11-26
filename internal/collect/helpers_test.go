package collect

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

// fakeEnv answers commands from outputs, keyed by the command line joined
// with spaces; a missing key fails like a missing program. files are
// readable paths.
func fakeEnv(outputs map[string]string, files map[string]string, vars map[string]string) *Env {
	return &Env{
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			key := strings.Join(append([]string{name}, args...), " ")
			if out, ok := outputs[key]; ok {
				if strings.HasPrefix(out, "ERR:") {
					return nil, errors.New(strings.TrimPrefix(out, "ERR:"))
				}
				return []byte(out), nil
			}
			return nil, errors.New(name + ": not found")
		},
		Home:   "/home/u",
		Getenv: func(k string) string { return vars[k] },
		ReadFile: func(p string) ([]byte, error) {
			if s, ok := files[p]; ok {
				return []byte(s), nil
			}
			return nil, fs.ErrNotExist
		},
		Now: func() time.Time { return testNow },
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// lines collects through c and renders the result, failing on an error.
func lines(t *testing.T, c Collector, env *Env) []string {
	t.Helper()
	data, err := c.Collect(context.Background(), env)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return render(t, c, env, data)
}

func render(t *testing.T, c Collector, env *Env, data any) []string {
	t.Helper()
	raw, err := marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	l, err := c.Lines(raw, env)
	if err != nil {
		t.Fatalf("lines: %v", err)
	}
	return l
}

func assertHas(t *testing.T, l []string, want ...string) {
	t.Helper()
	all := strings.Join(l, "\n")
	for _, w := range want {
		if !strings.Contains(all, w) {
			t.Errorf("missing %q in:\n%s", w, all)
		}
	}
}

func assertLacks(t *testing.T, l []string, unwanted ...string) {
	t.Helper()
	all := strings.Join(l, "\n")
	for _, w := range unwanted {
		if strings.Contains(all, w) {
			t.Errorf("unexpected %q in:\n%s", w, all)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
