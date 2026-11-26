// Command shdash prints a width-aware snapshot of this system and shell
// environment from cached panel data, refreshing stale panels in the
// background.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/njhoffman/shdash/internal/cache"
	"github.com/njhoffman/shdash/internal/collect"
	"github.com/njhoffman/shdash/internal/dash"
	"github.com/njhoffman/shdash/internal/render"
	"github.com/njhoffman/shdash/internal/version"
)

// deps is everything run touches outside its arguments, so tests can
// replace it.
type deps struct {
	env        *collect.Env
	collectors []collect.Collector
	cacheDir   string
	// termSize is the terminal's size when stdout is one.
	termSize func() (w, h int, ok bool)
	// spawn starts the detached background refresh of the named panels.
	spawn func(names []string) error
}

func main() {
	env := collect.HostEnv()
	d := deps{
		env:        env,
		collectors: collect.All(),
		cacheDir:   cache.Dir(os.Getenv, env.Home),
		termSize: func() (int, int, bool) {
			w, h, err := term.GetSize(int(os.Stdout.Fd()))
			return w, h, err == nil
		},
	}
	d.spawn = func(names []string) error { return spawnRefresh(d.cacheDir, names) }
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, d))
}

const usage = `usage: shdash [--refresh] [--width N] [--height N] [--no-color] [--version]

Prints this system and shell environment as panels: System, Projects, Git,
Audit, Shell. Each panel is read from its cache under $XDG_CACHE_HOME/shdash;
panels older than their TTL are refreshed in the background, so the next run
shows them fresh.

  --refresh    collect every panel now, then print
  --width N    lay out for N columns (default: the terminal's, else $COLUMNS, else 80)
  --height N   drop the lowest-priority panels to fit N lines (default: the
               terminal's height less one; 0: no limit; piped output has none)
  --no-color   no colors (also NO_COLOR)
`

func run(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("shdash", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	refresh := fs.Bool("refresh", false, "")
	width := fs.Int("width", 0, "")
	height := fs.Int("height", -1, "")
	noColor := fs.Bool("no-color", false, "")
	showVersion := fs.Bool("version", false, "")
	background := fs.String("background-refresh", "", "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "shdash: unexpected argument %q\n", fs.Arg(0))
		fs.Usage()
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "shdash", version.String())
		return 0
	}
	ctx := context.Background()
	if *background != "" {
		return backgroundRefresh(ctx, strings.Split(*background, ","), stderr, d)
	}
	if *noColor {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	var states []dash.State
	if *refresh {
		unlock, _, err := cache.Lock(d.cacheDir, true)
		if err != nil {
			fmt.Fprintln(stderr, "shdash:", err)
			return 1
		}
		states, err = dash.RefreshAll(ctx, d.cacheDir, dash.Load(d.cacheDir, d.collectors), d.env)
		unlock()
		if err != nil {
			fmt.Fprintln(stderr, "shdash:", err)
			return 1
		}
	} else {
		states = refreshInline(ctx, dash.Load(d.cacheDir, d.collectors), stderr, d)
		var due []string
		for _, s := range states {
			if s.Due(d.env.Now()) {
				due = append(due, s.C.Name)
			}
		}
		if len(due) > 0 && d.spawn != nil && !refreshRunning(d.cacheDir) {
			if err := d.spawn(due); err != nil {
				fmt.Fprintln(stderr, "shdash: background refresh:", err)
			}
		}
	}

	w, h := layoutSize(*width, *height, d)
	fmt.Fprint(stdout, render.Layout(dash.Panels(states, d.env), w, h))
	return 0
}

// refreshInline collects the cheap panels that have no cache yet, so a
// first run is not all "collecting".
func refreshInline(ctx context.Context, states []dash.State, stderr io.Writer, d deps) []dash.State {
	var idx []int
	var todo []dash.State
	for i, s := range states {
		if s.E == nil && s.C.Inline {
			idx = append(idx, i)
			todo = append(todo, s)
		}
	}
	if len(todo) == 0 {
		return states
	}
	done, err := dash.RefreshAll(ctx, d.cacheDir, todo, d.env)
	if err != nil {
		fmt.Fprintln(stderr, "shdash:", err)
	}
	for j, i := range idx {
		states[i] = done[j]
	}
	return states
}

func layoutSize(width, height int, d deps) (int, int) {
	tw, th, tty := 0, 0, false
	if d.termSize != nil {
		tw, th, tty = d.termSize()
	}
	if width <= 0 {
		switch {
		case tty && tw > 0:
			width = tw
		default:
			width = 80
			if n, err := strconv.Atoi(d.env.Getenv("COLUMNS")); err == nil && n > 0 {
				width = n
			}
		}
	}
	if height < 0 {
		height = 0
		if tty && th > 1 {
			height = th - 1
		}
	}
	return width, height
}

// refreshRunning is true when another process holds the refresh lock, so a
// new background refresh would only exit at once. An error counts as not
// running: the child then reports it in refresh.log.
func refreshRunning(cacheDir string) bool {
	unlock, ok, err := cache.Lock(cacheDir, false)
	if err != nil {
		return false
	}
	if ok {
		unlock()
	}
	return !ok
}

// backgroundRefresh is the detached child: it refreshes the named panels
// that are still due, unless another refresh holds the lock.
func backgroundRefresh(ctx context.Context, names []string, stderr io.Writer, d deps) int {
	unlock, ok, err := cache.Lock(d.cacheDir, false)
	if err != nil {
		fmt.Fprintln(stderr, "shdash:", err)
		return 1
	}
	if !ok {
		return 0
	}
	defer unlock()
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var due []dash.State
	for _, s := range dash.Load(d.cacheDir, d.collectors) {
		if want[s.C.Name] && s.Due(d.env.Now()) {
			due = append(due, s)
		}
	}
	if len(due) == 0 {
		return 0
	}
	start := d.env.Now()
	fmt.Fprintf(stderr, "%s refresh %s\n", start.Format(time.RFC3339), strings.Join(stateNames(due), ","))
	done, err := dash.RefreshAll(ctx, d.cacheDir, due, d.env)
	for _, s := range done {
		if s.E != nil && s.E.Error != "" && !s.E.AttemptedAt.Before(start) {
			fmt.Fprintf(stderr, "  %s: %s\n", s.C.Name, s.E.Error)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "shdash:", err)
		return 1
	}
	return 0
}

func stateNames(states []dash.State) []string {
	names := make([]string, len(states))
	for i, s := range states {
		names[i] = s.C.Name
	}
	return names
}

// logLimit is the size past which refresh.log is moved to refresh.log.1.
const logLimit = 256 << 10

// openLog opens refresh.log for appending, first moving it to refresh.log.1
// when it has grown past limit. Callers only open it with no refresh
// running, so no child is writing the file being moved.
func openLog(cacheDir string, limit int64) (*os.File, error) {
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, err
	}
	name := filepath.Join(cacheDir, "refresh.log")
	if fi, err := os.Stat(name); err == nil && fi.Size() > limit {
		if err := os.Rename(name, name+".1"); err != nil {
			return nil, err
		}
	}
	//nolint:gosec // G304: the cache directory is ours
	return os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

// spawnRefresh starts this binary again, detached in its own session, to
// refresh the panels; its stderr is appended to refresh.log in the cache
// directory.
func spawnRefresh(cacheDir string, names []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	logf, err := openLog(cacheDir, logLimit)
	if err != nil {
		return err
	}
	defer func() { _ = logf.Close() }()
	//nolint:gosec // G204: re-runs this binary with panel names from the collector list.
	cmd := exec.Command(self, "--background-refresh", strings.Join(names, ","))
	cmd.Stdout, cmd.Stderr = nil, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
