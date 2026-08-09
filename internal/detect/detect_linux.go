// SPDX-License-Identifier: GPL-3.0-only

//go:build linux

package detect

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// platformRAMBytes reads total physical memory from /proc/meminfo's
// MemTotal line, which is reported in kibibytes.
func platformRAMBytes() (uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("detect: opening /proc/meminfo: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		kib, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("detect: parsing MemTotal: %w", err)
		}
		return kib * 1024, nil
	}
	return 0, fmt.Errorf("detect: MemTotal not found in /proc/meminfo")
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

// platformOSInfo parses /etc/os-release (the systemd/freedesktop.org
// standard every mainstream distro ships) for distro name and version.
func platformOSInfo() (OSInfo, error) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return OSInfo{}, fmt.Errorf("detect: opening /etc/os-release: %w", err)
	}
	defer func() { _ = f.Close() }()

	fields := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[key] = strings.Trim(value, `"`)
	}

	name := fields["PRETTY_NAME"]
	if name == "" {
		name = fields["NAME"]
	}
	return OSInfo{
		Family:  "linux",
		Name:    name,
		Version: fields["VERSION_ID"],
	}, nil
}
