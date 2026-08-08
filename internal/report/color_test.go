// SPDX-License-Identifier: GPL-3.0-only

package report

import (
	"strings"
	"testing"

	"github.com/sayjavajava/rigfit/internal/score"
)

func TestColorizeWrapsWithANSIWhenEnabled(t *testing.T) {
	got := colorize(true, score.StatusFail, "✗")
	if !strings.HasPrefix(got, ansiRed) || !strings.HasSuffix(got, ansiReset) {
		t.Errorf("colorize(true, Fail, ...) = %q, want it wrapped in red/reset codes", got)
	}
	if !strings.Contains(got, "✗") {
		t.Errorf("colorize must still contain the original text: %q", got)
	}
}

func TestColorizeNoOpWhenDisabled(t *testing.T) {
	if got := colorize(false, score.StatusFail, "✗"); got != "✗" {
		t.Errorf("colorize(false, ...) = %q, want the unmodified input", got)
	}
}

func TestStatusSymbolColorPerStatus(t *testing.T) {
	cases := []struct {
		status score.Status
		code   string
	}{
		{score.StatusPass, ansiGreen},
		{score.StatusMarginal, ansiYellow},
		{score.StatusFail, ansiRed},
		{score.StatusUnknown, ansiGray},
	}
	for _, c := range cases {
		got := statusSymbol(c.status, true)
		if !strings.HasPrefix(got, c.code) {
			t.Errorf("statusSymbol(%v, true) = %q, want prefix %q", c.status, got, c.code)
		}
	}
}

// TestVerdictHumanColorDoesNotBreakPlainTextAdjacency checks that
// colorizing only the symbol (not surrounding text) still leaves the rest
// of a line's plain text intact and findable.
func TestVerdictHumanColorDoesNotBreakPlainTextAdjacency(t *testing.T) {
	gv := GameVerdict{
		GameName: "Test",
		Minimum: &score.Verdict{
			Overall: score.StatusFail,
			Components: []score.ComponentVerdict{
				{Component: "storage", Status: score.StatusFail, Note: "free disk space is below the requirement"},
			},
		},
	}
	out := VerdictHuman(gv, true)
	if !strings.Contains(out, ansiRed) {
		t.Errorf("expected ANSI red in coloured output: %q", out)
	}
	if !strings.Contains(out, "free disk space is below the requirement") {
		t.Errorf("plain text detail must survive colourisation: %q", out)
	}
}
