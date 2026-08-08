# Known antivirus / SmartScreen false positives

Go binaries — especially small, statically-linked, unsigned ones from a project too new to have built up reputation with antivirus vendors — are frequently flagged incorrectly. This is a known, common problem across the Go ecosystem, not something specific to rigfit's behaviour. This document exists so you can check here first instead of assuming the worst.

## Why this happens

- **No code-signing certificate.** rigfit's releases are GPG-signed (see `SECURITY.md`) but not signed with a Windows Authenticode certificate — those cost money annually and this is a volunteer open-source project. Unsigned executables get flagged by Windows SmartScreen and some antivirus heuristics regardless of what the code actually does.
- **New/low-reputation binary.** Each release produces a new binary with a new hash. Reputation-based heuristics (which flag "rare" files) will flag every fresh release until enough users have run it.
- **Static linking + `-trimpath` + `-s -w`.** rigfit ships as a single static binary with debug symbols and path information stripped (§2, §12.3) for reproducible, minimal builds. This combination — small, dependency-free, symbol-stripped — overlaps with characteristics some heuristic engines associate with packed/obfuscated malware, even though rigfit does neither (§2 explicitly forbids UPX/packing and obfuscation).
- **Makes outbound network connections.** rigfit talks to Steam, ProtonDB, and AreWeAntiCheatYet (see `PRIVACY.md`). Some heuristic engines flag any unfamiliar binary that makes network calls, regardless of destination.

## What rigfit does NOT do

Worth stating plainly, since this is exactly what a false positive makes you second-guess: rigfit has no telemetry, no auto-updater, no obfuscation, no runtime code generation, no process injection, and writes only to its own cache directory (§2). The source is entirely open — if in doubt, read it, or build it yourself with `go build ./cmd/rigfit`.

## What to do if your antivirus flags rigfit

1. **Verify the binary first** (`SECURITY.md`): checksum, GPG signature, and build provenance attestation all confirm the binary you have matches what this repository's CI actually built. If verification passes, the flag is a false positive.
2. **Report it to your antivirus vendor.** Most vendors have a false-positive submission process; this is the only real fix, since it improves reputation data for everyone, not just you.
3. **Check this file's table below** for a known, previously-reported flag from a specific vendor before assuming you've found a new one.
4. **Open an issue** if you hit a new false positive, so it can be tracked and reported upstream to the vendor. Include the vendor name, the specific detection name it reported, and the release version.

## Reported false positives

| Vendor | Detection name | Release(s) affected | Status |
|---|---|---|---|
| _(none reported yet)_ | | | |

This table is maintainer/contributor-updated as reports come in — it starts empty because no release has shipped yet.
