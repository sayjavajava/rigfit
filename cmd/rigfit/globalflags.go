// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// resolveUseColor implements §11's --no-color flag plus the well-known
// NO_COLOR convention (https://no-color.org) as an additional, unasked-for
// opt-out — any non-empty value disables colour, no fuller TTY detection
// is attempted (no approved dependency provides isatty, and requiring one
// just for this felt like scope creep for an explicit opt-in/opt-out flag
// the user already controls directly).
func resolveUseColor(noColor bool) bool {
	if noColor {
		return false
	}
	return os.Getenv("NO_COLOR") == ""
}

// newExplainFunc implements §11's --explain flag ("print every network
// call before making it"): when enabled, returns a callback that writes
// each URL to stderr (so it never corrupts --json output on stdout, the
// same reasoning that keeps the --assume-gpu disclaimer out of stdout —
// see report.GameVerdict.AssumedGPU); when disabled, returns nil, which
// every network client here treats as "don't explain".
func newExplainFunc(cmd *cobra.Command, explain bool) func(url string) {
	if !explain {
		return nil
	}
	errOut := cmd.ErrOrStderr()
	return func(url string) {
		_, _ = fmt.Fprintf(errOut, "→ %s\n", url)
	}
}
