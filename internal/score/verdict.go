// SPDX-License-Identifier: GPL-3.0-only

package score

import (
	"fmt"
	"strconv"

	"github.com/sayjavajava/rigfit/internal/detect"
	"github.com/sayjavajava/rigfit/pkg/hwid"
)

// Status is a single component's (or the overall) pass/fail state (§7).
type Status int

const (
	// StatusUnknown is the zero value deliberately: an uninitialised or
	// uncomputed ComponentVerdict reads as Unknown, never a silently
	// confident Pass.
	StatusUnknown Status = iota
	StatusPass
	StatusMarginal
	StatusFail
)

func (s Status) String() string {
	switch s {
	case StatusPass:
		return "pass"
	case StatusMarginal:
		return "marginal"
	case StatusFail:
		return "fail"
	default:
		return "unknown"
	}
}

// MarshalJSON renders Status as its String() form, matching hwid.Confidence's approach.
func (s Status) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// ComponentVerdict is one hardware component's comparison against a
// game's requirement (§7).
type ComponentVerdict struct {
	Component    string  `json:"component"` // "gpu" | "cpu" | "ram" | "storage" | "os" | "vram"
	Status       Status  `json:"status"`
	UserTier     int     `json:"user_tier,omitempty"`
	RequiredTier int     `json:"required_tier,omitempty"`
	Headroom     float64 `json:"headroom,omitempty"`
	Note         string  `json:"note,omitempty"`
}

// OSRequirement mirrors reqs.OSRequirement. See Requirement's doc comment
// for why this package defines its own copy instead of importing
// internal/reqs.
type OSRequirement struct {
	Family     string
	MinVersion string
}

// Requirement is the game-side input to Verdict — a decoupled mirror of
// reqs.Tier's fields. internal/reqs already imports internal/score (for
// GPUCanonicalIDs/CPUCanonicalIDs, its requirement-text matcher's target
// set), so internal/score importing internal/reqs back would be an import
// cycle. reqs.Tier has a ToRequirement() method that converts to this
// shape, keeping the translation on the already-permitted reqs -> score
// side of the dependency.
type Requirement struct {
	OS           []OSRequirement
	CPUAnyOf     []string
	GPUAnyOf     []string
	GPUVRAMBytes uint64
	RAMBytes     uint64
	StorageBytes uint64
}

// Verdict is the full §7 comparison of a user's machine against one
// requirement tier (minimum or recommended).
type Verdict struct {
	Overall    Status             `json:"overall"`
	Bottleneck string             `json:"bottleneck,omitempty"` // the single component most responsible, if not Pass
	Components []ComponentVerdict `json:"components"`
}

// marginalBand is §7's "Marginal within ±10%": a soft comparison whose
// headroom (user perf vs required perf, as a signed fraction) falls
// within this band on either side of the exact threshold is Marginal
// rather than a confident Pass or Fail.
const marginalBand = 0.10

// Verdict computes the full §7 verdict for one requirement tier.
//
// Hard gates (RAM, storage, OS, VRAM) are absolute: below threshold is
// Fail with no ratio. Soft comparisons (CPU, GPU) compare tiers via a
// ratio and can land Marginal. Any ConfidenceUnknown input (unrecognised
// user hardware, or a requirement alternative with no tier data) makes
// that component Unknown, never a guess.
func (b *Builtin) Verdict(specs detect.Specs, req Requirement) Verdict {
	renderer, hasRenderer := findRenderer(specs.GPUs)

	components := []ComponentVerdict{
		verdictOS(specs.OS, req.OS),
		verdictRAM(specs.RAM.TotalBytes, req.RAMBytes),
		verdictStorage(specs.Storage.FreeBytes, req.StorageBytes),
		b.verdictCPU(specs.CPU, req.CPUAnyOf),
		b.verdictGPU(renderer, hasRenderer, req.GPUAnyOf),
	}
	if vc := verdictVRAM(renderer, hasRenderer, req.GPUVRAMBytes); vc != nil {
		components = append(components, *vc)
	}

	overall := StatusPass
	for _, c := range components {
		if severity(c.Status) > severity(overall) {
			overall = c.Status
		}
	}

	return Verdict{
		Overall:    overall,
		Bottleneck: bottleneck(components),
		Components: components,
	}
}

func findRenderer(gpus []hwid.GPU) (hwid.GPU, bool) {
	for _, g := range gpus {
		if g.IsRenderer {
			return g, true
		}
	}
	return hwid.GPU{}, false
}

// verdictOS is a hard gate: family+version must satisfy at least one
// listed OSRequirement — a set, per §6.2 step 3, so clearing any one
// passes (e.g. a "Windows 10/11" requirement expands to two
// OSRequirements, either satisfies it).
func verdictOS(user detect.OSInfo, want []OSRequirement) ComponentVerdict {
	if len(want) == 0 {
		return ComponentVerdict{Component: "os", Status: StatusUnknown, Note: "requirement not recognised"}
	}
	for _, w := range want {
		if w.Family == user.Family && osVersionSatisfies(user.Version, w.MinVersion) {
			return ComponentVerdict{Component: "os", Status: StatusPass, Note: fmt.Sprintf("%s %s meets %s %s+", displayOSFamily(user.Family), user.Version, displayOSFamily(w.Family), w.MinVersion)}
		}
	}
	return ComponentVerdict{Component: "os", Status: StatusFail, Note: fmt.Sprintf("%s %s does not meet any listed requirement", displayOSFamily(user.Family), user.Version)}
}

// displayOSFamily capitalises a family value for human-readable Notes.
// The family field itself (§6.3's schema, detect.OSInfo.Family) stays
// lowercase everywhere else — this only affects freeform display text.
func displayOSFamily(family string) string {
	switch family {
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	case "macos":
		return "macOS"
	default:
		return family
	}
}

// osVersionSatisfies compares leading major version numbers only (e.g.
// detected "10.0.26200.8875" against a requirement of "10"). This is a
// deliberate simplification: OS versioning schemes differ too much across
// families to compare more precisely without per-family special-casing,
// and the major number is what game requirements actually gate on.
func osVersionSatisfies(userVersion, minVersion string) bool {
	userMajor, uok := leadingInt(userVersion)
	wantMajor, wok := leadingInt(minVersion)
	return uok && wok && userMajor >= wantMajor
}

func leadingInt(s string) (int, bool) {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:end])
	return n, err == nil
}

func verdictRAM(userBytes, wantBytes uint64) ComponentVerdict {
	if wantBytes == 0 {
		return ComponentVerdict{Component: "ram", Status: StatusUnknown, Note: "requirement not recognised"}
	}
	status := StatusFail
	if userBytes >= wantBytes {
		status = StatusPass
	}
	return ComponentVerdict{Component: "ram", Status: status, Note: fmt.Sprintf("%s (needs %s)", formatGiB(userBytes), formatGiB(wantBytes))}
}

// verdictStorage compares free space, not total (§7).
func verdictStorage(userFreeBytes, wantBytes uint64) ComponentVerdict {
	if wantBytes == 0 {
		return ComponentVerdict{Component: "storage", Status: StatusUnknown, Note: "requirement not recognised"}
	}
	status := StatusFail
	if userFreeBytes >= wantBytes {
		status = StatusPass
	}
	return ComponentVerdict{Component: "storage", Status: status, Note: fmt.Sprintf("%s free (needs %s)", formatGiB(userFreeBytes), formatGiB(wantBytes))}
}

// formatGiB is a minimal local formatter (score package intentionally has
// no dependency on internal/report's FormatBytes — internal/report
// already depends on internal/score, and a reverse dependency would
// cycle). GiB matches the convention parseGBBytes/gbPattern use elsewhere
// for interpreting "N GB" in requirement text.
func formatGiB(b uint64) string {
	const gib = 1024 * 1024 * 1024
	return fmt.Sprintf("%.1f GB", float64(b)/gib)
}

// verdictVRAM implements §7's "VRAM is evaluated separately from GPU
// tier": a card can clear the GPU tier and still fail here. It returns
// nil — no component at all, not Unknown — when the requirement didn't
// state a VRAM figure; that's "not applicable" rather than "uncertain".
func verdictVRAM(renderer hwid.GPU, hasRenderer bool, wantBytes uint64) *ComponentVerdict {
	if wantBytes == 0 {
		return nil
	}
	if !hasRenderer || renderer.Confidence == hwid.ConfidenceUnknown {
		return &ComponentVerdict{Component: "vram", Status: StatusUnknown, Note: "GPU not recognised"}
	}
	if renderer.VRAMBytes == 0 {
		return &ComponentVerdict{Component: "vram", Status: StatusUnknown, Note: "VRAM size could not be determined"}
	}
	status := StatusFail
	if renderer.VRAMBytes >= wantBytes {
		status = StatusPass
	}
	return &ComponentVerdict{Component: "vram", Status: status, Note: fmt.Sprintf("%s (needs %s)", formatGiB(renderer.VRAMBytes), formatGiB(wantBytes))}
}

func (b *Builtin) verdictCPU(user hwid.CPU, anyOf []string) ComponentVerdict {
	if len(anyOf) == 0 {
		return ComponentVerdict{Component: "cpu", Status: StatusUnknown, Note: "requirement not recognised"}
	}
	if user.Confidence == hwid.ConfidenceUnknown {
		return ComponentVerdict{Component: "cpu", Status: StatusUnknown, Note: "CPU not recognised"}
	}
	userScore, userConf, err := b.CPUScore(user.CanonicalID)
	if err != nil || userConf == hwid.ConfidenceUnknown {
		return ComponentVerdict{Component: "cpu", Status: StatusUnknown, Note: "CPU has no tier data yet"}
	}
	required, ok := easiestRequirement(anyOf, b.CPUScore)
	if !ok {
		return ComponentVerdict{Component: "cpu", Status: StatusUnknown, Note: "none of the required CPUs have tier data yet"}
	}
	return softVerdict("cpu", userScore, required)
}

func (b *Builtin) verdictGPU(renderer hwid.GPU, hasRenderer bool, anyOf []string) ComponentVerdict {
	if len(anyOf) == 0 {
		return ComponentVerdict{Component: "gpu", Status: StatusUnknown, Note: "requirement not recognised"}
	}
	if !hasRenderer || renderer.Confidence == hwid.ConfidenceUnknown {
		return ComponentVerdict{Component: "gpu", Status: StatusUnknown, Note: "GPU not recognised"}
	}
	userScore, userConf, err := b.GPUScore(renderer.CanonicalID)
	if err != nil || userConf == hwid.ConfidenceUnknown {
		return ComponentVerdict{Component: "gpu", Status: StatusUnknown, Note: "GPU has no tier data yet"}
	}
	required, ok := easiestRequirement(anyOf, b.GPUScore)
	if !ok {
		return ComponentVerdict{Component: "gpu", Status: StatusUnknown, Note: "none of the required GPUs have tier data yet"}
	}
	return softVerdict("gpu", userScore, required)
}

// easiestRequirement finds the lowest-performance (easiest to clear)
// alternative among a PartSet's any_of list that has tier data. That's
// the correct bar for a soft comparison: the user only needs to clear
// ONE listed alternative (§6.2 step 3), so the easiest one defines the
// true minimum. Alternatives with no tier data are skipped, not treated
// as failures — ok is false only if none of them have tier data at all.
func easiestRequirement(anyOf []string, scoreFn func(string) (Score, hwid.Confidence, error)) (Score, bool) {
	var easiest Score
	found := false
	for _, id := range anyOf {
		s, conf, err := scoreFn(id)
		if err != nil || conf == hwid.ConfidenceUnknown {
			continue
		}
		if !found || s.Perf < easiest.Perf {
			easiest = s
			found = true
		}
	}
	return easiest, found
}

func softVerdict(component string, user, required Score) ComponentVerdict {
	headroom := (user.Perf - required.Perf) / required.Perf
	status := StatusFail
	switch {
	case headroom > marginalBand:
		status = StatusPass
	case headroom >= -marginalBand:
		status = StatusMarginal
	}
	return ComponentVerdict{
		Component:    component,
		Status:       status,
		UserTier:     user.Tier,
		RequiredTier: required.Tier,
		Headroom:     headroom,
	}
}

// severity orders statuses worst-to-best for computing the overall
// verdict and picking a bottleneck: a definite Fail always outranks
// everything else; an Unknown outranks Marginal because uncertainty
// about whether a component even qualifies is treated as more urgent to
// resolve than a confirmed-but-borderline one (§1: never launder Unknown
// into a confident answer).
func severity(s Status) int {
	switch s {
	case StatusFail:
		return 3
	case StatusUnknown:
		return 2
	case StatusMarginal:
		return 1
	default:
		return 0
	}
}

// bottleneck implements §7's "Output MUST name the bottleneck": the
// single worst-status component, ties broken by lowest headroom (for
// soft comparisons; hard gates have no ratio, so ties among hard-gate
// failures are broken by whichever was evaluated first).
func bottleneck(components []ComponentVerdict) string {
	var worst *ComponentVerdict
	for i := range components {
		c := &components[i]
		if c.Status == StatusPass {
			continue
		}
		if worst == nil || severity(c.Status) > severity(worst.Status) {
			worst = c
			continue
		}
		if severity(c.Status) == severity(worst.Status) && c.Headroom < worst.Headroom {
			worst = c
		}
	}
	if worst == nil {
		return ""
	}
	return worst.Component
}
