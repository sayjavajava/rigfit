// SPDX-License-Identifier: GPL-3.0-only

package report

import (
	"strings"
	"testing"
	"time"

	"github.com/sayjavajava/rigfit/internal/cache"
	"github.com/sayjavajava/rigfit/internal/reqs"
	"github.com/sayjavajava/rigfit/internal/score"
)

func TestVerdictHumanNamesBottleneck(t *testing.T) {
	gv := GameVerdict{
		GameName: "ELDEN RING",
		Minimum: &score.Verdict{
			Overall:    score.StatusFail,
			Bottleneck: "storage",
			Components: []score.ComponentVerdict{
				{Component: "cpu", Status: score.StatusPass, UserTier: 10, RequiredTier: 8},
				{Component: "gpu", Status: score.StatusPass, UserTier: 10, RequiredTier: 8},
				{Component: "storage", Status: score.StatusFail, Note: "free disk space is below the requirement"},
			},
		},
		Source:       "steam",
		FetchedAt:    time.Now().Add(-2 * time.Hour),
		Freshness:    cache.Fresh,
		ReviewStatus: reqs.ReviewAuto,
	}

	out := VerdictHuman(gv, false)
	if !strings.Contains(out, "ELDEN RING") {
		t.Error("output must mention the game name")
	}
	if !strings.Contains(out, "Storage is the blocker") {
		t.Errorf("output must name the bottleneck component: %s", out)
	}
	if !strings.Contains(out, "✗") {
		t.Error("output must mark the failing component")
	}
	if !strings.Contains(out, "not human-reviewed") {
		t.Error("output must flag review_status:auto data")
	}
}

func TestVerdictHumanUnknownComponent(t *testing.T) {
	gv := GameVerdict{
		GameName: "Some Game",
		Minimum: &score.Verdict{
			Overall:    score.StatusUnknown,
			Bottleneck: "gpu",
			Components: []score.ComponentVerdict{
				{Component: "gpu", Status: score.StatusUnknown, Note: "GPU not recognised"},
			},
		},
		Source:    "steam",
		FetchedAt: time.Now(),
		Freshness: cache.Fresh,
	}

	out := VerdictHuman(gv, false)
	if !strings.Contains(out, "Unrecognised") {
		t.Errorf("output must flag the unrecognised component rather than a guess: %s", out)
	}
}

func TestVerdictJSONRoundTrips(t *testing.T) {
	gv := GameVerdict{
		GameName: "Test",
		Minimum: &score.Verdict{
			Overall:    score.StatusPass,
			Components: []score.ComponentVerdict{{Component: "ram", Status: score.StatusPass}},
		},
		Source:    "steam",
		FetchedAt: time.Now(),
	}
	body, err := VerdictJSON(gv)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"overall": "pass"`) {
		t.Errorf("expected Status to marshal as a string, got: %s", body)
	}
}
