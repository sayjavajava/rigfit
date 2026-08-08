// SPDX-License-Identifier: GPL-3.0-only

package reqs

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/sayjavajava/rigfit/internal/cache"
)

// backdateCacheFile sets a cached file's modification time into the past,
// so tests can exercise Stale/Expired paths without waiting real time.
func backdateCacheFile(t *testing.T, elem string, age time.Duration) {
	t.Helper()
	path, err := cache.Path(elem)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-age)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
}

func withTempCacheDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LOCALAPPDATA", dir)
}

func TestFetchRequirementsCachesFreshResult(t *testing.T) {
	withTempCacheDir(t)
	calls := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = fmt.Fprint(w, `{"1":{"success":true,"data":{"name":"Test Game","type":"game","pc_requirements":[]}}}`)
	}, nil)

	ctx := context.Background()
	first, freshness, err := client.FetchRequirements(ctx, 1, FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if freshness != cache.Fresh {
		t.Errorf("first fetch freshness = %v, want Fresh", freshness)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	second, freshness2, err := client.FetchRequirements(ctx, 1, FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("calls = %d after a second fetch, want 1 (should be served from cache)", calls)
	}
	if freshness2 != cache.Fresh {
		t.Errorf("second fetch freshness = %v, want Fresh", freshness2)
	}
	if second.Name != first.Name {
		t.Errorf("cached record name = %q, want %q", second.Name, first.Name)
	}
}

func TestFetchRequirementsOfflineWithNoCacheErrors(t *testing.T) {
	withTempCacheDir(t)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("--offline must never make a network call")
	}, nil)

	if _, _, err := client.FetchRequirements(context.Background(), 999, FetchOptions{Offline: true}); err == nil {
		t.Fatal("expected an error when offline with nothing cached")
	}
}

func TestFetchRequirementsOfflineServesCacheLabelledStale(t *testing.T) {
	withTempCacheDir(t)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"1":{"success":true,"data":{"name":"Test Game","type":"game","pc_requirements":[]}}}`)
	}, nil)

	ctx := context.Background()
	if _, _, err := client.FetchRequirements(ctx, 1, FetchOptions{}); err != nil {
		t.Fatal(err)
	}

	// Even though the cache is fresh, --offline always labels it stale
	// (§9 point 4) since rigfit can't confirm it's still accurate without
	// a network call it was told not to make.
	_, freshness, err := client.FetchRequirements(ctx, 1, FetchOptions{Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if freshness != cache.Stale {
		t.Errorf("offline freshness = %v, want Stale", freshness)
	}
}

func TestFetchRequirementsExpiredWithoutStaleOKRefuses(t *testing.T) {
	withTempCacheDir(t)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"1":{"success":true,"data":{"name":"Test Game","type":"game","pc_requirements":[]}}}`)
	}, nil)

	ctx := context.Background()
	fresh, err := client.AppDetails(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveCachedRequirements(fresh); err != nil {
		t.Fatal(err)
	}
	// Backdate the cache file well past the hard TTL.
	backdateCacheFile(t, requirementsCacheKey(1), 200*24*time.Hour)

	// Server now errors, simulating "can't refresh"; the expired,
	// non-stale-ok cache must be refused rather than silently served.
	client.appDetailsURL = client.appDetailsURL + "-unreachable"
	if _, _, err := client.FetchRequirements(ctx, 1, FetchOptions{}); err == nil {
		t.Fatal("expected an error for expired cache without --stale-ok")
	}

	if _, freshness, err := client.FetchRequirements(ctx, 1, FetchOptions{StaleOK: true}); err != nil {
		t.Fatal(err)
	} else if freshness != cache.Expired {
		t.Errorf("freshness = %v, want Expired", freshness)
	}
}
