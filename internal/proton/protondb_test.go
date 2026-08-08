// SPDX-License-Identifier: GPL-3.0-only

package proton

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/sayjavajava/rigfit/internal/cache"
)

func newTestProtonDBClient(t *testing.T, handler http.HandlerFunc) *ProtonDBClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c := NewProtonDBClient()
	c.baseURL = server.URL
	return c
}

// realShapeRating mirrors the real ProtonDB response for ELDEN RING
// (appid 1245620), verified live 2026-08-08.
const realShapeRating = `{"bestReportedTier":"platinum","confidence":"strong","score":0.79,"tier":"gold","total":2085,"trendingTier":"platinum"}`

func TestFetchRatingParsesRealShape(t *testing.T) {
	withTempCacheDir(t)
	client := newTestProtonDBClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, realShapeRating)
	})

	got, err := client.FetchRating(context.Background(), 1245620, false)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Available || got.Tier != "gold" || got.Total != 2085 || got.BestReportedTier != "platinum" {
		t.Errorf("got %+v", got)
	}
}

func TestFetchRatingCachesResult(t *testing.T) {
	withTempCacheDir(t)
	calls := 0
	client := newTestProtonDBClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = fmt.Fprint(w, realShapeRating)
	})

	ctx := context.Background()
	if _, err := client.FetchRating(ctx, 1245620, false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FetchRating(ctx, 1245620, false); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestFetchRatingUnavailableWhenOfflineNoCache(t *testing.T) {
	withTempCacheDir(t)
	client := newTestProtonDBClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("offline must never make a network call")
	})

	got, err := client.FetchRating(context.Background(), 1245620, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Available {
		t.Errorf("got %+v, want Available == false", got)
	}
}

func TestFetchRatingDegradesGracefullyOnFetchFailure(t *testing.T) {
	withTempCacheDir(t)
	client := newTestProtonDBClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	got, err := client.FetchRating(context.Background(), 1245620, false)
	if err != nil {
		t.Fatalf("a fetch failure must degrade gracefully, not return an error: %v", err)
	}
	if got.Available {
		t.Errorf("got %+v, want Available == false", got)
	}
}

func TestFetchRatingNeverServesExpiredData(t *testing.T) {
	withTempCacheDir(t)
	client := newTestProtonDBClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, realShapeRating)
	})

	ctx := context.Background()
	if _, err := client.FetchRating(ctx, 1245620, false); err != nil {
		t.Fatal(err)
	}

	// Backdate the cache past the 30-day hard TTL.
	path, err := cache.Path(protonDBCacheKey(1245620))
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}

	// Server now fails, simulating "can't refresh" — expired cache must
	// not be served regardless.
	client.baseURL = client.baseURL + "-unreachable"
	got, err := client.FetchRating(ctx, 1245620, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Available {
		t.Errorf("got %+v, want Available == false for hard-expired data (§9: do not guess)", got)
	}
}
