// SPDX-License-Identifier: GPL-3.0-only

package reqs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient builds a SteamClient pointed at a local httptest server
// instead of the real Steam endpoints, with the rate limiter opened up so
// tests aren't artificially slowed by the production §6.1 limit.
func newTestClient(t *testing.T, appDetailsHandler, storeSearchHandler http.HandlerFunc) *SteamClient {
	t.Helper()
	mux := http.NewServeMux()
	if appDetailsHandler != nil {
		mux.HandleFunc("/appdetails", appDetailsHandler)
	}
	if storeSearchHandler != nil {
		mux.HandleFunc("/storesearch/", storeSearchHandler)
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c := NewSteamClient()
	c.appDetailsURL = server.URL + "/appdetails"
	c.storeSearchURL = server.URL + "/storesearch/"
	c.limiter = newTokenBucket(1e6, 1e6) // effectively unlimited for tests
	return c
}

func TestAppDetailsParsesRealShapeResponse(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"1245620":{"success":true,"data":{"name":"ELDEN RING","type":"game","pc_requirements":{"minimum":%q,"recommended":%q}}}}`,
			eldenRingMinimumHTML, eldenRingRecommendedHTML)
	}, nil)

	got, err := client.AppDetails(context.Background(), 1245620)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "ELDEN RING" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Tiers.Minimum == nil || got.Tiers.Minimum.CPU == nil {
		t.Fatalf("expected a parsed minimum tier with a CPU match, got %+v", got.Tiers)
	}
	if got.ReviewStatus != ReviewAuto {
		t.Errorf("ReviewStatus = %q, want %q", got.ReviewStatus, ReviewAuto)
	}
	if !strings.HasPrefix(got.RawHash, "sha256:") {
		t.Errorf("RawHash = %q, want a sha256: prefix", got.RawHash)
	}
}

func TestAppDetailsHandlesUnsuccessfulResponse(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"99999999":{"success":false}}`)
	}, nil)

	if _, err := client.AppDetails(context.Background(), 99999999); err == nil {
		t.Fatal("expected an error for a success:false response")
	}
}

func TestAppDetailsHandlesEmptyArrayRequirements(t *testing.T) {
	// Verified live: Steam sometimes returns pc_requirements as `[]`
	// instead of an object when a listing has no requirements text.
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"10":{"success":true,"data":{"name":"Some Game","type":"game","pc_requirements":[]}}}`)
	}, nil)

	got, err := client.AppDetails(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tiers.Minimum != nil {
		t.Errorf("Tiers.Minimum = %+v, want nil for an empty pc_requirements array", got.Tiers.Minimum)
	}
}

// TestAppDetailsParsesLinuxPlatformAndRequirements mirrors a real
// captured Counter-Strike 2 (appid 730) response shape: a title with
// native Linux support, whose linux_requirements shares pc_requirements'
// HTML label structure but describes a different OS/CPU/GPU baseline
// (Ubuntu, Vulkan-capable GPUs, not Windows/DirectX).
func TestAppDetailsParsesLinuxPlatformAndRequirements(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"730":{"success":true,"data":{
			"name":"Counter-Strike 2",
			"type":"game",
			"platforms":{"windows":true,"mac":false,"linux":true},
			"pc_requirements":{"minimum":"<strong>OS:</strong> Windows 10<br><strong>Memory:</strong> 8 GB RAM<br><strong>Storage:</strong> 85 GB available space"},
			"linux_requirements":{"minimum":"<strong>OS:</strong> Ubuntu 20.04<br><strong>Memory:</strong> 8 GB RAM<br><strong>Storage:</strong> 85 GB available space"}
		}}}`)
	}, nil)

	got, err := client.AppDetails(context.Background(), 730)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Platforms.Linux || !got.Platforms.Windows || got.Platforms.Mac {
		t.Errorf("Platforms = %+v", got.Platforms)
	}
	if got.LinuxTiers == nil || got.LinuxTiers.Minimum == nil {
		t.Fatal("expected LinuxTiers.Minimum to be parsed for a native-Linux title")
	}
	if len(got.LinuxTiers.Minimum.OS) != 1 || got.LinuxTiers.Minimum.OS[0].Family != "linux" {
		t.Errorf("LinuxTiers.Minimum.OS = %+v, want a single linux entry", got.LinuxTiers.Minimum.OS)
	}
	if got.Tiers.Minimum == nil || len(got.Tiers.Minimum.OS) != 1 || got.Tiers.Minimum.OS[0].Family != "windows" {
		t.Errorf("Tiers.Minimum (Windows) = %+v", got.Tiers.Minimum)
	}
}

func TestAppDetailsSkipsLinuxRequirementsWhenNotNative(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"1":{"success":true,"data":{"name":"Windows Only Game","type":"game","platforms":{"windows":true,"mac":false,"linux":false},"pc_requirements":[]}}}`)
	}, nil)

	got, err := client.AppDetails(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.LinuxTiers != nil {
		t.Errorf("LinuxTiers = %+v, want nil when Platforms.Linux is false", got.LinuxTiers)
	}
}

func TestAppDetailsRetriesOn429(t *testing.T) {
	attempts := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = fmt.Fprint(w, `{"1":{"success":true,"data":{"name":"Test Game","type":"game","pc_requirements":[]}}}`)
	}, nil)

	got, err := client.AppDetails(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (2 rate-limited + 1 success)", attempts)
	}
	if got.Name != "Test Game" {
		t.Errorf("Name = %q", got.Name)
	}
}

func TestSearchAppPrefersExactNameMatch(t *testing.T) {
	client := newTestClient(t, nil, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"total":2,"items":[
			{"type":"app","name":"ELDEN RING NIGHTREIGN","id":2622380},
			{"type":"app","name":"ELDEN RING","id":1245620}
		]}`)
	})

	id, name, err := client.SearchApp(context.Background(), "ELDEN RING")
	if err != nil {
		t.Fatal(err)
	}
	if id != 1245620 || name != "ELDEN RING" {
		t.Errorf("got (%d, %q), want (1245620, \"ELDEN RING\") — exact match should win over the first/most-relevant result", id, name)
	}
}

func TestSearchAppFallsBackToTopResult(t *testing.T) {
	client := newTestClient(t, nil, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"total":1,"items":[{"type":"app","name":"ELDEN RING","id":1245620}]}`)
	})

	id, _, err := client.SearchApp(context.Background(), "eldne ring") // typo
	if err != nil {
		t.Fatal(err)
	}
	if id != 1245620 {
		t.Errorf("id = %d, want 1245620", id)
	}
}

func TestSearchAppNoResults(t *testing.T) {
	client := newTestClient(t, nil, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"total":0,"items":[]}`)
	})

	if _, _, err := client.SearchApp(context.Background(), "asdkfjhasdlkjfh"); err == nil {
		t.Fatal("expected an error for zero search results")
	}
}

func TestTokenBucketBlocksWhenExhausted(t *testing.T) {
	b := newTokenBucket(1, 1) // 1 token, refills 1/sec
	ctx := context.Background()

	if err := b.wait(ctx); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := b.wait(ctx); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 500*time.Millisecond {
		t.Errorf("second wait() returned after %v, expected it to block for close to 1s", elapsed)
	}
}

func TestTokenBucketRespectsContextCancellation(t *testing.T) {
	b := newTokenBucket(1, 0.01) // effectively never refills within the test
	ctx := context.Background()
	if err := b.wait(ctx); err != nil {
		t.Fatal(err)
	}

	cancelCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if err := b.wait(cancelCtx); err == nil {
		t.Fatal("expected wait() to return the context's error once it's done")
	}
}
