// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package library

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindSteamRoot locates the Steam installation directory at macOS' single
// standard location.
func FindSteamRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, "Library", "Application Support", "Steam")
	if info, err := os.Stat(filepath.Join(root, "steamapps")); err != nil || !info.IsDir() {
		return "", fmt.Errorf("library: Steam installation not found at %s", root)
	}
	return root, nil
}
