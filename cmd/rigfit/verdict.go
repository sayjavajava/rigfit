// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"runtime"
	"strconv"

	"github.com/sayjavajava/rigfit/internal/cache"
	"github.com/sayjavajava/rigfit/internal/detect"
	"github.com/sayjavajava/rigfit/internal/proton"
	"github.com/sayjavajava/rigfit/internal/report"
	"github.com/sayjavajava/rigfit/internal/reqs"
	"github.com/sayjavajava/rigfit/internal/score"
	"github.com/sayjavajava/rigfit/pkg/hwid"
	"github.com/spf13/cobra"
)

// verdictFlags are §11's global flags, as they apply to the primary
// `rigfit "<game>"` verdict.
type verdictFlags struct {
	asJSON      bool
	offline     bool
	staleOK     bool
	gpuOverride string
	assumeGPU   string
	explain     bool
	noColor     bool
}

func newVerdictFlags(cmd *cobra.Command) *verdictFlags {
	f := &verdictFlags{}
	cmd.Flags().BoolVar(&f.asJSON, "json", false, "print machine-readable JSON instead of human-readable text")
	cmd.Flags().BoolVar(&f.offline, "offline", false, "never make a network call; use cached requirements only")
	cmd.Flags().BoolVar(&f.staleOK, "stale-ok", false, "serve hard-expired cached requirements instead of refusing")
	cmd.Flags().StringVar(&f.gpuOverride, "gpu", "", "override renderer selection among detected GPUs by canonical ID")
	cmd.Flags().StringVar(&f.assumeGPU, "assume-gpu", "", "evaluate as if this canonical GPU ID were installed, e.g. to check an upgrade (§10)")
	cmd.Flags().BoolVar(&f.explain, "explain", false, "print every network call before making it")
	cmd.Flags().BoolVar(&f.noColor, "no-color", false, "disable ANSI colour in human-readable output")
	return f
}

func runVerdict(cmd *cobra.Command, gameName string, f *verdictFlags) error {
	ctx := context.Background()
	out := cmd.OutOrStdout()
	useColor := resolveUseColor(f.noColor)

	specs, err := detect.Collect(f.gpuOverride)
	if err != nil {
		return fmt.Errorf("detecting hardware: %w", err)
	}
	applyAssumeGPU(&specs, f.assumeGPU)

	client := reqs.NewSteamClient()
	client.Explain = newExplainFunc(cmd, f.explain)
	appID, matchedName, err := resolveAppID(ctx, client, gameName, f.offline)
	if err != nil {
		return err
	}

	requirements, freshness, err := client.FetchRequirements(ctx, appID, reqs.FetchOptions{Offline: f.offline, StaleOK: f.staleOK})
	if err != nil {
		return err
	}
	resolvedName := firstNonEmpty(requirements.Name, matchedName, gameName)

	// §8: on Linux, a title with no native Linux Steam listing needs
	// rigfit's Proton-compatibility layer (anti-cheat gate first, then
	// ProtonDB + driver stack, then the Windows requirements evaluated
	// through Proton). A title WITH native Linux support has no Proton
	// layer to reason about at all — it just uses its own
	// linux_requirements, evaluated the normal way below.
	if runtime.GOOS == "linux" && !requirements.Platforms.Linux {
		return runProtonVerdict(cmd, ctx, specs, appID, resolvedName, requirements, freshness, f, useColor)
	}

	tiers := requirements.Tiers
	if runtime.GOOS == "linux" && requirements.Platforms.Linux && requirements.LinuxTiers != nil {
		tiers = *requirements.LinuxTiers
	}

	builtin := score.NewBuiltin()
	gv := report.GameVerdict{
		GameName:     resolvedName,
		Source:       requirements.Source,
		FetchedAt:    requirements.FetchedAt,
		Freshness:    freshness,
		ReviewStatus: requirements.ReviewStatus,
		AssumedGPU:   f.assumeGPU,
	}
	if tiers.Minimum != nil {
		v := builtin.Verdict(specs, tiers.Minimum.ToRequirement())
		gv.Minimum = &v
	}
	if tiers.Recommended != nil {
		v := builtin.Verdict(specs, tiers.Recommended.ToRequirement())
		gv.Recommended = &v
	}

	if f.asJSON {
		body, err := report.VerdictJSON(gv)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(body))
		return err
	}
	_, err = fmt.Fprint(out, report.VerdictHuman(gv, useColor))
	return err
}

// runProtonVerdict implements §8 for a Windows-only title being checked
// on Linux: the anti-cheat gate is checked first and, if it blocks,
// nothing else is even computed (§8 point 1 — "before any spec
// comparison"). Otherwise ProtonDB and the local driver stack are added,
// and the same Windows pc_requirements the standard flow would use are
// evaluated through the usual verdict engine.
func runProtonVerdict(cmd *cobra.Command, ctx context.Context, specs detect.Specs, appID int, gameName string, requirements *reqs.Requirements, freshness cache.Freshness, f *verdictFlags, useColor bool) error {
	out := cmd.OutOrStdout()
	explain := newExplainFunc(cmd, f.explain)

	acClient := proton.NewAntiCheatClient()
	acClient.Explain = explain
	antiCheat, err := acClient.CheckAppID(ctx, appID, f.offline)
	if err != nil {
		return err
	}

	pr := report.ProtonReport{
		GameName:     gameName,
		AntiCheat:    antiCheat,
		Source:       requirements.Source,
		FetchedAt:    requirements.FetchedAt,
		Freshness:    freshness,
		ReviewStatus: requirements.ReviewStatus,
		AssumedGPU:   f.assumeGPU,
	}

	if !antiCheat.Blocks() {
		dbClient := proton.NewProtonDBClient()
		dbClient.Explain = explain
		rating, err := dbClient.FetchRating(ctx, appID, f.offline)
		if err != nil {
			return err
		}
		pr.ProtonDB = rating

		stack, err := proton.DetectStack()
		if err != nil {
			return fmt.Errorf("detecting Linux graphics stack: %w", err)
		}
		pr.Stack = stack

		// The OS hard gate compares against the requirement's stated OS
		// family, which for pc_requirements is always "windows" — that's
		// the whole reason this function exists. Proton IS the answer to
		// that requirement, by construction, once the anti-cheat gate and
		// ProtonDB rating have already spoken to compatibility; gating on
		// a literal Linux-vs-Windows family mismatch here would be wrong,
		// not more correct, so the OS comparison is neutralised to
		// whatever the requirement itself asks for.
		protonSpecs := specs
		protonSpecs.OS = detect.OSInfo{Family: "windows", Version: "11"}

		builtin := score.NewBuiltin()
		if requirements.Tiers.Minimum != nil {
			v := builtin.Verdict(protonSpecs, requirements.Tiers.Minimum.ToRequirement())
			pr.Minimum = &v
		}
		if requirements.Tiers.Recommended != nil {
			v := builtin.Verdict(protonSpecs, requirements.Tiers.Recommended.ToRequirement())
			pr.Recommended = &v
		}
	}

	if f.asJSON {
		body, err := report.ProtonJSON(pr)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(body))
		return err
	}
	_, err = fmt.Fprint(out, report.ProtonHuman(pr, useColor))
	return err
}

// resolveAppID accepts either a bare Steam appid (e.g. `rigfit 1245620`)
// or a game name to resolve via search. --offline never touches the
// network, so a name (rather than a numeric appid) can't be resolved
// offline — there is no local app catalogue to search per SearchApp's own
// doc comment.
func resolveAppID(ctx context.Context, client *reqs.SteamClient, input string, offline bool) (appID int, matchedName string, err error) {
	if id, err := strconv.Atoi(input); err == nil {
		return id, "", nil
	}
	if offline {
		return 0, "", fmt.Errorf("cannot resolve game name %q to a Steam appid while --offline; pass the numeric appid instead", input)
	}
	return client.SearchApp(ctx, input)
}

// applyAssumeGPU implements §10 point 2: evaluate as if a specific
// (possibly not currently installed) GPU were present. It replaces the
// entire detected GPU list so exactly one, assumed, GPU renders; the
// caller is responsible for attributing this in output (report.GameVerdict.
// AssumedGPU), since it's not a real detection result.
func applyAssumeGPU(specs *detect.Specs, canonicalID string) {
	if canonicalID == "" {
		return
	}
	specs.GPUs = []hwid.GPU{{
		CanonicalID: canonicalID,
		IsRenderer:  true,
		Confidence:  hwid.ConfidenceExact,
	}}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
