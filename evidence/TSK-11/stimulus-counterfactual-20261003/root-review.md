## Diff Inspector

Scope: CLEAN.
Reviewed: ticket 35; new counterfactual implementation and tests; existing CLI and rollout diff; source/targets/bundle producers; report and evidence consumers; root and example README, ENG, INDEX, delivery status and requirement evidence.
Adversarial review: RUN — the change introduces a paired evaluation command and touches causal history/rollout integration. Spark returned `Unknown model gpt-5.3-codex-spark`; the actual reviewer used gpt-5.6-luna / max through collaboration, read-only.

### Findings

No confirmed findings. The baseline review's cutoff and shuffled-schedule integration risks are resolved by the final implementation and focused regressions. The original path is unchanged with a zero clear boundary; all 234 legacy complete trace fingerprints and summaries match. Both paired future-stimulus inputs remain exactly equal.

The reviewer noted a possible future change to inference optimizer behavior. It is not a present failure: NewIndividual constructs an inference individual, only Advance is called, and Advance's current contract changes neural state only. The saved TrainingSnapshot is never restored for updates or passed to a mutating trainer; its entire bundle hash is asserted unchanged, including the original optimizer, and the raw bundle file is independently pinned before and after each run. No unsupported optimizer-restoration claim is made.

### Compatibility and safety

The new versioned report is separate from existing snapshots and reports. The old prepare/train/infer/rollout entries retain their contracts. Public input remains source-pinned and bundle-validated before publication. Output creation and writing reuse exclusive paths with owned-output cleanup; real concurrent attempts at an existing output are refused. No SQL, remote authentication, migration, external transmission or dependency change is involved.

## Test Report

Verification commands: 8 passed, 0 failed. Full normal: 57 packages without cache. Full race: 57 packages, 51 cached; changed example also passed fresh race. Source fingerprint stayed unchanged throughout these checks. CLI acceptance covers successful paired runs, help, invalid arguments, wrong input/model, existing/concurrent output refusal and SIGTERM cancellation without output.

Regression coverage demonstrates an independently calculated continuous-state decay, zero-readout and no-stimulus equality, full snapshot immutability, invalid source relocation cutoffs, non-aliasing inputs and positive delayed shuffled continuation. The shuffled fixture detects the erroneous erase-before-transform implementation. Paired real evaluation verifies 234 pairs, zero exclusions, old behavior equality and byte-identical independent reports. Recommendation: SHIP.

## Subtraction review

Reuse the original model, control transforms and evaluator; omit full repeated stdout copies and retain one report plus independent hashes. No new training, control group, tunable threshold, reward signal or biological interpretation is needed for the stated check. Do not treat response to erased history as a reason to lengthen imitation training without a navigation acceptance criterion.
