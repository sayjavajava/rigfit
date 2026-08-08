// SPDX-License-Identifier: GPL-3.0-only

package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// InstalledApp is one game found in a steamapps/appmanifest_<appid>.acf.
type InstalledApp struct {
	AppID       int
	Name        string
	LibraryPath string // which library folder (steamapps' parent) it's in
}

// Scan finds the Steam installation, enumerates every library folder via
// libraryfolders.vdf, and returns every installed app found across all of
// them. Every file it touches is one Steam itself wrote, under a
// directory Steam itself reported (the registry key on Windows, or the
// paths libraryfolders.vdf names) — §11's "Library scan constraints":
// read-only, scoped strictly to Steam directories, never a walk of the
// wider filesystem.
func Scan() ([]InstalledApp, error) {
	root, err := FindSteamRoot()
	if err != nil {
		return nil, err
	}

	folders, err := libraryFolders(root)
	if err != nil {
		return nil, err
	}

	var apps []InstalledApp
	for _, folder := range folders {
		found, err := scanLibraryFolder(folder)
		if err != nil {
			// One missing/corrupt library folder (e.g. an external drive
			// that's since been unplugged, still listed in
			// libraryfolders.vdf) shouldn't fail the whole scan.
			continue
		}
		apps = append(apps, found...)
	}
	return apps, nil
}

// libraryFolders parses steamRoot/steamapps/libraryfolders.vdf, returning
// every library path it lists — including steamRoot itself, which Steam
// always lists as library "0" alongside any additional drives the user
// added (verified against a real installation: entry "0" is the Steam
// install directory itself).
func libraryFolders(steamRoot string) ([]string, error) {
	path := filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("library: reading %s: %w", path, err)
	}
	root, rootKey, err := parseVDF(data)
	if err != nil {
		return nil, fmt.Errorf("library: parsing %s: %w", path, err)
	}
	if rootKey != "libraryfolders" {
		return nil, fmt.Errorf("library: %s: expected root key \"libraryfolders\", got %q", path, rootKey)
	}

	var out []string
	for _, entry := range root.Children {
		if p, ok := entry.str("path"); ok {
			out = append(out, filepath.Clean(p))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("library: %s named no library folders", path)
	}
	return out, nil
}

// scanLibraryFolder parses every appmanifest_*.acf directly inside
// libraryPath/steamapps — a flat glob, not a recursive walk.
func scanLibraryFolder(libraryPath string) ([]InstalledApp, error) {
	steamapps := filepath.Join(libraryPath, "steamapps")
	matches, err := filepath.Glob(filepath.Join(steamapps, "appmanifest_*.acf"))
	if err != nil {
		return nil, err
	}

	var apps []InstalledApp
	for _, path := range matches {
		app, ok := parseAppManifest(path)
		if !ok {
			continue // a malformed manifest is skipped, not fatal to the scan
		}
		app.LibraryPath = libraryPath
		apps = append(apps, app)
	}
	return apps, nil
}

func parseAppManifest(path string) (InstalledApp, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return InstalledApp{}, false
	}
	root, rootKey, err := parseVDF(data)
	if err != nil || rootKey != "AppState" {
		return InstalledApp{}, false
	}
	appIDStr, ok := root.str("appid")
	if !ok {
		return InstalledApp{}, false
	}
	appID, err := strconv.Atoi(strings.TrimSpace(appIDStr))
	if err != nil {
		return InstalledApp{}, false
	}
	name, _ := root.str("name")
	return InstalledApp{AppID: appID, Name: name}, true
}
