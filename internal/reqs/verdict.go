// SPDX-License-Identifier: GPL-3.0-only

package reqs

import "github.com/sayjavajava/rigfit/internal/score"

// ToRequirement converts a Tier into internal/score's decoupled
// Requirement shape (see score.Requirement's doc comment for why score
// can't just import this package's Tier type directly: score is itself a
// dependency of this package's matcher, so the reverse import would
// cycle). A nil Tier (e.g. a game with no recommended tier published)
// converts to a zero Requirement, which every hard/soft comparison in
// internal/score reads as "requirement not recognised" -> Unknown.
func (t *Tier) ToRequirement() score.Requirement {
	if t == nil {
		return score.Requirement{}
	}

	req := score.Requirement{
		RAMBytes:     t.RAMBytes,
		StorageBytes: t.StorageBytes,
	}
	for _, os := range t.OS {
		req.OS = append(req.OS, score.OSRequirement{Family: os.Family, MinVersion: os.MinVersion})
	}
	if t.CPU != nil {
		req.CPUAnyOf = t.CPU.AnyOf
	}
	if t.GPU != nil {
		req.GPUAnyOf = t.GPU.AnyOf
		req.GPUVRAMBytes = t.GPU.VRAMBytes
	}
	return req
}
