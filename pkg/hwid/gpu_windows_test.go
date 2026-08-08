// SPDX-License-Identifier: MPL-2.0

//go:build windows

package hwid

import (
	"reflect"
	"strings"
	"testing"
)

// TestVRAMAbove4GBReadsRegistryNotWMI is the named §4.7 regression test
// for the §4.5 trap: Win32_VideoController.AdapterRAM is a uint32 and
// silently wraps above 4 GiB, so a 12 GB card must read its VRAM from the
// registry qwMemorySize value instead. Writing a fake adapter subkey under
// the real HKLM display class hive would require admin rights, which
// rigfit itself never requires, so this substitutes vramReader (the seam
// production code calls through) and asserts the value survives
// end-to-end without truncation.
func TestVRAMAbove4GBReadsRegistryNotWMI(t *testing.T) {
	const twelveGB = 12 * 1024 * 1024 * 1024 // overflows uint32; fits uint64/REG_QWORD

	orig := vramReader
	defer func() { vramReader = orig }()
	vramReader = func(pnpDeviceID string) uint64 { return twelveGB }

	raw, ok := buildRawGPUDevice(wmiVideoController{
		PNPDeviceID:   `PCI\VEN_10DE&DEV_2206&SUBSYS_14583FC8&REV_A1`,
		DriverVersion: "31.0.15.3623",
	})
	if !ok {
		t.Fatal("expected PNPDeviceID to parse")
	}
	if raw.VRAMBytes != twelveGB {
		t.Fatalf("VRAMBytes = %d, want %d (registry qwMemorySize, not truncating WMI AdapterRAM)", raw.VRAMBytes, twelveGB)
	}
}

// TestWMIStructNeverExposesAdapterRAM is a structural guard: if
// wmiVideoController ever grows an AdapterRAM field, it becomes possible
// to accidentally read VRAM from it again (§4.5). Fail loudly instead.
func TestWMIStructNeverExposesAdapterRAM(t *testing.T) {
	typ := reflect.TypeOf(wmiVideoController{})
	for i := 0; i < typ.NumField(); i++ {
		if strings.EqualFold(typ.Field(i).Name, "AdapterRAM") {
			t.Fatal("wmiVideoController must never read AdapterRAM (uint32, overflows above 4GB per §4.5); use the registry qwMemorySize path instead")
		}
	}
}
