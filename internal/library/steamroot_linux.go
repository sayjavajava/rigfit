// SPDX-License-Identifier: GPL-3.0-only

//go:build linux

package library

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindSteamRoot locates the Steam installation directory. Linux has no
// single canonical install location (unlike Windows' registry key or
// macOS' one default path) — Steam's own installer, most distro
// packages, and Flatpak Steam each use a different default. Each
// candidate is checked in order for a steamapps subdirectory, the
// signal that it's a real Steam root and not just an empty/stale path.
func FindSteamRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	candidates := []string{
		filepath.Join(home, ".steam", "steam"),                                                    // Steam's own installer / most distro packages
		filepath.Join(home, ".local", "share", "Steam"),                                           // Steam's newer default
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"), // Flatpak
	}
	for _, c := range candidates {
		if info, err := os.Stat(filepath.Join(c, "steamapps")); err == nil && info.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("library: Steam installation not found in any known location")
}
