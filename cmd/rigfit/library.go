// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"

	"github.com/sayjavajava/rigfit/internal/detect"
	"github.com/sayjavajava/rigfit/internal/library"
	"github.com/sayjavajava/rigfit/internal/report"
	"github.com/sayjavajava/rigfit/internal/reqs"
	"github.com/sayjavajava/rigfit/internal/score"
	"github.com/spf13/cobra"
)

func newLibraryCmd() *cobra.Command {
	var (
		asJSON      bool
		offline     bool
		staleOK     bool
		gpuOverride string
		explain     bool
		noColor     bool
	)

	cmd := &cobra.Command{
		Use:   "library",
		Short: "Scan your installed Steam library for underperforming titles",
		Long: "library scans steamapps/libraryfolders.vdf and appmanifest_*.acf, read-only and " +
			"scoped strictly to Steam's own directories (§11), and reports which installed " +
			"titles don't comfortably clear their minimum requirements plus which single " +
			"upgrade would help the most.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLibrary(cmd, libraryFlags{asJSON, offline, staleOK, gpuOverride, explain, noColor})
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON instead of human-readable text")
	cmd.Flags().BoolVar(&offline, "offline", false, "never make a network call; use cached requirements only")
	cmd.Flags().BoolVar(&staleOK, "stale-ok", false, "serve hard-expired cached requirements instead of skipping a title")
	cmd.Flags().StringVar(&gpuOverride, "gpu", "", "override renderer selection among detected GPUs by canonical ID")
	cmd.Flags().BoolVar(&explain, "explain", false, "print every network call before making it")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable ANSI colour in human-readable output")
	return cmd
}

type libraryFlags struct {
	asJSON      bool
	offline     bool
	staleOK     bool
	gpuOverride string
	explain     bool
	noColor     bool
}

func runLibrary(cmd *cobra.Command, f libraryFlags) error {
	ctx := context.Background()
	out := cmd.OutOrStdout()
	useColor := resolveUseColor(f.noColor)

	apps, err := library.Scan()
	if err != nil {
		return fmt.Errorf("scanning Steam library: %w", err)
	}

	specs, err := detect.Collect(f.gpuOverride)
	if err != nil {
		return fmt.Errorf("detecting hardware: %w", err)
	}

	client := reqs.NewSteamClient()
	client.Explain = newExplainFunc(cmd, f.explain)
	builtin := score.NewBuiltin()

	results := make([]report.LibraryTitleResult, 0, len(apps))
	for _, app := range apps {
		requirements, _, err := client.FetchRequirements(ctx, app.AppID, reqs.FetchOptions{Offline: f.offline, StaleOK: f.staleOK})
		if err != nil || requirements.Tiers.Minimum == nil {
			continue // no usable requirements data for this title; skip, don't fail the scan
		}
		v := builtin.Verdict(specs, requirements.Tiers.Minimum.ToRequirement())
		name := app.Name
		if requirements.Name != "" {
			name = requirements.Name
		}
		results = append(results, report.LibraryTitleResult{
			AppID:      app.AppID,
			Name:       name,
			Overall:    v.Overall,
			Bottleneck: v.Bottleneck,
		})
	}

	lr := report.NewLibraryReport(len(apps), results)

	if f.asJSON {
		body, err := report.LibraryJSON(lr)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(body))
		return err
	}
	_, err = fmt.Fprint(out, report.LibraryHuman(lr, useColor))
	return err
}
