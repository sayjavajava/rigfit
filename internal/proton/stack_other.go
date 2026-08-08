// SPDX-License-Identifier: GPL-3.0-only

//go:build !linux

package proton

import "errors"

// LinuxStack is the graphics/compatibility stack detail §8 point 3 asks
// for. This platform's build carries only the type (so internal/proton
// compiles into the cross-platform rigfit binary, §12.2's build matrix)
// — DetectStack itself is a hard error here, since Proton/Wine
// compatibility has no meaning outside Linux and callers are expected to
// gate on runtime.GOOS == "linux" before ever calling it.
type LinuxStack struct {
	MesaVersion         string `json:"mesa_version,omitempty"`
	DriverBackend       string `json:"driver_backend,omitempty"`
	NVIDIADriverVersion string `json:"nvidia_driver_version,omitempty"`
	VulkanAPIVersion    string `json:"vulkan_api_version,omitempty"`
	SessionType         string `json:"session_type,omitempty"`
	IsSteamDeck         bool   `json:"is_steam_deck"`
}

// DetectStack always errors on non-Linux platforms; see the package doc
// comment on why this package (anti-cheat gate, ProtonDB) is Linux-only
// in practice even though it must compile everywhere.
func DetectStack() (LinuxStack, error) {
	return LinuxStack{}, errors.New("proton: stack detection is only implemented for linux")
}
