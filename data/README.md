# data/

## pci.ids

- Source: https://pci-ids.ucw.cz/v2.2/pci.ids
- Retrieved: 2026-08-08
- Upstream version: 2026.08.08 (per file header)
- Licence: dual BSD-3-Clause / GPL-2.0-or-later, as stated in the file header. Vendored verbatim, unmodified.
- Embedded into the `rigfit` binary via `go:embed` in `pkg/hwid`; never fetched at runtime.

To refresh: re-download the file from the source above, verify the header still declares the same dual licence, and replace `data/pci.ids` in place. Record the new retrieval date here.

## gpudb.json / cpudb.json / coefficients.json

Not yet populated — introduced in M3 (`internal/score`). Placeholders are not shipped until real spec-sheet data is sourced.

## Licence

Content under `data/` (excluding vendored third-party files noted above, which retain their own licence) is licensed under CC-BY-SA-4.0. See `data/LICENSE`.
