# Contributing

Thanks for considering contributing to rigfit. The project's full technical spec lives in `rigfit-BUILD-SPEC.md` — read the relevant section before touching a given package; it's normative (MUST/MUST NOT), not a suggestion.

## The one rule that governs everything

**Unrecognised hardware returns `Unknown`, never a fuzzy guess.** This shows up everywhere: `pkg/hwid` identity resolution, `internal/score` tiering, `internal/reqs` requirement parsing, the verdict engine. If you're tempted to add a fallback that produces a plausible-looking answer for a case rigfit can't actually confirm — don't. Surface it as unrecognised instead. PRs that launder uncertainty into a confident-looking result will be asked to change, regardless of how much better the UX looks in the common case.

## Before you start

- **Hardware/CPU/GPU data changes** (`data/gpudb.json`, `data/cpudb.json`, `data/coefficients.json`): use `tools/dbbuild` rather than hand-editing the JSON — it resolves GPU canonical IDs against the real `pci.ids` data the same way live detection does, so a hand-typed ID can't silently drift out of sync with what `pkg/hwid` would actually report. See `data/METHODOLOGY.md` for how the existing entries and coefficients were derived, and cite your sources the same way.
- **Requirement-parsing changes** (`internal/reqs`): test against real captured Steam HTML, not hand-constructed samples — Steam's actual markup has real quirks (see the `Windows®` trademark-symbol handling in `parse.go`, found by testing against a live response, not guessed).
- **Anything touching `pkg/hwid`**: it's a standalone library and must not import anything from `internal/`. It's also dual-purpose — both the identity resolution used throughout rigfit, and a public API third parties could use directly, so its exported surface should stay minimal and well-documented.
- **Coefficient changes** (`data/coefficients.json`) require owner review (see `CODEOWNERS`) and must not shift an unrelated part's tier — the drift guard (`internal/score`'s `TestTierDriftGuard`, §5.5) enforces this in CI. If you deliberately intend a shift, regenerate the golden file explicitly (the test's doc comment has the command) and explain the shift in your PR description.

## Development

```sh
go build ./...
go test ./...
go vet ./...
golangci-lint run ./...
govulncheck ./...
gofmt -l .          # must be empty
```

All of the above run in CI (`.github/workflows/ci.yml`); a PR won't merge if any of them fail.

- `CGO_ENABLED=0` always — the project ships pure static binaries.
- Every new resolution path (identity, tier, requirement parsing) must return a `Confidence`/`Status` and have a test proving the unrecognised case doesn't silently guess.
- SPDX headers are required in every source file — `GPL-3.0-only` for `cmd/`/`internal/`, `MPL-2.0` for `pkg/hwid/`, `CC-BY-SA-4.0` for `data/`.

## Reporting hardware rigfit doesn't recognise

Run `rigfit report-unknown` — it writes an anonymised hardware dump locally and gives you a pre-filled GitHub issue URL. This is the primary way `pkg/hwid`'s fixture coverage grows; see `testdata/fixtures/` for the existing synthetic set and how golden tests are structured.

## Pull requests

- Keep PRs scoped to one concern; a hardware-data PR and an engine-logic PR should be separate.
- If you're adding a dependency, justify it in the PR description against §2's "minimal dependency tree" constraint and the approved-dependency list — most additions should be stdlib instead.
- Tests are expected alongside behavioural changes, not as a follow-up.

## Security

Don't open a public issue for a vulnerability — see `SECURITY.md` for the private reporting process.
