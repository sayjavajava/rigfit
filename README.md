# rigfit

A CLI that determines whether a given PC game will run on the current machine, and names the bottleneck when it won't.

```
$ rigfit "ELDEN RING"
ELDEN RING on your machine

  CPU:      ✓ Intel Core i5-8400 (tier 10 vs 8 required, +14% headroom)
  GPU:      ✓ GeForce GTX 1070 (tier 14 vs 10 required, +40% headroom)
  RAM:      ✓ 15.9 GB (needs 12.0 GB)
  Storage:  ✗ 14.7 GB free (needs 60.0 GB)
  OS:       ✓ Windows 11 meets Windows 10+

  Verdict: does not meet minimum requirements — Storage is the blocker

  Requirements: steam, fetched 2 minutes ago
```

**Status:** M0-M8 implemented per `rigfit-BUILD-SPEC.md`'s build order (§13) — identity resolution, hardware detection, tiering, requirements ingestion, the verdict engine, the Linux/Proton compatibility layer, and full release/distribution automation are all in place. Nothing has been released yet (see `docs/RELEASE-SETUP.md`), and `rigfit library` (M9) is not yet built.

## Correctness principle

Every hardware and requirement resolution returns a `Confidence`. When a part is not recognised, rigfit returns `Unknown` and never falls back to a fuzzy guess — that uncertainty propagates all the way to the final verdict rather than being laundered into a confident answer.

| Signal | Source | When it can't confirm |
|---|---|---|
| GPU identity | Exact match against embedded `pci.ids` | `Unknown` — never a fuzzy name match |
| CPU identity | Exact `(vendor, family, model, stepping)` match | `Unknown` — brand string is never authoritative |
| GPU/CPU tier | `gpudb.json`/`cpudb.json` + `coefficients.json` | `Unknown` — a recognised part with no spec-sheet entry still doesn't get a guessed tier |
| Game requirements | Steam's requirement HTML, layered parser | Unparseable fragments are preserved verbatim and flagged, not dropped |
| Anti-cheat status (Linux) | AreWeAntiCheatYet dataset | Not in the dataset ≠ unknown — it means no known anti-cheat concern; a failed fetch is reported as unavailable |
| ProtonDB rating (Linux) | protondb.com API | Reported unavailable past its cache TTL, never guessed |

## Installing

Building from source works everywhere Go does; the package-manager channels below are still being set up (see `docs/RELEASE-SETUP.md`) and aren't live yet.

**From source** (requires Go — version pinned in `go.mod`):

```sh
go build -trimpath -o rigfit ./cmd/rigfit
```

**Scoop** (Windows, once published):

```powershell
scoop bucket add rigfit https://github.com/sayjavajava/scoop-rigfit
scoop install rigfit
```

**AUR** (Arch Linux and derivatives, once published):

```sh
yay -S rigfit-bin   # or your AUR helper of choice
```

**winget** (Windows, once the first manifest submission is merged — see `docs/RELEASE-SETUP.md`):

```powershell
winget install sayjavajava.rigfit
```

### Verifying a release

Every release ships `checksums.txt`, a detached GPG signature, and a build provenance attestation. See `SECURITY.md` for the exact verification commands and the canonical-source statement (releases only ever come from this repo's Releases page and the package managers above — nowhere else).

## Usage

```
rigfit "<game name>"          # primary verdict — will it run, and what's the bottleneck if not
rigfit specs                  # detected hardware
rigfit explain-tier <part-id> # full tier computation for a canonical GPU/CPU ID
rigfit report-unknown         # anonymised hardware dump + prefilled GitHub issue URL
rigfit dump --anonymous       # raw detection JSON, no identifying data
```

Global flags: `--json` (stable, documented output — see below) · `--offline` · `--stale-ok` · `--gpu <id>` (pick the renderer among detected GPUs) · `--assume-gpu <id>` (evaluate as if a different GPU were installed, e.g. to check an upgrade).

On Linux, `rigfit "<game>"` automatically routes through the anti-cheat gate, ProtonDB rating, and driver-stack check for any title with no native Linux Steam listing — see `rigfit-BUILD-SPEC.md` §8.

`--json` output is a stable, versioned public contract — scripts and tooling can depend on its shape.

## Building and testing from source

```sh
go build ./...
go test ./...
go vet ./...
golangci-lint run ./...
govulncheck ./...
```

`pkg/hwid` includes golden-file tests (`testdata/fixtures/*.json`) and named regression tests (spec §4.7) guarding known real-world hardware-identification failures — e.g. AMD Ryzen 5 5500 vs. A8-5500 Trinity, Intel Iris Xe vs. Iris Graphics 6100, and the Windows VRAM-registry-vs-WMI trap. `internal/score` has a drift-guard golden test (§5.5) locking reference-GPU tiers so a coefficient change can't silently shift an unrelated part.

## Non-goals

FPS prediction, a GUI/web UI, telemetry of any kind, an auto-updater, console/mobile hardware support, overclocking or upgrade-shopping advice, non-Steam storefronts, real-time monitoring, runtime LLM calls, and PassMark data integration are explicitly out of scope. See `rigfit-BUILD-SPEC.md` §1.

## Contributing

See `CONTRIBUTING.md`. `data/METHODOLOGY.md` documents how the current GPU/CPU spec-sheet data and tier coefficients were derived, including their confidence levels.

## Privacy and security

See `PRIVACY.md` (no telemetry, exactly what's stored locally and sent over the network) and `SECURITY.md` (vulnerability reporting, release verification). `KNOWN_AV_ISSUES.md` explains why antivirus software sometimes flags unsigned Go binaries like this one incorrectly.

## Licence

- `cmd/`, `internal/`: GPL-3.0-only (see `LICENSE`)
- `pkg/hwid/`: MPL-2.0 (see `pkg/hwid/LICENSE`)
- `data/`: CC-BY-SA-4.0 (see `data/LICENSE`), except vendored third-party files noted in `data/README.md`, which retain their own licence
