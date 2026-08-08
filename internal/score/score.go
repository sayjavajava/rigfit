// SPDX-License-Identifier: GPL-3.0-only

// Package score implements rigfit's GPU/CPU performance tiering
// (rigfit-BUILD-SPEC.md §5). It joins pkg/hwid's canonical part IDs
// against spec-sheet data (data/gpudb.json, data/cpudb.json) and
// architecture correction coefficients (data/coefficients.json) to
// produce a single comparable Tier per part.
package score

import (
	"math"

	"github.com/sayjavajava/rigfit/pkg/hwid"
)

// Score is a part's computed performance tier.
type Score struct {
	Tier int     `json:"tier"`
	Perf float64 `json:"perf"` // the PERF value the tier was derived from; exposed for explain-tier
}

// ScoreProvider is the pluggable interface tier lookups go through (§5.4).
// rigfit ships only the Builtin provider; no external provider is
// implemented.
type ScoreProvider interface {
	Name() string
	GPUScore(canonicalID string) (Score, hwid.Confidence, error)
	CPUScore(canonicalID string) (Score, hwid.Confidence, error)
}

// tierFromPerf implements the shared §5.1/§5.2 tiering step: TIER =
// round(8 * log2(PERF / BASELINE_PERF)) + 10. The 8x factor makes one tier
// step approximately 9% of performance; log scaling is mandatory so that
// tier differences represent proportional, not absolute, gaps.
func tierFromPerf(perf, baselinePerf float64) int {
	return int(math.Round(8*math.Log2(perf/baselinePerf))) + 10
}
