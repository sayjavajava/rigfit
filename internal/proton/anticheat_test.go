// SPDX-License-Identifier: GPL-3.0-only

package proton

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// realShapeDataset is a trimmed excerpt of the real AreWeAntiCheatYet
// games.json, verified live 2026-08-08, covering every status value the
// dataset uses (Supported, Running, Denied, Broken, Planned).
const realShapeDataset = `[
	{"name":"Elden Ring","native":false,"status":"Supported","anticheats":["Easy Anti-Cheat"],"storeIds":{"steam":"1245620"}},
	{"name":"Battlefield 2042","native":false,"status":"Denied","anticheats":["EA anticheat"],"storeIds":{"steam":"1517290"}},
	{"name":"Rust","native":false,"status":"Running","anticheats":["EasyAntiCheat"],"storeIds":{"steam":"252490"}},
	{"name":"SomeBrokenGame","native":false,"status":"Broken","anticheats":["BattlEye"],"storeIds":{"steam":"999001"}},
	{"name":"SomePlannedGame","native":false,"status":"Planned","anticheats":["Vanguard"],"storeIds":{"steam":"999002"}}
]`

func newTestAntiCheatClient(t *testing.T, handler http.HandlerFunc) *AntiCheatClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c := NewAntiCheatClient()
	c.datasetURL = server.URL
	return c
}

func withTempCacheDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LOCALAPPDATA", dir)
}

func TestCheckAppIDBlockedIsPermanent(t *testing.T) {
	withTempCacheDir(t)
	client := newTestAntiCheatClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, realShapeDataset)
	})

	got, err := client.CheckAppID(context.Background(), 1517290, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != GateBlocked || !got.Blocks() {
		t.Errorf("Battlefield 2042 (Denied) = %+v, want GateBlocked and Blocks()==true", got)
	}
}

func TestCheckAppIDSupportedIsPlayable(t *testing.T) {
	withTempCacheDir(t)
	client := newTestAntiCheatClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, realShapeDataset)
	})

	got, err := client.CheckAppID(context.Background(), 1245620, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != GatePlayable || got.Blocks() {
		t.Errorf("Elden Ring (Supported) = %+v, want GatePlayable and Blocks()==false", got)
	}
	if len(got.AntiCheats) != 1 || got.AntiCheats[0] != "Easy Anti-Cheat" {
		t.Errorf("AntiCheats = %v", got.AntiCheats)
	}
}

func TestCheckAppIDRunningIsPlayable(t *testing.T) {
	withTempCacheDir(t)
	client := newTestAntiCheatClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, realShapeDataset)
	})
	got, err := client.CheckAppID(context.Background(), 252490, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != GatePlayable {
		t.Errorf("Rust (Running) status = %v, want playable", got.Status)
	}
}

func TestCheckAppIDBrokenAndPlannedBlockCurrently(t *testing.T) {
	withTempCacheDir(t)
	client := newTestAntiCheatClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, realShapeDataset)
	})

	for _, appID := range []int{999001, 999002} {
		got, err := client.CheckAppID(context.Background(), appID, false)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != GateCurrentlyBroken || !got.Blocks() {
			t.Errorf("appid %d = %+v, want GateCurrentlyBroken and Blocks()==true", appID, got)
		}
	}
}

// TestCheckAppIDNotInDatasetIsClear guards the key design decision:
// absence from the dataset means "no known anti-cheat", not Unknown —
// most Steam games have no invasive anti-cheat at all.
func TestCheckAppIDNotInDatasetIsClear(t *testing.T) {
	withTempCacheDir(t)
	client := newTestAntiCheatClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, realShapeDataset)
	})

	got, err := client.CheckAppID(context.Background(), 10, false) // Counter-Strike, not in this excerpt
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != GateNoKnownAntiCheat || !got.Available || got.Blocks() {
		t.Errorf("got %+v, want GateNoKnownAntiCheat, Available, not blocking", got)
	}
}

func TestCheckAppIDOfflineWithNoCacheIsUnavailable(t *testing.T) {
	withTempCacheDir(t)
	client := newTestAntiCheatClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("offline must never make a network call")
	})

	got, err := client.CheckAppID(context.Background(), 1245620, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Available {
		t.Errorf("got %+v, want Available == false when offline with no cache", got)
	}
}

func TestCheckAppIDFetchFailureDegradesGracefully(t *testing.T) {
	withTempCacheDir(t)
	client := newTestAntiCheatClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	got, err := client.CheckAppID(context.Background(), 1245620, false)
	if err != nil {
		t.Fatalf("a fetch failure must degrade gracefully, not return an error: %v", err)
	}
	if got.Available {
		t.Errorf("got %+v, want Available == false on fetch failure", got)
	}
}

func TestCheckAppIDUsesCacheOnSecondCall(t *testing.T) {
	withTempCacheDir(t)
	calls := 0
	client := newTestAntiCheatClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = fmt.Fprint(w, realShapeDataset)
	})

	ctx := context.Background()
	if _, err := client.CheckAppID(ctx, 1245620, false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CheckAppID(ctx, 1245620, false); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (second lookup should be served from the cached dataset)", calls)
	}
}
