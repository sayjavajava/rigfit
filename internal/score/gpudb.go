// SPDX-License-Identifier: GPL-3.0-only

package score

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// gpudb.json is a synced copy of data/gpudb.json (go:embed cannot reach
// outside the package directory; see pkg/hwid/pciids.go's rawPCIIDs
// comment for the same constraint, and sync_test.go for the drift guard).
//
//go:embed gpudb.json
var rawGPUDB []byte

// gpuDBEntry is one gpudb.json record: the spec-sheet fields §5.1's raw
// performance formula needs, keyed by the same canonical_id pkg/hwid
// produces for that hardware.
type gpuDBEntry struct {
	CanonicalID          string  `json:"canonical_id"`
	MarketName           string  `json:"market_name"`
	Architecture         string  `json:"architecture"`
	ShaderUnits          int     `json:"shader_units"`
	BoostClockGHz        float64 `json:"boost_clock_ghz"`
	BusWidthBits         int     `json:"bus_width_bits"`
	EffectiveMemClockGHz float64 `json:"effective_mem_clock_ghz"`
}

type gpuDBFile struct {
	SchemaVersion int          `json:"schema_version"`
	GPUs          []gpuDBEntry `json:"gpus"`
}

var (
	gpuDBOnce  sync.Once
	gpuDBIndex map[string]gpuDBEntry
)

func getGPUDB() map[string]gpuDBEntry {
	gpuDBOnce.Do(func() {
		var f gpuDBFile
		if err := json.Unmarshal(rawGPUDB, &f); err != nil {
			panic("score: embedded gpudb.json is invalid: " + err.Error())
		}
		gpuDBIndex = make(map[string]gpuDBEntry, len(f.GPUs))
		for _, e := range f.GPUs {
			gpuDBIndex[e.CanonicalID] = e
		}
	})
	return gpuDBIndex
}

// lookupGPU is the §5's join point with pkg/hwid: an exact canonical_id
// match, or not found. There is no fuzzy fallback — §1 applies to scoring
// exactly as it does to identity resolution: an unrecognised part's score
// is Unknown, never estimated from a similar-sounding name.
func lookupGPU(canonicalID string) (gpuDBEntry, bool) {
	e, ok := getGPUDB()[canonicalID]
	return e, ok
}

// GPUCanonicalIDs returns every canonical GPU ID gpudb.json knows about.
// It exists for callers outside this package (internal/reqs' requirement
// matcher) that need to enumerate known parts rather than look one up —
// the set of valid targets a free-text requirement string is allowed to
// resolve to.
func GPUCanonicalIDs() []string {
	db := getGPUDB()
	ids := make([]string, 0, len(db))
	for id := range db {
		ids = append(ids, id)
	}
	return ids
}
