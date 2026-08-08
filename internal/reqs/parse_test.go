// SPDX-License-Identifier: GPL-3.0-only

package reqs

import (
	"reflect"
	"strings"
	"testing"
)

// These fixtures are verbatim captures of real Steam appdetails responses
// (ELDEN RING, appid 1245620, and Counter-Strike, appid 10), fetched
// 2026-08-08, so the parser is tested against real-world HTML rather than
// an assumed shape.

const eldenRingMinimumHTML = `<strong>Minimum:</strong><br><ul class="bb_ul"><li>Requires a 64-bit processor and operating system<br></li><li><strong>OS:</strong> Windows 10<br></li><li><strong>Processor:</strong> INTEL CORE I5-8400 or AMD RYZEN 3 3300X<br></li><li><strong>Memory:</strong> 12 GB RAM<br></li><li><strong>Graphics:</strong> NVIDIA GEFORCE GTX 1060 3 GB or AMD RADEON RX 580 4 GB<br></li><li><strong>DirectX:</strong> Version 12<br></li><li><strong>Storage:</strong> 60 GB available space<br></li><li><strong>Sound Card:</strong> Windows Compatible Audio Device<br></li><li><strong>Additional Notes:</strong> </li></ul>`

const eldenRingRecommendedHTML = `<strong>Recommended:</strong><br><ul class="bb_ul"><li>Requires a 64-bit processor and operating system<br></li><li><strong>OS:</strong> Windows 10/11<br></li><li><strong>Processor:</strong> INTEL CORE I7-8700K or AMD RYZEN 5 3600X<br></li><li><strong>Memory:</strong> 16 GB RAM<br></li><li><strong>Graphics:</strong> NVIDIA GEFORCE GTX 1070 8 GB or AMD RADEON RX VEGA 56 8 GB<br></li><li><strong>DirectX:</strong> Version 12<br></li><li><strong>Storage:</strong> 60 GB available space<br></li><li><strong>Sound Card:</strong> Windows Compatible Audio Device<br></li><li><strong>Additional Notes:</strong> </li></ul>`

const counterStrikeHTML = "\n\t\t\t<p><strong>Minimum:</strong> 500 mhz processor, 96mb ram, 16mb video card, Windows XP, Mouse, Keyboard, Internet Connection<br /></p>\n\t\t\t"

func TestParseTierEldenRingMinimum(t *testing.T) {
	tier := ParseTier(eldenRingMinimumHTML)
	if tier == nil {
		t.Fatal("expected a non-nil tier")
	}

	if !reflect.DeepEqual(tier.OS, []OSRequirement{{Family: "windows", MinVersion: "10"}}) {
		t.Errorf("OS = %+v", tier.OS)
	}

	// AMD RYZEN 3 3300X is not in the seeded cpudb.json — only the Intel
	// alternative should resolve, and the AMD one must be surfaced as
	// unparsed rather than silently dropped.
	if tier.CPU == nil || !reflect.DeepEqual(tier.CPU.AnyOf, []string{"intel-core-i5-8400"}) {
		t.Errorf("CPU = %+v", tier.CPU)
	}
	if !containsSubstring(tier.Unparsed, "AMD RYZEN 3 3300X") {
		t.Errorf("Unparsed = %v, want it to mention the unresolved AMD CPU", tier.Unparsed)
	}

	// Both GPU alternatives (GTX 1060 3GB, RX 580) are absent from the
	// seeded gpudb.json (only the 6GB GTX 1060 variant is seeded, a
	// genuinely different SKU) — GPU must be nil, not a wrong guess.
	if tier.GPU != nil {
		t.Errorf("GPU = %+v, want nil (neither alternative is a known part)", tier.GPU)
	}
	if !containsSubstring(tier.Unparsed, "GTX 1060 3 GB") {
		t.Errorf("Unparsed = %v, want it to mention the unresolved GPUs", tier.Unparsed)
	}

	const twelveGiB = 12 * 1024 * 1024 * 1024
	if tier.RAMBytes != twelveGiB {
		t.Errorf("RAMBytes = %d, want %d", tier.RAMBytes, twelveGiB)
	}
	const sixtyGiB = 60 * 1024 * 1024 * 1024
	if tier.StorageBytes != sixtyGiB {
		t.Errorf("StorageBytes = %d, want %d", tier.StorageBytes, sixtyGiB)
	}
}

func TestParseTierEldenRingRecommended(t *testing.T) {
	tier := ParseTier(eldenRingRecommendedHTML)
	if tier == nil {
		t.Fatal("expected a non-nil tier")
	}

	want := []OSRequirement{{Family: "windows", MinVersion: "10"}, {Family: "windows", MinVersion: "11"}}
	if !reflect.DeepEqual(tier.OS, want) {
		t.Errorf("OS = %+v, want %+v", tier.OS, want)
	}

	// Neither CPU alternative (i7-8700K, Ryzen 5 3600X) is seeded.
	if tier.CPU != nil {
		t.Errorf("CPU = %+v, want nil", tier.CPU)
	}

	// GTX 1070 (no VRAM qualifier needed) IS seeded; RX Vega 56 is not.
	if tier.GPU == nil || !reflect.DeepEqual(tier.GPU.AnyOf, []string{"nvidia-geforce-gtx-1070"}) {
		t.Errorf("GPU = %+v", tier.GPU)
	}
	const eightGiB = 8 * 1024 * 1024 * 1024
	if tier.GPU != nil && tier.GPU.VRAMBytes != eightGiB {
		t.Errorf("GPU.VRAMBytes = %d, want %d", tier.GPU.VRAMBytes, eightGiB)
	}
	if !containsSubstring(tier.Unparsed, "VEGA 56") {
		t.Errorf("Unparsed = %v, want it to mention the unresolved Vega 56", tier.Unparsed)
	}

	const sixteenGiB = 16 * 1024 * 1024 * 1024
	if tier.RAMBytes != sixteenGiB {
		t.Errorf("RAMBytes = %d, want %d", tier.RAMBytes, sixteenGiB)
	}
}

// TestParseTierLegacyListingDegradesToUnparsed guards §6.2 step 5 for the
// real worst case: a decades-old store listing with no structured
// <strong>Processor:</strong>-style labels at all must never be
// misinterpreted as "no requirements" — it must come back entirely
// unparsed.
func TestParseTierLegacyListingDegradesToUnparsed(t *testing.T) {
	tier := ParseTier(counterStrikeHTML)
	if tier == nil {
		t.Fatal("expected a non-nil tier")
	}
	if tier.OS != nil || tier.CPU != nil || tier.GPU != nil || tier.RAMBytes != 0 || tier.StorageBytes != 0 {
		t.Errorf("expected every structured field to be empty for an unstructured listing, got %+v", tier)
	}
	if len(tier.Unparsed) == 0 {
		t.Fatal("expected the raw listing to be preserved in Unparsed, not silently dropped")
	}
}

// TestParseOSToleratesTrademarkSymbol guards a real-world encoding quirk
// found in a live appdetails response (Counter-Strike 2, appid 730):
// "Windows® 10" has a registered-trademark symbol directly after
// "Windows" with no preceding space, which a naive `windows\s*(\d+)`
// regex fails to match.
func TestParseOSToleratesTrademarkSymbol(t *testing.T) {
	got := parseOS("Windows® 10")
	want := []OSRequirement{{Family: "windows", MinVersion: "10"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseOS(Windows(R) 10) = %+v, want %+v", got, want)
	}
}

func TestParseOSDetectsLinuxAndMacOS(t *testing.T) {
	if got := parseOS("Ubuntu 20.04"); len(got) != 1 || got[0] != (OSRequirement{Family: "linux", MinVersion: "20.04"}) {
		t.Errorf("Ubuntu 20.04 -> %+v", got)
	}
	if got := parseOS("MacOS X 10.11 (El Capitan) or later"); len(got) != 1 || got[0].Family != "macos" || got[0].MinVersion != "10.11" {
		t.Errorf("MacOS X 10.11 -> %+v", got)
	}
}

func TestParseTierEmptyInput(t *testing.T) {
	if tier := ParseTier(""); tier != nil {
		t.Errorf("ParseTier(\"\") = %+v, want nil", tier)
	}
	if tier := ParseTier("   "); tier != nil {
		t.Errorf("ParseTier(whitespace) = %+v, want nil", tier)
	}
}

func containsSubstring(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
