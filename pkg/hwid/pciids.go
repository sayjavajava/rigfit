// SPDX-License-Identifier: MPL-2.0

package hwid

import (
	"bufio"
	"bytes"
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

// pci.ids is vendored from https://pci-ids.ucw.cz/v2.2/pci.ids (dual
// BSD-3-Clause / GPL-2.0-or-later; see data/README.md for provenance and
// the refresh procedure). Go's //go:embed directive cannot reach outside
// the package directory, so this is a synced copy of data/pci.ids; the
// TestEmbeddedPCIIDsMatchesVendoredCopy test in sync_test.go fails CI if
// the two ever drift apart.
//
//go:embed pci.ids
var rawPCIIDs []byte

// pciEntry is a single resolved pci.ids record: either a top-level
// vendor:device entry, or a more specific vendor:device pair reached via a
// subsystem match (§4.2 step 3), in which case VendorID/DeviceID are the
// *owning* chip's IDs (not the subsystem vendor's).
type pciEntry struct {
	VendorID uint16
	DeviceID uint16
	Name     string
}

// pciDatabase is the parsed form of pci.ids, indexed for the two lookups
// §4.2 requires: an exact vendor:device match, and a fallback match on
// subsystem vendor:device for OEM-rebadged laptop parts.
type pciDatabase struct {
	devices    map[uint32]pciEntry // key: vendorID<<16 | deviceID
	subsystems map[uint32]pciEntry // key: subsystemVendorID<<16 | subsystemDeviceID
	ambiguous  map[uint32]struct{} // subsystem keys seen under more than one distinct device; never used
}

var (
	pciDBOnce sync.Once
	pciDB     *pciDatabase
)

func getPCIDatabase() *pciDatabase {
	pciDBOnce.Do(func() {
		pciDB = parsePCIIDs(rawPCIIDs)
	})
	return pciDB
}

// parsePCIIDs parses the pci.ids text format:
//
//	VVVV  Vendor Name
//	\tDDDD  Device Name
//	\t\tSSSS VVVV  Subsystem Name
//
// The file also has a "C xx  Class Name" section after the vendor/device
// section; parsing stops there since rigfit has no use for PCI class data.
func parsePCIIDs(raw []byte) *pciDatabase {
	db := &pciDatabase{
		devices:    make(map[uint32]pciEntry, 8192),
		subsystems: make(map[uint32]pciEntry, 8192),
		ambiguous:  make(map[uint32]struct{}),
	}

	var (
		curVendor      uint16
		curVendorValid bool
		curDevice      uint16
		curDeviceValid bool
	)

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "C ") {
			break // device-class section; vendor/device parsing is done
		}

		switch {
		case strings.HasPrefix(line, "\t\t"):
			if !curVendorValid || !curDeviceValid {
				continue
			}
			idsPart, subName, ok := splitTwoSpace(strings.TrimPrefix(line, "\t\t"))
			if !ok {
				continue
			}
			fields := strings.Fields(idsPart)
			if len(fields) != 2 {
				continue
			}
			subVendor, err1 := parseHexID(fields[0])
			subDevice, err2 := parseHexID(fields[1])
			if err1 != nil || err2 != nil {
				continue
			}
			key := uint32(subVendor)<<16 | uint32(subDevice)
			if existing, ok := db.subsystems[key]; ok {
				if existing.VendorID != curVendor || existing.DeviceID != curDevice {
					db.ambiguous[key] = struct{}{}
				}
				continue
			}
			db.subsystems[key] = pciEntry{VendorID: curVendor, DeviceID: curDevice, Name: subName}

		case strings.HasPrefix(line, "\t"):
			curDeviceValid = false
			if !curVendorValid {
				continue
			}
			idPart, name, ok := splitTwoSpace(strings.TrimPrefix(line, "\t"))
			if !ok {
				continue
			}
			id, err := parseHexID(idPart)
			if err != nil {
				continue
			}
			curDevice = id
			curDeviceValid = true
			db.devices[uint32(curVendor)<<16|uint32(curDevice)] = pciEntry{VendorID: curVendor, DeviceID: curDevice, Name: name}

		default:
			curVendorValid = false
			curDeviceValid = false
			idPart, _, ok := splitTwoSpace(line)
			if !ok {
				continue
			}
			id, err := parseHexID(idPart)
			if err != nil {
				continue
			}
			curVendor = id
			curVendorValid = true
		}
	}

	return db
}

// splitTwoSpace splits a pci.ids line on the two-space separator between
// the hex ID field(s) and the free-text name.
func splitTwoSpace(s string) (idPart, name string, ok bool) {
	i := strings.Index(s, "  ")
	if i < 0 {
		return "", "", false
	}
	return s[:i], strings.TrimSpace(s[i+2:]), true
}

func parseHexID(s string) (uint16, error) {
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, err
	}
	return uint16(v), nil
}

// lookupDevice is §4.2 step 2: an exact VendorID:DeviceID match.
func (db *pciDatabase) lookupDevice(vendorID, deviceID uint16) (pciEntry, bool) {
	e, ok := db.devices[uint32(vendorID)<<16|uint32(deviceID)]
	return e, ok
}

// lookupSubsystem is §4.2 step 3: a fallback match on subsystem
// vendor:device, used to identify OEM-rebadged laptop parts whose
// top-level PCI device ID pci.ids does not otherwise recognise. A
// subsystem ID pair that maps to more than one distinct owning device
// across the database is ambiguous and is never used, per §1: no fuzzy
// fallback.
func (db *pciDatabase) lookupSubsystem(subVendorID, subDeviceID uint16) (pciEntry, bool) {
	key := uint32(subVendorID)<<16 | uint32(subDeviceID)
	if _, ambiguous := db.ambiguous[key]; ambiguous {
		return pciEntry{}, false
	}
	e, ok := db.subsystems[key]
	return e, ok
}

// slugify lowercases s and replaces every run of non-alphanumeric
// characters with a single hyphen, trimming leading/trailing hyphens.
func slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevDash := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r - 'A' + 'a')
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}
