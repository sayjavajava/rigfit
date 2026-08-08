// SPDX-License-Identifier: GPL-3.0-only

package score

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// cpudb.json is a synced copy of data/cpudb.json — the same file
// pkg/hwid/cpudb.json embeds for identity resolution, extended with the
// scoring-only fields (architecture, all_core_clock_mhz) this package
// reads. One canonical data file, two embedded projections; see
// sync_test.go for the drift guard.
//
//go:embed cpudb.json
var rawCPUDB []byte

type cpuDBEntry struct {
	CanonicalID     string  `json:"canonical_id"`
	MarketName      string  `json:"market_name"`
	Architecture    string  `json:"architecture"`
	PhysCores       int     `json:"phys_cores"`
	LogicalCores    int     `json:"logical_cores"`
	AllCoreClockMHz float64 `json:"all_core_clock_mhz"`
}

type cpuDBFile struct {
	SchemaVersion int          `json:"schema_version"`
	CPUs          []cpuDBEntry `json:"cpus"`
}

var (
	cpuDBOnce  sync.Once
	cpuDBIndex map[string]cpuDBEntry
)

func getCPUDB() map[string]cpuDBEntry {
	cpuDBOnce.Do(func() {
		var f cpuDBFile
		if err := json.Unmarshal(rawCPUDB, &f); err != nil {
			panic("score: embedded cpudb.json is invalid: " + err.Error())
		}
		cpuDBIndex = make(map[string]cpuDBEntry, len(f.CPUs))
		for _, e := range f.CPUs {
			cpuDBIndex[e.CanonicalID] = e
		}
	})
	return cpuDBIndex
}

func lookupCPU(canonicalID string) (cpuDBEntry, bool) {
	e, ok := getCPUDB()[canonicalID]
	return e, ok
}

// CPUCanonicalIDs returns every canonical CPU ID cpudb.json knows about;
// see GPUCanonicalIDs for why this is exported.
func CPUCanonicalIDs() []string {
	db := getCPUDB()
	ids := make([]string, 0, len(db))
	for id := range db {
		ids = append(ids, id)
	}
	return ids
}
