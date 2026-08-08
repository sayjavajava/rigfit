// SPDX-License-Identifier: MPL-2.0

package hwid

import (
	"fmt"
	"strings"
)

// Known GPU vendor PCI IDs, used both for canonical-ID formatting and for
// the discrete/integrated classification in isDiscreteGPU.
const (
	VendorNVIDIA uint16 = 0x10DE
	VendorAMD    uint16 = 0x1002
	VendorIntel  uint16 = 0x8086
)

// GPU is a single hardware-identified graphics device.
//
// JSON field names are part of rigfit's public --json contract (§11) and
// must not change casually.
type GPU struct {
	VendorID    uint16     `json:"vendor_id"` // 0x10DE nvidia, 0x1002 amd, 0x8086 intel
	DeviceID    uint16     `json:"device_id"`
	SubsystemID uint32     `json:"subsystem_id"` // (subsystem vendor ID << 16) | subsystem device ID
	CanonicalID string     `json:"canonical_id"` // "nvidia-geforce-gtx-1660-ti"
	MarketName  string     `json:"market_name"`  // "GeForce GTX 1660 Ti"
	VRAMBytes   uint64     `json:"vram_bytes"`
	IsDiscrete  bool       `json:"is_discrete"`
	IsRenderer  bool       `json:"is_renderer"` // the GPU games will actually use
	DriverVer   string     `json:"driver_version"`
	Confidence  Confidence `json:"confidence"`
}

// rawGPUDevice is what a platform-specific detector reports before
// canonicalisation: raw PCI identity plus whatever the OS could tell us
// about VRAM and driver version. Implemented per OS in gpu_<os>.go via
// platformGPUDevices.
type rawGPUDevice struct {
	VendorID    uint16
	DeviceID    uint16
	SubsystemID uint32
	VRAMBytes   uint64
	DriverVer   string
}

// platformGPUDevices enumerates every GPU the OS reports. Implemented per
// OS in gpu_linux.go, gpu_windows.go, gpu_darwin.go (each build-tagged so
// exactly one definition compiles per GOOS).

// ResolveGPUs enumerates every GPU the OS reports and resolves each to a
// canonical identity via the embedded pci.ids database (§4.2). A device
// with no pci.ids match is returned with ConfidenceUnknown rather than
// hidden or guessed at (§1). Among the results, the GPU that will actually
// render games is marked IsRenderer per the hybrid-graphics rule (§4.6).
func ResolveGPUs() ([]GPU, error) {
	return resolveGPUsWithOverride("")
}

// ResolveGPUsWithOverride behaves like ResolveGPUs but honours the CLI
// `--gpu <canonical-id>` override (§4.6 point 4): if overrideCanonicalID
// names a detected GPU, that GPU is forced to be the renderer regardless
// of the discrete-GPU heuristic. An override that matches nothing detected
// leaves no GPU marked as the renderer.
func ResolveGPUsWithOverride(overrideCanonicalID string) ([]GPU, error) {
	return resolveGPUsWithOverride(overrideCanonicalID)
}

// CanonicalGPUID resolves a PCI vendor:device pair to the same canonical
// ID and market name ResolveGPUs would produce for that hardware, without
// needing to run on a machine that actually has it installed. It exists so
// downstream tooling (tools/dbbuild, §5) can generate gpudb.json entries
// whose canonical_id is guaranteed to match what real detection produces,
// instead of a hand-typed string that could silently drift out of sync.
// ok is false if the pair has no pci.ids entry — §1: no fuzzy fallback.
func CanonicalGPUID(vendorID, deviceID uint16) (canonicalID, marketName string, ok bool) {
	entry, found := getPCIDatabase().lookupDevice(vendorID, deviceID)
	if !found {
		return "", "", false
	}
	return canonicalGPUID(entry.VendorID, entry.Name), entry.Name, true
}

func resolveGPUsWithOverride(overrideCanonicalID string) ([]GPU, error) {
	raw, err := platformGPUDevices()
	if err != nil {
		return nil, err
	}
	gpus := make([]GPU, 0, len(raw))
	for _, r := range raw {
		gpus = append(gpus, resolveGPU(r))
	}
	selectRenderer(gpus, overrideCanonicalID)
	return gpus, nil
}

// resolveGPU identifies a single raw device against the pci.ids database.
// It never falls through to string/fuzzy matching: a miss on both the
// device and subsystem lookups leaves Confidence at ConfidenceUnknown with
// CanonicalID/MarketName empty.
func resolveGPU(r rawGPUDevice) GPU {
	g := GPU{
		VendorID:    r.VendorID,
		DeviceID:    r.DeviceID,
		SubsystemID: r.SubsystemID,
		VRAMBytes:   r.VRAMBytes,
		DriverVer:   r.DriverVer,
		Confidence:  ConfidenceUnknown,
	}

	db := getPCIDatabase()

	if entry, ok := db.lookupDevice(r.VendorID, r.DeviceID); ok {
		applyPCIEntry(&g, entry)
		return g
	}

	if r.SubsystemID != 0 {
		subVendor := uint16(r.SubsystemID >> 16)
		subDevice := uint16(r.SubsystemID)
		if entry, ok := db.lookupSubsystem(subVendor, subDevice); ok {
			applyPCIEntry(&g, entry)
		}
	}

	return g
}

func applyPCIEntry(g *GPU, entry pciEntry) {
	g.CanonicalID = canonicalGPUID(entry.VendorID, entry.Name)
	g.MarketName = entry.Name
	g.IsDiscrete = isDiscreteGPU(entry.VendorID, entry.Name)
	g.Confidence = ConfidenceExact
}

func canonicalGPUID(vendorID uint16, name string) string {
	return vendorSlug(vendorID) + "-" + slugify(stripCodename(name))
}

// stripCodename removes a leading architecture codename that pci.ids often
// prefixes onto the marketing name, e.g. "GA102 [GeForce RTX 3080]" ->
// "GeForce RTX 3080". Entries with no bracketed marketing name keep the
// raw name as-is.
func stripCodename(name string) string {
	if i := strings.IndexByte(name, '['); i >= 0 {
		if j := strings.LastIndexByte(name, ']'); j > i {
			return name[i+1 : j]
		}
	}
	return name
}

func vendorSlug(vendorID uint16) string {
	switch vendorID {
	case VendorNVIDIA:
		return "nvidia"
	case VendorAMD:
		return "amd"
	case VendorIntel:
		return "intel"
	default:
		return fmt.Sprintf("vendor%04x", vendorID)
	}
}

// isDiscreteGPU classifies an already-identified device as discrete or
// integrated. This is downstream of identity resolution (Confidence is
// already Exact by the time this runs) and only affects which GPU is
// selected as the renderer in a hybrid-graphics system (§4.6); it never
// affects a device's Confidence.
//
// NVIDIA has not shipped an integrated desktop/laptop GPU in the current
// market, so every NVIDIA PCI display device is discrete. Intel and AMD
// both ship integrated and discrete parts under the same vendor ID;
// pci.ids' own product naming reliably distinguishes them (Intel's
// discrete Arc/DG1/DG2 line names itself as such; AMD's discrete Radeon RX
// / Pro / VII / HD / R-series cards do too, while AMD APU graphics
// functions are named generically, e.g. "Renoir" or "[Radeon Graphics]").
func isDiscreteGPU(vendorID uint16, name string) bool {
	switch vendorID {
	case VendorNVIDIA:
		return true
	case VendorIntel:
		return strings.Contains(name, "Arc") || strings.Contains(name, "DG1") || strings.Contains(name, "DG2")
	case VendorAMD:
		for _, marker := range []string{"Radeon RX", "Radeon Pro", "Radeon VII", "Radeon HD", "Radeon R9", "Radeon R7", "Radeon R5", "FirePro", "Radeon Instinct"} {
			if strings.Contains(name, marker) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// selectRenderer implements §4.6: exactly one discrete GPU is the
// renderer; with both an iGPU and a dGPU present, the discrete one renders
// and the iGPU is still reported, never hidden; --gpu overrides the choice
// outright. With zero or more than one discrete GPU and no override,
// rigfit does not guess: no GPU is forced as the renderer beyond the
// single-GPU trivial case.
func selectRenderer(gpus []GPU, overrideCanonicalID string) {
	if overrideCanonicalID != "" {
		for i := range gpus {
			gpus[i].IsRenderer = gpus[i].CanonicalID == overrideCanonicalID
		}
		return
	}

	discreteIdx := -1
	discreteCount := 0
	for i := range gpus {
		if gpus[i].IsDiscrete {
			discreteCount++
			discreteIdx = i
		}
	}

	switch {
	case discreteCount == 1:
		gpus[discreteIdx].IsRenderer = true
	case discreteCount == 0 && len(gpus) == 1:
		gpus[0].IsRenderer = true
	}
}
