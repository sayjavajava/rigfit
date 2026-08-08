# Tiering methodology

This documents how `data/gpudb.json`, `data/cpudb.json`, and `data/coefficients.json` were derived, per rigfit-BUILD-SPEC.md §5. It exists so a contributor can check, challenge, or improve any number here — none of this is meant to be taken as more certain than it is.

## Formulas

See rigfit-BUILD-SPEC.md §5.1/§5.2 for the exact GPU/CPU tier formulas. Both reduce to the same shape: a raw performance estimate from spec-sheet numbers, corrected by an architecture coefficient, compared on a log scale against a fixed baseline part pinned to tier 10.

## Baselines

- **GPU baseline: GeForce GTX 1060 6GB**, per §5.3, permanently tier 10. Its architecture (`nvidia-pascal`) is set as the GPU reference coefficient (1.00) specifically so the baseline doesn't depend on cross-architecture calibration to even bootstrap.
- **CPU baseline: Intel Core i5-8400.** §5.3 only names a GPU baseline explicitly; the CPU formula needs its own anchor in a completely different unit space (core-count/clock/IPC, not GFLOPS/bandwidth), so a second, independent baseline was picked. The i5-8400 was chosen because it's already rigfit-BUILD-SPEC.md's own example minimum-CPU requirement (§6.3), making it a spec-consistent, broadly-recognised choice. Its architecture (`intel-skylake`) is the CPU reference coefficient (1.00) for the same bootstrapping reason.

Both baseline identifiers are Go constants in `internal/score/builtin.go`, not data-file fields — changing which part anchors tier 10 requires a reviewed code change (CODEOWNERS gates `internal/score/`), consistent with §5.3's "MUST NOT be re-baselined."

## Spec-sheet fields (shader_units, boost_clock_ghz, bus_width_bits, effective_mem_clock_ghz, all_core_clock_mhz)

These are manufacturer-published, independently verifiable facts, cross-checked against public spec-sheet sources (Intel ARK, AMD product pages, and widely-syndicated launch specifications) at the time each entry was added. `effective_mem_clock_ghz` is the "Gbps effective" figure spec sheets quote (e.g. GDDR6 "18 Gbps"), not the raw clock — `bus_width_bits / 8 × effective_mem_clock_ghz` reproduces the card's published bandwidth figure directly (verified against GTX 1060 6GB's known 192 GB/s and RX 6950 XT's known 576 GB/s).

`all_core_clock_mhz` (CPU) is the sustained all-core turbo clock under full load, not the single-core max boost spec sheets headline — these can differ by 200-500 MHz on parts with only a few cores, and by more on higher core-count parts. Verified per-CPU where a specific figure was available; treated as an estimate (documented per-entry) otherwise.

## Architecture coefficients

Coefficients correct for the fact that raw GFLOPS/bandwidth (or core-count/clock, for CPUs) don't translate 1:1 into real gaming performance across architecture generations — the same theoretical throughput can perform very differently depending on how efficiently an architecture actually uses it.

**Method:** for a well-known pair of parts from different architectures that launch reviews and/or the vendor's own marketing widely agree perform equivalently in games, solve for the coefficient that makes the engine's own PERF formula produce equal PERF for both parts, chaining forward from the reference architecture (coefficient 1.00). This is the same idea as photographic exposure bracketing — anchor to real, checkable claims rather than inventing a number.

| Architecture | Anchor pair | Basis |
|---|---|---|
| `nvidia-turing` | RTX 2070 ≈ GTX 1080 Ti | Widely repeated 2018 launch-review consensus |
| `nvidia-ampere` | RTX 3070 ≈ RTX 2080 Ti | NVIDIA's own RTX 3070 launch marketing claim |
| `nvidia-ada` | RTX 4070 Ti ≈ RTX 3090 | Commonly cited launch-review comparison |
| `amd-rdna2` | RX 6900 XT ≈ RTX 3080 | Well-known head-to-head launch rivalry |
| `intel-alchemist` | Arc A770 ≈ RTX 3060 Ti | Commonly cited launch-review comparison |

**Known caveat — AMD Infinity Cache:** RDNA2 and later AMD architectures pair a comparatively narrow VRAM bus with a large on-die Infinity Cache that substantially raises *effective* bandwidth above what `bus_width_bits × effective_mem_clock_ghz` alone implies. The tiering formula has no separate term for this, so the `amd-rdna2` coefficient (1.56) is compensating for both true per-FLOP architectural efficiency *and* the untracked Infinity Cache effect combined into one number. This means the coefficient may not transfer cleanly to AMD parts with a very different cache-to-bus-width ratio than the RX 6900 XT/6950 XT it was calibrated from.

**Confidence:** the NVIDIA-generation and AMD-vs-NVIDIA anchor pairs above are high-confidence, extremely widely-repeated claims. CPU IPC coefficients (`amd-zen3`, `amd-zen4`, `amd-piledriver` in `coefficients.json`) are lower-confidence order-of-magnitude estimates based on widely-cited generational IPC claims rather than a solved equal-performance pair, because no equally well-known "these two specific parts perform identically" claim was available among rigfit's small CPU seed set. `amd-piledriver` in particular spans a large, multi-generation architecture gap and should be treated as a rough placeholder.

**All of this is a starting point, not a ground truth.** Real benchmark-sourced calibration (replacing the anchor-pair method with actual measured relative performance across many titles, e.g. an aggregate index) is expected to refine these numbers over time; `source_refs` fields are intentionally empty pending that work, matching rigfit-BUILD-SPEC.md §5.3's own coefficients.json example.

## Coverage

`gpudb.json` seeds ~25 GPUs across 6 architectures (targeting the §5.5 drift guard's "~30 reference GPUs"); `cpudb.json` seeds 5 CPUs across 4 architectures. Full ~300-GPU/~400-CPU coverage (§2A) is maintainer/contributor-sourced and lands incrementally via `tools/dbbuild`, the same seed-then-grow approach `pkg/hwid`'s fixtures and `cpudb.json` identity data followed in earlier milestones.

Several well-known AMD desktop SKUs (RX 6600, 6700 XT, 6800 XT, 7600, 7700 XT, 7800 XT, 7900 XT/XTX) are deliberately **absent** from `gpudb.json`: in the vendored `pci.ids` snapshot, each shares one PCI device ID with two or three sibling SKUs that have different real shader counts (e.g. device `0x73bf` covers the RX 6800, 6800 XT, *and* 6900 XT). Assigning one sibling's spec-sheet numbers to that shared canonical ID would silently misscore the others. Per §1, an identity that can't be pinned to one exact spec sheet is correctly `Unknown` for scoring rather than a guess — these cards still resolve correctly at the `pkg/hwid` identity layer (with a combined market name), they just don't yet have a tier. Disambiguating them (via subsystem-ID-level entries, which pci.ids does carry for specific AIB cards) is future `tools/dbbuild` work.
