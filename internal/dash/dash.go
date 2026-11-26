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

// Due is true when the entry is missing or its last attempt is older than
// the TTL.
func (s State) Due(now time.Time) bool {
	if s.E == nil {
		return true
	}
	last := s.E.AttemptedAt
	if s.E.FetchedAt.After(last) {
		last = s.E.FetchedAt
	}
	return now.Sub(last) >= s.C.TTL
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
	data, err := s.C.Collect(ctx, env)
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
		lines, err := draw(s.E.Data, env)
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
