// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"testing"

	"github.com/sayjavajava/rigfit/internal/reqs"
)

func TestValidateAcceptsKnownIDs(t *testing.T) {
	tier := &reqs.Tier{
		CPU: &reqs.PartSet{AnyOf: []string{"intel-core-i5-8400"}},
		GPU: &reqs.PartSet{AnyOf: []string{"nvidia-geforce-gtx-1060-6gb"}},
	}
	if got := validate(tier); len(got) != 0 {
		t.Errorf("validate() = %v, want no invalid names for known canonical IDs", got)
	}
}

// TestValidateRejectsHallucinatedID is §6.4's core requirement: a
// plausible-looking but nonexistent part name (the kind an LLM-backed
// normalizer could hallucinate) must be caught, not silently merged.
func TestValidateRejectsHallucinatedID(t *testing.T) {
	tier := &reqs.Tier{
		GPU: &reqs.PartSet{AnyOf: []string{"nvidia-geforce-rtx-3080-super-ti-max"}},
	}
	got := validate(tier)
	if len(got) != 1 || got[0] != "gpu:nvidia-geforce-rtx-3080-super-ti-max" {
		t.Errorf("validate() = %v, want exactly one flagged gpu entry", got)
	}
}

func TestValidateNilTier(t *testing.T) {
	if got := validate(nil); got != nil {
		t.Errorf("validate(nil) = %v, want nil", got)
	}
}
