// SPDX-License-Identifier: GPL-3.0-only

// Package detect composes pkg/hwid's GPU/CPU identity resolution with the
// remaining signals rigfit-BUILD-SPEC.md §5-§7 need for a verdict: total
// RAM, storage free/total space, and OS version (rigfit-BUILD-SPEC.md
// §13 M2). It is the backing implementation for `rigfit specs`.
package detect

import (
	"os"

	"github.com/sayjavajava/rigfit/pkg/hwid"
)

// Specs is a full hardware/OS snapshot of the current machine.
type Specs struct {
	CPU     hwid.CPU    `json:"cpu"`
	GPUs    []hwid.GPU  `json:"gpus"`
	RAM     RAMInfo     `json:"ram"`
	Storage StorageInfo `json:"storage"`
	OS      OSInfo      `json:"os"`
}

// RAMInfo is total physical memory. rigfit never reports available/free
// RAM: it fluctuates from one moment to the next and isn't a stable input
// to a compatibility verdict the way total RAM is.
type RAMInfo struct {
	TotalBytes uint64 `json:"total_bytes"`
}

// StorageInfo is free/total space for the volume containing Path. M2
// reports the user's home directory as a stable, always-present
// reference point; per-game checks against the actual Steam library
// location are a verdict-engine concern (M5/M9), not a `specs` concern.
type StorageInfo struct {
	Path       string `json:"path"`
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
}

// OSInfo identifies the operating system. Family is lowercase
// ("windows" | "linux" | "darwin") to match the family field already used
// in the requirements schema (§6.3), so the same strings can eventually be
// compared directly against a game's minimum OS requirement.
type OSInfo struct {
	Family  string `json:"family"`
	Name    string `json:"name"`    // e.g. "Windows 11 Home Single Language"
	Version string `json:"version"` // e.g. "10.0.26200.8875"
}

// The platform-specific resolution steps are indirected through package
// vars, both so gpu_windows.go-style seams exist for testing and so
// Collect itself stays platform-agnostic.
var (
	resolveCPUFn              = hwid.ResolveCPU
	resolveGPUsFn             = hwid.ResolveGPUs
	resolveGPUsWithOverrideFn = hwid.ResolveGPUsWithOverride
	platformRAMBytesFn        = platformRAMBytes
	platformStorageFn         = platformStorage
	platformOSInfoFn          = platformOSInfo
)

// Collect gathers a full Specs snapshot. gpuOverride, if non-empty, forces
// renderer selection per §4.6 point 4 the same way `--gpu` does everywhere
// else in rigfit.
func Collect(gpuOverride string) (Specs, error) {
	var (
		gpus []hwid.GPU
		err  error
	)
	if gpuOverride != "" {
		gpus, err = resolveGPUsWithOverrideFn(gpuOverride)
	} else {
		gpus, err = resolveGPUsFn()
	}
	if err != nil {
		return Specs{}, err
	}

	ram, err := platformRAMBytesFn()
	if err != nil {
		return Specs{}, err
	}

	storagePath, err := os.UserHomeDir()
	if err != nil {
		return Specs{}, err
	}
	storage, err := platformStorageFn(storagePath)
	if err != nil {
		return Specs{}, err
	}

	osInfo, err := platformOSInfoFn()
	if err != nil {
		return Specs{}, err
	}

	return Specs{
		CPU:     resolveCPUFn(),
		GPUs:    gpus,
		RAM:     RAMInfo{TotalBytes: ram},
		Storage: storage,
		OS:      osInfo,
	}, nil
}
