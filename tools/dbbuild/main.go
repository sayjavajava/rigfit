// SPDX-License-Identifier: GPL-3.0-only

// Command dbbuild is the build-pipeline tool that turns spec-sheet input
// into data/gpudb.json and data/cpudb.json entries (rigfit-BUILD-SPEC.md
// §5, §13 M3). It is not shipped in the rigfit binary.
//
// GPU canonical IDs are never hand-typed: dbbuild resolves them from a
// vendor:device PCI ID pair via pkg/hwid.CanonicalGPUID, the same function
// real hardware detection uses, so a gpudb.json entry is guaranteed to
// match what `rigfit specs` would actually report for that card. A device
// ID absent from the vendored pci.ids is a hard error, not a guess.
//
// Usage:
//
//	go run ./tools/dbbuild -input spec.json [-repo .] [-sync]
//
// -sync also copies the updated data/*.json files to their embedded
// copies in pkg/hwid/ and internal/score/ (see those packages' sync_test.go
// files for why the copies exist).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/sayjavajava/rigfit/pkg/hwid"
)

type gpuInput struct {
	VendorID             string  `json:"vendor_id"` // hex, e.g. "0x10de"
	DeviceID             string  `json:"device_id"`
	Architecture         string  `json:"architecture"`
	ShaderUnits          int     `json:"shader_units"`
	BoostClockGHz        float64 `json:"boost_clock_ghz"`
	BusWidthBits         int     `json:"bus_width_bits"`
	EffectiveMemClockGHz float64 `json:"effective_mem_clock_ghz"`
}

type cpuInput struct {
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

type specSheetInput struct {
	GPUs []gpuInput `json:"gpus"`
	CPUs []cpuInput `json:"cpus"`
}

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
	Note          string       `json:"note,omitempty"`
	GPUs          []gpuDBEntry `json:"gpus"`
}

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
	Note          string       `json:"note,omitempty"`
	CPUs          []cpuDBEntry `json:"cpus"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "dbbuild:", err)
		os.Exit(1)
	}
}

func run() error {
	inputPath := flag.String("input", "", "path to a spec-sheet input JSON file (required)")
	repoRoot := flag.String("repo", ".", "repository root (containing data/, pkg/hwid/, internal/score/)")
	sync := flag.Bool("sync", false, "also copy updated data/*.json files to their embedded copies")
	flag.Parse()

	if *inputPath == "" {
		flag.Usage()
		return fmt.Errorf("-input is required")
	}

	raw, err := os.ReadFile(*inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *inputPath, err)
	}
	var input specSheetInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return fmt.Errorf("parsing %s: %w", *inputPath, err)
	}

	gpudbPath := filepath.Join(*repoRoot, "data", "gpudb.json")
	gpudb, err := loadGPUDB(gpudbPath)
	if err != nil {
		return err
	}
	for _, g := range input.GPUs {
		entry, err := resolveGPUEntry(g)
		if err != nil {
			return err
		}
		gpudb = upsertGPU(gpudb, entry)
		fmt.Printf("gpu:  %-45s <- %s\n", entry.CanonicalID, *inputPath)
	}
	if err := writeGPUDB(gpudbPath, gpudb); err != nil {
		return err
	}

	cpudbPath := filepath.Join(*repoRoot, "data", "cpudb.json")
	cpudb, err := loadCPUDB(cpudbPath)
	if err != nil {
		return err
	}
	for _, c := range input.CPUs {
		entry, err := resolveCPUEntry(c)
		if err != nil {
			return err
		}
		cpudb = upsertCPU(cpudb, entry)
		fmt.Printf("cpu:  %-45s <- %s\n", entry.CanonicalID, *inputPath)
	}
	if err := writeCPUDB(cpudbPath, cpudb); err != nil {
		return err
	}

	if *sync {
		if err := syncEmbedded(*repoRoot); err != nil {
			return err
		}
	}
	return nil
}

// resolveGPUEntry resolves a GPU's canonical_id/market_name via
// pkg/hwid.CanonicalGPUID rather than trusting anything hand-typed in the
// input file, so gpudb.json can never drift from what real detection
// produces for that PCI ID.
func resolveGPUEntry(g gpuInput) (gpuDBEntry, error) {
	vendorID, err := parseHexUint16(g.VendorID)
	if err != nil {
		return gpuDBEntry{}, fmt.Errorf("gpu vendor_id %q: %w", g.VendorID, err)
	}
	deviceID, err := parseHexUint16(g.DeviceID)
	if err != nil {
		return gpuDBEntry{}, fmt.Errorf("gpu device_id %q: %w", g.DeviceID, err)
	}
	canonicalID, marketName, ok := hwid.CanonicalGPUID(vendorID, deviceID)
	if !ok {
		return gpuDBEntry{}, fmt.Errorf("gpu %s:%s has no pci.ids entry — refusing to invent a canonical ID (§1)", g.VendorID, g.DeviceID)
	}
	if g.Architecture == "" {
		return gpuDBEntry{}, fmt.Errorf("gpu %s (%s): architecture is required", canonicalID, marketName)
	}
	return gpuDBEntry{
		CanonicalID:          canonicalID,
		MarketName:           marketName,
		Architecture:         g.Architecture,
		ShaderUnits:          g.ShaderUnits,
		BoostClockGHz:        g.BoostClockGHz,
		BusWidthBits:         g.BusWidthBits,
		EffectiveMemClockGHz: g.EffectiveMemClockGHz,
	}, nil
}

// resolveCPUEntry validates a CPU input. Unlike GPUs, CPU canonical IDs
// have no derivation function (cpudb.json is the identity source of truth
// for CPUs, not a formula over a hardware ID) so the input's canonical_id
// is trusted as-is, but must be present and must not collide with a
// different vendor/family/model/stepping tuple already in cpudb.json.
func resolveCPUEntry(c cpuInput) (cpuDBEntry, error) {
	if c.CanonicalID == "" {
		return cpuDBEntry{}, fmt.Errorf("cpu entry (%s): canonical_id is required", c.MarketName)
	}
	if c.VendorID != "GenuineIntel" && c.VendorID != "AuthenticAMD" {
		return cpuDBEntry{}, fmt.Errorf("cpu %s: vendor_id must be GenuineIntel or AuthenticAMD, got %q", c.CanonicalID, c.VendorID)
	}
	if c.Architecture == "" {
		return cpuDBEntry{}, fmt.Errorf("cpu %s: architecture is required", c.CanonicalID)
	}
	return cpuDBEntry(c), nil
}

func parseHexUint16(s string) (uint16, error) {
	v, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(s), "0x"), 16, 16)
	if err != nil {
		return 0, err
	}
	return uint16(v), nil
}

func loadGPUDB(path string) (gpuDBFile, error) {
	db := gpuDBFile{SchemaVersion: 1}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return db, nil
	}
	if err != nil {
		return db, err
	}
	if err := json.Unmarshal(raw, &db); err != nil {
		return db, fmt.Errorf("parsing %s: %w", path, err)
	}
	return db, nil
}

func upsertGPU(db gpuDBFile, entry gpuDBEntry) gpuDBFile {
	for i, e := range db.GPUs {
		if e.CanonicalID == entry.CanonicalID {
			db.GPUs[i] = entry
			return db
		}
	}
	db.GPUs = append(db.GPUs, entry)
	return db
}

func writeGPUDB(path string, db gpuDBFile) error {
	sort.Slice(db.GPUs, func(i, j int) bool { return db.GPUs[i].CanonicalID < db.GPUs[j].CanonicalID })
	return writeJSON(path, db)
}

func loadCPUDB(path string) (cpuDBFile, error) {
	db := cpuDBFile{SchemaVersion: 1}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return db, nil
	}
	if err != nil {
		return db, err
	}
	if err := json.Unmarshal(raw, &db); err != nil {
		return db, fmt.Errorf("parsing %s: %w", path, err)
	}
	return db, nil
}

func upsertCPU(db cpuDBFile, entry cpuDBEntry) cpuDBFile {
	for i, e := range db.CPUs {
		if e.CanonicalID == entry.CanonicalID {
			db.CPUs[i] = entry
			return db
		}
	}
	db.CPUs = append(db.CPUs, entry)
	return db
}

func writeCPUDB(path string, db cpuDBFile) error {
	sort.Slice(db.CPUs, func(i, j int) bool { return db.CPUs[i].CanonicalID < db.CPUs[j].CanonicalID })
	return writeJSON(path, db)
}

func writeJSON(path string, v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

// syncEmbedded copies data/*.json to the embedded copies pkg/hwid and
// internal/score each keep (go:embed cannot reach outside its own package
// directory — see pkg/hwid/pciids.go's rawPCIIDs comment).
func syncEmbedded(repoRoot string) error {
	copies := map[string][]string{
		"gpudb.json": {filepath.Join(repoRoot, "internal", "score", "gpudb.json")},
		"cpudb.json": {filepath.Join(repoRoot, "pkg", "hwid", "cpudb.json"), filepath.Join(repoRoot, "internal", "score", "cpudb.json")},
	}
	for name, dests := range copies {
		src := filepath.Join(repoRoot, "data", name)
		body, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		for _, dest := range dests {
			if err := os.WriteFile(dest, body, 0o644); err != nil {
				return err
			}
			fmt.Printf("sync: %s -> %s\n", src, dest)
		}
	}
	return nil
}
