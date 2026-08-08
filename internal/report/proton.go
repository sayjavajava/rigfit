// SPDX-License-Identifier: GPL-3.0-only

package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sayjavajava/rigfit/internal/cache"
	"github.com/sayjavajava/rigfit/internal/proton"
	"github.com/sayjavajava/rigfit/internal/reqs"
	"github.com/sayjavajava/rigfit/internal/score"
)

// ProtonReport bundles everything `rigfit "<game>"` needs to render §8's
// Linux/Proton output: the anti-cheat gate (checked first, overrides
// everything), ProtonDB rating, driver stack, and — only if the
// anti-cheat gate didn't block — the usual component verdicts.
type ProtonReport struct {
	GameName     string                 `json:"game_name"`
	AntiCheat    proton.AntiCheatResult `json:"anti_cheat"`
	ProtonDB     proton.ProtonDBRating  `json:"protondb"`
	Stack        proton.LinuxStack      `json:"stack"`
	Minimum      *score.Verdict         `json:"minimum,omitempty"`
	Recommended  *score.Verdict         `json:"recommended,omitempty"`
	Source       string                 `json:"source"`
	FetchedAt    time.Time              `json:"fetched_at"`
	Freshness    cache.Freshness        `json:"freshness"`
	ReviewStatus reqs.ReviewStatus      `json:"review_status"`
	AssumedGPU   string                 `json:"assumed_gpu,omitempty"`
}

// ProtonJSON renders ProtonReport as rigfit's stable --json output (§11).
func ProtonJSON(pr ProtonReport) ([]byte, error) {
	return json.MarshalIndent(pr, "", "  ")
}

// ProtonHuman renders ProtonReport as §8's human-readable output shape.
// When the anti-cheat gate blocks, this stops right after the Anti-cheat
// line with "No — and no upgrade changes this" — §8 point 1: checked
// first, overrides everything, before any spec comparison is even shown.
func ProtonHuman(pr ProtonReport, useColor bool) string {
	var b strings.Builder

	if pr.AssumedGPU != "" {
		fmt.Fprintf(&b, "Assumption: evaluating as if %s were installed (--assume-gpu). This is not your detected hardware.\n\n", pr.AssumedGPU)
	}
	fmt.Fprintf(&b, "%s on your machine\n\n", pr.GameName)
	fmt.Fprintf(&b, "  %-12s %s\n", "Anti-cheat:", antiCheatLine(pr.AntiCheat))

	if pr.AntiCheat.Blocks() {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "  No — and no upgrade changes this.")
		return b.String()
	}

	fmt.Fprintf(&b, "  %-12s %s\n", "ProtonDB:", protonDBLine(pr.ProtonDB))
	fmt.Fprintln(&b)

	if pr.Minimum != nil {
		writeComponents(&b, pr.Minimum.Components, useColor)
		fmt.Fprintf(&b, "  %-12s %s\n", "Driver:", driverLine(pr.Stack))
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "  Verdict: %s\n", verdictSummary("minimum", *pr.Minimum))
	} else {
		fmt.Fprintln(&b, "  Minimum requirements could not be determined for this title.")
	}
	if pr.Recommended != nil {
		fmt.Fprintf(&b, "  Recommended: %s\n", verdictSummary("recommended", *pr.Recommended))
	}

	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "  Requirements: %s, %s\n", pr.Source, provenanceNote(pr.FetchedAt, pr.Freshness))
	if pr.ReviewStatus == reqs.ReviewAuto {
		fmt.Fprintln(&b, "  Note: parsed automatically, not human-reviewed — treat any \"?\" component literally, not as a guess.")
	}

	return b.String()
}

func antiCheatLine(r proton.AntiCheatResult) string {
	if !r.Available {
		return "? Could not be checked (offline or data unavailable)"
	}
	names := "anti-cheat"
	if len(r.AntiCheats) > 0 {
		names = strings.Join(r.AntiCheats, " + ")
	}
	switch r.Status {
	case proton.GatePlayable:
		return fmt.Sprintf("✓ Playable (%s has Proton support enabled)", names)
	case proton.GateBlocked:
		return fmt.Sprintf("✗ Blocked (%s has no Proton support — denied)", names)
	case proton.GateCurrentlyBroken:
		return fmt.Sprintf("✗ Not currently working (%s — status: %s)", names, r.SourceStatus)
	default:
		return "✓ No known anti-cheat concerns"
	}
}

func protonDBLine(r proton.ProtonDBRating) string {
	if !r.Available {
		return "unavailable"
	}
	tier := r.Tier
	if tier == "" {
		tier = "unrated"
	}
	return fmt.Sprintf("%s (%s reports)", capitalize(tier), formatThousands(r.Total))
}

// capitalize upper-cases the first byte of a known-ASCII ProtonDB tier
// name ("gold", "platinum", ...) — strings.Title is deprecated stdlib and
// overkill (Unicode-aware word-boundary logic) for this narrow use.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}

func driverLine(s proton.LinuxStack) string {
	var parts []string
	if s.MesaVersion != "" {
		mesa := "Mesa " + s.MesaVersion
		if s.DriverBackend != "" {
			mesa += fmt.Sprintf(" (%s)", strings.ToUpper(s.DriverBackend))
		}
		parts = append(parts, mesa)
	}
	if s.NVIDIADriverVersion != "" {
		parts = append(parts, "NVIDIA "+s.NVIDIADriverVersion)
	}
	if s.VulkanAPIVersion != "" {
		parts = append(parts, "Vulkan "+s.VulkanAPIVersion)
	}
	if s.SessionType != "" {
		parts = append(parts, s.SessionType)
	}
	if s.IsSteamDeck {
		parts = append(parts, "Steam Deck")
	}
	if len(parts) == 0 {
		return "unavailable"
	}
	return strings.Join(parts, ", ")
}

func formatThousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
