// SPDX-License-Identifier: MPL-2.0

//go:build windows

package hwid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// wmiVideoController mirrors the WMI Win32_VideoController fields rigfit
// needs to enumerate GPUs and read the PCI hardware ID out of
// PNPDeviceID. It deliberately has no AdapterRAM field: that WMI property
// is a uint32 and silently overflows above 4 GiB (§4.5), so it must never
// be read as a VRAM source. See TestWMIStructNeverExposesAdapterRAM.
type wmiVideoController struct {
	PNPDeviceID   string
	DriverVersion string
}

// platformGPUDevices implements §4.4 for Windows.
func platformGPUDevices() ([]rawGPUDevice, error) {
	controllers, err := queryVideoControllers()
	if err != nil {
		return nil, err
	}
	out := make([]rawGPUDevice, 0, len(controllers))
	for _, c := range controllers {
		if r, ok := buildRawGPUDevice(c); ok {
			out = append(out, r)
		}
	}
	return out, nil
}

// buildRawGPUDevice turns one WMI record into a rawGPUDevice, sourcing
// VRAM from the registry (readVRAMFromRegistry / §4.5) rather than from
// WMI. It is split out from platformGPUDevices so it can be unit tested
// without shelling out to PowerShell.
func buildRawGPUDevice(c wmiVideoController) (rawGPUDevice, bool) {
	vendorID, deviceID, subsystemID, ok := parsePNPDeviceID(c.PNPDeviceID)
	if !ok {
		return rawGPUDevice{}, false
	}
	return rawGPUDevice{
		VendorID:    vendorID,
		DeviceID:    deviceID,
		SubsystemID: subsystemID,
		DriverVer:   c.DriverVersion,
		VRAMBytes:   vramReader(c.PNPDeviceID),
	}, true
}

// queryVideoControllers enumerates Win32_VideoController via
// Get-CimInstance, the modern WMI client shipped with Windows. rigfit
// shells out to PowerShell rather than linking a COM/WMI client library,
// since none is in the approved dependency list (§2) and PowerShell is
// present and usable unprivileged on every supported Windows version.
func queryVideoControllers() ([]wmiVideoController, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		"Get-CimInstance Win32_VideoController | Select-Object PNPDeviceID,DriverVersion | ConvertTo-Json -Compress")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("hwid: querying Win32_VideoController: %w", err)
	}

	trimmed := bytes.TrimSpace(stdout.Bytes())
	if len(trimmed) == 0 {
		return nil, nil
	}
	// Get-CimInstance | ConvertTo-Json returns a single object, not an
	// array, when only one controller is present.
	if trimmed[0] == '{' {
		trimmed = append(append([]byte{'['}, trimmed...), ']')
	}

	var controllers []wmiVideoController
	if err := json.Unmarshal(trimmed, &controllers); err != nil {
		return nil, fmt.Errorf("hwid: parsing Win32_VideoController JSON: %w", err)
	}
	return controllers, nil
}

var pnpDeviceIDPattern = regexp.MustCompile(`(?i)VEN_([0-9A-F]{4})&DEV_([0-9A-F]{4})(?:&SUBSYS_([0-9A-F]{8}))?`)

// parsePNPDeviceID extracts vendor/device/subsystem IDs from a Windows PCI
// hardware ID, format PCI\VEN_10DE&DEV_2206&SUBSYS_14583FC8&REV_A1 (§4.4).
// The SUBSYS field is 8 hex digits: subsystem device ID (first 4) then
// subsystem vendor ID (last 4) — the reverse order of VEN_/DEV_ — which
// this repacks into the same (vendor<<16 | device) layout GPU.SubsystemID
// uses everywhere else.
func parsePNPDeviceID(id string) (vendorID, deviceID uint16, subsystemID uint32, ok bool) {
	m := pnpDeviceIDPattern.FindStringSubmatch(id)
	if m == nil {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(m[1], 16, 16)
	if err != nil {
		return 0, 0, 0, false
	}
	d, err := strconv.ParseUint(m[2], 16, 16)
	if err != nil {
		return 0, 0, 0, false
	}
	var sub uint32
	if m[3] != "" {
		if s, err := strconv.ParseUint(m[3], 16, 32); err == nil {
			subDevice := uint16(s >> 16)
			subVendor := uint16(s)
			sub = uint32(subVendor)<<16 | uint32(subDevice)
		}
	}
	return uint16(v), uint16(d), sub, true
}

// vramReader is overridden in tests so the §4.5 regression test can assert
// end-to-end behaviour without writing to the real (admin-protected)
// registry hive.
var vramReader = readVRAMFromRegistry

const displayClassGUID = `{4d36e968-e325-11ce-bfc1-08002be10318}`

// qwMemorySizeValueName is the REG_QWORD value holding accurate VRAM size
// under a display adapter's registry subkey. Verified against a live
// machine: the same subkey also carries a legacy "HardwareInformation.
// MemorySize" DWORD that under-reports (it reserves some memory and is
// capped at 32 bits), while "HardwareInformation.qwMemorySize" reports the
// true size. rigfit-BUILD-SPEC.md §4.5 names the value "qwMemorySize"
// without the "HardwareInformation." prefix that Windows actually uses;
// this is the corrected, hardware-verified name.
const qwMemorySizeValueName = "HardwareInformation.qwMemorySize"

// readVRAMFromRegistry implements §4.5: Win32_VideoController.AdapterRAM
// is a uint32 and silently wraps above 4 GiB, so a 12 GB card reports
// nonsense there. The correct source is the REG_QWORD qwMemorySizeValueName
// value under the display adapter's driver registry subkey
// (HKLM\SYSTEM\CurrentControlSet\Control\Class\{4d36e968-...}\<NNNN>),
// found by enumerating the numbered subkeys and matching MatchingDeviceId
// against the PCI hardware ID already resolved from WMI. Returns 0 (not
// determined) if no match is found, rather than falling back to WMI.
func readVRAMFromRegistry(pnpDeviceID string) uint64 {
	base := `SYSTEM\CurrentControlSet\Control\Class\` + displayClassGUID
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, base, registry.READ)
	if err != nil {
		return 0
	}
	defer func() { _ = root.Close() }()

	names, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return 0
	}

	want := normalizeHardwareID(pnpDeviceID)
	for _, name := range names {
		// Adapter subkeys are 4-digit indices ("0000", "0001", ...);
		// everything else under this class GUID (e.g. "Properties") is not
		// an adapter and has no MatchingDeviceId/HardwareInformation.qwMemorySize.
		if len(name) != 4 {
			continue
		}
		sub, err := registry.OpenKey(root, name, registry.READ)
		if err != nil {
			continue
		}
		matchingID, _, err := sub.GetStringValue("MatchingDeviceId")
		if err == nil && strings.EqualFold(normalizeHardwareID(matchingID), want) {
			vram, _, err := sub.GetIntegerValue(qwMemorySizeValueName)
			_ = sub.Close()
			if err == nil {
				return vram
			}
			return 0
		}
		_ = sub.Close()
	}
	return 0
}

// normalizeHardwareID reduces a PCI hardware ID to VEN_xxxx&DEV_xxxx so
// PNPDeviceID (which may carry &SUBSYS_/&REV_ suffixes) can be compared
// against MatchingDeviceId (which sometimes doesn't).
func normalizeHardwareID(id string) string {
	m := pnpDeviceIDPattern.FindStringSubmatch(id)
	if m == nil {
		return strings.ToUpper(id)
	}
	return fmt.Sprintf("VEN_%s&DEV_%s", strings.ToUpper(m[1]), strings.ToUpper(m[2]))
}
