// SPDX-License-Identifier: GPL-3.0-only

// Package reqs ingests a game's system requirements: fetching them from
// Steam, parsing the requirement HTML into structured data, matching
// hardware mentions against pkg/hwid/internal-score canonical IDs, and
// caching the result per rigfit-BUILD-SPEC.md §6/§9.
package reqs

import "time"

// ReviewStatus records how much a Requirements record can be trusted.
type ReviewStatus string

const (
	// ReviewAuto is the runtime layered parser's own best-effort output
	// (§6.2) — never LLM-touched, but also never human-checked.
	ReviewAuto ReviewStatus = "auto"
	// ReviewHumanVerified means a maintainer/contributor checked the
	// record (typically after tools/normalizer produced a candidate).
	ReviewHumanVerified ReviewStatus = "human_verified"
	// ReviewFlagged means tools/normalizer's validator rejected at least
	// one emitted part name against gpudb.json/cpudb.json (§6.4) — never
	// merged automatically.
	ReviewFlagged ReviewStatus = "flagged"
)

// Requirements is the on-disk/cache schema for a single game's system
// requirements (rigfit-BUILD-SPEC.md §6.3). JSON field names are a public,
// documented contract — see data/requirements/<appid>.json.
type Requirements struct {
	AppID             int          `json:"appid"`
	Name              string       `json:"name"`
	Source            string       `json:"source"` // "steam"
	FetchedAt         time.Time    `json:"fetched_at"`
	RawHash           string       `json:"raw_hash"` // "sha256:<hex>", of the raw HTML blob(s) this was parsed from
	NormalizerVersion string       `json:"normalizer_version"`
	ReviewStatus      ReviewStatus `json:"review_status"`
	Platforms         Platforms    `json:"platforms"`
	Tiers             Tiers        `json:"tiers"` // Windows requirements (pc_requirements)
	// LinuxTiers is the store listing's native Linux requirements
	// (linux_requirements), present only when Platforms.Linux is true. A
	// Linux user on a title with native support needs these, not Tiers —
	// and has no need for internal/proton's Proton-compatibility layer at
	// all, since no Proton/Wine translation is involved (§8).
	LinuxTiers *Tiers `json:"linux_tiers,omitempty"`
	// SourceURL cites what a human checked against when hand-correcting a
	// record (§12.5: "Require a source_url field on requirement
	// corrections"). Required whenever ReviewStatus is
	// ReviewHumanVerified; meaningless — and not required — for
	// ReviewAuto/ReviewFlagged records nothing has verified yet.
	SourceURL string `json:"source_url,omitempty"`
}

// Platforms records which OSes a Steam listing has native support for
// (appdetails' own "platforms" object). Platforms.Linux == false is what
// tells rigfit a Linux user needs internal/proton's compatibility layer
// (anti-cheat gate, ProtonDB, driver stack) rather than a native verdict.
type Platforms struct {
	Windows bool `json:"windows"`
	Mac     bool `json:"mac"`
	Linux   bool `json:"linux"`
}

// Tiers holds the minimum and recommended requirement sets. Recommended
// is a pointer/omittable because not every store listing publishes one
// (§9's "Anything unparsed" logic applies per-tier, not just per-field).
type Tiers struct {
	Minimum     *Tier `json:"minimum,omitempty"`
	Recommended *Tier `json:"recommended,omitempty"`
}

// Tier is one requirement tier (minimum or recommended). Every field is
// optional: a field the parser could not confidently extract is simply
// absent, and its raw source text (if any) is preserved in Unparsed so a
// human reviewer or tools/normalizer can look at exactly what defeated
// the layered parser (§6.2 step 5).
type Tier struct {
	OS           []OSRequirement `json:"os,omitempty"`
	CPU          *PartSet        `json:"cpu,omitempty"`
	GPU          *PartSet        `json:"gpu,omitempty"`
	RAMBytes     uint64          `json:"ram_bytes,omitempty"`
	StorageBytes uint64          `json:"storage_bytes,omitempty"`
	Unparsed     []string        `json:"unparsed,omitempty"`
}

// OSRequirement is one acceptable OS family/minimum-version pair. Like
// CPU/GPU, OS requirements are a set (§6.2 step 3): "Windows 10/11" means
// either satisfies it.
type OSRequirement struct {
	Family     string `json:"family"`      // "windows" | "linux" | "macos"
	MinVersion string `json:"min_version"` // e.g. "10"
}

// PartSet is a CPU or GPU requirement: any_of is a set where clearing any
// one member passes (§6.2 step 3). VRAMBytes, GPU-only, is evaluated
// separately from GPU tier per §7 — a card can clear the tier and still
// fail on VRAM.
type PartSet struct {
	AnyOf     []string `json:"any_of"`
	VRAMBytes uint64   `json:"vram_bytes,omitempty"`
}
