// SPDX-License-Identifier: GPL-3.0-only

package score

import (
	"testing"

	"github.com/sayjavajava/rigfit/pkg/hwid"
)

// TestBaselineGPUIsTier10 locks §5.3's central invariant: the baseline
// GPU (GTX 1060 6GB) is always exactly tier 10, by construction — its own
// PERF divided by itself is 1, and log2(1) is 0.
func TestBaselineGPUIsTier10(t *testing.T) {
	b := NewBuiltin()
	score, conf, err := b.GPUScore(baselineGPUCanonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if conf != hwid.ConfidenceExact {
		t.Fatalf("confidence = %v, want exact", conf)
	}
	if score.Tier != 10 {
		t.Errorf("baseline GPU tier = %d, want 10", score.Tier)
	}
}

func TestBaselineCPUIsTier10(t *testing.T) {
	b := NewBuiltin()
	score, conf, err := b.CPUScore(baselineCPUCanonicalID)
	if err != nil {
		t.Fatal(err)
	}
	if conf != hwid.ConfidenceExact {
		t.Fatalf("confidence = %v, want exact", conf)
	}
	if score.Tier != 10 {
		t.Errorf("baseline CPU tier = %d, want 10", score.Tier)
	}
}

// TestUnknownGPUReturnsConfidenceUnknown guards §1 for the scoring layer:
// a canonical ID with no gpudb.json entry must report Unknown, never a
// fuzzy/estimated tier.
func TestUnknownGPUReturnsConfidenceUnknown(t *testing.T) {
	b := NewBuiltin()
	score, conf, err := b.GPUScore("nvidia-geforce-rtx-9999-does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if conf != hwid.ConfidenceUnknown {
		t.Fatalf("confidence = %v, want unknown", conf)
	}
	if score.Tier != 0 {
		t.Errorf("tier = %d, want zero value for an unscored part", score.Tier)
	}
}

func TestUnknownCPUReturnsConfidenceUnknown(t *testing.T) {
	b := NewBuiltin()
	_, conf, err := b.CPUScore("amd-ryzen-ai-does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if conf != hwid.ConfidenceUnknown {
		t.Fatalf("confidence = %v, want unknown", conf)
	}
}

// TestHigherEndGPUsScoreHigherTiers is a monotonicity sanity check within
// one architecture: a strictly larger, faster GPU must never score a lower
// tier than a smaller one from the same generation.
func TestHigherEndGPUsScoreHigherTiers(t *testing.T) {
	b := NewBuiltin()
	ordered := []string{
		"nvidia-geforce-gtx-1050-ti",
		"nvidia-geforce-gtx-1060-6gb",
		"nvidia-geforce-gtx-1070",
		"nvidia-geforce-gtx-1080",
		"nvidia-geforce-gtx-1080-ti",
	}
	prevTier := -1000
	for _, id := range ordered {
		score, conf, err := b.GPUScore(id)
		if err != nil {
			t.Fatal(err)
		}
		if conf != hwid.ConfidenceExact {
			t.Fatalf("%s: confidence = %v, want exact", id, conf)
		}
		if score.Tier < prevTier {
			t.Errorf("%s: tier %d is lower than the previous (weaker) card's tier %d", id, score.Tier, prevTier)
		}
		prevTier = score.Tier
	}
}

func TestExplainGPUBreakdownIsInternallyConsistent(t *testing.T) {
	b := NewBuiltin()
	exp, err := b.ExplainGPU("nvidia-geforce-rtx-3080")
	if err != nil {
		t.Fatal(err)
	}
	wantFP32 := float64(exp.ShaderUnits) * 2 * exp.BoostClockGHz
	if exp.FP32GFLOPS != wantFP32 {
		t.Errorf("FP32GFLOPS = %v, want %v", exp.FP32GFLOPS, wantFP32)
	}
	if exp.Perf != exp.RawPerf*exp.ArchCoefficient {
		t.Errorf("Perf = %v, want RawPerf(%v) * ArchCoefficient(%v) = %v", exp.Perf, exp.RawPerf, exp.ArchCoefficient, exp.RawPerf*exp.ArchCoefficient)
	}
	if exp.Tier <= 10 {
		t.Errorf("RTX 3080 tier = %d, expected well above the GTX 1060 6GB baseline of 10", exp.Tier)
	}
}

func TestNameIsBuiltin(t *testing.T) {
	if got := NewBuiltin().Name(); got != "builtin" {
		t.Errorf("Name() = %q, want %q", got, "builtin")
	}
}
