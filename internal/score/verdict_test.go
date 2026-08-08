// SPDX-License-Identifier: GPL-3.0-only

package score

import (
	"testing"

	"github.com/sayjavajava/rigfit/internal/detect"
	"github.com/sayjavajava/rigfit/pkg/hwid"
)

func referenceSpecs() detect.Specs {
	return detect.Specs{
		CPU: hwid.CPU{CanonicalID: "intel-core-i5-8400", Confidence: hwid.ConfidenceExact}, // baseline, tier 10
		GPUs: []hwid.GPU{
			{CanonicalID: "nvidia-geforce-gtx-1060-6gb", VRAMBytes: 6 * 1024 * 1024 * 1024, IsRenderer: true, Confidence: hwid.ConfidenceExact}, // baseline, tier 10
		},
		RAM:     detect.RAMInfo{TotalBytes: 16 * 1024 * 1024 * 1024},
		Storage: detect.StorageInfo{FreeBytes: 100 * 1024 * 1024 * 1024},
		OS:      detect.OSInfo{Family: "windows", Version: "10.0.19045"},
	}
}

func componentByName(components []ComponentVerdict, name string) (ComponentVerdict, bool) {
	for _, c := range components {
		if c.Component == name {
			return c, true
		}
	}
	return ComponentVerdict{}, false
}

func TestVerdictAllPass(t *testing.T) {
	b := NewBuiltin()
	// referenceSpecs() runs baseline-tier parts (i5-8400, GTX 1060 6GB);
	// requirements here deliberately name weaker parts so headroom clears
	// the +10% band into a confident Pass, not just Marginal.
	req := Requirement{
		OS:           []OSRequirement{{Family: "windows", MinVersion: "10"}},
		CPUAnyOf:     []string{"amd-a8-5500"},
		GPUAnyOf:     []string{"nvidia-geforce-gtx-1050-ti"},
		GPUVRAMBytes: 3 * 1024 * 1024 * 1024,
		RAMBytes:     8 * 1024 * 1024 * 1024,
		StorageBytes: 20 * 1024 * 1024 * 1024,
	}

	v := b.Verdict(referenceSpecs(), req)
	if v.Overall != StatusPass {
		t.Fatalf("Overall = %v, want Pass; components: %+v", v.Overall, v.Components)
	}
	if v.Bottleneck != "" {
		t.Errorf("Bottleneck = %q, want empty when everything passes", v.Bottleneck)
	}
}

func TestVerdictHardGateFailHasNoRatio(t *testing.T) {
	b := NewBuiltin()
	req := Requirement{RAMBytes: 64 * 1024 * 1024 * 1024} // way more than referenceSpecs' 16 GiB

	v := b.Verdict(referenceSpecs(), req)
	ram, ok := componentByName(v.Components, "ram")
	if !ok {
		t.Fatal("expected a ram component")
	}
	if ram.Status != StatusFail {
		t.Errorf("ram.Status = %v, want Fail", ram.Status)
	}
	if ram.Headroom != 0 || ram.UserTier != 0 || ram.RequiredTier != 0 {
		t.Errorf("hard gate should carry no ratio: %+v", ram)
	}
	if v.Bottleneck != "ram" {
		t.Errorf("Bottleneck = %q, want ram", v.Bottleneck)
	}
}

func TestVerdictStorageUsesFreeNotTotal(t *testing.T) {
	b := NewBuiltin()
	specs := referenceSpecs()
	specs.Storage.FreeBytes = 5 * 1024 * 1024 * 1024
	specs.Storage.TotalBytes = 1000 * 1024 * 1024 * 1024 // plenty of total space, but not free

	v := b.Verdict(specs, Requirement{StorageBytes: 20 * 1024 * 1024 * 1024})
	storage, _ := componentByName(v.Components, "storage")
	if storage.Status != StatusFail {
		t.Errorf("storage.Status = %v, want Fail (free space is what matters, not total)", storage.Status)
	}
}

func TestVerdictVRAMFailsSeparatelyFromGPUTier(t *testing.T) {
	b := NewBuiltin()
	specs := referenceSpecs()
	specs.GPUs[0].VRAMBytes = 1 * 1024 * 1024 * 1024 // this card's VRAM is insufficient despite its tier

	req := Requirement{
		GPUAnyOf:     []string{"nvidia-geforce-gtx-1050-ti"}, // comfortably weaker: tier clears well
		GPUVRAMBytes: 6 * 1024 * 1024 * 1024,                 // but VRAM requirement isn't met
	}
	v := b.Verdict(specs, req)

	gpu, _ := componentByName(v.Components, "gpu")
	if gpu.Status != StatusPass {
		t.Errorf("gpu.Status = %v, want Pass (tier is identical)", gpu.Status)
	}
	vram, ok := componentByName(v.Components, "vram")
	if !ok {
		t.Fatal("expected a vram component")
	}
	if vram.Status != StatusFail {
		t.Errorf("vram.Status = %v, want Fail — a GPU can clear tier and still fail on VRAM (§7)", vram.Status)
	}
	if v.Bottleneck != "vram" {
		t.Errorf("Bottleneck = %q, want vram", v.Bottleneck)
	}
}

func TestVerdictVRAMOmittedWhenRequirementUnspecified(t *testing.T) {
	b := NewBuiltin()
	v := b.Verdict(referenceSpecs(), Requirement{GPUAnyOf: []string{"nvidia-geforce-gtx-1060-6gb"}})
	if _, ok := componentByName(v.Components, "vram"); ok {
		t.Error("expected no vram component when the requirement didn't specify one (not applicable, not Unknown)")
	}
}

func TestVerdictUnknownCPUPropagates(t *testing.T) {
	b := NewBuiltin()
	specs := referenceSpecs()
	specs.CPU = hwid.CPU{Confidence: hwid.ConfidenceUnknown}

	v := b.Verdict(specs, Requirement{CPUAnyOf: []string{"intel-core-i5-8400"}})
	cpu, _ := componentByName(v.Components, "cpu")
	if cpu.Status != StatusUnknown {
		t.Errorf("cpu.Status = %v, want Unknown for an unrecognised user CPU", cpu.Status)
	}
	if v.Overall != StatusUnknown {
		t.Errorf("Overall = %v, want Unknown", v.Overall)
	}
}

// TestVerdictMarginalWhenExactlyMeetingRequirement checks the ±10% band's
// boundary case: a user GPU that's the exact card the requirement asks
// for has headroom 0, which falls inside "±10%" — Marginal, not a
// confident Pass. This matches how "minimum spec" is used in practice:
// having precisely the minimum-listed card is borderline, not comfortable.
func TestVerdictMarginalWhenExactlyMeetingRequirement(t *testing.T) {
	b := NewBuiltin()
	v := b.Verdict(referenceSpecs(), Requirement{GPUAnyOf: []string{"nvidia-geforce-gtx-1060-6gb"}})
	gpu, _ := componentByName(v.Components, "gpu")
	if gpu.Status != StatusMarginal {
		t.Errorf("gpu.Status = %v, want Marginal (headroom = %v)", gpu.Status, gpu.Headroom)
	}
}

// TestVerdictPassWithComfortableHeadroom checks a GPU well above the
// requirement (GTX 1070, tier 14, against a GTX 1060 6GB, tier 10,
// requirement) clears the +10% band into a confident Pass.
func TestVerdictPassWithComfortableHeadroom(t *testing.T) {
	b := NewBuiltin()
	specs := referenceSpecs()
	specs.GPUs[0] = hwid.GPU{CanonicalID: "nvidia-geforce-gtx-1070", IsRenderer: true, Confidence: hwid.ConfidenceExact}

	v := b.Verdict(specs, Requirement{GPUAnyOf: []string{"nvidia-geforce-gtx-1060-6gb"}})
	gpu, _ := componentByName(v.Components, "gpu")
	if gpu.Status != StatusPass {
		t.Errorf("gpu.Status = %v, want Pass (headroom = %v)", gpu.Status, gpu.Headroom)
	}
}

// TestVerdictFailWellBelowRequirement checks a GPU well under the
// requirement (GTX 1050 Ti, tier 2, against a GTX 1080 Ti, tier 21,
// requirement) lands a confident Fail, not Marginal.
func TestVerdictFailWellBelowRequirement(t *testing.T) {
	b := NewBuiltin()
	specs := referenceSpecs()
	specs.GPUs[0] = hwid.GPU{CanonicalID: "nvidia-geforce-gtx-1050-ti", IsRenderer: true, Confidence: hwid.ConfidenceExact}

	v := b.Verdict(specs, Requirement{GPUAnyOf: []string{"nvidia-geforce-gtx-1080-ti"}})
	gpu, _ := componentByName(v.Components, "gpu")
	if gpu.Status != StatusFail {
		t.Errorf("gpu.Status = %v, want Fail (headroom = %v)", gpu.Status, gpu.Headroom)
	}
}

func TestEasiestRequirementPicksLowestPerf(t *testing.T) {
	b := NewBuiltin()
	// GTX 1050 Ti is far below GTX 1080 Ti; the easiest (1050 Ti) must be
	// the one that sets the bar for a soft comparison's required tier.
	got, ok := easiestRequirement([]string{"nvidia-geforce-gtx-1080-ti", "nvidia-geforce-gtx-1050-ti"}, b.GPUScore)
	if !ok {
		t.Fatal("expected a match")
	}
	want, _, _ := b.GPUScore("nvidia-geforce-gtx-1050-ti")
	if got.Tier != want.Tier {
		t.Errorf("easiestRequirement tier = %d, want %d (the 1050 Ti's)", got.Tier, want.Tier)
	}
}

func TestOSVersionSatisfies(t *testing.T) {
	cases := []struct {
		user, want string
		satisfies  bool
	}{
		{"10.0.26200.8875", "10", true},
		{"10.0.19045", "11", false},
		{"11.0.22000", "10", true},
	}
	for _, c := range cases {
		if got := osVersionSatisfies(c.user, c.want); got != c.satisfies {
			t.Errorf("osVersionSatisfies(%q, %q) = %v, want %v", c.user, c.want, got, c.satisfies)
		}
	}
}
