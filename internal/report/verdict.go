// SPDX-License-Identifier: GPL-3.0-only

package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sayjavajava/rigfit/internal/cache"
	"github.com/sayjavajava/rigfit/internal/reqs"
	"github.com/sayjavajava/rigfit/internal/score"
)

// GameVerdict bundles the minimum/recommended verdicts for one game, the
// data that produced them, and provenance — everything VerdictHuman and
// VerdictJSON need. It's assembled by the caller (cmd/rigfit), which owns
// the detect -> reqs -> score orchestration.
type GameVerdict struct {
	GameName     string            `json:"game_name"`
	Minimum      *score.Verdict    `json:"minimum,omitempty"`
	Recommended  *score.Verdict    `json:"recommended,omitempty"`
	Source       string            `json:"source"`
	FetchedAt    time.Time         `json:"fetched_at"`
	Freshness    cache.Freshness   `json:"freshness"`
	ReviewStatus reqs.ReviewStatus `json:"review_status"`
	// AssumedGPU is set when --assume-gpu was used (§10 point 2): the
	// verdict was computed against a GPU that may not be what's actually
	// installed. It's a struct field (not a separate stdout line) so it
	// appears in --json output too, rather than corrupting it or being
	// invisible to a machine consumer.
	AssumedGPU string `json:"assumed_gpu,omitempty"`
}

// VerdictJSON renders GameVerdict as rigfit's stable --json output (§11).
func VerdictJSON(gv GameVerdict) ([]byte, error) {
	return json.MarshalIndent(gv, "", "  ")
}

// VerdictHuman renders GameVerdict as the human-readable text
// `rigfit "<game>"` prints by default. The primary answer is the minimum
// tier (§1: "determines whether a given PC game will run"); recommended,
// when available, is a supplementary line. useColor is §11's --no-color
// flag, inverted — callers resolve --no-color and the NO_COLOR env var
// convention before calling this; rendering itself stays a pure function
// of its arguments.
func VerdictHuman(gv GameVerdict, useColor bool) string {
	var b strings.Builder

	if gv.AssumedGPU != "" {
		fmt.Fprintf(&b, "Assumption: evaluating as if %s were installed (--assume-gpu). This is not your detected hardware.\n\n", gv.AssumedGPU)
	}
	fmt.Fprintf(&b, "%s on your machine\n\n", gv.GameName)

	if gv.Minimum == nil {
		fmt.Fprintln(&b, "  Minimum requirements could not be determined for this title.")
	} else {
		writeComponents(&b, gv.Minimum.Components, useColor)
		fmt.Fprintf(&b, "\n  Verdict: %s\n", verdictSummary("minimum", *gv.Minimum))
	}

	if gv.Recommended != nil {
		fmt.Fprintf(&b, "  Recommended: %s\n", verdictSummary("recommended", *gv.Recommended))
	}

	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "  Requirements: %s, %s\n", gv.Source, provenanceNote(gv.FetchedAt, gv.Freshness))
	if gv.ReviewStatus == reqs.ReviewAuto {
		fmt.Fprintln(&b, "  Note: parsed automatically, not human-reviewed — treat any \"?\" component literally, not as a guess.")
	}

	return b.String()
}

var componentLabels = map[string]string{
	"cpu":     "CPU",
	"gpu":     "GPU",
	"vram":    "VRAM",
	"ram":     "RAM",
	"storage": "Storage",
	"os":      "OS",
}

// componentOrder is display order, not evaluation order: hard gates the
// user can act on immediately (storage, RAM) read naturally after the
// parts comparison.
var componentOrder = []string{"cpu", "gpu", "vram", "ram", "storage", "os"}

func writeComponents(b *strings.Builder, components []score.ComponentVerdict, useColor bool) {
	byName := make(map[string]score.ComponentVerdict, len(components))
	for _, c := range components {
		byName[c.Component] = c
	}
	for _, name := range componentOrder {
		c, ok := byName[name]
		if !ok {
			continue
		}
		fmt.Fprintf(b, "  %-9s %s %s\n", componentLabels[name]+":", statusSymbol(c.Status, useColor), componentDetail(c))
	}
}

func statusSymbol(s score.Status, useColor bool) string {
	var symbol string
	switch s {
	case score.StatusPass:
		symbol = "✓"
	case score.StatusMarginal:
		symbol = "⚠"
	case score.StatusFail:
		symbol = "✗"
	default:
		symbol = "?"
	}
	return colorize(useColor, s, symbol)
}

func componentDetail(c score.ComponentVerdict) string {
	if c.Status == score.StatusUnknown {
		if c.Note != "" {
			return "Unrecognised — " + c.Note
		}
		return "Unrecognised"
	}
	if c.RequiredTier != 0 || c.UserTier != 0 {
		// Soft comparison (CPU/GPU): show the tier ratio.
		return fmt.Sprintf("tier %d vs %d required (%+.0f%% headroom)", c.UserTier, c.RequiredTier, c.Headroom*100)
	}
	if c.Note != "" {
		return c.Note
	}
	return c.Status.String()
}

func verdictSummary(tierName string, v score.Verdict) string {
	switch v.Overall {
	case score.StatusPass:
		return fmt.Sprintf("clears %s requirements", tierName)
	case score.StatusMarginal:
		return fmt.Sprintf("borderline against %s requirements (%s is the closest margin)", tierName, componentLabels[v.Bottleneck])
	case score.StatusFail:
		return fmt.Sprintf("does not meet %s requirements — %s is the blocker", tierName, componentLabels[v.Bottleneck])
	default:
		return fmt.Sprintf("cannot be determined against %s requirements (%s unrecognised)", tierName, componentLabels[v.Bottleneck])
	}
}

func provenanceNote(fetchedAt time.Time, freshness cache.Freshness) string {
	age := time.Since(fetchedAt)
	ageStr := formatAge(age)
	switch freshness {
	case cache.Stale:
		return fmt.Sprintf("fetched %s ago (stale)", ageStr)
	case cache.Expired:
		return fmt.Sprintf("fetched %s ago (expired)", ageStr)
	default:
		return fmt.Sprintf("fetched %s ago", ageStr)
	}
}

func formatAge(age time.Duration) string {
	switch {
	case age < time.Minute:
		return "moments"
	case age < time.Hour:
		return pluralUnit(int(age.Minutes()), "minute")
	case age < 24*time.Hour:
		return pluralUnit(int(age.Hours()), "hour")
	default:
		return pluralUnit(int(age.Hours()/24), "day")
	}
}

func pluralUnit(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
