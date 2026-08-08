// SPDX-License-Identifier: MPL-2.0

//go:build darwin

package hwid

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
)

// macOS is best-effort (rigfit-BUILD-SPEC.md §1): system_profiler's JSON
// keys for GPU vendor/device ID are undocumented and have changed across
// macOS releases, so this parses the keys known to be present on current
// Apple Silicon and Intel Macs and returns nothing for a display entry it
// cannot confidently parse, rather than guessing.
type spDisplaysReport struct {
	SPDisplaysDataType []spGPUEntry `json:"SPDisplaysDataType"`
}

type spGPUEntry struct {
	VendorID string `json:"spdisplays_vendor-id"`
	DeviceID string `json:"spdisplays_device-id"`
}

// platformGPUDevices implements §4.4 for macOS via `system_profiler
// SPDisplaysDataType -json`.
func platformGPUDevices() ([]rawGPUDevice, error) {
	cmd := exec.Command("system_profiler", "SPDisplaysDataType", "-json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var report spDisplaysReport
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &report); err != nil {
		return nil, err
	}

	out := make([]rawGPUDevice, 0, len(report.SPDisplaysDataType))
	for _, e := range report.SPDisplaysDataType {
		vendorID, ok := parseHexPrefixed(e.VendorID)
		if !ok {
			continue
		}
		deviceID, ok := parseHexPrefixed(e.DeviceID)
		if !ok {
			continue
		}
		out = append(out, rawGPUDevice{
			VendorID: vendorID,
			DeviceID: deviceID,
		})
	}
	return out, nil
}

func parseHexPrefixed(s string) (uint16, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, false
	}
	return uint16(v), true
}
