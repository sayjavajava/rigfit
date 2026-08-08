# rigfit — Build Specification

Implementation spec for a coding agent. Normative: MUST / MUST NOT / SHOULD.

---

## 1. Project

**rigfit** — a CLI that determines whether a given PC game will run on the current machine, and names the bottleneck when it won't.

- Language: **Go**, single static binary, no runtime dependency
- Platforms: **Linux** and **Windows** (both first-class). macOS best-effort.
- Module: `github.com/<owner>/rigfit`
- Binary/command: `rigfit`

### Licences (SPDX headers required in every source file)

| Path | Licence |
|---|---|
| `cmd/`, `internal/` | `GPL-3.0-only` |
| `pkg/hwid/` | `MPL-2.0` |
| `data/` | `CC-BY-SA-4.0` |

### Correctness principle (governs all resolution code)

Every hardware and requirement resolution MUST return a `Confidence`. When a part is not recognised, the system MUST return `ConfidenceUnknown` and MUST NOT fall back to a fuzzy guess. Uncertainty MUST propagate to the final verdict as `Unknown`; it MUST NOT be laundered into a confident answer.

### Out of scope — do not implement

FPS prediction · GUI or web UI · telemetry or analytics of any kind · auto-updater · console/mobile hardware · overclocking or upgrade-shopping advice · non-Steam storefronts · real-time monitoring · runtime LLM calls · any PassMark data integration.

---

## 2. Global Constraints

MUST:
- `CGO_ENABLED=0`, `-trimpath`, pinned Go toolchain
- Write only to the cache directory: `~/.cache/rigfit/` (Linux/macOS), `%LOCALAPPDATA%\rigfit\` (Windows)
- Run unprivileged; no root/admin requirement
- Embed Windows version-info resource (product name, version, description)
- Minimal dependency tree

MUST NOT:
- Emit telemetry. No analytics dependency may appear in `go.mod`.
- Compress with UPX or any packer
- Implement self-update
- Use obfuscation, runtime code generation, or process injection
- Write outside the cache directory or to the registry
- Make network calls other than: Steam API, ProtonDB, AreWeAntiCheatYet

### Approved dependencies
`github.com/klauspost/cpuid/v2` · `golang.org/x/sys/windows` (registry, WMI) · `github.com/spf13/cobra` (CLI) · stdlib elsewhere. Justify any addition.

---

## 2A. External Inputs

Data the agent must fetch or the maintainer must supply, by milestone. Nothing here blocks M1 except `pci.ids`.

| Input | Needed by | Source | Notes |
|---|---|---|---|
| `pci.ids` | **M1** | `https://pci-ids.ucw.cz/v2.2/pci.ids` | Dual BSD-3-Clause / GPL-2.0. Vendor a pinned copy into `data/`; embed with `go:embed`. Record the retrieval date and licence text in `data/README.md`. Do not fetch at runtime. |
| Hardware fixtures | **M1** (tests) | Maintainer-collected via `report-unknown` | Write golden tests against synthetic fixtures first so M1 is not blocked; real dumps are added incrementally. Target 50+. |
| GPU/CPU spec sheets | M3 | Manufacturer spec pages; TechPowerUp GPU DB as index | Fields: shader units, boost clock, bus width, effective memory clock, architecture. ~300 GPUs, ~400 CPUs covers ~95% of machines. Maintainer-supplied or contributor-sourced. |
| Steam `appdetails` | M4 | `https://store.steampowered.com/api/appdetails` | Undocumented, no key. See §6.1 for rate limits. |
| ProtonDB ratings | M6 | ProtonDB API | Cache per §9. Degrade gracefully if unavailable. |
| Anti-cheat status | M6 | AreWeAntiCheatYet dataset | Never served stale (§9). |

If an external source is unreachable or its terms prohibit the intended use, the affected feature MUST degrade to `Unknown` rather than guess, and the limitation MUST be recorded in `README.md`.

---

## 3. Repository Layout

```
rigfit/
├── cmd/rigfit/               # CLI entrypoint (cobra)
├── internal/
│   ├── detect/               # OS hardware detection
│   │   ├── detect.go         # interface + shared types
│   │   ├── detect_linux.go
│   │   ├── detect_windows.go
│   │   └── detect_darwin.go
│   ├── reqs/                 # steam.go, parse.go, cache.go
│   ├── score/                # tiering + verdict engine
│   ├── proton/               # Linux compatibility layer
│   ├── report/               # human + JSON rendering
│   └── cache/                # TTL-aware local cache
├── pkg/hwid/                 # PUBLIC identity library (MPL-2.0)
│   ├── gpu.go  cpu.go  pciids.go  confidence.go
├── data/
│   ├── pci.ids               # bundled, embed via go:embed
│   ├── gpudb.json  cpudb.json  coefficients.json
│   └── METHODOLOGY.md
├── tools/
│   ├── dbbuild/              # spec sheets -> gpudb/cpudb
│   └── normalizer/           # offline LLM requirements normaliser
├── testdata/fixtures/        # real-machine detection dumps
└── .github/workflows/
```

---

## 4. `pkg/hwid` — Identity Resolution

Build this first. It ships as a standalone library and MUST NOT import anything from `internal/`.

### 4.1 Types

```go
type GPU struct {
    VendorID    uint16   // 0x10DE nvidia, 0x1002 amd, 0x8086 intel
    DeviceID    uint16
    SubsystemID uint32
    CanonicalID string   // "nvidia-gtx-1660-ti"
    MarketName  string   // "NVIDIA GeForce GTX 1660 Ti"
    VRAMBytes   uint64
    IsDiscrete  bool
    IsRenderer  bool     // the GPU games will actually use
    DriverVer   string
    Confidence  Confidence
}

type CPU struct {
    VendorID     string  // "GenuineIntel" | "AuthenticAMD"
    Family       uint16
    Model        uint16
    Stepping     uint8
    CanonicalID  string  // "amd-ryzen-5-5500"
    MarketName   string
    PhysCores    int
    LogicalCores int
    BaseClockMHz uint32
    Confidence   Confidence
}

type Confidence int
const (
    ConfidenceExact   Confidence = iota // matched on hardware ID
    ConfidenceHigh                      // matched on normalised string
    ConfidenceFuzzy                     // best-effort; warn
    ConfidenceUnknown                   // refuse to guess
)
```

`CanonicalID` format: lowercase, hyphenated, `<vendor>-<family>-<model>[-<variant>]`.

### 4.2 GPU resolution

1. Read PCI vendor/device ID from the OS (§4.4)
2. Look up `VendorID:DeviceID` in the embedded `pci.ids` → `ConfidenceExact`
3. On miss, try `SubsystemID` for OEM-rebadged laptop parts → `ConfidenceExact`
4. On miss, `ConfidenceUnknown`. MUST NOT fall through to string matching without downgrading confidence.

### 4.3 CPU resolution

1. `klauspost/cpuid/v2` → vendor, family, model, stepping
2. Look up `(vendor, family, model, stepping)` in `cpudb.json` → `ConfidenceExact`
3. Brand string is a **secondary** signal only, never primary
4. Unknown families (AMD "Ryzen AI", Intel "Core Ultra") MUST return `ConfidenceUnknown`

### 4.4 Per-OS ID sources

| OS | GPU | CPU |
|---|---|---|
| Linux | `/sys/bus/pci/devices/*/vendor`, `/device`, `/subsystem_device` | CPUID + `/proc/cpuinfo` |
| Windows | WMI `Win32_VideoController.PNPDeviceID`, format `PCI\VEN_10DE&DEV_2191&SUBSYS_...` — parse hex | CPUID |
| macOS | `system_profiler SPDisplaysDataType -json` | CPUID |

### 4.5 Windows trap 1 — VRAM overflow (MUST handle)

`Win32_VideoController.AdapterRAM` is **uint32** and silently wraps above 4 GB; a 12 GB card reports nonsense. It MUST NOT be used.

Correct source — registry `REG_QWORD`:
```
HKLM\SYSTEM\CurrentControlSet\Control\Class\
  {4d36e968-e325-11ce-bfc1-08002be10318}\<NNNN>\qwMemorySize
```
Enumerate `0000`, `0001`, … subkeys; match each by `MatchingDeviceId` against the resolved PCI ID.

### 4.6 Windows trap 2 — hybrid graphics (MUST handle)

Laptops report both an iGPU and a dGPU. Scoring the wrong one produces a wrong verdict, and this is the most common real-world laptop case.

1. Exactly one discrete GPU present → set `IsRenderer = true` on it
2. Both present → the discrete GPU is the renderer; the iGPU may be display output only. This does not change the verdict.
3. Output MUST report both. Never hide the iGPU.
4. `--gpu <canonical-id>` overrides selection.

### 4.7 Required regression tests

Golden-file tests over `testdata/fixtures/*.json`, plus these named cases (real competitor failures):

```go
func TestRyzen5500NeverResolvesToA8Trinity(t *testing.T)   // AMD Ryzen 5 5500 != A8-5500 Trinity
func TestRyzen7700NeverResolvesToIntelI77700(t *testing.T) // cross-vendor mismatch
func TestIrisXeNeverResolvesToIris6100(t *testing.T)       // Tiger Lake != Broadwell
func TestUnknownDeviceIDReturnsUnknown(t *testing.T)       // no fuzzy fallback
func TestHybridGraphicsSelectsDiscrete(t *testing.T)
func TestVRAMAbove4GBReadsRegistryNotWMI(t *testing.T)
```

---

## 5. `internal/score` — Tiering

### 5.1 GPU tier

```
FP32_GFLOPS   = shader_units × 2 × boost_clock_GHz
BANDWIDTH_GBs = (bus_width_bits / 8) × effective_mem_clock_GHz

RAW  = (FP32_GFLOPS ^ 0.65) × (BANDWIDTH_GBs ^ 0.35)
PERF = RAW × arch_coefficient
TIER = round( 8 × log2(PERF / BASELINE_PERF) ) + 10
```

### 5.2 CPU tier

```
PERF = phys_cores^0.5 × logical_cores^0.15 × all_core_clock_GHz × ipc_coefficient
TIER = round( 8 × log2(PERF / BASELINE_PERF) ) + 10
```

### 5.3 Invariants

- `BASELINE_PERF` = **GTX 1060 6GB**, permanently tier 10. MUST NOT be re-baselined.
- The `8 ×` factor makes one tier ≈ 9% performance.
- Log scaling is mandatory.
- Coefficients live in `data/coefficients.json`, never hardcoded:

```json
{
  "schema_version": 1,
  "gpu_arch": {
    "nvidia-ampere": {"coefficient": 1.00, "note": "reference"},
    "nvidia-ada":    {"coefficient": 1.12, "source_refs": []},
    "amd-rdna3":     {"coefficient": 0.94, "source_refs": []}
  },
  "cpu_ipc": {}
}
```

### 5.4 Provider interface

```go
type ScoreProvider interface {
    Name() string
    GPUScore(canonicalID string) (Score, Confidence, error)
    CPUScore(canonicalID string) (Score, Confidence, error)
}
```
Ship **only** the `builtin` provider. Do not implement any external provider.

### 5.5 Drift guard

Golden test locking expected tiers for ~30 reference GPUs. Any coefficient change that shifts an unrelated part MUST fail CI.

---

## 6. `internal/reqs` — Requirements Ingestion

### 6.1 Steam client

- `https://store.steampowered.com/api/appdetails?appids=<id>`
- Undocumented, no key. Rate limit ≈ **200 req / 5 min / IP** → token-bucket limiter, honour 429 with exponential backoff
- Name → appid via `GetAppList`, cached locally
- Requirements return as **HTML strings**, not structured fields

### 6.2 Parser (layered, in order)

1. Strip HTML; split on `<strong>` labels: `Processor:`, `Graphics:`, `Memory:`, `Storage:`, `OS:`
2. Regex the well-formed cases: RAM `(\d+)\s*GB`, storage, OS version
3. Split alternatives on `/`, ` or `, `,` — a requirement is a **set**; clearing **any one** member passes
4. Handle modifiers: `"or better"`, `"or equivalent"`, `"minimum"`, embedded `"6GB VRAM"`
5. Anything unparsed → `ConfidenceUnknown`, surfaced in output

### 6.3 Schema (`data/requirements/<appid>.json`)

```json
{
  "appid": 1245620,
  "name": "ELDEN RING",
  "source": "steam",
  "fetched_at": "2026-08-08T00:00:00Z",
  "raw_hash": "sha256:...",
  "normalizer_version": "1.2.0",
  "review_status": "human_verified",
  "tiers": {
    "minimum": {
      "os": [{"family": "windows", "min_version": "10"}],
      "cpu": {"any_of": ["intel-core-i5-8400", "amd-ryzen-3-3300x"]},
      "gpu": {"any_of": ["nvidia-gtx-1060-3gb", "amd-rx-580-4gb"],
              "vram_bytes": 3221225472},
      "ram_bytes": 12884901888,
      "storage_bytes": 64424509440
    },
    "recommended": {}
  }
}
```

`review_status`: `auto` | `human_verified` | `flagged`.

### 6.4 `tools/normalizer` (offline only)

Build-pipeline tool. **The shipped binary MUST NOT make LLM calls.**

- Input: raw Steam requirement blob → Output: strict JSON per §6.3
- Prompt MUST instruct returning `null` over guessing
- **Mandatory validator:** every emitted part name is checked against `gpudb.json`/`cpudb.json`. Unrecognised names → record marked `flagged`, never merged automatically. Unvalidated LLM output MUST NOT enter `data/`.

---

## 7. Verdict Engine

```go
type Status int // Pass, Marginal, Fail, Unknown

type ComponentVerdict struct {
    Component    string  // "gpu" | "cpu" | "ram" | "storage" | "os" | "vram"
    Status       Status
    UserTier     int
    RequiredTier int
    Headroom     float64
    Note         string
}
```

Rules:
- **Hard gates** — RAM, storage (free space, not total), OS, VRAM — are absolute. Below threshold → `Fail`, no ratio.
- **Soft comparisons** — CPU, GPU tier → ratio; `Marginal` within ±10%.
- **VRAM is evaluated separately from GPU tier.** A GPU may clear the tier and still fail on VRAM.
- Output MUST name the bottleneck.
- Any `ConfidenceUnknown` input → `Unknown` verdict for that component.

---

## 8. `internal/proton` — Linux

Evaluation order matters: anti-cheat is checked **first** and overrides everything.

1. **Anti-cheat gate.** Source: AreWeAntiCheatYet / ProtonDB. If blocked, output "No — and no upgrade changes this" **before** any spec comparison.
2. **ProtonDB rating** + known workarounds.
3. **Stack detection:** Mesa version (`glxinfo` or `/sys`), NVIDIA driver version, Vulkan level (`vulkaninfo`), Wayland vs X11, Steam Deck via DMI product name.

Output shape:
```
ELDEN RING on your machine

  Anti-cheat:  ✓ Playable (EAC has Proton support enabled)
  ProtonDB:    Gold (1,247 reports)
  GPU:         ✓ RX 6700 XT — clears recommended (tier 14 vs 11)
  VRAM:        ✓ 12 GB (needs 8 GB)
  CPU:         ⚠ Ryzen 5 3600 — marginal vs recommended
  RAM:         ✓ 32 GB
  Storage:     ✗ 41 GB free, needs 60 GB      ← blocker
  Driver:      Mesa 24.1.2 (RADV) — current

  Verdict: Free up 19 GB and you're good. Expect ~recommended settings.

  Requirements: Steam, fetched 12 days ago
  Tier data:    rigfit v1.2.0 (coefficients v4)
```

---

## 9. Cache & Staleness

| Data | Soft TTL | Hard TTL | On hard expiry |
|---|---|---|---|
| Game requirements | 30 d | 180 d | Refuse; require refresh or `--stale-ok` |
| Steam app list | 7 d | 30 d | Refresh silently |
| ProtonDB ratings | 7 d | 30 d | Report unavailable; do not guess |
| Anti-cheat status | 3 d | 14 d | **Never serve stale** |
| Hardware/tier DB | ships with binary | — | version-locked |

Soft expiry = serve with visible age note. Hard expiry = refuse and explain.

MUST:
1. Every output shows data provenance and age
2. Content-hash the raw requirements blob (`raw_hash`); hash change invalidates and re-normalises, independent of TTL
3. Anti-cheat status never served stale
4. `--offline` serves cache, labelled stale
5. Cache keyed by `normalizer_version`; bumping it invalidates affected entries

---

## 10. Unknown Hardware (MUST NOT dead-end)

Three behaviours are mandatory:

**1. Partial verdict** — report every component that *is* known:
```
  CPU:      ✓ Ryzen 7 7800X3D — clears recommended
  RAM:      ✓ 32 GB
  GPU:      ? Unrecognised device — PCI 10DE:2C05

  Partial verdict: everything except your GPU clears recommended.
```

**2. Override** — `rigfit "Game" --assume-gpu nvidia-rtx-4070`, output clearly labelled assumption-based.

**3. `rigfit report-unknown`** — collects an anonymised hardware dump (no serials, hostnames, MACs, or user paths), writes it locally, prints the path, and prints a **pre-filled GitHub issue URL** with the payload in the body. This is the fixture-collection mechanism; implement it in the first milestone.

---

## 11. CLI Surface

```
rigfit "<game name>"          # primary verdict
rigfit specs                  # detected hardware
rigfit library                # scan installed Steam library
rigfit explain-tier <part-id> # print full tier computation
rigfit report-unknown         # anonymised dump + prefilled issue URL
rigfit dump --anonymous       # raw detection JSON, no identifying data
```

Global flags: `--json` · `--offline` · `--stale-ok` · `--gpu <id>` · `--assume-gpu <id>` · `--explain` (print every network call before making it) · `--no-color`

`--json` output MUST be stable and documented; it is a public contract.

### Library scan constraints
Parse `steamapps/libraryfolders.vdf` and `appmanifest_*.acf`, **read-only**, scoped strictly to Steam directories. MUST NOT walk the wider filesystem. Output: which titles underperform, and which single upgrade unlocks the most.

---

## 12. CI, Release & Distribution Automation

All files in this section are agent-authored. Secrets and repo settings are supplied by the maintainer (§12.7).

### 12.1 Workflow files to create

| File | Trigger | Purpose |
|---|---|---|
| `.github/workflows/ci.yml` | push, PR | test, vet, lint, build matrix, govulncheck |
| `.github/workflows/release.yml` | tag `v*` | GoReleaser, provenance, SBOM, publish |
| `.github/workflows/data-validate.yml` | PR touching `data/**` | schema + canonical-name validation |
| `.github/workflows/aur.yml` | release published | push updated PKGBUILD to AUR |
| `.github/workflows/winget.yml` | release published | open winget-pkgs PR via wingetcreate |
| `.github/dependabot.yml` | weekly | gomod + github-actions updates |

**Every `uses:` MUST be pinned to a full commit SHA with the version in a trailing comment.** Tags are mutable and are a supply-chain risk. Dependabot updates the pins.

```yaml
- uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
```

MUST NOT use self-hosted runners.

### 12.2 `ci.yml`

Jobs:
1. **test** — `ubuntu-latest` and `windows-latest`; `go test ./... -race -coverprofile=coverage.out`
2. **lint** — `go vet ./...`, `golangci-lint run`, `gofmt -l .` (non-empty output fails)
3. **build** — matrix `linux/amd64`, `linux/arm64`, `windows/amd64`, `darwin/arm64`; `CGO_ENABLED=0`, `-trimpath`; verify each binary is produced
4. **vuln** — `govulncheck ./...`
5. **fixtures** — golden-file tests in `testdata/fixtures/` must pass on both OSes

Go version comes from a single source: the `go` directive in `go.mod`, read via `actions/setup-go` with `go-version-file: go.mod`. Do not hardcode it in workflows.

Concurrency group per ref, cancel-in-progress on PRs.

### 12.3 `.goreleaser.yaml`

```yaml
version: 2

before:
  hooks:
    - go mod tidy

builds:
  - id: rigfit
    main: ./cmd/rigfit
    binary: rigfit
    env: [CGO_ENABLED=0]
    flags: [-trimpath]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.CommitDate}}
    goos: [linux, windows, darwin]
    goarch: [amd64, arm64]
    ignore:
      - goos: windows
        goarch: arm64
    mod_timestamp: '{{ .CommitTimestamp }}'   # reproducible builds

archives:
  - formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]                        # zip on Windows; may strip MOTW
    files: [README.md, LICENSE, PRIVACY.md, SECURITY.md]

checksum:
  name_template: 'checksums.txt'

signs:
  - artifacts: checksum
    args: ["--batch", "-u", "{{ .Env.GPG_FINGERPRINT }}",
           "--output", "${signature}", "--detach-sign", "${artifact}"]

sboms:
  - artifacts: archive

scoops:
  - repository:
      owner: OWNER
      name: scoop-rigfit
      branch: main
      token: "{{ .Env.SCOOP_TAP_TOKEN }}"
    homepage: https://rigfit.dev
    description: Correct, local-first game compatibility checker
    license: GPL-3.0-only

release:
  draft: false
  footer: |
    ## Verify this release
    ```
    gh attestation verify rigfit_<version>_<os>_<arch>.tar.gz --repo OWNER/rigfit
    gpg --verify checksums.txt.sig checksums.txt
    ```

changelog:
  use: github
  sort: asc
```

**Do not configure a Homebrew tap.** Targets are Linux and Windows; macOS is best-effort. Add only if macOS demand appears.

### 12.4 `release.yml`

```yaml
permissions:
  contents: write
  id-token: write
  attestations: write
```

Steps, in order:
1. checkout with `fetch-depth: 0` (GoReleaser needs full history)
2. setup-go from `go.mod`
3. import GPG key from `GPG_PRIVATE_KEY` secret
4. run GoReleaser with `SCOOP_TAP_TOKEN` and `GPG_FINGERPRINT` in env
5. `actions/attest-build-provenance` with `subject-path: 'dist/*'`

Provenance MUST be attested after artifacts are built and before the job ends.

### 12.5 `data-validate.yml`

On PRs touching `data/**`:
- JSON schema validation for `requirements/*.json`, `gpudb.json`, `cpudb.json`, `coefficients.json`
- **Reject** any `cpu`/`gpu` `any_of` entry whose canonical ID is absent from `gpudb.json`/`cpudb.json` — this is the guard against hallucinated and typo'd part names
- **Require** a `source_url` field on requirement corrections
- Run the §5.5 tier drift guard: if `coefficients.json` changed, recompute the 30 reference GPU tiers and fail if any *unrelated* part shifted

### 12.6 Distribution automation

**Scoop** — fully automated by GoReleaser (§12.3). The bucket repo is written to on each release and never touched manually.

**AUR** (`aur.yml`, triggers on release published):
- Generate `PKGBUILD` and `.SRCINFO` for package `rigfit-bin` from the release tarball
- Set `pkgver`, `sha256sums` from `checksums.txt`
- Push via SSH using the `AUR_SSH_PRIVATE_KEY` secret
- Use `KSXGitHub/github-actions-deploy-aur` (SHA-pinned) or an equivalent scripted push

**winget** (`winget.yml`, triggers on release published):
- Use `wingetcreate update` to generate the manifest and open a PR against `microsoft/winget-pkgs`
- Package identifier: `OWNER.rigfit`
- **`InstallerType: portable`** — required; the binary is not an installer and portable type avoids executing anything at install time
- Requires the `WINGET_PAT` secret (a PAT on a fork of winget-pkgs)
- This workflow MUST tolerate rejection: a failed winget PR MUST NOT fail the release

**Ordering:** AUR and winget jobs depend on a successful release job and MUST NOT block it. Use `continue-on-error: true` on the distribution jobs.

### 12.7 Maintainer-supplied inputs (agent cannot provide)

Generate a `docs/RELEASE-SETUP.md` documenting exactly these:

| Secret | Where from |
|---|---|
| `GPG_PRIVATE_KEY`, `GPG_FINGERPRINT` | `gpg --full-generate-key`, export armoured private key |
| `SCOOP_TAP_TOKEN` | Fine-grained PAT, scoped **only** to `scoop-rigfit`, Contents: read/write |
| `AUR_SSH_PRIVATE_KEY` | SSH key registered on the AUR account |
| `WINGET_PAT` | Classic PAT with `public_repo`, on a fork of `microsoft/winget-pkgs` |

Repo settings the maintainer applies: branch protection on `main`, require signed commits, enable Dependabot, enable private vulnerability reporting.

### 12.8 Repo files to generate

`README.md` (correctness table, install instructions per channel, verification commands, non-goals) · `SECURITY.md` (canonical-source statement: releases come only from this repo's Releases page and the named package managers) · `PRIVACY.md` · `KNOWN_AV_ISSUES.md` · `CONTRIBUTING.md` · `CODEOWNERS` (`data/coefficients.json` and `internal/score/` require owner review) · `data/METHODOLOGY.md` · `.github/ISSUE_TEMPLATE/hardware-report.yml` (structured template matching `report-unknown` output) · `docs/RELEASE-SETUP.md`

---

## 13. Build Order

**M0 — CI skeleton.** `ci.yml`, `dependabot.yml`, `CODEOWNERS`, licence files with SPDX headers. Do this alongside M1 so tests run from the first commit; leave `release.yml` until M7.

**M1 — `pkg/hwid`.** Types, `pci.ids` parsing, Linux + Windows detection, both Windows traps, `report-unknown`, `dump --anonymous`, regression tests (§4.7). Ships as a standalone release.

**M2 — `internal/detect`.** `rigfit specs` on both platforms; RAM, storage, OS; hybrid graphics verified.

**M3 — `internal/score`.** Tier formula, `dbbuild`, `coefficients.json`, `explain-tier`, drift guard.

**M4 — `internal/reqs`.** Steam client + rate limiter, parser, cache with TTLs, `normalizer` + validator.

**M5 — verdict engine.** End-to-end `rigfit "<game>"`, human + JSON output.

**M6 — `internal/proton`.** Anti-cheat gate, ProtonDB, driver/Vulkan/Deck detection.

**M7 — release automation.** `.goreleaser.yaml`, `release.yml`, provenance attestation, GPG signing, Scoop bucket publishing, `aur.yml`, `docs/RELEASE-SETUP.md`. No Homebrew tap.

**M8 — Windows distribution.** `winget.yml` with `wingetcreate`, `InstallerType: portable`, tolerant of rejection.

**M9 — `rigfit library`.**

---

## 14. Definition of Done (per milestone)

- `go test ./...` passes; `go vet` and `golangci-lint` clean
- `govulncheck` clean
- Cross-compiles to all four targets with `CGO_ENABLED=0`
- No new dependency without justification
- No telemetry; no writes outside the cache directory
- Every new resolution path returns a `Confidence`
- Public API in `pkg/hwid` documented with examples
