// SPDX-License-Identifier: MPL-2.0

//go:build linux

package hwid

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const sysfsPCIDevices = "/sys/bus/pci/devices"

// pciDisplayControllerClass is the PCI class code for "Display controller"
// (VGA-compatible, 3D, or other); the class file reports class, subclass
// and prog-if packed into one value, so this masks off the subclass/prog-if
// bits.
const pciDisplayControllerClass = 0x03

// platformGPUDevices implements §4.4 for Linux: enumerate
// /sys/bus/pci/devices/*/{vendor,device,subsystem_vendor,subsystem_device}
// and keep only display controllers.
func platformGPUDevices() ([]rawGPUDevice, error) {
	entries, err := os.ReadDir(sysfsPCIDevices)
	if err != nil {
		return nil, err
	}

	out := make([]rawGPUDevice, 0, len(entries))
	for _, entry := range entries {
		devDir := filepath.Join(sysfsPCIDevices, entry.Name())

		class, err := readSysfsHex(filepath.Join(devDir, "class"))
		if err != nil || (class>>16) != pciDisplayControllerClass {
			continue
		}

		vendor, err := readSysfsHex16(filepath.Join(devDir, "vendor"))
		if err != nil {
			continue
		}
		device, err := readSysfsHex16(filepath.Join(devDir, "device"))
		if err != nil {
			continue
		}

		var subsystemID uint32
		subVendor, errV := readSysfsHex16(filepath.Join(devDir, "subsystem_vendor"))
		subDevice, errD := readSysfsHex16(filepath.Join(devDir, "subsystem_device"))
		if errV == nil && errD == nil {
			subsystemID = uint32(subVendor)<<16 | uint32(subDevice)
		}

		out = append(out, rawGPUDevice{
			VendorID:    vendor,
			DeviceID:    device,
			SubsystemID: subsystemID,
			VRAMBytes:   readLinuxVRAMBytes(devDir),
		})
	}

	return out, nil
}

func readSysfsHex(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	s := strings.TrimPrefix(strings.TrimSpace(string(b)), "0x")
	return strconv.ParseUint(s, 16, 64)
}

func readSysfsHex16(path string) (uint16, error) {
	v, err := readSysfsHex(path)
	if err != nil {
		return 0, err
	}
	return uint16(v), nil
}

// readLinuxVRAMBytes is best-effort: the kernel exposes no driver-neutral
// VRAM-size file. amdgpu reports mem_info_vram_total; other drivers report
// nothing here, and VRAMBytes is left 0 (not determined) rather than
// guessed — 0 is not treated as a real VRAM size anywhere downstream.
func readLinuxVRAMBytes(devDir string) uint64 {
	v, err := readSysfsHex(filepath.Join(devDir, "mem_info_vram_total"))
	if err != nil {
		return 0
	}
	return v
}
