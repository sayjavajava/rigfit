// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"runtime"

	"github.com/sayjavajava/rigfit/pkg/hwid"
)

const snapshotSchemaVersion = 1

// snapshot is the anonymised hardware dump shared by `dump --anonymous`
// and `report-unknown` (rigfit-BUILD-SPEC.md §10, §11). hwid.CPU and
// hwid.GPU carry only hardware-identity fields (vendor/device IDs,
// canonical/market names, core counts, clocks, VRAM, driver version) —
// never a serial number, hostname, MAC address, or filesystem path — so
// the snapshot is safe to publish as-is.
type snapshot struct {
	SchemaVersion int        `json:"schema_version"`
	OS            string     `json:"os"`
	Arch          string     `json:"arch"`
	CPU           hwid.CPU   `json:"cpu"`
	GPUs          []hwid.GPU `json:"gpus"`
}

func (s snapshot) hasUnknown() bool {
	if s.CPU.Confidence == hwid.ConfidenceUnknown {
		return true
	}
	for _, g := range s.GPUs {
		if g.Confidence == hwid.ConfidenceUnknown {
			return true
		}
	}
	return false
}

func buildSnapshot(gpuOverride string) (snapshot, error) {
	var (
		gpus []hwid.GPU
		err  error
	)
	if gpuOverride != "" {
		gpus, err = hwid.ResolveGPUsWithOverride(gpuOverride)
	} else {
		gpus, err = hwid.ResolveGPUs()
	}
	if err != nil {
		return snapshot{}, err
	}
	return snapshot{
		SchemaVersion: snapshotSchemaVersion,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		CPU:           hwid.ResolveCPU(),
		GPUs:          gpus,
	}, nil
}
