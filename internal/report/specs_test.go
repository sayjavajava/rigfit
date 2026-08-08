// SPDX-License-Identifier: GPL-3.0-only

package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sayjavajava/rigfit/internal/detect"
	"github.com/sayjavajava/rigfit/pkg/hwid"
)

func sampleSpecs() detect.Specs {
	return detect.Specs{
		CPU: hwid.CPU{
			CanonicalID: "amd-ryzen-5-5500", MarketName: "AMD Ryzen 5 5500",
			PhysCores: 6, LogicalCores: 12, BaseClockMHz: 3600, Confidence: hwid.ConfidenceExact,
		},
		GPUs: []hwid.GPU{
			{CanonicalID: "intel-uhd-graphics", MarketName: "Intel UHD Graphics", Confidence: hwid.ConfidenceExact},
			{CanonicalID: "nvidia-geforce-rtx-3080", MarketName: "GeForce RTX 3080", VRAMBytes: 10_737_418_240, IsRenderer: true, Confidence: hwid.ConfidenceExact},
		},
		RAM:     detect.RAMInfo{TotalBytes: 34_359_738_368},
		Storage: detect.StorageInfo{Path: "/home/user", TotalBytes: 1_000_000_000_000, FreeBytes: 250_000_000_000},
		OS:      detect.OSInfo{Family: "linux", Name: "Ubuntu 24.04", Version: "24.04"},
	}
}

// TestSpecsHumanShowsBothHybridGPUs guards against the human renderer
// collapsing a hybrid-graphics system down to a single reported GPU.
func TestSpecsHumanShowsBothHybridGPUs(t *testing.T) {
	out := SpecsHuman(sampleSpecs(), false)
	if !strings.Contains(out, "Intel UHD Graphics") {
		t.Error("output must mention the integrated GPU")
	}
	if !strings.Contains(out, "GeForce RTX 3080") {
		t.Error("output must mention the discrete GPU")
	}
	if !strings.Contains(out, "renderer") {
		t.Error("output must mark which GPU renders")
	}
}

func TestSpecsHumanShowsUnknownCPU(t *testing.T) {
	specs := sampleSpecs()
	specs.CPU = hwid.CPU{VendorID: "AuthenticAMD", Family: 23, Model: 96, Confidence: hwid.ConfidenceUnknown}
	out := SpecsHuman(specs, false)
	if !strings.Contains(out, "Unrecognised CPU") {
		t.Errorf("output = %q, want it to flag the unrecognised CPU rather than presenting a guess", out)
	}
}

func TestSpecsJSONRoundTrips(t *testing.T) {
	body, err := SpecsJSON(sampleSpecs())
	if err != nil {
		t.Fatal(err)
	}
	var decoded detect.Specs
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decoding: %v; body = %s", err, body)
	}
	if len(decoded.GPUs) != 2 {
		t.Fatalf("decoded %d GPUs, want 2", len(decoded.GPUs))
	}
	if decoded.CPU.CanonicalID != "amd-ryzen-5-5500" {
		t.Errorf("CPU.CanonicalID = %q", decoded.CPU.CanonicalID)
	}
}
