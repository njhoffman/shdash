// Package collect defines the panels: how each one collects its data (by
// running the tools that already know it) and how it turns that data into
// lines.
package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runner runs a command and returns its stdout.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// ExecRunner runs the command with NO_COLOR=1; on failure the error carries
// the first line of its stderr.
func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	//nolint:gosec // G204: collectors run fixed tools; only their arguments vary.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		if msg != "" {
			return out, fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// Env is what collectors see of the host; tests replace every field.
type Env struct {
	Run      Runner
	Home     string
	Getenv   func(string) string
	ReadFile func(string) ([]byte, error)
	// IsDir reports whether a path is an existing directory; nil counts
	// every path as one.
	IsDir func(string) bool
	Now   func() time.Time
}

// HostEnv is the real host.
func HostEnv() *Env {
	home, _ := os.UserHomeDir()
	return &Env{
		Run: ExecRunner, Home: home, Getenv: os.Getenv, ReadFile: os.ReadFile,
		IsDir: func(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() },
		Now:   time.Now,
	}
}

// XDG is the directory in the XDG variable, else fallback under $HOME.
func (e *Env) XDG(variable, fallback string) string {
	if v := e.Getenv(variable); v != "" {
		return v
	}
	return filepath.Join(e.Home, fallback)
}

// Tilde shortens a path under $HOME to ~/...
func (e *Env) Tilde(p string) string {
	if e.Home != "" && (p == e.Home || strings.HasPrefix(p, e.Home+"/")) {
		return "~" + p[len(e.Home):]
	}
	return p
}

// Collector is one panel.
type Collector struct {
	Name  string
	Title string
	// Icon is a Nerd Font glyph shown before the title when icons are on.
	Icon string
	// Priority orders panels; the highest number is dropped first when the
	// terminal is too short.
	Priority int
	TTL      time.Duration
	Timeout  time.Duration
	// Inline collectors are cheap enough to run in the foreground when
	// nothing is cached yet; the others show "collecting" until the
	// background refresh has run.
	Inline bool
	// Schema changes when the data's shape does, so an old cache file is
	// collected again rather than misread.
	Schema int
	// Collect may return data together with an error when only part of it
	// failed.
	Collect func(ctx context.Context, env *Env) (any, error)
	Lines   func(raw json.RawMessage, env *Env) ([]string, error)
}

// All is every panel, in priority order.
func All() []Collector {
	return []Collector{System(), Projects(), Git(), Repos(), Audit(), Shell()}
}

func linesOf[T any](f func(*T, *Env) []string) func(json.RawMessage, *Env) ([]string, error) {
	return func(raw json.RawMessage, env *Env) ([]string, error) {
		var d T
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		return f(&d, env), nil
	}
}

func marshal(v any) (json.RawMessage, error) { return json.Marshal(v) }
