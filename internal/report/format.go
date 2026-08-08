// SPDX-License-Identifier: GPL-3.0-only

// Package report renders detection and (eventually) verdict data as
// human-readable text or as rigfit's stable --json contract (§11).
package report

import "fmt"

// FormatBytes renders a byte count as a human-readable size using decimal
// (SI, 1000-based) units, matching how storage and RAM capacities are
// marketed and how Windows/Linux/macOS all report disk sizes by default.
func FormatBytes(b uint64) string {
	const unit = 1000.0
	if b < uint64(unit) {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := unit, 0
	for n := float64(b) / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(b)/div, units[exp])
}
