// SPDX-License-Identifier: GPL-3.0-only

// Package cache implements rigfit's single writable location and its
// TTL/staleness rules (rigfit-BUILD-SPEC.md §2, §9). It knows nothing
// about what any particular cached payload means — that's each caller's
// concern — only where files live and how "fresh" is defined.
package cache

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Dir resolves rigfit's single writable directory (§2): ~/.cache/rigfit
// on Linux/macOS, %LOCALAPPDATA%\rigfit on Windows. rigfit MUST NOT write
// anywhere else.
func Dir() (string, error) {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			return "", errors.New("cache: %LOCALAPPDATA% is not set")
		}
		return filepath.Join(base, "rigfit"), nil
	}

	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "rigfit"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "rigfit"), nil
}

// Path joins Dir() with the given path elements, creating no directories.
func Path(elem ...string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{dir}, elem...)...), nil
}

// WriteFile writes data to Path(elem...), creating parent directories as
// needed. It's the only way anything in rigfit should touch disk outside
// its own process memory.
func WriteFile(data []byte, elem ...string) error {
	path, err := Path(elem...)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ReadFile reads back a file previously written with WriteFile, along
// with its age (time.Now() minus the file's modification time).
func ReadFile(elem ...string) (data []byte, age time.Duration, err error) {
	path, err := Path(elem...)
	if err != nil {
		return nil, 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	return data, time.Since(info.ModTime()), nil
}

// TTL is a soft/hard staleness policy (§9's table, one row per data
// class).
type TTL struct {
	Soft time.Duration
	Hard time.Duration
}

// Freshness classifies a cache entry's age against a TTL policy.
type Freshness int

const (
	// Fresh: within the soft TTL. Serve without comment.
	Fresh Freshness = iota
	// Stale: past the soft TTL but within the hard TTL. Serve, but the
	// caller MUST show a visible age note (§9 point 1).
	Stale
	// Expired: past the hard TTL. Refuse to serve unless the caller
	// passed --offline or --stale-ok.
	Expired
)

func (f Freshness) String() string {
	switch f {
	case Fresh:
		return "fresh"
	case Stale:
		return "stale"
	case Expired:
		return "expired"
	default:
		return "invalid"
	}
}

// Classify returns how fresh an entry of the given age is under ttl.
func Classify(age time.Duration, ttl TTL) Freshness {
	switch {
	case age <= ttl.Soft:
		return Fresh
	case age <= ttl.Hard:
		return Stale
	default:
		return Expired
	}
}
