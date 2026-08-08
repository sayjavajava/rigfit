// SPDX-License-Identifier: GPL-3.0-only

//go:build generate_golden

package score

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestGenerateGoldenTiers (re)writes testdata/golden_tiers.json from the
// engine's current output. It is build-tag gated (generate_golden) so it
// never runs as part of the normal test suite — the whole point of the
// drift guard is that testdata/golden_tiers.json does NOT silently update
// itself. Run deliberately after an intentional coefficients.json change:
//
//	go test ./internal/score/... -tags generate_golden -run TestGenerateGoldenTiers -v
func TestGenerateGoldenTiers(t *testing.T) {
	b := NewBuiltin()

	type record struct {
		CanonicalID string `json:"canonical_id"`
		MarketName  string `json:"market_name"`
		Tier        int    `json:"tier"`
	}

	var gpuTiers []record
	for _, e := range getGPUDB() {
		score, _, err := b.GPUScore(e.CanonicalID)
		if err != nil {
			t.Fatal(err)
		}
		gpuTiers = append(gpuTiers, record{e.CanonicalID, e.MarketName, score.Tier})
	}
	sort.Slice(gpuTiers, func(i, j int) bool { return gpuTiers[i].CanonicalID < gpuTiers[j].CanonicalID })

	var cpuTiers []record
	for _, e := range getCPUDB() {
		score, _, err := b.CPUScore(e.CanonicalID)
		if err != nil {
			t.Fatal(err)
		}
		cpuTiers = append(cpuTiers, record{e.CanonicalID, e.MarketName, score.Tier})
	}
	sort.Slice(cpuTiers, func(i, j int) bool { return cpuTiers[i].CanonicalID < cpuTiers[j].CanonicalID })

	out := struct {
		GPUs []record `json:"gpus"`
		CPUs []record `json:"cpus"`
	}{gpuTiers, cpuTiers}

	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')

	path := filepath.Join("testdata", "golden_tiers.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}
