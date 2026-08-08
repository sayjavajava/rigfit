// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"

	"github.com/sayjavajava/rigfit/internal/detect"
	"github.com/sayjavajava/rigfit/internal/report"
	"github.com/spf13/cobra"
)

func newSpecsCmd() *cobra.Command {
	var (
		asJSON      bool
		gpuOverride string
		noColor     bool
	)

	cmd := &cobra.Command{
		Use:   "specs",
		Short: "Show detected hardware",
		RunE: func(cmd *cobra.Command, args []string) error {
			specs, err := detect.Collect(gpuOverride)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if asJSON {
				body, err := report.SpecsJSON(specs)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(out, string(body))
				return err
			}
			_, err = fmt.Fprint(out, report.SpecsHuman(specs, resolveUseColor(noColor)))
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON instead of human-readable text")
	cmd.Flags().StringVar(&gpuOverride, "gpu", "", "override renderer selection by canonical GPU ID")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable ANSI colour in human-readable output")
	return cmd
}
