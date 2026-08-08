// SPDX-License-Identifier: GPL-3.0-only

package library

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSteamapps builds a synthetic <root>/steamapps/ directory with the
// given libraryfolders.vdf content (if any) and appmanifest files.
func writeSteamapps(t *testing.T, root string, libraryFoldersVDF string, manifests map[string]string) {
	t.Helper()
	steamapps := filepath.Join(root, "steamapps")
	if err := os.MkdirAll(steamapps, 0o755); err != nil {
		t.Fatal(err)
	}
	if libraryFoldersVDF != "" {
		if err := os.WriteFile(filepath.Join(steamapps, "libraryfolders.vdf"), []byte(libraryFoldersVDF), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for filename, content := range manifests {
		if err := os.WriteFile(filepath.Join(steamapps, filename), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLibraryFoldersParsesRealShape(t *testing.T) {
	root := t.TempDir()
	writeSteamapps(t, root, sampleLibraryFoldersVDF, nil)

	got, err := libraryFolders(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		filepath.Clean(`C:\Program Files (x86)\Steam`): true,
		filepath.Clean(`D:\Games\SteamLibrary`):        true,
	}
	if len(got) != 2 {
		t.Fatalf("got %d folders, want 2: %v", len(got), got)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected folder %q", p)
		}
	}
}

func TestLibraryFoldersMissingFile(t *testing.T) {
	if _, err := libraryFolders(t.TempDir()); err == nil {
		t.Fatal("expected an error when libraryfolders.vdf doesn't exist")
	}
}

func TestScanLibraryFolderFindsInstalledApps(t *testing.T) {
	root := t.TempDir()
	writeSteamapps(t, root, "", map[string]string{
		"appmanifest_227300.acf": sampleAppManifestACF,
		"appmanifest_270880.acf": `"AppState"
{
	"appid"		"270880"
	"name"		"American Truck Simulator"
}
`,
		"not_a_manifest.txt": "should be ignored — not matched by the appmanifest_*.acf glob",
	})

	apps, err := scanLibraryFolder(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2: %+v", len(apps), apps)
	}

	byID := map[int]InstalledApp{}
	for _, a := range apps {
		byID[a.AppID] = a
	}
	if byID[227300].Name != "Euro Truck Simulator 2" {
		t.Errorf("227300 name = %q", byID[227300].Name)
	}
	if byID[270880].Name != "American Truck Simulator" {
		t.Errorf("270880 name = %q", byID[270880].Name)
	}
	if byID[227300].LibraryPath != root {
		t.Errorf("LibraryPath = %q, want %q", byID[227300].LibraryPath, root)
	}
}

func TestScanLibraryFolderIgnoresMalformedManifest(t *testing.T) {
	root := t.TempDir()
	writeSteamapps(t, root, "", map[string]string{
		"appmanifest_1.acf": `not valid vdf at all {{{`,
		"appmanifest_2.acf": sampleAppManifestACF,
	})

	apps, err := scanLibraryFolder(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 {
		t.Fatalf("got %d apps, want 1 (malformed manifest skipped, not fatal): %+v", len(apps), apps)
	}
}

func TestScanLibraryFolderDoesNotWalkSubdirectories(t *testing.T) {
	root := t.TempDir()
	writeSteamapps(t, root, "", nil)
	// A manifest-shaped file nested one level deeper must NOT be picked
	// up — the glob is intentionally flat (§11: "MUST NOT walk the wider
	// filesystem").
	nested := filepath.Join(root, "steamapps", "common", "SomeGame")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "appmanifest_999.acf"), []byte(sampleAppManifestACF), 0o644); err != nil {
		t.Fatal(err)
	}

	apps, err := scanLibraryFolder(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 0 {
		t.Errorf("got %d apps, want 0 — nested manifest must not be found by a flat scan", len(apps))
	}
}
