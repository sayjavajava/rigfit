// SPDX-License-Identifier: GPL-3.0-only

package detect

import (
	"errors"
	"testing"

	"github.com/sayjavajava/rigfit/pkg/hwid"
)

func withFakeCollectors(t *testing.T, gpus []hwid.GPU, cpu hwid.CPU) {
	t.Helper()

	origCPU, origGPUs, origGPUsOverride := resolveCPUFn, resolveGPUsFn, resolveGPUsWithOverrideFn
	origRAM, origStorage, origOS := platformRAMBytesFn, platformStorageFn, platformOSInfoFn
	t.Cleanup(func() {
		resolveCPUFn, resolveGPUsFn, resolveGPUsWithOverrideFn = origCPU, origGPUs, origGPUsOverride
		platformRAMBytesFn, platformStorageFn, platformOSInfoFn = origRAM, origStorage, origOS
	})

	resolveCPUFn = func() hwid.CPU { return cpu }
	resolveGPUsFn = func() ([]hwid.GPU, error) { return gpus, nil }
	resolveGPUsWithOverrideFn = func(string) ([]hwid.GPU, error) { return gpus, nil }
	platformRAMBytesFn = func() (uint64, error) { return 34359738368, nil } // 32 GiB
	platformStorageFn = func(path string) (StorageInfo, error) {
		return StorageInfo{Path: path, TotalBytes: 1_000_000_000_000, FreeBytes: 250_000_000_000}, nil
	}
	platformOSInfoFn = func() (OSInfo, error) {
		return OSInfo{Family: "windows", Name: "Windows 11 Pro", Version: "10.0.26200"}, nil
	}
}

// TestCollectPreservesHybridGraphics is M2's "hybrid graphics verified"
// requirement (§13): Collect must not collapse a laptop's integrated +
// discrete GPU pair down to one entry, and must preserve which one is
// marked as the renderer.
func TestCollectPreservesHybridGraphics(t *testing.T) {
	igpu := hwid.GPU{CanonicalID: "intel-uhd-graphics", IsDiscrete: false, IsRenderer: false, Confidence: hwid.ConfidenceExact}
	dgpu := hwid.GPU{CanonicalID: "nvidia-geforce-rtx-3080", IsDiscrete: true, IsRenderer: true, Confidence: hwid.ConfidenceExact}
	withFakeCollectors(t, []hwid.GPU{igpu, dgpu}, hwid.CPU{Confidence: hwid.ConfidenceExact})

	specs, err := Collect("")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs.GPUs) != 2 {
		t.Fatalf("got %d GPUs, want 2 (both must be reported, never collapsed)", len(specs.GPUs))
	}
	if specs.GPUs[0].IsRenderer {
		t.Error("integrated GPU must not be marked as renderer")
	}
	if !specs.GPUs[1].IsRenderer {
		t.Error("discrete GPU must be marked as renderer")
	}
}

func TestCollectComposesAllSignals(t *testing.T) {
	cpu := hwid.CPU{CanonicalID: "amd-ryzen-5-5500", Confidence: hwid.ConfidenceExact}
	gpu := hwid.GPU{CanonicalID: "nvidia-geforce-rtx-3080", IsRenderer: true, Confidence: hwid.ConfidenceExact}
	withFakeCollectors(t, []hwid.GPU{gpu}, cpu)

	specs, err := Collect("")
	if err != nil {
		t.Fatal(err)
	}
	if specs.CPU.CanonicalID != "amd-ryzen-5-5500" {
		t.Errorf("CPU = %+v", specs.CPU)
	}
	if specs.RAM.TotalBytes != 34359738368 {
		t.Errorf("RAM.TotalBytes = %d", specs.RAM.TotalBytes)
	}
	if specs.Storage.FreeBytes != 250_000_000_000 {
		t.Errorf("Storage.FreeBytes = %d", specs.Storage.FreeBytes)
	}
	if specs.OS.Family != "windows" {
		t.Errorf("OS.Family = %q", specs.OS.Family)
	}
}

func TestCollectPropagatesGPUResolutionError(t *testing.T) {
	withFakeCollectors(t, nil, hwid.CPU{})
	wantErr := errors.New("boom")
	resolveGPUsFn = func() ([]hwid.GPU, error) { return nil, wantErr }

	if _, err := Collect(""); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
