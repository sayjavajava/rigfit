// SPDX-License-Identifier: GPL-3.0-only

package reqs

import "testing"

func TestMatchCanonicalIDExactOnly(t *testing.T) {
	// RTX 3080 must not match a hypothetical "Ti" superset or vice versa —
	// only exact token-set equality counts.
	if _, ok := matchCanonicalID("NVIDIA GEFORCE RTX 3080", gpuMatcher); !ok {
		t.Error("expected an exact match for a known GPU")
	}
	if _, ok := matchCanonicalID("NVIDIA GEFORCE RTX 3080 TI", gpuMatcher); ok {
		t.Error("a superset (extra 'Ti' token) must not match the non-Ti canonical ID")
	}
	if _, ok := matchCanonicalID("NVIDIA GEFORCE RTX", gpuMatcher); ok {
		t.Error("a subset (missing the model number) must not match")
	}
}

func TestMatchCanonicalIDUnknownReturnsFalse(t *testing.T) {
	if _, ok := matchCanonicalID("SOME MADE UP GPU 9999", gpuMatcher); ok {
		t.Error("expected no match for a nonexistent part")
	}
}

func TestTokenSetEqualIgnoresOrder(t *testing.T) {
	if !tokenSetEqual([]string{"a", "b", "c"}, []string{"c", "a", "b"}) {
		t.Error("token sets differing only in order should be equal")
	}
	if tokenSetEqual([]string{"a", "b"}, []string{"a", "b", "b"}) {
		t.Error("token multisets of different length should not be equal")
	}
}

func TestStripTrailingVRAM(t *testing.T) {
	model, vram, found := stripTrailingVRAM("NVIDIA GEFORCE GTX 1060 3 GB")
	if !found || model != "NVIDIA GEFORCE GTX 1060" || vram != 3*1024*1024*1024 {
		t.Errorf("model=%q vram=%d found=%v", model, vram, found)
	}

	if _, _, found := stripTrailingVRAM("INTEL CORE I5-8400"); found {
		t.Error("a CPU string with no trailing GB figure must not report found=true")
	}
}
