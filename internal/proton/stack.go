// SPDX-License-Identifier: GPL-3.0-only

//go:build linux

package proton

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// LinuxStack is the graphics/compatibility stack detail §8 point 3 asks
// for. Every field is best-effort: a missing tool (glxinfo, vulkaninfo)
// or unreadable sysfs path leaves that field empty rather than erroring
// the whole detection — this is diagnostic detail for the report, not a
// hard-gate input the verdict depends on.
//
// Unlike pkg/hwid and internal/detect's Windows code (which this session
// verified against real hardware and fixed two real bugs from doing so),
// this file could only be cross-compile-checked, never run on real Linux
// — there is no Linux machine available in this environment. Treat the
// exact output-parsing regexes as a documented best effort, not something
// verified against live tool output the way the rest of this project is.
type LinuxStack struct {
	MesaVersion         string `json:"mesa_version,omitempty"`
	DriverBackend       string `json:"driver_backend,omitempty"`        // e.g. "radv", "amdvlk", "nvidia", "anv"
	NVIDIADriverVersion string `json:"nvidia_driver_version,omitempty"` // empty on non-NVIDIA systems
	VulkanAPIVersion    string `json:"vulkan_api_version,omitempty"`
	SessionType         string `json:"session_type,omitempty"` // "wayland" | "x11" | ""
	IsSteamDeck         bool   `json:"is_steam_deck"`
}

// DetectStack gathers the Linux graphics/compatibility stack (§8 point
// 3): Mesa version, NVIDIA driver version, Vulkan level, Wayland vs X11,
// and Steam Deck detection. The error return exists only so this
// signature matches stack_other.go's non-Linux stub (which always
// errors) — individual field detections here degrade to empty rather
// than failing the whole call, so this always returns a nil error.
func DetectStack() (LinuxStack, error) {
	mesaVersion, backend, vulkanAPIVersion := detectVulkanAndMesa()
	return LinuxStack{
		MesaVersion:         mesaVersion,
		DriverBackend:       backend,
		NVIDIADriverVersion: detectNVIDIADriverVersion(),
		VulkanAPIVersion:    vulkanAPIVersion,
		SessionType:         detectSessionType(),
		IsSteamDeck:         detectSteamDeck(),
	}, nil
}

// detectSessionType uses the same environment variables systemd-logind
// and every major desktop environment set, rather than shelling out —
// XDG_SESSION_TYPE is the standard, portable signal; WAYLAND_DISPLAY is
// checked first since it's authoritative when set (a session can carry a
// stale XDG_SESSION_TYPE=x11 under XWayland in some configurations).
func detectSessionType() string {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "wayland"
	}
	if v := os.Getenv("XDG_SESSION_TYPE"); v == "wayland" || v == "x11" {
		return v
	}
	if os.Getenv("DISPLAY") != "" {
		return "x11"
	}
	return ""
}

// Steam Deck's DMI product name: "Jupiter" for the LCD model, "Galileo"
// for the OLED model — the standard detection signal used across the
// Linux gaming tooling ecosystem (gamescope, various distro/compat
// layers). Not independently verified against real Steam Deck hardware
// in this session; documented in Valve/community tooling.
const (
	steamDeckProductJupiter = "Jupiter"
	steamDeckProductGalileo = "Galileo"
)

func detectSteamDeck() bool {
	name, err := readSysfsString("/sys/class/dmi/id/product_name")
	if err != nil {
		return false
	}
	return name == steamDeckProductJupiter || name == steamDeckProductGalileo
}

// detectNVIDIADriverVersion prefers the sysfs path the proprietary NVIDIA
// kernel module exposes (no subprocess needed); nvidia-smi is a fallback
// for setups where that path differs. Absent both, this is simply not an
// NVIDIA system and the field stays empty.
func detectNVIDIADriverVersion() string {
	if v, err := readSysfsString("/sys/module/nvidia/version"); err == nil && v != "" {
		return v
	}
	out, err := exec.Command("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

var mesaVersionPattern = regexp.MustCompile(`Mesa\s+([0-9]+\.[0-9]+(?:\.[0-9]+)?)`)

// detectVulkanAndMesa parses `vulkaninfo --summary`'s per-GPU block for
// the driver name (RADV/AMDVLK/NVIDIA/ANV — the actual backend Proton
// games render through via DXVK/VKD3D, per §8's own example output
// "(RADV)") and API version, and falls back to `glxinfo -B`'s "Mesa
// X.Y.Z" string purely for the Mesa release number when vulkaninfo isn't
// available or the driver isn't Mesa-based. Only the first GPU block is
// read — a reasonable simplification for single-GPU systems; a hybrid
// laptop's second GPU isn't distinguished here.
func detectVulkanAndMesa() (mesaVersion, backend, apiVersion string) {
	if out, err := exec.Command("vulkaninfo", "--summary").Output(); err == nil {
		text := string(out)
		apiVersion = firstSubmatch(vulkanAPIVersionPattern, text)
		backend = strings.ToLower(firstSubmatch(vulkanDriverNamePattern, text))
		mesaVersion = firstSubmatch(mesaVersionPattern, text)
	}
	if mesaVersion == "" {
		if out, err := exec.Command("glxinfo", "-B").Output(); err == nil {
			mesaVersion = firstSubmatch(mesaVersionPattern, string(out))
		}
	}
	return mesaVersion, backend, apiVersion
}

var (
	vulkanAPIVersionPattern = regexp.MustCompile(`apiVersion\s*=\s*([0-9]+\.[0-9]+\.[0-9]+)`)
	vulkanDriverNamePattern = regexp.MustCompile(`driverName\s*=\s*(\S+)`)
)

func firstSubmatch(re *regexp.Regexp, text string) string {
	m := re.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

func readSysfsString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
