// SPDX-License-Identifier: GPL-3.0-only

package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sayjavajava/rigfit/internal/score"
)

// LibraryTitleResult is one Steam library title's minimum-tier verdict.
type LibraryTitleResult struct {
	AppID      int          `json:"appid"`
	Name       string       `json:"name"`
	Overall    score.Status `json:"overall"`
	Bottleneck string       `json:"bottleneck,omitempty"`
}

// LibraryReport is `rigfit library`'s full output (§11: "which titles
// underperform, and which single upgrade unlocks the most").
type LibraryReport struct {
	Scanned         int                  `json:"scanned"`   // titles found installed
	Evaluated       int                  `json:"evaluated"` // titles that had usable requirements data
	Underperforming []LibraryTitleResult `json:"underperforming"`
	TopUpgrade      string               `json:"top_upgrade,omitempty"`
	TopUpgradeCount int                  `json:"top_upgrade_count,omitempty"`
}

// NewLibraryReport builds a LibraryReport from every evaluated title's
// result, computing which single component is the most common bottleneck
// among titles that don't comfortably pass. "Underperforming" means a
// confident Fail or Marginal — a title with an Unknown verdict (no
// recognised hardware or no requirements data) is neither confirmed
// underperforming nor confirmed fine, so it's excluded from both the list
// and the upgrade-recommendation count rather than counted either way
// (§1: never launder Unknown into a confident answer).
func NewLibraryReport(scanned int, results []LibraryTitleResult) LibraryReport {
	r := LibraryReport{Scanned: scanned, Evaluated: len(results)}

	bottleneckCounts := map[string]int{}
	for _, res := range results {
		if res.Overall != score.StatusFail && res.Overall != score.StatusMarginal {
			continue
		}
		r.Underperforming = append(r.Underperforming, res)
		if res.Bottleneck != "" {
			bottleneckCounts[res.Bottleneck]++
		}
	}

	sort.Slice(r.Underperforming, func(i, j int) bool {
		si, sj := r.Underperforming[i], r.Underperforming[j]
		if si.Overall != sj.Overall {
			return si.Overall == score.StatusFail // Fail sorts before Marginal
		}
		return si.Name < sj.Name
	})

	var topCount int
	var topComponent string
	// Iterate component names in componentOrder for deterministic
	// tie-breaking (map iteration order is random in Go).
	for _, c := range componentOrder {
		if n := bottleneckCounts[c]; n > topCount {
			topCount, topComponent = n, c
		}
	}
	if topComponent != "" {
		r.TopUpgrade = topComponent
		r.TopUpgradeCount = topCount
	}

	return r
}

// LibraryJSON renders LibraryReport as rigfit's stable --json output (§11).
func LibraryJSON(r LibraryReport) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// LibraryHuman renders LibraryReport as human-readable text.
func LibraryHuman(r LibraryReport, useColor bool) string {
	var b strings.Builder

	fmt.Fprintln(&b, "Steam library scan")
	fmt.Fprintln(&b)
	skipped := r.Scanned - r.Evaluated
	fmt.Fprintf(&b, "Scanned %d installed title(s), evaluated %d", r.Scanned, r.Evaluated)
	if skipped > 0 {
		fmt.Fprintf(&b, " (%d had no usable requirements data)", skipped)
	}
	fmt.Fprintln(&b, ".")
	fmt.Fprintln(&b)

	if len(r.Underperforming) == 0 {
		fmt.Fprintln(&b, "Nothing underperforms minimum requirements on this machine.")
		return b.String()
	}

	fmt.Fprintf(&b, "Underperforming (%d):\n", len(r.Underperforming))
	for _, t := range r.Underperforming {
		label := componentLabels[t.Bottleneck]
		if label == "" {
			label = t.Bottleneck
		}
		fmt.Fprintf(&b, "  %s %s — %s vs minimum (%s)\n", statusSymbol(t.Overall, useColor), t.Name, t.Overall, label)
	}
	fmt.Fprintln(&b)

	if r.TopUpgrade != "" {
		label := componentLabels[r.TopUpgrade]
		if label == "" {
			label = r.TopUpgrade
		}
		fmt.Fprintf(&b, "Upgrade recommendation: a %s upgrade would help the most — it's the bottleneck for %d of %d underperforming titles.\n", label, r.TopUpgradeCount, len(r.Underperforming))
	}

	return b.String()
}
