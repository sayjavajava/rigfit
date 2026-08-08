// SPDX-License-Identifier: MPL-2.0

package hwid

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type fixtureCPU struct {
	VendorID     string `json:"vendor_id"`
	Family       uint16 `json:"family"`
	Model        uint16 `json:"model"`
	Stepping     uint8  `json:"stepping"`
	PhysCores    int    `json:"phys_cores"`
	LogicalCores int    `json:"logical_cores"`
}

type fixtureGPU struct {
	VendorID    string `json:"vendor_id"`
	DeviceID    string `json:"device_id"`
	SubsystemID string `json:"subsystem_id"`
	VRAMBytes   uint64 `json:"vram_bytes"`
}

type expectCPU struct {
	CanonicalID string `json:"canonical_id"`
	Confidence  string `json:"confidence"`
}

type expectGPU struct {
	CanonicalID string `json:"canonical_id"`
	Confidence  string `json:"confidence"`
	IsRenderer  bool   `json:"is_renderer"`
	IsDiscrete  bool   `json:"is_discrete"`
}

type fixture struct {
	Description string       `json:"description"`
	CPU         fixtureCPU   `json:"cpu"`
	GPUs        []fixtureGPU `json:"gpus"`
	GPUOverride string       `json:"gpu_override"`
	Expect      struct {
		CPU  expectCPU   `json:"cpu"`
		GPUs []expectGPU `json:"gpus"`
	} `json:"expect"`
}

func mustParseHex(t *testing.T, s string) uint64 {
	t.Helper()
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 64)
	if err != nil {
		t.Fatalf("invalid hex %q: %v", s, err)
	}
	return v
}

// TestGoldenFixtures runs every testdata/fixtures/*.json fixture through
// the same resolution logic real detection uses (resolveGPU,
// resolveCPUIdentity, selectRenderer), independent of the test runner's
// actual hardware. Fixtures are synthetic for M1 (rigfit-BUILD-SPEC.md
// §2A); real-machine dumps collected via `rigfit report-unknown` are added
// incrementally.
func TestGoldenFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "fixtures", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no fixtures found under testdata/fixtures")
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var fx fixture
			if err := json.Unmarshal(raw, &fx); err != nil {
				t.Fatalf("parsing fixture: %v", err)
			}

			cpu := resolveCPUIdentity(fx.CPU.VendorID, fx.CPU.Family, fx.CPU.Model, fx.CPU.Stepping, fx.CPU.PhysCores, fx.CPU.LogicalCores)
			if cpu.CanonicalID != fx.Expect.CPU.CanonicalID {
				t.Errorf("cpu canonical_id = %q, want %q", cpu.CanonicalID, fx.Expect.CPU.CanonicalID)
			}
			if cpu.Confidence.String() != fx.Expect.CPU.Confidence {
				t.Errorf("cpu confidence = %q, want %q", cpu.Confidence.String(), fx.Expect.CPU.Confidence)
			}

			gpus := make([]GPU, len(fx.GPUs))
			for i, g := range fx.GPUs {
				gpus[i] = resolveGPU(rawGPUDevice{
					VendorID:    uint16(mustParseHex(t, g.VendorID)),
					DeviceID:    uint16(mustParseHex(t, g.DeviceID)),
					SubsystemID: uint32(mustParseHex(t, g.SubsystemID)),
					VRAMBytes:   g.VRAMBytes,
				})
			}
			selectRenderer(gpus, fx.GPUOverride)

			if len(gpus) != len(fx.Expect.GPUs) {
				t.Fatalf("resolved %d GPUs, fixture expects %d", len(gpus), len(fx.Expect.GPUs))
			}
			for i, want := range fx.Expect.GPUs {
				got := gpus[i]
				if got.CanonicalID != want.CanonicalID {
					t.Errorf("gpu[%d] canonical_id = %q, want %q", i, got.CanonicalID, want.CanonicalID)
				}
				if got.Confidence.String() != want.Confidence {
					t.Errorf("gpu[%d] confidence = %q, want %q", i, got.Confidence.String(), want.Confidence)
				}
				if got.IsRenderer != want.IsRenderer {
					t.Errorf("gpu[%d] is_renderer = %v, want %v", i, got.IsRenderer, want.IsRenderer)
				}
				if got.IsDiscrete != want.IsDiscrete {
					t.Errorf("gpu[%d] is_discrete = %v, want %v", i, got.IsDiscrete, want.IsDiscrete)
				}
			}
		})
	}
}
