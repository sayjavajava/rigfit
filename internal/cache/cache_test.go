// SPDX-License-Identifier: GPL-3.0-only

package cache

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	ttl := TTL{Soft: 7 * 24 * time.Hour, Hard: 30 * 24 * time.Hour}
	cases := []struct {
		age  time.Duration
		want Freshness
	}{
		{0, Fresh},
		{7 * 24 * time.Hour, Fresh},
		{7*24*time.Hour + time.Minute, Stale},
		{30 * 24 * time.Hour, Stale},
		{30*24*time.Hour + time.Minute, Expired},
	}
	for _, c := range cases {
		if got := Classify(c.age, ttl); got != c.want {
			t.Errorf("Classify(%v) = %v, want %v", c.age, got, c.want)
		}
	}
}

func withTempCacheDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LOCALAPPDATA", dir)
}

func TestWriteReadRoundTrip(t *testing.T) {
	withTempCacheDir(t)

	want := []byte(`{"hello":"world"}`)
	if err := WriteFile(want, "subdir", "entry.json"); err != nil {
		t.Fatal(err)
	}

	got, age, err := ReadFile("subdir", "entry.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if age < 0 || age > time.Minute {
		t.Errorf("age = %v, want a small non-negative duration", age)
	}
}

func TestReadFileMissing(t *testing.T) {
	withTempCacheDir(t)
	if _, _, err := ReadFile("does", "not", "exist.json"); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
