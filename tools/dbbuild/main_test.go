// SPDX-License-Identifier: GPL-3.0-only

package main

import "testing"

func TestResolveGPUEntryUsesRealCanonicalID(t *testing.T) {
	entry, err := resolveGPUEntry(gpuInput{
		VendorID: "0x10de", DeviceID: "0x2206", // real GA102 [GeForce RTX 3080]
		Architecture: "nvidia-ampere", ShaderUnits: 8704, BoostClockGHz: 1.71,
		BusWidthBits: 320, EffectiveMemClockGHz: 19.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.CanonicalID != "nvidia-geforce-rtx-3080" {
		t.Errorf("canonical_id = %q, want nvidia-geforce-rtx-3080", entry.CanonicalID)
	}
	if entry.MarketName != "GA102 [GeForce RTX 3080]" {
		t.Errorf("market_name = %q", entry.MarketName)
	}
}

func TestResolveGPUEntryRejectsUnknownDevice(t *testing.T) {
	_, err := resolveGPUEntry(gpuInput{VendorID: "0x10de", DeviceID: "0xFFFE", Architecture: "nvidia-ampere"})
	if err == nil {
		t.Fatal("expected an error for a device with no pci.ids entry, got nil (§1: no fuzzy fallback)")
	}
}

func TestResolveGPUEntryRequiresArchitecture(t *testing.T) {
	_, err := resolveGPUEntry(gpuInput{VendorID: "0x10de", DeviceID: "0x2206"})
	if err == nil {
		t.Fatal("expected an error for a missing architecture")
	}
}

func TestResolveCPUEntryRejectsUnknownVendor(t *testing.T) {
	_, err := resolveCPUEntry(cpuInput{CanonicalID: "x", VendorID: "BogusVendor", Architecture: "intel-skylake"})
	if err == nil {
		t.Fatal("expected an error for a vendor_id that isn't GenuineIntel/AuthenticAMD")
	}
}

func TestResolveCPUEntryRoundTrips(t *testing.T) {
	in := cpuInput{
		CanonicalID: "intel-core-i5-8400", MarketName: "Intel Core i5-8400",
		VendorID: "GenuineIntel", Family: 6, Model: 158, Steppings: []int{10},
		PhysCores: 6, LogicalCores: 6, BaseClockMHz: 2800, AllCoreClockMHz: 3800,
		Architecture: "intel-skylake",
	}
	entry, err := resolveCPUEntry(in)
	if err != nil {
		t.Fatal(err)
	}
	if entry.CanonicalID != in.CanonicalID || entry.Family != in.Family || entry.Steppings[0] != 10 {
		t.Errorf("entry = %+v, want it to mirror input %+v", entry, in)
	}
}

func TestUpsertGPUReplacesExistingByCanonicalID(t *testing.T) {
	db := gpuDBFile{GPUs: []gpuDBEntry{{CanonicalID: "a", ShaderUnits: 100}}}
	db = upsertGPU(db, gpuDBEntry{CanonicalID: "a", ShaderUnits: 200})
	db = upsertGPU(db, gpuDBEntry{CanonicalID: "b", ShaderUnits: 300})
	if len(db.GPUs) != 2 {
		t.Fatalf("got %d entries, want 2 (replace, not duplicate)", len(db.GPUs))
	}
	for _, e := range db.GPUs {
		if e.CanonicalID == "a" && e.ShaderUnits != 200 {
			t.Errorf("entry %q not replaced: %+v", e.CanonicalID, e)
		}
	}
}

func TestParseHexUint16(t *testing.T) {
	v, err := parseHexUint16("0x10DE")
	if err != nil || v != 0x10de {
		t.Errorf("parseHexUint16(0x10DE) = %v, %v", v, err)
	}
	if _, err := parseHexUint16("not-hex"); err == nil {
		t.Error("expected an error for non-hex input")
	}
}
