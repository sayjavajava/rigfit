// SPDX-License-Identifier: GPL-3.0-only

// Command datavalidate is the check behind
// .github/workflows/data-validate.yml (§12.5): schema validation for
// data/gpudb.json, data/cpudb.json, data/coefficients.json, and
// data/requirements/*.json, plus the canonical-ID cross-check that
// rejects a hallucinated or typo'd part name before it can merge.
//
// It deliberately does NOT import internal/score's embedded gpudb/cpudb
// (via GPUCanonicalIDs()/CPUCanonicalIDs()) — that's a synced *copy*
// (pkg/hwid/pciids.go's rawPCIIDs comment explains why go:embed needs
// one), and mid-PR, before a contributor has run the sync step, the
// embedded copy could be stale relative to what's actually in data/. This
// tool parses data/ directly every time, so it always validates the real
// state of the files under review.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sayjavajava/rigfit/internal/reqs"
)

func main() {
	dataDir := flag.String("data", "data", "path to the data/ directory")
	flag.Parse()

	var failures []string
	fail := func(format string, args ...any) {
		failures = append(failures, fmt.Sprintf(format, args...))
	}

	gpuIDs, gpuArchesUsed := validateGPUDB(filepath.Join(*dataDir, "gpudb.json"), fail)
	cpuIDs, cpuArchesUsed := validateCPUDB(filepath.Join(*dataDir, "cpudb.json"), fail)
	validateCoefficients(filepath.Join(*dataDir, "coefficients.json"), gpuArchesUsed, cpuArchesUsed, fail)
	validateRequirements(filepath.Join(*dataDir, "requirements"), gpuIDs, cpuIDs, fail)

	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintln(os.Stderr, "FAIL:", f)
		}
		fmt.Fprintf(os.Stderr, "\n%d validation failure(s)\n", len(failures))
		os.Exit(1)
	}
	fmt.Println("data/ validation passed")
}

type failFunc func(format string, args ...any)

// --- gpudb.json -------------------------------------------------------

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

// validateGPUDB returns the set of valid canonical GPU IDs and the set of
// architectures gpudb.json actually uses (for the coefficients
// cross-check).
func validateGPUDB(path string, fail failFunc) (ids, arches map[string]bool) {
	ids, arches = map[string]bool{}, map[string]bool{}
	var f gpuDBFile
	if !readJSON(path, &f, fail) {
		return ids, arches
	}
	for i, e := range f.GPUs {
		loc := fmt.Sprintf("%s[%d]", path, i)
		if e.CanonicalID == "" {
			fail("%s: canonical_id is required", loc)
			continue
		}
		if ids[e.CanonicalID] {
			fail("%s: duplicate canonical_id %q", loc, e.CanonicalID)
		}
		ids[e.CanonicalID] = true
		if e.Architecture == "" {
			fail("%s (%s): architecture is required", loc, e.CanonicalID)
		} else {
			arches[e.Architecture] = true
		}
		if e.ShaderUnits <= 0 {
			fail("%s (%s): shader_units must be positive", loc, e.CanonicalID)
		}
		if e.BoostClockGHz <= 0 {
			fail("%s (%s): boost_clock_ghz must be positive", loc, e.CanonicalID)
		}
		if e.BusWidthBits <= 0 {
			fail("%s (%s): bus_width_bits must be positive", loc, e.CanonicalID)
		}
		if e.EffectiveMemClockGHz <= 0 {
			fail("%s (%s): effective_mem_clock_ghz must be positive", loc, e.CanonicalID)
		}
	}
	return ids, arches
}

// --- cpudb.json -------------------------------------------------------

type cpuDBEntry struct {
	CanonicalID     string  `json:"canonical_id"`
	MarketName      string  `json:"market_name"`
	VendorID        string  `json:"vendor_id"`
	Family          uint16  `json:"family"`
	Model           uint16  `json:"model"`
	Steppings       []int   `json:"steppings"`
	PhysCores       int     `json:"phys_cores"`
	LogicalCores    int     `json:"logical_cores"`
	BaseClockMHz    uint32  `json:"base_clock_mhz"`
	AllCoreClockMHz float64 `json:"all_core_clock_mhz"`
	Architecture    string  `json:"architecture"`
}

type cpuDBFile struct {
	SchemaVersion int          `json:"schema_version"`
	CPUs          []cpuDBEntry `json:"cpus"`
}

func validateCPUDB(path string, fail failFunc) (ids, arches map[string]bool) {
	ids, arches = map[string]bool{}, map[string]bool{}
	var f cpuDBFile
	if !readJSON(path, &f, fail) {
		return ids, arches
	}
	for i, e := range f.CPUs {
		loc := fmt.Sprintf("%s[%d]", path, i)
		if e.CanonicalID == "" {
			fail("%s: canonical_id is required", loc)
			continue
		}
		if ids[e.CanonicalID] {
			fail("%s: duplicate canonical_id %q", loc, e.CanonicalID)
		}
		ids[e.CanonicalID] = true
		if e.VendorID != "GenuineIntel" && e.VendorID != "AuthenticAMD" {
			fail("%s (%s): vendor_id must be GenuineIntel or AuthenticAMD, got %q", loc, e.CanonicalID, e.VendorID)
		}
		if e.PhysCores <= 0 || e.LogicalCores <= 0 {
			fail("%s (%s): phys_cores/logical_cores must be positive", loc, e.CanonicalID)
		}
		// Architecture/all_core_clock_mhz are scoring-only fields (§5.2,
		// consumed by internal/score, not pkg/hwid identity resolution)
		// and may legitimately be absent for identity-only seed entries —
		// only cross-check coefficients.json for the ones that do have it.
		if e.Architecture != "" {
			arches[e.Architecture] = true
		}
	}
	return ids, arches
}

// --- coefficients.json --------------------------------------------------

type archCoefficient struct {
	Coefficient float64  `json:"coefficient"`
	Note        string   `json:"note,omitempty"`
	SourceRefs  []string `json:"source_refs,omitempty"`
}

type coefficientsFile struct {
	SchemaVersion int                        `json:"schema_version"`
	GPUArch       map[string]archCoefficient `json:"gpu_arch"`
	CPUIPC        map[string]archCoefficient `json:"cpu_ipc"`
}

// validateCoefficients checks the file parses, every coefficient is
// positive, and — the valuable cross-file check — every architecture
// gpudb.json/cpudb.json actually reference has a coefficient defined,
// so a new part can never silently score with a missing multiplier.
func validateCoefficients(path string, gpuArchesUsed, cpuArchesUsed map[string]bool, fail failFunc) {
	var f coefficientsFile
	if !readJSON(path, &f, fail) {
		return
	}
	for name, c := range f.GPUArch {
		if c.Coefficient <= 0 {
			fail("%s: gpu_arch %q: coefficient must be positive, got %v", path, name, c.Coefficient)
		}
	}
	for name, c := range f.CPUIPC {
		if c.Coefficient <= 0 {
			fail("%s: cpu_ipc %q: coefficient must be positive, got %v", path, name, c.Coefficient)
		}
	}
	for arch := range gpuArchesUsed {
		if _, ok := f.GPUArch[arch]; !ok {
			fail("%s: gpudb.json uses architecture %q with no gpu_arch coefficient", path, arch)
		}
	}
	for arch := range cpuArchesUsed {
		if _, ok := f.CPUIPC[arch]; !ok {
			fail("%s: cpudb.json uses architecture %q with no cpu_ipc coefficient", path, arch)
		}
	}
}

// --- data/requirements/*.json -----------------------------------------

// validateRequirements implements §12.5's core guard: every cpu/gpu
// any_of entry across every tier (minimum, recommended, and Linux
// variants of both) must name a canonical ID that actually exists in
// gpudb.json/cpudb.json — the defence against a hallucinated or
// typo'd part name reaching data/. It also enforces the source_url
// requirement on human-verified corrections. A missing directory (no
// requirements curated yet) is not an error.
func validateRequirements(dir string, gpuIDs, cpuIDs map[string]bool, fail failFunc) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // nothing curated yet; not a failure
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		var r reqs.Requirements
		if !readJSON(path, &r, fail) {
			continue
		}
		if r.ReviewStatus == reqs.ReviewHumanVerified && r.SourceURL == "" {
			fail("%s: review_status is human_verified but source_url is empty", path)
		}
		checkTier(path, "tiers.minimum", r.Tiers.Minimum, gpuIDs, cpuIDs, fail)
		checkTier(path, "tiers.recommended", r.Tiers.Recommended, gpuIDs, cpuIDs, fail)
		if r.LinuxTiers != nil {
			checkTier(path, "linux_tiers.minimum", r.LinuxTiers.Minimum, gpuIDs, cpuIDs, fail)
			checkTier(path, "linux_tiers.recommended", r.LinuxTiers.Recommended, gpuIDs, cpuIDs, fail)
		}
	}
}

func checkTier(path, label string, tier *reqs.Tier, gpuIDs, cpuIDs map[string]bool, fail failFunc) {
	if tier == nil {
		return
	}
	if tier.CPU != nil {
		for _, id := range tier.CPU.AnyOf {
			if !cpuIDs[id] {
				fail("%s: %s.cpu.any_of: %q is not a known canonical CPU ID in cpudb.json", path, label, id)
			}
		}
	}
	if tier.GPU != nil {
		for _, id := range tier.GPU.AnyOf {
			if !gpuIDs[id] {
				fail("%s: %s.gpu.any_of: %q is not a known canonical GPU ID in gpudb.json", path, label, id)
			}
		}
	}
}

// readJSON reads and unmarshals path into v, reporting a failure (not a
// fatal error) and returning false if the file is missing or malformed —
// callers proceed to check whatever else they can rather than aborting
// the whole run on the first bad file.
func readJSON(path string, v any, fail failFunc) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		fail("%s: %v", path, err)
		return false
	}
	if err := json.Unmarshal(raw, v); err != nil {
		fail("%s: %v", path, err)
		return false
	}
	return true
}
