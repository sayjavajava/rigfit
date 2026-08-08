// SPDX-License-Identifier: MPL-2.0

package hwid

import "testing"

// The tests in this file are the named regression cases required by
// rigfit-BUILD-SPEC.md §4.7. They encode real competitor failures and are
// non-negotiable: a change that breaks one of these has reintroduced a
// known bug, not found a new correct behaviour.

// TestRyzen5500NeverResolvesToA8Trinity guards against confusing AMD's
// Zen3-based Ryzen 5 5500 (family 0x19/25, model 0x50/80) with the
// unrelated Bulldozer-based A8-5500 "Trinity" APU (family 0x15/21, model
// 0x10/16) merely because both are marketed as "5500".
func TestRyzen5500NeverResolvesToA8Trinity(t *testing.T) {
	ryzen, ok := lookupCPU("AuthenticAMD", 25, 80, 0)
	if !ok {
		t.Fatal("expected AMD Ryzen 5 5500 (family 25 model 80) to resolve")
	}
	if ryzen.CanonicalID != "amd-ryzen-5-5500" {
		t.Fatalf("canonical ID = %q, want amd-ryzen-5-5500", ryzen.CanonicalID)
	}

	trinity, ok := lookupCPU("AuthenticAMD", 21, 16, 0)
	if !ok {
		t.Fatal("expected AMD A8-5500 Trinity (family 21 model 16) to resolve")
	}
	if trinity.CanonicalID != "amd-a8-5500" {
		t.Fatalf("canonical ID = %q, want amd-a8-5500", trinity.CanonicalID)
	}

	if ryzen.CanonicalID == trinity.CanonicalID {
		t.Fatalf("Ryzen 5 5500 and A8-5500 Trinity resolved to the same canonical ID: %q", ryzen.CanonicalID)
	}
}

// TestRyzen7700NeverResolvesToIntelI77700 guards against a cross-vendor
// mismatch between AMD's Ryzen 7 7700 (Zen4/Raphael) and Intel's Core
// i7-7700 (Kaby Lake), which share nothing but the "7700" digits.
func TestRyzen7700NeverResolvesToIntelI77700(t *testing.T) {
	amd, ok := lookupCPU("AuthenticAMD", 25, 97, 0)
	if !ok {
		t.Fatal("expected AMD Ryzen 7 7700 (family 25 model 97) to resolve")
	}
	intel, ok := lookupCPU("GenuineIntel", 6, 158, 9)
	if !ok {
		t.Fatal("expected Intel Core i7-7700 (family 6 model 158 stepping 9) to resolve")
	}
	if amd.CanonicalID == intel.CanonicalID {
		t.Fatalf("AMD Ryzen 7 7700 and Intel Core i7-7700 resolved to the same canonical ID: %q", amd.CanonicalID)
	}
}

// TestIrisXeNeverResolvesToIris6100 guards against confusing Tiger Lake's
// Iris Xe Graphics (device 0x9A49) with Broadwell's five-generations-older
// Iris Graphics 6100 (device 0x162B), both Intel and both named "Iris".
func TestIrisXeNeverResolvesToIris6100(t *testing.T) {
	xe := resolveGPU(rawGPUDevice{VendorID: VendorIntel, DeviceID: 0x9A49})
	if xe.Confidence != ConfidenceExact {
		t.Fatalf("Iris Xe: confidence = %v, want exact", xe.Confidence)
	}

	iris6100 := resolveGPU(rawGPUDevice{VendorID: VendorIntel, DeviceID: 0x162B})
	if iris6100.Confidence != ConfidenceExact {
		t.Fatalf("Iris Graphics 6100: confidence = %v, want exact", iris6100.Confidence)
	}

	if xe.CanonicalID == iris6100.CanonicalID {
		t.Fatalf("Iris Xe Graphics and Iris Graphics 6100 resolved to the same canonical ID: %q", xe.CanonicalID)
	}
}

// TestUnknownDeviceIDReturnsUnknown guards §1: a PCI device with no
// pci.ids entry must report ConfidenceUnknown, never a fuzzy guess.
func TestUnknownDeviceIDReturnsUnknown(t *testing.T) {
	g := resolveGPU(rawGPUDevice{VendorID: VendorNVIDIA, DeviceID: 0xFFFE})
	if g.Confidence != ConfidenceUnknown {
		t.Fatalf("confidence = %v, want ConfidenceUnknown", g.Confidence)
	}
	if g.CanonicalID != "" {
		t.Fatalf("CanonicalID = %q, want empty for an unrecognised device", g.CanonicalID)
	}
	if g.MarketName != "" {
		t.Fatalf("MarketName = %q, want empty for an unrecognised device", g.MarketName)
	}
}

// TestHybridGraphicsSelectsDiscrete guards §4.6: on a laptop reporting
// both an integrated and a discrete GPU, the discrete GPU must be selected
// as the renderer, and the integrated GPU must still be reported, never
// hidden.
func TestHybridGraphicsSelectsDiscrete(t *testing.T) {
	gpus := []GPU{
		resolveGPU(rawGPUDevice{VendorID: VendorIntel, DeviceID: 0x46A3}),  // Alder Lake-P GT1 [UHD Graphics] (integrated)
		resolveGPU(rawGPUDevice{VendorID: VendorNVIDIA, DeviceID: 0x2206}), // GA102 [GeForce RTX 3080] (discrete)
	}
	selectRenderer(gpus, "")

	if gpus[0].IsRenderer {
		t.Fatal("integrated Intel UHD Graphics must not be selected as renderer when a discrete GPU is present")
	}
	if !gpus[1].IsRenderer {
		t.Fatal("discrete NVIDIA GPU must be selected as renderer in a hybrid-graphics system")
	}
	if gpus[0].CanonicalID == "" || gpus[0].Confidence != ConfidenceExact {
		t.Fatal("the integrated GPU must still be reported (§4.6 point 3), never hidden")
	}
}
