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

// Save writes a panel's entry through a temporary file renamed into place,
// so a reader never sees a partial file.
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
	if _, err := f.Write(b); err != nil {
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
