// SPDX-License-Identifier: GPL-3.0-only

package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sayjavajava/rigfit/internal/detect"
	"github.com/sayjavajava/rigfit/pkg/hwid"
)

// SpecsJSON renders Specs as rigfit's stable --json output (§11).
func SpecsJSON(specs detect.Specs) ([]byte, error) {
	return json.MarshalIndent(specs, "", "  ")
}

// SpecsHuman renders Specs as the human-readable text `rigfit specs`
// prints by default.
func SpecsHuman(specs detect.Specs, useColor bool) string {
	var b strings.Builder

	fmt.Fprintf(&b, "CPU:      %s\n", formatCPULine(specs.CPU, useColor))
	for i, g := range specs.GPUs {
		label := "GPU:"
		if i > 0 {
			label = "" // align continuation lines under "GPU:"
		}
		fmt.Fprintf(&b, "%-9s %s\n", label, formatGPULine(g, useColor))
	}
	fmt.Fprintf(&b, "RAM:      %s\n", FormatBytes(specs.RAM.TotalBytes))
	fmt.Fprintf(&b, "Storage:  %s free of %s (%s)\n",
		FormatBytes(specs.Storage.FreeBytes), FormatBytes(specs.Storage.TotalBytes), specs.Storage.Path)
	fmt.Fprintf(&b, "OS:       %s (%s)\n", specs.OS.Name, specs.OS.Version)

	return b.String()
}

// unknownMarker renders "?" in gray (when useColor) — the same "uncertain,
// not a guess" visual treatment score.Status's Unknown gets elsewhere.
func unknownMarker(useColor bool) string {
	if !useColor {
		return "?"
	}
	return ansiGray + "?" + ansiReset
}

func formatCPULine(c hwid.CPU, useColor bool) string {
	if c.Confidence == hwid.ConfidenceUnknown {
		return fmt.Sprintf("%s Unrecognised CPU — vendor %s, family %d, model %d, stepping %d",
			unknownMarker(useColor), c.VendorID, c.Family, c.Model, c.Stepping)
	}
	return fmt.Sprintf("%s (%dc/%dt, %.1f GHz base) [%s]",
		c.MarketName, c.PhysCores, c.LogicalCores, float64(c.BaseClockMHz)/1000, c.Confidence)
}

func formatGPULine(g hwid.GPU, useColor bool) string {
	if g.Confidence == hwid.ConfidenceUnknown {
		return fmt.Sprintf("%s Unrecognised device — PCI %04X:%04X", unknownMarker(useColor), g.VendorID, g.DeviceID)
	}
	role := "display only"
	if g.IsRenderer {
		role = "renderer"
	}
	vram := ""
	if g.VRAMBytes > 0 {
		vram = fmt.Sprintf(" — %s VRAM", FormatBytes(g.VRAMBytes))
	}
	return fmt.Sprintf("%s%s — %s [%s]", g.MarketName, vram, role, g.Confidence)
}
