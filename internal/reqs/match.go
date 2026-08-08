// SPDX-License-Identifier: GPL-3.0-only

package reqs

import (
	"regexp"
	"strings"

	"github.com/sayjavajava/rigfit/internal/score"
)

type matcherKind int

const (
	cpuMatcher matcherKind = iota
	gpuMatcher
)

// resolvePartSet extracts alternatives from text (§6.2 step 3) and
// resolves each against known canonical IDs in one pass, returning both
// the matched PartSet (nil if nothing matched) and the alternatives that
// didn't resolve to anything, for the caller to surface in Tier.Unparsed
// (§6.2 step 5) rather than silently drop.
func resolvePartSet(text string, kind matcherKind) (set *PartSet, unmatched []string) {
	var anyOf []string
	var maxVRAM uint64

	for _, alt := range splitAlternatives(text) {
		candidate := alt
		var vram uint64
		if kind == gpuMatcher {
			if model, v, found := stripTrailingVRAM(alt); found {
				candidate = model
				vram = v
			}
		}

		id, ok := matchCanonicalID(candidate, kind)
		if !ok && candidate != alt {
			// The trailing "N GB" might genuinely be part of the card's
			// own product name (e.g. a memory-differentiated SKU not
			// modelled that way in gpudb.json yet) rather than separate
			// VRAM info — fall back to matching the untouched text.
			id, ok = matchCanonicalID(alt, kind)
			vram = 0
		}
		if !ok {
			unmatched = append(unmatched, alt)
			continue
		}
		anyOf = append(anyOf, id)
		if vram > maxVRAM {
			maxVRAM = vram
		}
	}

	if len(anyOf) == 0 {
		return nil, unmatched
	}
	set = &PartSet{AnyOf: anyOf}
	if kind == gpuMatcher {
		set.VRAMBytes = maxVRAM
	}
	return set, unmatched
}

// matchCanonicalID resolves free text to a canonical GPU/CPU ID by exact
// token-set equality only — text tokenises to precisely the same set of
// tokens a canonical ID's hyphen-separated segments do, no more, no
// fewer. This is deliberately strict: §1 applies to text matching exactly
// as it does to hardware identity resolution, so "RTX 3080" must not
// match "nvidia-geforce-rtx-3080-ti" (superset) or match nothing at all
// rather than guess. An unmatched candidate is surfaced by the caller,
// never silently dropped.
func matchCanonicalID(text string, kind matcherKind) (string, bool) {
	wantTokens := tokenize(text)
	if len(wantTokens) == 0 {
		return "", false
	}

	var ids []string
	switch kind {
	case cpuMatcher:
		ids = score.CPUCanonicalIDs()
	case gpuMatcher:
		ids = score.GPUCanonicalIDs()
	}

	for _, id := range ids {
		if tokenSetEqual(wantTokens, strings.Split(id, "-")) {
			return id, true
		}
	}
	return "", false
}

var nonAlnumPattern = regexp.MustCompile(`[^a-z0-9]+`)

func tokenize(s string) []string {
	s = strings.ToLower(s)
	s = nonAlnumPattern.ReplaceAllString(s, " ")
	return strings.Fields(s)
}

func tokenSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, t := range a {
		counts[t]++
	}
	for _, t := range b {
		if counts[t] == 0 {
			return false
		}
		counts[t]--
	}
	return true
}
