// SPDX-License-Identifier: MPL-2.0

package hwid_test

import (
	"fmt"

	"github.com/sayjavajava/rigfit/pkg/hwid"
)

// A GPU's Confidence must always be checked before trusting CanonicalID or
// MarketName: an unrecognised device reports ConfidenceUnknown rather than
// a guess, and callers must propagate that rather than paper over it.
func Example_confidenceGate() {
	gpus, err := hwid.ResolveGPUs()
	if err != nil {
		fmt.Println("detection failed:", err)
		return
	}

	for _, g := range gpus {
		if g.Confidence == hwid.ConfidenceUnknown {
			fmt.Printf("PCI %04x:%04x — unrecognised, reporting Unknown rather than guessing\n", g.VendorID, g.DeviceID)
			continue
		}
		fmt.Printf("%s (%s confidence)\n", g.CanonicalID, g.Confidence)
	}
}

// On a laptop with both integrated and discrete graphics, exactly the
// discrete GPU is marked IsRenderer; the integrated GPU is still present
// in the slice, never hidden.
func Example_hybridGraphics() {
	gpus, err := hwid.ResolveGPUs()
	if err != nil {
		fmt.Println("detection failed:", err)
		return
	}

	for _, g := range gpus {
		role := "display only"
		if g.IsRenderer {
			role = "renders games"
		}
		fmt.Printf("%s: discrete=%v, %s\n", g.MarketName, g.IsDiscrete, role)
	}
}

// --gpu <canonical-id> overrides which detected GPU is treated as the
// renderer, regardless of the discrete-GPU heuristic.
func Example_gpuOverride() {
	gpus, err := hwid.ResolveGPUsWithOverride("intel-uhd-graphics")
	if err != nil {
		fmt.Println("detection failed:", err)
		return
	}
	for _, g := range gpus {
		fmt.Printf("%s renderer=%v\n", g.CanonicalID, g.IsRenderer)
	}
}

func Example_confidenceString() {
	fmt.Println(hwid.ConfidenceExact)
	fmt.Println(hwid.ConfidenceUnknown)
	// Output:
	// exact
	// unknown
}
