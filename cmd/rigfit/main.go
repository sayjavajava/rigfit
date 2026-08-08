// SPDX-License-Identifier: GPL-3.0-only

// Command rigfit determines whether a given PC game will run on the
// current machine, and names the bottleneck when it won't.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is set via -ldflags at release build time (§12.3); "dev" for
// local/unreleased builds.
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     `rigfit "<game name>"`,
		Short:   "Determine whether a game will run on this machine",
		Version: version,
		Args:    cobra.MaximumNArgs(1),
	}
	flags := newVerdictFlags(root)
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return runVerdict(cmd, args[0], flags)
	}

	root.AddCommand(newDumpCmd())
	root.AddCommand(newReportUnknownCmd())
	root.AddCommand(newSpecsCmd())
	root.AddCommand(newExplainTierCmd())
	root.AddCommand(newLibraryCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
