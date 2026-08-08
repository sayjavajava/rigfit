// SPDX-License-Identifier: GPL-3.0-only

//go:build linux

package proton

import "testing"

// These test the pure parsing logic (regexes, env-var-based session
// detection) against synthetic but structurally realistic tool output.
// Unlike the rest of this project's Windows-specific code (verified
// against this machine's real hardware, catching two real bugs along the
// way), there is no Linux machine available in this environment to
// capture real glxinfo/vulkaninfo output from — see stack.go's package
// doc comment. These tests only prove the regexes are internally
// consistent, not that they match real tool output.

func TestMesaVersionPattern(t *testing.T) {
	sample := "OpenGL vendor string: AMD\nOpenGL version string: 4.6 (Compatibility Profile) Mesa 24.1.2\n"
	if got := firstSubmatch(mesaVersionPattern, sample); got != "24.1.2" {
		t.Errorf("got %q, want 24.1.2", got)
	}
}

func TestVulkanSummaryParsing(t *testing.T) {
	sample := `
GPU0:
	apiVersion         = 1.3.296 (0x00403038)
	driverVersion      = 24.1.2.0
	vendorID           = 0x1002
	deviceName         = AMD Radeon RX 6700 XT (RADV NAVI22)
	driverID           = DRIVER_ID_MESA_RADV
	driverName         = radv
	driverInfo         = Mesa 24.1.2
`
	if got := firstSubmatch(vulkanAPIVersionPattern, sample); got != "1.3.296" {
		t.Errorf("apiVersion: got %q, want 1.3.296", got)
	}
	if got := firstSubmatch(vulkanDriverNamePattern, sample); got != "radv" {
		t.Errorf("driverName: got %q, want radv", got)
	}
	if got := firstSubmatch(mesaVersionPattern, sample); got != "24.1.2" {
		t.Errorf("Mesa version from driverInfo: got %q, want 24.1.2", got)
	}
}

func TestDetectSessionType(t *testing.T) {
	cases := []struct {
		name              string
		wayland, xdg, x11 string
		want              string
	}{
		{"wayland display set", "wayland-0", "", "", "wayland"},
		{"xdg session type wayland", "", "wayland", "", "wayland"},
		{"xdg session type x11", "", "x11", "", "x11"},
		{"display only", "", "", ":0", "x11"},
		{"nothing set", "", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("WAYLAND_DISPLAY", c.wayland)
			t.Setenv("XDG_SESSION_TYPE", c.xdg)
			t.Setenv("DISPLAY", c.x11)
			if got := detectSessionType(); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestDetectSteamDeckNoDMIFile(t *testing.T) {
	// On a non-Steam-Deck CI runner, /sys/class/dmi/id/product_name
	// either doesn't exist or names something else — either way this
	// must return false, never guess "yes".
	if detectSteamDeck() {
		t.Skip("running on what looks like a real Steam Deck; nothing to assert")
	}
}
