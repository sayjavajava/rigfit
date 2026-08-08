// SPDX-License-Identifier: GPL-3.0-only

package score

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sayjavajava/rigfit/pkg/hwid"
)

type goldenRecord struct {
	CanonicalID string `json:"canonical_id"`
	MarketName  string `json:"market_name"`
	Tier        int    `json:"tier"`
}

type goldenTiers struct {
	GPUs []goldenRecord `json:"gpus"`
	CPUs []goldenRecord `json:"cpus"`
}

// TestTierDriftGuard implements §5.5: it locks the expected tier for every
// reference part in gpudb.json/cpudb.json against testdata/golden_tiers.json.
// Any coefficients.json (or formula) change that shifts an unrelated part's
// tier fails this test — that's the point. A deliberate, reviewed
// coefficient change must regenerate the golden file explicitly:
//
//	go test ./internal/score/... -tags generate_golden -run TestGenerateGoldenTiers -v
//
// and the diff in that regeneration is exactly what a coefficients.json
// change should be reviewed against.
func TestTierDriftGuard(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "golden_tiers.json"))
	if err != nil {
		t.Fatalf("reading golden_tiers.json: %v", err)
	}
	var golden goldenTiers
	if err := json.Unmarshal(body, &golden); err != nil {
		t.Fatalf("parsing golden_tiers.json: %v", err)
	}

	b := NewBuiltin()

	if len(golden.GPUs) < 20 {
		t.Fatalf("golden_tiers.json has only %d GPUs; §5.5 wants ~30 reference GPUs locked", len(golden.GPUs))
	}

	for _, want := range golden.GPUs {
		got, conf, err := b.GPUScore(want.CanonicalID)
		if err != nil {
			t.Fatal(err)
		}
		if conf != hwid.ConfidenceExact {
			t.Errorf("%s (%s): confidence = %v, want exact — a part silently dropping out of gpudb.json is itself a drift bug", want.CanonicalID, want.MarketName, conf)
			continue
		}
		if got.Tier != want.Tier {
			t.Errorf("%s (%s): tier drifted from %d to %d", want.CanonicalID, want.MarketName, want.Tier, got.Tier)
		}
	}

	for _, want := range golden.CPUs {
		got, _, err := b.CPUScore(want.CanonicalID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Tier != want.Tier {
			t.Errorf("%s (%s): tier drifted from %d to %d", want.CanonicalID, want.MarketName, want.Tier, got.Tier)
		}
	}

	// Every gpudb.json/cpudb.json entry should be represented in the
	// golden file — an entry added without a locked expectation isn't
	// actually guarded by this test.
	goldenGPUIDs := make(map[string]bool, len(golden.GPUs))
	for _, g := range golden.GPUs {
		goldenGPUIDs[g.CanonicalID] = true
	}
	for id := range getGPUDB() {
		if !goldenGPUIDs[id] {
			t.Errorf("gpudb.json entry %q has no golden_tiers.json expectation; regenerate the golden file", id)
		}
	}

	goldenCPUIDs := make(map[string]bool, len(golden.CPUs))
	for _, c := range golden.CPUs {
		goldenCPUIDs[c.CanonicalID] = true
	}
	for id := range getCPUDB() {
		if !goldenCPUIDs[id] {
			t.Errorf("cpudb.json entry %q has no golden_tiers.json expectation; regenerate the golden file", id)
		}
	}
}
