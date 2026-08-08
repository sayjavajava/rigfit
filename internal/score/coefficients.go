// SPDX-License-Identifier: GPL-3.0-only

package score

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// coefficients.json is a synced copy of data/coefficients.json — the
// architecture correction factors §5.3 requires to never be hardcoded.
// See sync_test.go for the drift guard against data/coefficients.json.
//
//go:embed coefficients.json
var rawCoefficients []byte

type archCoefficient struct {
	Coefficient float64  `json:"coefficient"`
	Note        string   `json:"note,omitempty"`
	SourceRefs  []string `json:"source_refs,omitempty"`
}

type coefficientsFile struct {
	SchemaVersion int                        `json:"schema_version"`
	GPUArch       map[string]archCoefficient `json:"gpu_arch"`
	CPUIPC        map[string]archCoefficient `json:"cpu_ipc"`
}

var (
	coefficientsOnce sync.Once
	coefficients     coefficientsFile
)

func getCoefficients() coefficientsFile {
	coefficientsOnce.Do(func() {
		if err := json.Unmarshal(rawCoefficients, &coefficients); err != nil {
			panic("score: embedded coefficients.json is invalid: " + err.Error())
		}
	})
	return coefficients
}

// gpuArchCoefficient returns an architecture's correction coefficient and
// whether it's defined in coefficients.json. An architecture named by a
// gpudb.json entry but missing from coefficients.json is a data
// consistency bug the data-validate CI job (§12.5) is meant to catch
// before merge; at runtime it surfaces as ok=false, which callers turn
// into ConfidenceUnknown rather than silently defaulting to 1.0.
func gpuArchCoefficient(arch string) (float64, bool) {
	c, ok := getCoefficients().GPUArch[arch]
	return c.Coefficient, ok
}

func cpuIPCCoefficient(arch string) (float64, bool) {
	c, ok := getCoefficients().CPUIPC[arch]
	return c.Coefficient, ok
}
