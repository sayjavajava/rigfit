// SPDX-License-Identifier: MPL-2.0

package hwid

import "github.com/klauspost/cpuid/v2"

// CPU is a single hardware-identified processor.
//
// JSON field names are part of rigfit's public --json contract (§11) and
// must not change casually.
type CPU struct {
	VendorID     string     `json:"vendor_id"` // "GenuineIntel" | "AuthenticAMD"
	Family       uint16     `json:"family"`
	Model        uint16     `json:"model"`
	Stepping     uint8      `json:"stepping"`
	CanonicalID  string     `json:"canonical_id"` // "amd-ryzen-5-5500"
	MarketName   string     `json:"market_name"`
	PhysCores    int        `json:"phys_cores"`
	LogicalCores int        `json:"logical_cores"`
	BaseClockMHz uint32     `json:"base_clock_mhz"`
	Confidence   Confidence `json:"confidence"`
}

// ResolveCPU identifies the current machine's CPU (§4.3). Vendor, family,
// model and stepping come from the CPUID instruction via
// github.com/klauspost/cpuid/v2, and are the primary lookup key into
// cpudb.json (§4.3 steps 1-2). The CPUID brand string is available as
// cpuid.CPU.BrandName but is deliberately not consulted here: it is a
// secondary signal only, never primary (§4.3 step 3), so it plays no part
// in canonical-ID resolution. An unrecognised vendor/family/model/stepping
// tuple — including new families such as AMD "Ryzen AI" or Intel "Core
// Ultra" that are not yet in cpudb.json — returns ConfidenceUnknown (§4.3
// step 4).
func ResolveCPU() CPU {
	return resolveCPUIdentity(
		cpuVendorString(cpuid.CPU.VendorID),
		uint16(cpuid.CPU.Family),
		uint16(cpuid.CPU.Model),
		uint8(cpuid.CPU.Stepping),
		cpuid.CPU.PhysicalCores,
		cpuid.CPU.LogicalCores,
	)
}

// resolveCPUIdentity is the pure resolution logic behind ResolveCPU,
// separated out so tests (and golden fixtures) can exercise it against an
// arbitrary declared identity without depending on the test runner's
// actual CPU.
func resolveCPUIdentity(vendorID string, family, model uint16, stepping uint8, physCores, logicalCores int) CPU {
	c := CPU{
		VendorID:     vendorID,
		Family:       family,
		Model:        model,
		Stepping:     stepping,
		PhysCores:    physCores,
		LogicalCores: logicalCores,
		Confidence:   ConfidenceUnknown,
	}

	entry, ok := lookupCPU(vendorID, family, model, stepping)
	if !ok {
		return c
	}

	c.CanonicalID = entry.CanonicalID
	c.MarketName = entry.MarketName
	c.BaseClockMHz = entry.BaseClockMHz
	if entry.PhysCores > 0 {
		c.PhysCores = entry.PhysCores
	}
	if entry.LogicalCores > 0 {
		c.LogicalCores = entry.LogicalCores
	}
	c.Confidence = ConfidenceExact
	return c
}

func cpuVendorString(v cpuid.Vendor) string {
	switch v {
	case cpuid.Intel:
		return "GenuineIntel"
	case cpuid.AMD:
		return "AuthenticAMD"
	default:
		return v.String()
	}
}
