// SPDX-License-Identifier: MPL-2.0

// Package hwid resolves PCI and CPU hardware identifiers to canonical part
// identities (GPU and CPU). It is a standalone identity-resolution
// library: it does not import anything from rigfit's internal/ packages
// and must not be extended to do so.
//
// The governing rule (see rigfit-BUILD-SPEC.md §1) is that every
// resolution returns a Confidence, and unrecognised hardware reports
// ConfidenceUnknown rather than a fuzzy guess.
package hwid

import (
	"encoding/json"
	"fmt"
)

// Confidence reports how a hardware identity was resolved. Callers MUST
// check Confidence before trusting CanonicalID or MarketName: an
// unrecognised part reports ConfidenceUnknown rather than a best guess,
// and that MUST propagate to any verdict built on top of it.
type Confidence int

const (
	// ConfidenceExact means the part was matched on a hardware ID: a PCI
	// vendor:device (or vendor:subsystem) pair for a GPU, or a
	// vendor/family/model/stepping tuple for a CPU.
	ConfidenceExact Confidence = iota

	// ConfidenceHigh means the part was matched on a normalised string
	// signal rather than a raw hardware ID.
	ConfidenceHigh

	// ConfidenceFuzzy means the match is best-effort; callers SHOULD
	// surface a warning to the user rather than presenting it as settled.
	ConfidenceFuzzy

	// ConfidenceUnknown means the part could not be identified. Callers
	// MUST NOT substitute a guess; propagate Unknown to the verdict.
	ConfidenceUnknown
)

// String returns the lowercase name used in rigfit's JSON output and logs.
func (c Confidence) String() string {
	switch c {
	case ConfidenceExact:
		return "exact"
	case ConfidenceHigh:
		return "high"
	case ConfidenceFuzzy:
		return "fuzzy"
	case ConfidenceUnknown:
		return "unknown"
	default:
		return "invalid"
	}
}

// MarshalJSON renders Confidence as its String() form so `--json` output is
// self-describing instead of a bare integer.
func (c Confidence) MarshalJSON() ([]byte, error) {
	return []byte(`"` + c.String() + `"`), nil
}

// UnmarshalJSON is the inverse of MarshalJSON, so rigfit's own JSON output
// round-trips (e.g. through cached files or tooling that re-parses it).
func (c *Confidence) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "exact":
		*c = ConfidenceExact
	case "high":
		*c = ConfidenceHigh
	case "fuzzy":
		*c = ConfidenceFuzzy
	case "unknown":
		*c = ConfidenceUnknown
	default:
		return fmt.Errorf("hwid: invalid Confidence %q", s)
	}
	return nil
}
