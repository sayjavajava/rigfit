# Security policy

## Canonical source

Official `rigfit` builds come from exactly two places:

1. This repository's [GitHub Releases](https://github.com/sayjavajava/rigfit/releases) page.
2. The package managers this project publishes to directly: Scoop (`scoop-rigfit` bucket), the AUR (`rigfit-bin`), and winget (`sayjavajava.rigfit`).

Every one of those is populated by the same automated pipeline (`.github/workflows/release.yml` → `.goreleaser.yaml`), never a manual upload. rigfit is not published anywhere else — no third-party download sites, no unofficial mirrors, no browser extension stores. If you found a `rigfit` binary somewhere other than the two sources above, it did not come from this project, and you should not trust it.

## Verifying a release

Every release includes `checksums.txt`, a detached GPG signature (`checksums.txt.sig`), and build provenance attestation.

```sh
# Verify the checksums file itself was signed by the maintainer's key
gpg --verify checksums.txt.sig checksums.txt

# Verify a specific archive matches its checksum
sha256sum --check checksums.txt --ignore-missing

# Verify GitHub Actions build provenance (requires the gh CLI)
gh attestation verify rigfit_<version>_<os>_<arch>.tar.gz --repo sayjavajava/rigfit
```

The maintainer's GPG fingerprint used to sign releases is documented in `docs/RELEASE-SETUP.md` and should also be published somewhere independently verifiable (a keyserver, or a signed statement linked from the README) — don't trust a fingerprint that only appears bundled with the thing it's meant to verify.

## Reporting a vulnerability

Please use GitHub's [private vulnerability reporting](https://github.com/sayjavajava/rigfit/security/advisories/new) for this repository rather than opening a public issue. This lets us assess and patch before the details are public.

If private reporting isn't available for some reason, open an issue that says only "possible security issue, will follow up privately" with no details, and we'll reach out for a secure channel.

## What's in scope

- The `rigfit` binary itself: identity resolution (`pkg/hwid`), tiering (`internal/score`), requirements parsing (`internal/reqs`), and anything that touches the network (Steam, ProtonDB, AreWeAntiCheatYet) or the filesystem (the cache directory, §2).
- The release pipeline: anything that could let a malicious release reach a user through the canonical channels listed above.

## What's out of scope

- Vulnerabilities requiring you to already have modified the source and be running your own build.
- Denial-of-service reports against the third-party APIs rigfit calls (Steam, ProtonDB, AreWeAntiCheatYet) — that's between you and them.

See also `KNOWN_AV_ISSUES.md` for why antivirus software sometimes flags rigfit incorrectly — that's a false-positive tracking document, not a vulnerability report.
