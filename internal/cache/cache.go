// Package cache stores each panel's collected data as one JSON file under
// the cache directory, and the lock that keeps refreshes from overlapping.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Entry is one panel's cached data. FetchedAt is when Data was collected;
// AttemptedAt is the last collection attempt, successful or not, so a
// failing collector is retried once per TTL rather than on every call.
type Entry struct {
	Schema      int             `json:"schema"`
	FetchedAt   time.Time       `json:"fetched_at"`
	AttemptedAt time.Time       `json:"attempted_at"`
	Error       string          `json:"error,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}

// Dir is $SHDASH_CACHE_DIR, else $XDG_CACHE_HOME/shdash, else
// ~/.cache/shdash.
func Dir(getenv func(string) string, home string) string {
	if d := getenv("SHDASH_CACHE_DIR"); d != "" {
		return d
	}
	if d := getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "shdash")
	}
	return filepath.Join(home, ".cache", "shdash")
}

func path(dir, name string) string { return filepath.Join(dir, name+".json") }

// Stamp is the panels' entries' modification times and sizes as one
// string: it changes whenever any entry is written, so a reader can skip
// decoding entries that are as they were.
func Stamp(dir string, names []string) string {
	var b strings.Builder
	for _, n := range names {
		b.WriteString(n)
		if fi, err := os.Stat(path(dir, n)); err == nil {
			fmt.Fprintf(&b, ":%d:%d ", fi.ModTime().UnixNano(), fi.Size())
		} else {
			b.WriteString(":- ")
		}
	}
	return b.String()
}

// Load reads a panel's entry; a missing file is (nil, nil).
func Load(dir, name string) (*Entry, error) {
	b, err := os.ReadFile(path(dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("%s: %w", path(dir, name), err)
	}
	return &e, nil
}

// Save writes a panel's entry through a temporary file, synced and renamed
// into place, so a reader never sees a partial file, even after a crash.
func Save(dir, name string, e *Entry) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+name+"-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// Synced before the rename: otherwise a crash soon after can leave the
	// renamed file empty, and the panel loses its data until a refresh.
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path(dir, name)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Lock takes the refresh lock. With wait it blocks until the lock is free;
// without, ok is false when another process holds it. The returned func
// releases it.
func Lock(dir string, wait bool) (unlock func(), ok bool, err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile( //nolint:gosec // G304: the cache directory is ours
		filepath.Join(dir, ".refresh.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	how := syscall.LOCK_EX
	if !wait {
		how |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, true, nil
}

// tmpMaxAge is the age past which a temporary file is left over from a Save
// that did not finish, not one being written.
const tmpMaxAge = 24 * time.Hour

// orphanMaxAge is how long an entry no collector here owns is kept. Another
// build sharing the directory (a dev binary beside the installed one) may
// own it: a build in use writes its entries every TTL (an hour at most), so
// only one unwritten for a week is a renamed or removed panel's.
const orphanMaxAge = 7 * 24 * time.Hour

// Clean removes what nothing will read again: temporary files from a Save
// killed before its rename, older than tmpMaxAge, and the entries of panels
// not in keep (renamed or removed ones), older than orphanMaxAge. Call it
// holding the lock. It returns the names removed.
func Clean(dir string, keep []string, now time.Time) ([]string, error) {
	des, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, n := range keep {
		known[n+".json"] = true
	}
	var removed []string
	var errs []error
	for _, de := range des {
		name := de.Name()
		if !de.Type().IsRegular() {
			continue
		}
		switch {
		case strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".tmp"):
			fi, err := de.Info()
			if err != nil || now.Sub(fi.ModTime()) < tmpMaxAge {
				continue
			}
		case strings.HasSuffix(name, ".json") && !strings.HasPrefix(name, "."):
			if known[name] {
				continue
			}
			fi, err := de.Info()
			if err != nil || now.Sub(fi.ModTime()) < orphanMaxAge {
				continue
			}
		default:
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
			continue
		}
		removed = append(removed, name)
	}
	return removed, errors.Join(errs...)
}
