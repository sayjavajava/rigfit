// SPDX-License-Identifier: GPL-3.0-only

package library

import "testing"

// These fixtures are modeled on real libraryfolders.vdf/appmanifest_*.acf
// structure captured from a real Steam installation during development
// (field names, nesting, and the Windows-path backslash-escaping
// convention are all real) — but with synthetic path/ID values, since the
// real file contained an actual Steam64 account ID that has no business
// being committed to a public test fixture.

const sampleLibraryFoldersVDF = `"libraryfolders"
{
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
		"label"		""
		"contentid"		"1111111111111111111"
		"totalsize"		"0"
		"apps"
		{
			"228980"		"0"
		}
	}
	"1"
	{
		"path"		"D:\\Games\\SteamLibrary"
		"label"		"Data"
		"contentid"		"2222222222222222222"
		"totalsize"		"1000187359232"
		"apps"
		{
			"227300"		"40442281076"
			"270880"		"25780909444"
		}
	}
}
`

const sampleAppManifestACF = `"AppState"
{
	"appid"		"227300"
	"Universe"		"1"
	"name"		"Euro Truck Simulator 2"
	"StateFlags"		"4"
	"installdir"		"Euro Truck Simulator 2"
	"LastUpdated"		"1785682649"
	"SizeOnDisk"		"40442281076"
	"UserConfig"
	{
		"BetaKey"		"public"
	}
}
`

func TestParseVDFLibraryFolders(t *testing.T) {
	root, rootKey, err := parseVDF([]byte(sampleLibraryFoldersVDF))
	if err != nil {
		t.Fatal(err)
	}
	if rootKey != "libraryfolders" {
		t.Fatalf("rootKey = %q, want libraryfolders", rootKey)
	}

	lib0, ok := root.obj("0")
	if !ok {
		t.Fatal("expected library folder \"0\"")
	}
	path, ok := lib0.str("path")
	if !ok || path != `C:\Program Files (x86)\Steam` {
		t.Errorf("path = %q, ok=%v — backslash escapes must decode correctly", path, ok)
	}

	lib1, ok := root.obj("1")
	if !ok {
		t.Fatal("expected library folder \"1\"")
	}
	apps, ok := lib1.obj("apps")
	if !ok {
		t.Fatal("expected an apps object under library folder 1")
	}
	if len(apps.Children) != 2 {
		t.Errorf("got %d apps, want 2", len(apps.Children))
	}
	if _, ok := apps.Children["227300"]; !ok {
		t.Error("expected appid 227300 under library folder 1")
	}
}

func TestParseVDFAppManifest(t *testing.T) {
	root, rootKey, err := parseVDF([]byte(sampleAppManifestACF))
	if err != nil {
		t.Fatal(err)
	}
	if rootKey != "AppState" {
		t.Fatalf("rootKey = %q, want AppState", rootKey)
	}
	name, ok := root.str("name")
	if !ok || name != "Euro Truck Simulator 2" {
		t.Errorf("name = %q, ok=%v", name, ok)
	}
	appID, ok := root.str("appid")
	if !ok || appID != "227300" {
		t.Errorf("appid = %q, ok=%v", appID, ok)
	}
}

func TestParseVDFMalformedInput(t *testing.T) {
	cases := []string{
		``,
		`"unterminated`,
		`"key" {`,
		`"key" { "a" }`, // "}" read as a value token — should error, not panic
	}
	for _, c := range cases {
		if _, _, err := parseVDF([]byte(c)); err == nil {
			t.Errorf("parseVDF(%q) succeeded, want an error", c)
		}
	}
}
