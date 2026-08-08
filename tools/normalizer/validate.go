// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"github.com/sayjavajava/rigfit/internal/reqs"
	"github.com/sayjavajava/rigfit/internal/score"
)

// validate implements §6.4's mandatory validator: every CPU/GPU canonical
// ID a Tier names is checked against cpudb.json/gpudb.json (via
// internal/score's exported ID lists — the same source of truth
// internal/reqs' own runtime matcher uses). It returns every invalid name
// found, prefixed by which field it came from; an empty result means the
// tier is safe to merge into data/requirements/.
func validate(tier *reqs.Tier) []string {
	if tier == nil {
		return nil
	}

	var invalid []string
	if tier.CPU != nil {
		known := toSet(score.CPUCanonicalIDs())
		for _, id := range tier.CPU.AnyOf {
			if !known[id] {
				invalid = append(invalid, "cpu:"+id)
			}
		}
	}
	if tier.GPU != nil {
		known := toSet(score.GPUCanonicalIDs())
		for _, id := range tier.GPU.AnyOf {
			if !known[id] {
				invalid = append(invalid, "gpu:"+id)
			}
		}
	}
	return invalid
}

func toSet(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}
