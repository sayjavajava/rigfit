// SPDX-License-Identifier: GPL-3.0-only

// Command normalizer is the offline build-pipeline tool that turns a raw
// Steam requirements blob into strict JSON per rigfit-BUILD-SPEC.md §6.3
// (§6.4, §13 M4). It is never invoked by the shipped rigfit binary — the
// runtime parser (internal/reqs.ParseTier) is deterministic and
// dependency-free by design, exactly so the shipped binary never needs to
// make an LLM call.
//
// This environment has no LLM API credentials to wire up, so the shipped
// default engine (deterministicNormalizer) simply reuses
// internal/reqs.ParseTier — the same logic the runtime uses, just run
// offline against a saved blob. Normalizer is an interface specifically
// so a maintainer/contributor can plug in a real LLM-backed
// implementation later without changing main.go's flow; the mandatory
// validator (validate.go) is the safety net regardless of which engine
// produced a candidate record — §6.4: "Unvalidated LLM output MUST NOT
// enter data/."
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sayjavajava/rigfit/internal/reqs"
)

// Normalizer turns a raw requirements HTML blob into a structured Tier.
// §6.4: "Prompt MUST instruct returning null over guessing" — a real
// LLM-backed Normalizer must honour that by leaving fields unset rather
// than inventing a plausible-looking canonical ID; the deterministic
// default already does this by construction (its matcher only ever
// returns IDs that exist in gpudb.json/cpudb.json).
type Normalizer interface {
	Name() string
	Normalize(rawHTML string) (*reqs.Tier, error)
}

type deterministicNormalizer struct{}

func (deterministicNormalizer) Name() string { return "deterministic-v1" }

func (deterministicNormalizer) Normalize(rawHTML string) (*reqs.Tier, error) {
	return reqs.ParseTier(rawHTML), nil
}

type input struct {
	AppID       int    `json:"appid"`
	Name        string `json:"name"`
	Minimum     string `json:"minimum"`
	Recommended string `json:"recommended"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "normalizer:", err)
		os.Exit(1)
	}
}

func run() error {
	inputPath := flag.String("input", "", "path to a JSON file: {appid, name, minimum, recommended} (required)")
	outDir := flag.String("out", filepath.Join("data", "requirements"), "output directory for validated <appid>.json records")
	flaggedDir := flag.String("flagged-out", filepath.Join("data", "requirements-flagged"), "output directory for records the validator rejected, kept out of data/requirements/")
	flag.Parse()

	if *inputPath == "" {
		flag.Usage()
		return fmt.Errorf("-input is required")
	}

	raw, err := os.ReadFile(*inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *inputPath, err)
	}
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("parsing %s: %w", *inputPath, err)
	}

	normalizer := Normalizer(deterministicNormalizer{})

	minTier, err := normalizer.Normalize(in.Minimum)
	if err != nil {
		return fmt.Errorf("normalizing minimum tier: %w", err)
	}
	recTier, err := normalizer.Normalize(in.Recommended)
	if err != nil {
		return fmt.Errorf("normalizing recommended tier: %w", err)
	}

	invalid := append(validate(minTier), validate(recTier)...)

	hash := sha256.Sum256([]byte(in.Minimum + "\x00" + in.Recommended))
	record := reqs.Requirements{
		AppID:             in.AppID,
		Name:              in.Name,
		Source:            "steam",
		FetchedAt:         time.Now().UTC(),
		RawHash:           "sha256:" + hex.EncodeToString(hash[:]),
		NormalizerVersion: normalizer.Name(),
		ReviewStatus:      reqs.ReviewAuto,
		Tiers:             reqs.Tiers{Minimum: minTier, Recommended: recTier},
	}

	if len(invalid) > 0 {
		record.ReviewStatus = reqs.ReviewFlagged
		return writeRecord(*flaggedDir, record, invalid)
	}
	return writeRecord(*outDir, record, nil)
}

func writeRecord(dir string, record reqs.Requirements, invalid []string) error {
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("%d.json", record.AppID))
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return err
	}

	if len(invalid) > 0 {
		fmt.Fprintf(os.Stderr, "normalizer: %d unrecognised part name(s) — record flagged, NOT written to data/requirements/:\n", len(invalid))
		for _, name := range invalid {
			fmt.Fprintf(os.Stderr, "  - %s\n", name)
		}
		fmt.Fprintf(os.Stderr, "normalizer: wrote flagged candidate to %s for human review\n", path)
		return nil
	}
	fmt.Printf("normalizer: wrote %s (review_status=%s)\n", path, record.ReviewStatus)
	return nil
}
