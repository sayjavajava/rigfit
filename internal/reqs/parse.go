// SPDX-License-Identifier: GPL-3.0-only

package reqs

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

// ParseTier implements the §6.2 layered parser over one pc_requirements
// HTML blob (e.g. appdetails' data.pc_requirements.minimum). It never
// guesses: a field or alternative it can't confidently extract is either
// omitted (leaving the caller to treat it as Unknown) or preserved
// verbatim in Tier.Unparsed (§6.2 step 5).
func ParseTier(rawHTML string) *Tier {
	if strings.TrimSpace(rawHTML) == "" {
		return nil
	}

	labels := splitLabels(rawHTML)
	tier := &Tier{}

	if text, ok := labels["os"]; ok {
		if parsed := parseOS(text); len(parsed) > 0 {
			tier.OS = parsed
		} else {
			tier.Unparsed = append(tier.Unparsed, "os: "+text)
		}
	}

	if text, ok := labels["processor"]; ok {
		set, unmatched := resolvePartSet(text, cpuMatcher)
		if set != nil {
			tier.CPU = set
		}
		if len(unmatched) > 0 {
			tier.Unparsed = append(tier.Unparsed, "processor: "+strings.Join(unmatched, " / "))
		}
	}

	if text, ok := labels["graphics"]; ok {
		set, unmatched := resolvePartSet(text, gpuMatcher)
		if set != nil {
			tier.GPU = set
		}
		if len(unmatched) > 0 {
			tier.Unparsed = append(tier.Unparsed, "graphics: "+strings.Join(unmatched, " / "))
		}
	}

	if text, ok := labels["memory"]; ok {
		if b, ok := parseGBBytes(text); ok {
			tier.RAMBytes = b
		} else {
			tier.Unparsed = append(tier.Unparsed, "memory: "+text)
		}
	}

	if text, ok := labels["storage"]; ok {
		if b, ok := parseGBBytes(text); ok {
			tier.StorageBytes = b
		} else {
			tier.Unparsed = append(tier.Unparsed, "storage: "+text)
		}
	}

	if tier.OS == nil && tier.CPU == nil && tier.GPU == nil && tier.RAMBytes == 0 && tier.StorageBytes == 0 && len(tier.Unparsed) == 0 {
		// None of the five target labels were even found (e.g. a legacy
		// free-text listing with no <strong> field labels at all) — the
		// whole raw blob is unparsed rather than silently empty.
		tier.Unparsed = append(tier.Unparsed, plainText(rawHTML))
	}

	return tier
}

// relevantLabels are the five §6.2 step 1 fields. Everything else Steam's
// HTML mentions (DirectX, Sound Card, Network, Additional Notes, the
// "Minimum:"/"Recommended:" header itself) is intentionally ignored: it
// has no corresponding field in the §6.3 schema.
var relevantLabels = map[string]string{
	"os":        "os",
	"processor": "processor",
	"graphics":  "graphics",
	"memory":    "memory",
	"storage":   "storage",
}

var strongTagPattern = regexp.MustCompile(`(?is)<strong>(.*?)</strong>`)

// splitLabels implements §6.2 step 1: strip HTML, split on <strong>
// labels. It returns plain-text content for each recognised label,
// keyed by normalised lowercase label name.
func splitLabels(rawHTML string) map[string]string {
	matches := strongTagPattern.FindAllStringSubmatchIndex(rawHTML, -1)
	out := make(map[string]string, len(matches))
	for i, m := range matches {
		label := normalizeLabel(rawHTML[m[2]:m[3]])
		key, relevant := relevantLabels[label]
		if !relevant {
			continue
		}
		contentStart := m[1]
		contentEnd := len(rawHTML)
		if i+1 < len(matches) {
			contentEnd = matches[i+1][0]
		}
		out[key] = plainText(rawHTML[contentStart:contentEnd])
	}
	return out
}

func normalizeLabel(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ":")
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "*")
	s = strings.TrimSpace(s)
	return strings.ToLower(s)
}

var (
	brTagPattern   = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlTagPattern = regexp.MustCompile(`<[^>]+>`)
	whitespaceRun  = regexp.MustCompile(`[ \t]+`)
)

// plainText turns an HTML fragment into normalised plain text: <br>
// becomes a line break (preserved, since it's a meaningful separator
// between e.g. multiple graphics-card lines), every other tag is
// stripped, entities are unescaped, and whitespace is collapsed.
func plainText(fragment string) string {
	withBreaks := brTagPattern.ReplaceAllString(fragment, "\n")
	noTags := htmlTagPattern.ReplaceAllString(withBreaks, " ")
	unescaped := html.UnescapeString(noTags)
	lines := strings.Split(unescaped, "\n")
	kept := lines[:0]
	for _, line := range lines {
		line = strings.TrimSpace(whitespaceRun.ReplaceAllString(line, " "))
		if line != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// modifierPattern strips §6.2 step 4's qualifier phrases before
// alternative-splitting, so "or better"/"or equivalent" aren't mistaken
// for a second alternative card.
var modifierPattern = regexp.MustCompile(`(?i)\bor\s+(better|equivalent|higher|greater|newer|above)\b`)

// alternativeSplitter implements §6.2 step 3: a requirement is a set;
// clearing any one member passes. Steam separates alternatives with " or
// ", "/", or ",".
var alternativeSplitter = regexp.MustCompile(`\s*(?:,|/|\bor\b)\s*`)

func splitAlternatives(text string) []string {
	text = modifierPattern.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\n", ", ")
	var out []string
	for _, part := range alternativeSplitter.Split(text, -1) {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "minimum")
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// gbPattern captures a "<number> GB" figure. §6.3's worked example
// (12 GB -> 12884901888 bytes, 60 GB -> 64424509440 bytes) treats "GB" as
// binary (GiB), not decimal, so that convention is matched here.
var gbPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*GB\b`)

const gibiByte = 1024 * 1024 * 1024

// parseGBBytes implements §6.2 step 2 for RAM/storage: a plain "N GB"
// figure anywhere in the text.
func parseGBBytes(text string) (uint64, bool) {
	m := gbPattern.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	gb, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return uint64(gb * gibiByte), true
}

// trailingVRAMPattern extracts a "<number> GB" suffix from an individual
// GPU alternative string (§6.2 step 4's "embedded '6GB VRAM'"), e.g.
// "NVIDIA GEFORCE GTX 1060 3 GB" -> model "NVIDIA GEFORCE GTX 1060",
// vram 3 GiB.
var trailingVRAMPattern = regexp.MustCompile(`(?i)^(.*?)\s+(\d+(?:\.\d+)?)\s*GB\s*$`)

// stripTrailingVRAM splits a GPU candidate into its model-name portion
// and any trailing VRAM figure. found is false if there's no trailing
// "N GB" — most CPU candidates and some GPU candidates (no VRAM quoted).
func stripTrailingVRAM(candidate string) (model string, vramBytes uint64, found bool) {
	m := trailingVRAMPattern.FindStringSubmatch(candidate)
	if m == nil {
		return candidate, 0, false
	}
	gb, err := strconv.ParseFloat(m[2], 64)
	if err != nil {
		return candidate, 0, false
	}
	return strings.TrimSpace(m[1]), uint64(gb * gibiByte), true
}

// windowsVersionPattern captures the version cluster after "Windows",
// e.g. "10", "10/11", "7/8/10". The [^0-9]{0,12} gap (rather than \s*)
// deliberately tolerates non-digit filler like the "®" Steam sometimes
// embeds right after "Windows" with no preceding space (verified against
// a real appdetails response: "Windows® 10").
var windowsVersionPattern = regexp.MustCompile(`(?i)windows[^0-9]{0,12}([0-9]+(?:\s*[/,]\s*[0-9]+)*)`)

// ubuntuVersionPattern and macOSVersionPattern are the Linux/macOS
// analogues, seen in appdetails' linux_requirements/mac_requirements
// blocks (e.g. "Ubuntu 20.04", "MacOS X 10.11").
var (
	ubuntuVersionPattern = regexp.MustCompile(`(?i)ubuntu[^0-9]{0,12}([0-9]+(?:\.[0-9]+)?)`)
	macOSVersionPattern  = regexp.MustCompile(`(?i)mac\s*os\s*x?[^0-9]{0,12}([0-9]+(?:\.[0-9]+)?)`)
	linuxMentionPattern  = regexp.MustCompile(`(?i)\blinux\b`)
)

var versionClusterSplitter = regexp.MustCompile(`[/,]`)

// parseOS implements §6.2 for the OS field: family + minimum version,
// expanding a "10/11"-style cluster into one OSRequirement per version
// (still a set — any one satisfies it, per step 3). All three families
// rigfit targets (§1: Linux and Windows first-class, macOS best-effort)
// are checked, since a single game can list several (pc_requirements is
// Windows-only, but linux_requirements/mac_requirements share the same
// <strong>OS:</strong> label structure and land in the same parser).
func parseOS(text string) []OSRequirement {
	var out []OSRequirement

	if m := windowsVersionPattern.FindStringSubmatch(text); m != nil {
		for _, v := range versionClusterSplitter.Split(m[1], -1) {
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, OSRequirement{Family: "windows", MinVersion: v})
			}
		}
	}
	if m := ubuntuVersionPattern.FindStringSubmatch(text); m != nil {
		out = append(out, OSRequirement{Family: "linux", MinVersion: m[1]})
	} else if linuxMentionPattern.MatchString(text) {
		out = append(out, OSRequirement{Family: "linux"})
	}
	if m := macOSVersionPattern.FindStringSubmatch(text); m != nil {
		out = append(out, OSRequirement{Family: "macos", MinVersion: m[1]})
	}

	return out
}
