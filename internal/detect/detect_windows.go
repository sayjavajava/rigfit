// SPDX-License-Identifier: GPL-3.0-only

//go:build windows

package detect

import (
	"fmt"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// memoryStatusEx mirrors the Win32 MEMORYSTATUSEX struct. golang.org/x/sys/
// windows does not wrap GlobalMemoryStatusEx, so it's called directly via
// LazyDLL — still within the approved dependency (§2: "golang.org/x/sys/
// windows (registry, WMI)"), no new module and no cgo.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var (
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatus = kernel32.NewProc("GlobalMemoryStatusEx")
)

func platformRAMBytes() (uint64, error) {
	var stat memoryStatusEx
	stat.Length = uint32(unsafe.Sizeof(stat))
	r1, _, err := procGlobalMemoryStatus.Call(uintptr(unsafe.Pointer(&stat)))
	if r1 == 0 {
		return 0, fmt.Errorf("detect: GlobalMemoryStatusEx: %w", err)
	}
	return stat.TotalPhys, nil
}

func platformStorage(path string) (StorageInfo, error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return StorageInfo{}, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &free, &total, &totalFree); err != nil {
		return StorageInfo{}, fmt.Errorf("detect: GetDiskFreeSpaceEx: %w", err)
	}
	return StorageInfo{Path: path, TotalBytes: total, FreeBytes: free}, nil
}

const currentVersionKey = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`

// platformOSInfo reads Windows version info from the registry rather than
// shelling out to `ver`/`systeminfo`, which are slower and locale-dependent.
//
// ProductName is notoriously stale on many real Windows 11 installs: it
// still literally reads "Windows 10 ..." because Microsoft never updated
// the value for in-place-upgraded and even some fresh installs (verified
// on the development machine: ProductName said "Windows 10 Home Single
// Language" while CurrentBuildNumber was 26200 — Windows 11 territory).
// The reliable signal is the build number: 22000+ is Windows 11.
func platformOSInfo() (OSInfo, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, currentVersionKey, registry.READ)
	if err != nil {
		return OSInfo{}, fmt.Errorf("detect: opening CurrentVersion key: %w", err)
	}
	defer func() { _ = key.Close() }()

	productName, _, err := key.GetStringValue("ProductName")
	if err != nil {
		return OSInfo{}, fmt.Errorf("detect: reading ProductName: %w", err)
	}
	buildStr, _, err := key.GetStringValue("CurrentBuildNumber")
	if err != nil {
		return OSInfo{}, fmt.Errorf("detect: reading CurrentBuildNumber: %w", err)
	}
	build, err := strconv.ParseUint(buildStr, 10, 32)
	if err != nil {
		return OSInfo{}, fmt.Errorf("detect: parsing CurrentBuildNumber %q: %w", buildStr, err)
	}

	const windows11BuildFloor = 22000
	if build >= windows11BuildFloor {
		productName = correctWindows11ProductName(productName)
	}

	major, _, majErr := key.GetIntegerValue("CurrentMajorVersionNumber")
	minor, _, minErr := key.GetIntegerValue("CurrentMinorVersionNumber")
	if majErr != nil || minErr != nil {
		// Pre-Windows-10 systems don't have these; fall back to the legacy
		// major.minor pair encoded differently. rigfit's Windows support
		// targets 10+, so this is a best-effort fallback, not a hard error.
		major, minor = 10, 0
	}
	ubr, _, ubrErr := key.GetIntegerValue("UBR")
	version := fmt.Sprintf("%d.%d.%d", major, minor, build)
	if ubrErr == nil {
		version = fmt.Sprintf("%s.%d", version, ubr)
	}

	return OSInfo{
		Family:  "windows",
		Name:    productName,
		Version: version,
	}, nil
}

// correctWindows11ProductName rewrites a stale "Windows 10 ..." product
// name to "Windows 11 ..." when the build number proves it's actually 11.
func correctWindows11ProductName(productName string) string {
	const stalePrefix = "Windows 10"
	if len(productName) >= len(stalePrefix) && productName[:len(stalePrefix)] == stalePrefix {
		return "Windows 11" + productName[len(stalePrefix):]
	}
	return productName
}
