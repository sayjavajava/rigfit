// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/sayjavajava/rigfit/internal/score"
	"github.com/spf13/cobra"
)

func newExplainTierCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "explain-tier <part-id>",
		Short: "Print the full tier computation for a canonical GPU or CPU ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExplainTier(cmd, args[0], asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON instead of human-readable text")
	return cmd
}

func runExplainTier(cmd *cobra.Command, partID string, asJSON bool) error {
	b := score.NewBuiltin()
	out := cmd.OutOrStdout()

	if gpuExp, err := b.ExplainGPU(partID); err == nil {
		return printExplanation(out, gpuExp, asJSON, formatGPUExplanation)
	}
	if cpuExp, err := b.ExplainCPU(partID); err == nil {
		return printExplanation(out, cpuExp, asJSON, formatCPUExplanation)
	}
	return fmt.Errorf("explain-tier: %q is not a known GPU or CPU canonical ID", partID)
}

func printExplanation[T any](out io.Writer, exp T, asJSON bool, formatHuman func(T) string) error {
	if asJSON {
		body, err := json.MarshalIndent(exp, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(body))
		return err
	}
	_, err := fmt.Fprint(out, formatHuman(exp))
	return err
}

func formatGPUExplanation(e score.GPUExplanation) string {
	return fmt.Sprintf(
		`%s (%s)

  shader_units             %d
  boost_clock_ghz          %.3f
  bus_width_bits           %d
  effective_mem_clock_ghz  %.1f

  FP32_GFLOPS  = shader_units * 2 * boost_clock_ghz        = %.2f
  BANDWIDTH_GBs = (bus_width_bits / 8) * eff_mem_clock_ghz = %.2f

  RAW  = FP32_GFLOPS^0.65 * BANDWIDTH_GBs^0.35 = %.2f
  PERF = RAW * arch_coefficient(%.3f)          = %.2f

  BASELINE_PERF (GTX 1060 6GB) = %.2f
  TIER = round(8 * log2(PERF / BASELINE_PERF)) + 10 = %d
`,
		e.MarketName, e.Architecture,
		e.ShaderUnits, e.BoostClockGHz, e.BusWidthBits, e.EffectiveMemClockGHz,
		e.FP32GFLOPS, e.BandwidthGBs,
		e.RawPerf, e.ArchCoefficient, e.Perf,
		e.BaselinePerf, e.Tier,
	)
}

func formatCPUExplanation(e score.CPUExplanation) string {
	return fmt.Sprintf(
		`%s (%s)

  phys_cores        %d
  logical_cores     %d
  all_core_clock_ghz %.3f

  PERF = phys_cores^0.5 * logical_cores^0.15 * all_core_clock_ghz * ipc_coefficient(%.3f) = %.3f

  BASELINE_PERF (Intel Core i5-8400) = %.3f
  TIER = round(8 * log2(PERF / BASELINE_PERF)) + 10 = %d
`,
		e.MarketName, e.Architecture,
		e.PhysCores, e.LogicalCores, e.AllCoreClockGHz,
		e.IPCCoefficient, e.Perf,
		e.BaselinePerf, e.Tier,
	)
}
