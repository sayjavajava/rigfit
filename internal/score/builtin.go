// SPDX-License-Identifier: GPL-3.0-only

package score

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/sayjavajava/rigfit/pkg/hwid"
)

// baselineGPUCanonicalID is the permanent GPU tier-10 anchor (§5.3): "MUST
// NOT be re-baselined." It is a Go constant, not a data-file field,
// specifically so changing it requires a reviewed code change (CODEOWNERS
// gates internal/score/) rather than an incidental data edit.
const baselineGPUCanonicalID = "nvidia-geforce-gtx-1060-6gb"

// baselineCPUCanonicalID is the CPU tier-10 anchor. §5.3 only names a GPU
// baseline explicitly; a CPU baseline is picked here on the same footing
// (a well-known, broadly-recognised mainstream part) since §5.2's formula
// needs its own reference point in a completely different unit space than
// the GPU formula's GFLOPS/bandwidth terms — the two BASELINE_PERF values
// are independent per-domain anchors, not one shared number. Intel Core
// i5-8400 is used because it's already rigfit-BUILD-SPEC.md's own example
// minimum-CPU requirement (§6.3), making it a spec-consistent choice.
const baselineCPUCanonicalID = "intel-core-i5-8400"

var errUnknownPart = errors.New("score: part not found in database")

// Builtin is rigfit's only ScoreProvider (§5.4: "Ship only the builtin
// provider. Do not implement any external provider.").
type Builtin struct{}

var _ ScoreProvider = (*Builtin)(nil)

// NewBuiltin constructs the builtin ScoreProvider.
func NewBuiltin() *Builtin { return &Builtin{} }

// Name implements ScoreProvider.
func (*Builtin) Name() string { return "builtin" }

// GPUScore implements ScoreProvider. A canonicalID absent from gpudb.json,
// or naming an architecture absent from coefficients.json, returns
// ConfidenceUnknown with a nil error — this is an expected, not
// exceptional, outcome, exactly mirroring how pkg/hwid handles unresolved
// hardware. A non-nil error means the embedded data itself is broken.
func (b *Builtin) GPUScore(canonicalID string) (Score, hwid.Confidence, error) {
	exp, err := b.ExplainGPU(canonicalID)
	if err != nil {
		if errors.Is(err, errUnknownPart) {
			return Score{}, hwid.ConfidenceUnknown, nil
		}
		return Score{}, hwid.ConfidenceUnknown, err
	}
	return Score{Tier: exp.Tier, Perf: exp.Perf}, hwid.ConfidenceExact, nil
}

// CPUScore implements ScoreProvider; see GPUScore's error-handling note.
func (b *Builtin) CPUScore(canonicalID string) (Score, hwid.Confidence, error) {
	exp, err := b.ExplainCPU(canonicalID)
	if err != nil {
		if errors.Is(err, errUnknownPart) {
			return Score{}, hwid.ConfidenceUnknown, nil
		}
		return Score{}, hwid.ConfidenceUnknown, err
	}
	return Score{Tier: exp.Tier, Perf: exp.Perf}, hwid.ConfidenceExact, nil
}

// GPUExplanation is the full §5.1 computation breakdown, for
// `rigfit explain-tier`.
type GPUExplanation struct {
	CanonicalID          string  `json:"canonical_id"`
	MarketName           string  `json:"market_name"`
	Architecture         string  `json:"architecture"`
	ShaderUnits          int     `json:"shader_units"`
	BoostClockGHz        float64 `json:"boost_clock_ghz"`
	BusWidthBits         int     `json:"bus_width_bits"`
	EffectiveMemClockGHz float64 `json:"effective_mem_clock_ghz"`
	ArchCoefficient      float64 `json:"arch_coefficient"`
	FP32GFLOPS           float64 `json:"fp32_gflops"`
	BandwidthGBs         float64 `json:"bandwidth_gbs"`
	RawPerf              float64 `json:"raw_perf"`
	Perf                 float64 `json:"perf"`
	BaselinePerf         float64 `json:"baseline_perf"`
	Tier                 int     `json:"tier"`
}

// ExplainGPU computes and returns every intermediate value in §5.1's
// formula for canonicalID.
func (*Builtin) ExplainGPU(canonicalID string) (GPUExplanation, error) {
	entry, ok := lookupGPU(canonicalID)
	if !ok {
		return GPUExplanation{}, errUnknownPart
	}
	coeff, ok := gpuArchCoefficient(entry.Architecture)
	if !ok {
		return GPUExplanation{}, fmt.Errorf("score: gpu architecture %q has no coefficient in coefficients.json", entry.Architecture)
	}

	fp32GFLOPS := float64(entry.ShaderUnits) * 2 * entry.BoostClockGHz
	bandwidthGBs := (float64(entry.BusWidthBits) / 8) * entry.EffectiveMemClockGHz
	raw := math.Pow(fp32GFLOPS, 0.65) * math.Pow(bandwidthGBs, 0.35)
	perf := raw * coeff

	baseline, err := getBaselineGPUPerf()
	if err != nil {
		return GPUExplanation{}, err
	}

	return GPUExplanation{
		CanonicalID:          entry.CanonicalID,
		MarketName:           entry.MarketName,
		Architecture:         entry.Architecture,
		ShaderUnits:          entry.ShaderUnits,
		BoostClockGHz:        entry.BoostClockGHz,
		BusWidthBits:         entry.BusWidthBits,
		EffectiveMemClockGHz: entry.EffectiveMemClockGHz,
		ArchCoefficient:      coeff,
		FP32GFLOPS:           fp32GFLOPS,
		BandwidthGBs:         bandwidthGBs,
		RawPerf:              raw,
		Perf:                 perf,
		BaselinePerf:         baseline,
		Tier:                 tierFromPerf(perf, baseline),
	}, nil
}

// CPUExplanation is the full §5.2 computation breakdown, for
// `rigfit explain-tier`.
type CPUExplanation struct {
	CanonicalID     string  `json:"canonical_id"`
	MarketName      string  `json:"market_name"`
	Architecture    string  `json:"architecture"`
	PhysCores       int     `json:"phys_cores"`
	LogicalCores    int     `json:"logical_cores"`
	AllCoreClockGHz float64 `json:"all_core_clock_ghz"`
	IPCCoefficient  float64 `json:"ipc_coefficient"`
	Perf            float64 `json:"perf"`
	BaselinePerf    float64 `json:"baseline_perf"`
	Tier            int     `json:"tier"`
}

// ExplainCPU computes and returns every intermediate value in §5.2's
// formula for canonicalID.
func (*Builtin) ExplainCPU(canonicalID string) (CPUExplanation, error) {
	entry, ok := lookupCPU(canonicalID)
	if !ok {
		return CPUExplanation{}, errUnknownPart
	}
	ipc, ok := cpuIPCCoefficient(entry.Architecture)
	if !ok {
		return CPUExplanation{}, fmt.Errorf("score: cpu architecture %q has no coefficient in coefficients.json", entry.Architecture)
	}

	allCoreGHz := entry.AllCoreClockMHz / 1000
	perf := math.Pow(float64(entry.PhysCores), 0.5) * math.Pow(float64(entry.LogicalCores), 0.15) * allCoreGHz * ipc

	baseline, err := getBaselineCPUPerf()
	if err != nil {
		return CPUExplanation{}, err
	}

	return CPUExplanation{
		CanonicalID:     entry.CanonicalID,
		MarketName:      entry.MarketName,
		Architecture:    entry.Architecture,
		PhysCores:       entry.PhysCores,
		LogicalCores:    entry.LogicalCores,
		AllCoreClockGHz: allCoreGHz,
		IPCCoefficient:  ipc,
		Perf:            perf,
		BaselinePerf:    baseline,
		Tier:            tierFromPerf(perf, baseline),
	}, nil
}

var (
	baselineGPUOnce sync.Once
	baselineGPUPerf float64
	baselineGPUErr  error

	baselineCPUOnce sync.Once
	baselineCPUPerf float64
	baselineCPUErr  error
)

func getBaselineGPUPerf() (float64, error) {
	baselineGPUOnce.Do(func() {
		entry, ok := lookupGPU(baselineGPUCanonicalID)
		if !ok {
			baselineGPUErr = fmt.Errorf("score: baseline GPU %q missing from gpudb.json", baselineGPUCanonicalID)
			return
		}
		coeff, ok := gpuArchCoefficient(entry.Architecture)
		if !ok {
			baselineGPUErr = fmt.Errorf("score: baseline GPU architecture %q missing from coefficients.json", entry.Architecture)
			return
		}
		fp32GFLOPS := float64(entry.ShaderUnits) * 2 * entry.BoostClockGHz
		bandwidthGBs := (float64(entry.BusWidthBits) / 8) * entry.EffectiveMemClockGHz
		raw := math.Pow(fp32GFLOPS, 0.65) * math.Pow(bandwidthGBs, 0.35)
		baselineGPUPerf = raw * coeff
	})
	return baselineGPUPerf, baselineGPUErr
}

func getBaselineCPUPerf() (float64, error) {
	baselineCPUOnce.Do(func() {
		entry, ok := lookupCPU(baselineCPUCanonicalID)
		if !ok {
			baselineCPUErr = fmt.Errorf("score: baseline CPU %q missing from cpudb.json", baselineCPUCanonicalID)
			return
		}
		ipc, ok := cpuIPCCoefficient(entry.Architecture)
		if !ok {
			baselineCPUErr = fmt.Errorf("score: baseline CPU architecture %q missing from coefficients.json", entry.Architecture)
			return
		}
		allCoreGHz := entry.AllCoreClockMHz / 1000
		baselineCPUPerf = math.Pow(float64(entry.PhysCores), 0.5) * math.Pow(float64(entry.LogicalCores), 0.15) * allCoreGHz * ipc
	})
	return baselineCPUPerf, baselineCPUErr
}
