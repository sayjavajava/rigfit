// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package detect

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// macOS is best-effort (rigfit-BUILD-SPEC.md §1). Total RAM and OS version
// are read by shelling out to sysctl/sw_vers, the same pragmatic approach
// gpu_darwin.go takes for GPU enumeration, rather than adding a cgo
// dependency (disallowed, §2) to call sysctlbyname directly.
func platformRAMBytes() (uint64, error) {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, fmt.Errorf("detect: sysctl hw.memsize: %w", err)
	}
	return strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
}

func platformStorage(path string) (StorageInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return StorageInfo{}, fmt.Errorf("detect: statfs %s: %w", path, err)
	}
	blockSize := uint64(stat.Bsize)
	return StorageInfo{
		Path:       path,
		TotalBytes: stat.Blocks * blockSize,
		FreeBytes:  stat.Bavail * blockSize,
	}, nil
}

func platformOSInfo() (OSInfo, error) {
	version, err := runSwVers("-productVersion")
	if err != nil {
		return OSInfo{}, err
	}
	name, err := runSwVers("-productName")
	if err != nil {
		return OSInfo{}, err
	}
	return OSInfo{
		Family:  "darwin",
		Name:    strings.TrimSpace(name),
		Version: strings.TrimSpace(version),
	}, nil
}

func runSwVers(flag string) (string, error) {
	var stdout bytes.Buffer
	cmd := exec.Command("sw_vers", flag)
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("detect: sw_vers %s: %w", flag, err)
	}
	return stdout.String(), nil
}
