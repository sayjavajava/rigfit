// SPDX-License-Identifier: GPL-3.0-only

package report

import (
	"strings"
	"testing"

	"github.com/sayjavajava/rigfit/internal/score"
)

func TestNewLibraryReportPicksMostCommonBottleneck(t *testing.T) {
	results := []LibraryTitleResult{
		{AppID: 1, Name: "A", Overall: score.StatusFail, Bottleneck: "gpu"},
		{AppID: 2, Name: "B", Overall: score.StatusFail, Bottleneck: "gpu"},
		{AppID: 3, Name: "C", Overall: score.StatusMarginal, Bottleneck: "gpu"},
		{AppID: 4, Name: "D", Overall: score.StatusFail, Bottleneck: "ram"},
		{AppID: 5, Name: "E", Overall: score.StatusPass, Bottleneck: ""},
		{AppID: 6, Name: "F", Overall: score.StatusUnknown, Bottleneck: "cpu"},
	}

	r := NewLibraryReport(10, results)
	if r.Scanned != 10 || r.Evaluated != 6 {
		t.Errorf("Scanned=%d Evaluated=%d", r.Scanned, r.Evaluated)
	}
	if len(r.Underperforming) != 4 {
		t.Fatalf("Underperforming = %+v, want 4 entries (Pass and Unknown excluded)", r.Underperforming)
	}
	if r.TopUpgrade != "gpu" || r.TopUpgradeCount != 3 {
		t.Errorf("TopUpgrade=%q TopUpgradeCount=%d, want gpu/3", r.TopUpgrade, r.TopUpgradeCount)
	}
}

func TestNewLibraryReportSortsFailBeforeMarginal(t *testing.T) {
	results := []LibraryTitleResult{
		{AppID: 1, Name: "ZMarginal", Overall: score.StatusMarginal, Bottleneck: "cpu"},
		{AppID: 2, Name: "AFail", Overall: score.StatusFail, Bottleneck: "gpu"},
	}
	r := NewLibraryReport(2, results)
	if len(r.Underperforming) != 2 || r.Underperforming[0].Name != "AFail" {
		t.Errorf("Underperforming = %+v, want Fail (AFail) first", r.Underperforming)
	}
}

func TestNewLibraryReportNoUnderperformingTitles(t *testing.T) {
	results := []LibraryTitleResult{{AppID: 1, Name: "A", Overall: score.StatusPass}}
	r := NewLibraryReport(1, results)
	if len(r.Underperforming) != 0 || r.TopUpgrade != "" {
		t.Errorf("got %+v, want no underperforming titles and no upgrade recommendation", r)
	}
	out := LibraryHuman(r, false)
	if !strings.Contains(out, "Nothing underperforms") {
		t.Errorf("output = %q", out)
	}
}

func TestLibraryHumanShowsSkippedCount(t *testing.T) {
	r := NewLibraryReport(5, []LibraryTitleResult{{AppID: 1, Name: "A", Overall: score.StatusPass}})
	out := LibraryHuman(r, false)
	if !strings.Contains(out, "4 had no usable requirements data") {
		t.Errorf("output = %q", out)
	}
}
