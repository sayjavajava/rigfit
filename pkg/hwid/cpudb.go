// SPDX-License-Identifier: MPL-2.0

package hwid

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// cpudb.json is a synced copy of data/cpudb.json, for the same reason
// pci.ids is synced rather than embedded directly from data/ (go:embed
// cannot reach outside the package directory); see the comment on
// rawPCIIDs in pciids.go and TestEmbeddedCPUDBMatchesVendoredCopy in
// sync_test.go.
//
//go:embed cpudb.json
var rawCPUDB []byte

type cpuDBEntry struct {
	CanonicalID  string  `json:"canonical_id"`
	MarketName   string  `json:"market_name"`
	VendorID     string  `json:"vendor_id"`
	Family       uint16  `json:"family"`
	Model        uint16  `json:"model"`
	Steppings    []uint8 `json:"steppings"`
	PhysCores    int     `json:"phys_cores"`
	LogicalCores int     `json:"logical_cores"`
	BaseClockMHz uint32  `json:"base_clock_mhz"`
}

type cpuDBFile struct {
	SchemaVersion int          `json:"schema_version"`
	CPUs          []cpuDBEntry `json:"cpus"`
}

var (
	cpuDBOnce    sync.Once
	cpuDBEntries []cpuDBEntry
)

func getCPUDB() []cpuDBEntry {
	cpuDBOnce.Do(func() {
		var f cpuDBFile
		if err := json.Unmarshal(rawCPUDB, &f); err != nil {
			panic("hwid: embedded cpudb.json is invalid: " + err.Error())
		}
		cpuDBEntries = f.CPUs
	})
	return cpuDBEntries
}

// lookupCPU is the §4.3 step 2 lookup: an exact match on vendor, family
// and model, refined by stepping. An entry with an empty Steppings list
// matches any stepping of that vendor/family/model — those three fields
// alone already pin a specific silicon revision in both AMD's and Intel's
// numbering, so a wildcard stepping is a deliberate, documented design
// choice rather than a fuzzy fallback: it still requires an exact
// vendor+family+model match, and Confidence is set by the caller only when
// this returns ok.
func lookupCPU(vendorID string, family, model uint16, stepping uint8) (cpuDBEntry, bool) {
	for _, e := range getCPUDB() {
		if e.VendorID != vendorID || e.Family != family || e.Model != model {
			continue
		}
		if len(e.Steppings) == 0 {
			return e, true
		}
		for _, s := range e.Steppings {
			if s == stepping {
				return e, true
			}
		}
	}
	return cpuDBEntry{}, false
}
