// SPDX-License-Identifier: MPL-2.0

package hwid

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestEmbeddedPCIIDsMatchesVendoredCopy guards against pkg/hwid/pci.ids
// (the go:embed source) drifting from data/pci.ids (the canonical,
// licence-documented copy). go:embed cannot reach outside the package
// directory, so the two are kept in sync manually; this test makes that
// drift a CI failure instead of a silent bug.
func TestEmbeddedPCIIDsMatchesVendoredCopy(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "data", "pci.ids"))
	if err != nil {
		t.Fatalf("reading canonical data/pci.ids: %v", err)
	}
	if !bytes.Equal(canonical, rawPCIIDs) {
		t.Fatal("pkg/hwid/pci.ids is out of sync with data/pci.ids; copy data/pci.ids over pkg/hwid/pci.ids")
	}
}

// TestEmbeddedCPUDBMatchesVendoredCopy is the same guard for cpudb.json.
func TestEmbeddedCPUDBMatchesVendoredCopy(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "data", "cpudb.json"))
	if err != nil {
		t.Fatalf("reading canonical data/cpudb.json: %v", err)
	}
	if !bytes.Equal(canonical, rawCPUDB) {
		t.Fatal("pkg/hwid/cpudb.json is out of sync with data/cpudb.json; copy data/cpudb.json over pkg/hwid/cpudb.json")
	}
}
