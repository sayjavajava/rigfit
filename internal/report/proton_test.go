// SPDX-License-Identifier: GPL-3.0-only

package report

import (
	"strings"
	"testing"
	"time"

	"github.com/sayjavajava/rigfit/internal/proton"
	"github.com/sayjavajava/rigfit/internal/score"
)

// TestProtonHumanBlockedStopsBeforeSpecComparison guards §8 point 1: a
// blocked anti-cheat gate must print "No — and no upgrade changes this"
// and nothing else — no GPU/CPU/RAM lines, no ProtonDB, no driver stack,
// even if those fields happen to be populated on the struct.
func TestProtonHumanBlockedStopsBeforeSpecComparison(t *testing.T) {
	pr := ProtonReport{
		GameName: "Battlefield 2042",
		AntiCheat: proton.AntiCheatResult{
			Status:       proton.GateBlocked,
			AntiCheats:   []string{"EA anticheat"},
			SourceStatus: "Denied",
			Available:    true,
		},
		// Populated to prove the renderer ignores them once blocked.
		ProtonDB: proton.ProtonDBRating{Available: true, Tier: "platinum"},
		Minimum:  &score.Verdict{Overall: score.StatusPass},
	}

	out := ProtonHuman(pr, false)
	if !strings.Contains(out, "No — and no upgrade changes this.") {
		t.Errorf("output must contain the permanent-block message: %s", out)
	}
	if strings.Contains(out, "ProtonDB") {
		t.Errorf("blocked output must not show ProtonDB: %s", out)
	}
	if strings.Contains(out, "platinum") || strings.Contains(out, "Platinum") {
		t.Errorf("blocked output must not leak spec-comparison detail: %s", out)
	}
}

func TestProtonHumanPlayableShowsFullReport(t *testing.T) {
	pr := ProtonReport{
		GameName: "ELDEN RING",
		AntiCheat: proton.AntiCheatResult{
			Status:       proton.GatePlayable,
			AntiCheats:   []string{"Easy Anti-Cheat"},
			SourceStatus: "Supported",
			Available:    true,
		},
		ProtonDB: proton.ProtonDBRating{Available: true, Tier: "gold", Total: 2085},
		Stack: proton.LinuxStack{
			MesaVersion:   "24.1.2",
			DriverBackend: "radv",
		},
		Minimum: &score.Verdict{
			Overall: score.StatusPass,
			Components: []score.ComponentVerdict{
				{Component: "gpu", Status: score.StatusPass, UserTier: 14, RequiredTier: 11},
			},
		},
		Source:    "steam",
		FetchedAt: time.Now(),
	}

	out := ProtonHuman(pr, false)
	if !strings.Contains(out, "Playable (Easy Anti-Cheat has Proton support enabled)") {
		t.Errorf("output must describe the playable anti-cheat status: %s", out)
	}
	if !strings.Contains(out, "Gold (2,085 reports)") {
		t.Errorf("output must show the ProtonDB tier and comma-formatted report count: %s", out)
	}
	if !strings.Contains(out, "Mesa 24.1.2 (RADV)") {
		t.Errorf("output must show the driver stack: %s", out)
	}
}

func TestProtonHumanNoKnownAntiCheatIsNotBlocking(t *testing.T) {
	pr := ProtonReport{
		GameName:  "Some Game",
		AntiCheat: proton.AntiCheatResult{Status: proton.GateNoKnownAntiCheat, Available: true},
		ProtonDB:  proton.ProtonDBRating{Available: false},
		Minimum:   &score.Verdict{Overall: score.StatusPass},
		FetchedAt: time.Now(),
	}
	out := ProtonHuman(pr, false)
	if strings.Contains(out, "No — and no upgrade changes this.") {
		t.Errorf("a game with no known anti-cheat must not be treated as blocked: %s", out)
	}
	if !strings.Contains(out, "No known anti-cheat concerns") {
		t.Errorf("output = %s", out)
	}
}

func TestFormatThousands(t *testing.T) {
	cases := map[int]string{0: "0", 5: "5", 999: "999", 1000: "1,000", 2085: "2,085", 1234567: "1,234,567"}
	for in, want := range cases {
		if got := formatThousands(in); got != want {
			t.Errorf("formatThousands(%d) = %q, want %q", in, got, want)
		}
	}
}
