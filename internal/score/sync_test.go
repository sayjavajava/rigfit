// SPDX-License-Identifier: GPL-3.0-only

package score

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// These guard against internal/score's embedded copies drifting from the
// canonical data/ files, the same way pkg/hwid/sync_test.go does — see
// the rawGPUDB comment in gpudb.go for why go:embed can't reach data/
// directly.

func TestEmbeddedGPUDBMatchesVendoredCopy(t *testing.T) {
	assertSynced(t, "gpudb.json", rawGPUDB)
}

func TestEmbeddedCPUDBMatchesVendoredCopy(t *testing.T) {
	assertSynced(t, "cpudb.json", rawCPUDB)
}

func TestEmbeddedCoefficientsMatchesVendoredCopy(t *testing.T) {
	assertSynced(t, "coefficients.json", rawCoefficients)
}

func assertSynced(t *testing.T, filename string, embedded []byte) {
	t.Helper()
	canonical, err := os.ReadFile(filepath.Join("..", "..", "data", filename))
	if err != nil {
		t.Fatalf("reading canonical data/%s: %v", filename, err)
	}
	if !bytes.Equal(canonical, embedded) {
		t.Fatalf("internal/score/%s is out of sync with data/%s; copy data/%s over internal/score/%s", filename, filename, filename, filename)
	}
}
