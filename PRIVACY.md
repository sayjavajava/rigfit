# Privacy

rigfit is local-first and does not collect telemetry or analytics of any kind (§1, §2 of the build spec — this isn't just a policy choice, it's an explicit constraint: no analytics dependency is permitted to appear in `go.mod` at all).

## What rigfit stores locally

Everything rigfit writes goes to one directory and nowhere else:

- Linux/macOS: `~/.cache/rigfit/`
- Windows: `%LOCALAPPDATA%\rigfit\`

That directory holds: cached game requirements, the Steam search cache, ProtonDB ratings, the AreWeAntiCheatYet dataset, and (only when you explicitly run `rigfit report-unknown`) anonymised hardware dumps. rigfit never writes anywhere else on your system — no registry, no other config directories, no telemetry endpoint.

## What rigfit sends over the network

rigfit only ever talks to three services, and only when actually needed for the command you ran:

- **Steam** (`store.steampowered.com`, for game requirements and search) — sends the game name or Steam appid you asked about.
- **ProtonDB** (Linux only, for compatibility ratings) — sends the Steam appid.
- **AreWeAntiCheatYet** (Linux only, for the anti-cheat compatibility gate) — downloads a public dataset; no per-request data about you is sent.

None of these calls include your hardware identity, an IP-linked account, a device ID, or anything else that identifies you beyond the ordinary network-level information any HTTP request carries (your IP address, as seen by the far end, same as visiting a website). `--offline` disables all network calls entirely.

## What "anonymised" means in `dump --anonymous` and `report-unknown`

Both commands emit only hardware **identity** fields: PCI vendor/device IDs, canonical part names, market names, core counts, clock speeds, VRAM size, and driver version. They never include a serial number, hostname, MAC address, IP address, or any filesystem path. `report-unknown` is entirely opt-in — it's never run automatically, and it shows you the exact file it wrote and the exact GitHub issue URL it built before you'd ever share either.

## Third-party data

The Steam, ProtonDB, and AreWeAntiCheatYet responses rigfit fetches and caches are, obviously, subject to those services' own privacy practices for the request itself — rigfit doesn't control what Valve, ProtonDB, or GitHub (which hosts AreWeAntiCheatYet's dataset) do with an incoming HTTP request. rigfit's own contribution to that request is limited to what's described above.

## Questions

If something here seems inconsistent with what the code actually does, that's a bug in either the code or this document — please open an issue.
