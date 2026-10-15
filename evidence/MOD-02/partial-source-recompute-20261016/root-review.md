# Partial chemical history and recomputation review

Result: no confirmed new defect in the audited supported combinations. Formal framework source and tests remain unchanged.

## Scope and evidence

Root read all 1,178 diagnostic lines across `probe/main.go`, `probe/audit.go` and `probe/fixture.go`, then executed the ordinary and race binaries independently. The exact files are fingerprinted in `frozen-diagnostic.json` and retained in `preserved-work.tar.gz`. The implementation worker did not run Go or alter the Root checks. Read-only review qualifications are in `readonly-review.md`; actual dispatch and fallback are in `delegation.json`.

Each build passed 192 distinct cases: two cores, four partial/full history fills, two delays, three reverse truncation windows and four recomputation segment lengths. Each case compared five alternating `StepFrom`/`Step` calls, including two accumulated Adam updates and one pending gradient. Full and recomputed results, normalized snapshots and predictions were bit-identical within that build. Only `Options.Recompute` is normalized. Pending full trainable parameter gradients match the public full-history gradient. The direct gradient-to-zero-initialized-sum comparison accepts both signs of numerical zero; all full/recompute checks retain exact scalar bits.

All 22 mechanism controls changed gradients; 19 also changed outputs. Three LIF source controls leave the discrete outputs unchanged while changing the surrogate gradient. Both caller ownership checks passed. Continuous finite differences passed all 54 parameter/input coordinates using the tolerance fixed before runtime. Every reference rejects a zero derivative. Maximum absolute error was 0.000028308030541879248, below 0.006 of the respective tolerance. LIF spike finite differences are not claimed.

All 192 first-update averaged gradient norms exceed 0.7 in each build (range 1.2404903356397619 to 3.0181671108961843). This uses the sum of the public upstream and MSE gradients divided by two, as the all-trainable accumulator does, rather than the per-call `StepResult.GradientNorm`. The clip condition in `learning/trainer.go:502` is therefore active for these updates.

Each build also restored 16 pending SDK snapshots in separate processes, then matched four further results, complete normalized snapshots and predictions against the full-history reference. These are SDK JSON restore checks; they do not add a bundle format or establish cross-build bitwise equality.

## Pre-runtime corrections

The initial Root fixture draft combined five source kinds with three channels. `modulation/config.go:199-220` requires one indexed source per channel, so Root corrected the fixture to five channels before any run. Overlapping same-kind effects must use identical coefficients (`modulation/effects.go:136-137`), so sensitivity effects uniformly use 0.2/0.02. The review also required explicit final counters and an averaged-gradient clipping observation. These were diagnostic design corrections, with no production failures or weakened comparisons selected from observed errors.

## Engineering applicability

All 834 formal source paths and SHA-256 values match delivered ticket118 exactly. Root rehashed its complete engineering logs and recounted 6,455/6,454 successful test events, with 22/23 skips and no failure. Format, full build, full vet and dependency receipts passed there. This round recompiled and ran the diagnostic both normally and with race detection, ran diagnostic vet, verified pinned Insyra v0.3.4/modules and checked current governance. It did not rerun unchanged full-framework tests.

## Limits and subtraction

This bounded two-node CPU audit adds evidence for a combination previously not exercised together. It does not establish biological calibration, whole-brain task quality, or the absence of every bug. Stateful chemistry recomputation, Mixed/GPU/vector chemical recomputation remain unsupported as documented. No production abstraction, new dependency or speculative repair was added. Retain the small source/input archive and receipts; discard binaries and caches after closure checks.
