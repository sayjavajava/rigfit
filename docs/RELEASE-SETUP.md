# Release setup

The release automation in this repo (`.goreleaser.yaml`, `.github/workflows/release.yml`, `aur.yml`, `winget.yml`) is agent-authored, but it can't provision secrets or repository settings on your behalf. This document is the checklist for what a maintainer must supply before `git push --tags v1.0.0` produces a real, signed release.

## Secrets

Add these under **Settings → Secrets and variables → Actions** on the repository.

| Secret | Used by | Where it comes from |
|---|---|---|
| `GPG_PRIVATE_KEY` | `release.yml` | `gpg --full-generate-key` (RSA 4096 or ed25519 recommended), then export the armoured private key: `gpg --export-secret-keys --armor <KEY_ID>`. Paste the full `-----BEGIN PGP PRIVATE KEY BLOCK-----` output. |
| `GPG_FINGERPRINT` | `release.yml`, `.goreleaser.yaml` | `gpg --list-secret-keys --with-colons \| grep fpr` — the 40-character fingerprint of the same key. This is not resolved automatically from `GPG_PRIVATE_KEY`; the workflow's GPG-import step also exposes it as a step output, but the secret is kept as the explicit source of truth so `.goreleaser.yaml`'s `signs` block has a stable value. |
| `SCOOP_TAP_TOKEN` | `.goreleaser.yaml` (`scoops`) | A **fine-grained** GitHub PAT scoped **only** to the `scoop-rigfit` repository, with Contents: read/write. Do not use a classic PAT with broader repo access. |
| `AUR_SSH_PRIVATE_KEY` | `aur.yml` | An SSH key pair registered on your AUR account (Account → My Account → SSH Public Key). Paste the private half here. |
| `WINGET_PAT` | `winget.yml` | A **classic** GitHub PAT with the `public_repo` scope, created on an account that has a fork of `microsoft/winget-pkgs`. `wingetcreate` opens PRs against the upstream repo from that fork. |

`GITHUB_TOKEN` is provided automatically by Actions and needs no setup.

## First winget submission (one-time, manual)

`winget.yml` runs `wingetcreate update`, which bumps an **existing** manifest's version/URL/checksum — it can't create the package's first-ever manifest, and critically, it has no flag to set `InstallerType` (verified against `wingetcreate`'s own docs: `update`'s only per-run overrides are architecture/scope). `InstallerType: portable` (§12.6 — required, since the binary is not an installer and running an actual installer at `winget install` time is exactly what portable type avoids) has to be set correctly the first time, by hand, before any automated `update` can run:

1. After your first tagged release exists, install `wingetcreate` locally (`winget install wingetcreate` or download from https://aka.ms/wingetcreate/latest).
2. Run it interactively: `wingetcreate new`, package identifier `sayjavajava.rigfit`, installer URL the release's `rigfit_<version>_windows_amd64.zip`. When prompted for installer type, choose **portable**.
3. Submit that PR yourself (or let the interactive flow do it) against `microsoft/winget-pkgs`, and get it merged.
4. From the next release onward, `winget.yml` automation takes over — `update` preserves the `portable` installer type from the manifest it's updating.

## Repository settings

Apply these under **Settings** on the repository itself — none of this can be done from a workflow file:

- **Branch protection on `main`**: require pull request review before merging, require status checks (the `ci.yml` jobs) to pass before merging.
- **Require signed commits** on `main`.
- **Enable Dependabot** (alerts + security updates) — `.github/dependabot.yml` already configures version-update PRs; this setting is the separate security-alert toggle.
- **Enable private vulnerability reporting** (Settings → Security → Private vulnerability reporting), so `SECURITY.md`'s reporting instructions actually work.

## First release checklist

1. All five secrets above are set.
2. The repository settings above are applied.
3. `scoop-rigfit` exists as an empty repository the `SCOOP_TAP_TOKEN` PAT can write to — GoReleaser pushes the generated manifest there, it doesn't create the repository itself.
4. Your GPG public key is published somewhere verifiable (a keyserver, or linked from `README.md`) so `gpg --verify checksums.txt.sig checksums.txt` is actually checkable by someone who doesn't already trust you.
5. Tag and push: `git tag v0.1.0 && git push origin v0.1.0`. `release.yml` triggers on any `v*` tag.
6. Watch the `release` job; `aur` and `winget` run afterward and are allowed to fail without affecting the GitHub release itself (`continue-on-error: true`) — check their logs separately.
7. `winget` won't have anything to update until the one-time manual submission above has been merged into `microsoft/winget-pkgs` — until then, expect it to fail every release, harmlessly, and that's expected rather than a sign something's broken.

## Rotating a compromised secret

Revoke the exposed credential at its source first (GitHub PAT settings, or `gpg --revoke` for a compromised key), *then* replace the repository secret — in that order, so there's no window where the old credential is deleted from Actions but still valid for direct use elsewhere.
