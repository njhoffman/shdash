// Package dash ties collectors to the cache: which panels are due, refreshing
// them, and turning cached entries into panels.
package dash

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/njhoffman/shdash/internal/cache"
	"github.com/njhoffman/shdash/internal/collect"
	"github.com/njhoffman/shdash/internal/format"
	"github.com/njhoffman/shdash/internal/render"
)

// State is a collector with its cached entry (nil when there is none, or
// when it was written for another schema).
type State struct {
	C collect.Collector
	E *cache.Entry
}

// Load reads every collector's entry. An unreadable or old-schema file
// counts as missing.
func Load(dir string, cs []collect.Collector) []State {
	out := make([]State, len(cs))
	for i, c := range cs {
		e, err := cache.Load(dir, c.Name)
		if err != nil || (e != nil && e.Schema != c.Schema) {
			e = nil
		}
		out[i] = State{c, e}
	}
	return out
}

// Due is true when the entry is missing, its last attempt is older than the
// TTL, or its source file changed since.
func (s State) Due(env *collect.Env) bool {
	if s.E == nil {
		return true
	}
	return env.Now().Sub(s.lastAttempt()) >= s.C.TTL || s.Changed(env)
}

// Changed is true when the collector's source file is newer than the last
// attempt to collect it (so a failed attempt is not repeated until the file
// changes again or the TTL runs out).
func (s State) Changed(env *collect.Env) bool {
	if s.E == nil || s.C.Source == nil || env.ModTime == nil {
		return false
	}
	t, ok := env.ModTime(s.C.Source(env))
	return ok && t.After(s.lastAttempt())
}

func (s State) lastAttempt() time.Time {
	if s.E.FetchedAt.After(s.E.AttemptedAt) {
		return s.E.FetchedAt
	}
	return s.E.AttemptedAt
}

// Refresh collects one panel and saves it. A failure keeps the previous
// data and records the error.
func Refresh(ctx context.Context, dir string, s State, env *collect.Env) (*cache.Entry, error) {
	ctx, cancel := context.WithTimeout(ctx, s.C.Timeout)
	defer cancel()
	now := env.Now()
	e := &cache.Entry{Schema: s.C.Schema, AttemptedAt: now}
	if s.E != nil {
		e.FetchedAt, e.Data = s.E.FetchedAt, s.E.Data
	}
	data, err := collectSafely(ctx, s.C, env)
	if data != nil {
		raw, merr := json.Marshal(data)
		if merr != nil {
			return nil, merr
		}
		e.Data, e.FetchedAt = raw, now
	}
	if err != nil {
		e.Error = err.Error()
	}
	return e, cache.Save(dir, s.C.Name, e)
}

// collectSafely runs the collector, turning a panic (a parser meeting
// output it did not expect) into that panel's error instead of ending the
// whole refresh.
func collectSafely(ctx context.Context, c collect.Collector, env *collect.Env) (data any, err error) {
	defer func() {
		if r := recover(); r != nil {
			data, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()
	return c.Collect(ctx, env)
}

// drawSafely is draw(raw, env), with a panic returned as an error.
func drawSafely(draw func(json.RawMessage, *collect.Env) ([]string, error), raw json.RawMessage,
	env *collect.Env) (lines []string, err error) {
	defer func() {
		if r := recover(); r != nil {
			lines, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()
	return draw(raw, env)
}

// RefreshAll refreshes the given states in parallel and returns them with
// their new entries.
func RefreshAll(ctx context.Context, dir string, states []State, env *collect.Env) ([]State, error) {
	out := append([]State(nil), states...)
	errs := make([]error, len(out))
	var wg sync.WaitGroup
	for i := range out {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e, err := Refresh(ctx, dir, out[i], env)
			if e != nil {
				out[i].E = e
			}
			errs[i] = err
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// Panels turns states into panels. A panel that has never collected
// anything shows "collecting"; one whose only result is an error, or whose
// cached data cannot be shown, shows that error; one whose last refresh
// failed keeps its data and says so. With icons, each title gets its
// collector's Nerd Font icon.
func Panels(states []State, env *collect.Env, icons bool) []render.Panel {
	out := make([]render.Panel, 0, len(states))
	for _, s := range states {
		out = append(out, panel(s, env, icons, false))
	}
	return out
}

// Detail is one state's panel with its full lists (the collector's Detail
// renderer, else its Lines), for interactive mode's full-screen view.
func Detail(s State, env *collect.Env, icons bool) render.Panel {
	return panel(s, env, icons, true)
}

func panel(s State, env *collect.Env, icons, full bool) render.Panel {
	now := env.Now()
	p := render.Panel{Title: s.C.Title, Priority: s.C.Priority}
	if icons {
		p.Icon = s.C.Icon
	}
	switch {
	case s.E == nil:
		p.Lines = []string{format.Dim.Render("collecting, shown on the next run")}
	case s.E.Data == nil:
		msg := s.E.Error
		if msg == "" {
			msg = "no data"
		}
		p.Note = format.Dim.Render(format.Age(now.Sub(s.E.AttemptedAt)))
		p.Lines = []string{format.Dim.Render("failed: " + msg)}
	default:
		draw := s.C.Lines
		if full && s.C.Detail != nil {
			draw = s.C.Detail
		}
		lines, err := drawSafely(draw, s.E.Data, env)
		if err != nil {
			p.Lines = []string{format.Dim.Render("cannot show cached data: " + err.Error())}
			break
		}
		p.Lines = lines
		age := format.Age(now.Sub(s.E.FetchedAt))
		if now.Sub(s.E.FetchedAt) >= s.C.TTL {
			p.Note = format.Strong.Render(age)
		} else {
			p.Note = format.Dim.Render(age)
		}
		if s.E.Error != "" {
			p.Lines = append(p.Lines, format.Dim.Render(fmt.Sprintf("refresh failed: %s", s.E.Error)))
		}
	}
	return p
}
