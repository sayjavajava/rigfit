// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

func newDumpCmd() *cobra.Command {
	var (
		anonymous   bool
		gpuOverride string
	)

	cmd := &cobra.Command{
		Use:   "dump",
		Short: "Print raw hardware detection JSON",
		Long: "dump prints the detected CPU and GPU(s) as JSON. The fields it collects " +
			"(vendor/device IDs, canonical/market names, core counts, clocks, VRAM, driver " +
			"version) never include a serial number, hostname, MAC address, or filesystem " +
			"path, so output is anonymous regardless of --anonymous.",
		RunE: func(cmd *cobra.Command, args []string) error {
			snap, err := buildSnapshot(gpuOverride)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(snap)
		},
	}
	cmd.Flags().BoolVar(&anonymous, "anonymous", false, "no-op: dump output never includes identifying data")
	cmd.Flags().StringVar(&gpuOverride, "gpu", "", "override renderer selection by canonical GPU ID")
	return cmd
}
