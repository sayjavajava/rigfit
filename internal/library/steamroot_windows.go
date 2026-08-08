// SPDX-License-Identifier: GPL-3.0-only

//go:build windows

package library

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// FindSteamRoot locates the Steam installation directory via the
// registry key Steam itself writes on install. Verified live: the real
// value uses forward slashes even on Windows (e.g.
// "c:/program files (x86)/steam") — a known Steam quirk — so it's
// normalised with filepath.FromSlash before use.
func FindSteamRoot() (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.READ)
	if err != nil {
		return "", fmt.Errorf("library: Steam does not appear to be installed (opening registry key: %w)", err)
	}
	defer func() { _ = key.Close() }()

	path, _, err := key.GetStringValue("SteamPath")
	if err != nil {
		return "", fmt.Errorf("library: reading SteamPath from registry: %w", err)
	}
	return filepath.Clean(filepath.FromSlash(path)), nil
}
