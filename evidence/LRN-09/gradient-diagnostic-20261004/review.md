# Fixed PPO gradient diagnostic review

Scope: CLEAN. The change adds two experiment test files, pinned synthetic diagnostic evidence, its independent Python validator and project documentation. Production Go code, dependencies, training settings, prior evidence and model behavior are unchanged.

The main agent reviewed both complete Go test files and the Python validator, checked the caller and optimizer contracts, and ran the numerical, replay, mutation and full repository checks. The adversarial reviewer was read-only. Requested `gpt-5.3-codex-spark/xhigh` was rejected with `Unknown model`; the project-directed fallback was `gpt-5.6-luna/max`.

## Findings and repairs

The first review confirmed two P2 validator gaps: the initial model was not bound to the source's first snapshot, and a swap of two parameter-delta coordinates preserved the norm and was accepted. Both regressions failed before the repair (`coordinate-red.log`). The repaired validator binds initial-model fingerprints to the pinned source and replays clipping and Adam for every coordinate. Both cases pass in `coordinate-green.log` and the final 17-case suite.

The final read-only review ran `verify_report.py` and `verify_report_test.py`, confirmed 600 updates and 17/17 mutation cases, and reported no remaining confirmed P0, P1 or P2 findings for the pinned artifact and documented entry point. The main agent independently ran the same final commands and verified their logs.

## Method limits

Python independently recomputes output derivatives, source PPOReport scalar values, all gradient-array statistics, optimizer norms and coordinate deltas. It does not reconstruct neural forward or VJP. A coordinated permutation of all gradient arrays and corresponding Adam deltas can pass a direct call to the internal `validate()` helper; the documented entry point also requires the pinned whole-report SHA, so such an altered artifact is rejected. The Go checks supply seeded-model replay, finite differences and exact comparisons against the original update path.

The fixed model uses one epoch, one rollout, no accumulation, sharing, schedule, projection or weight decay. This evidence does not establish behavior under those other settings, a unique long-term cause of the failed cue task, or a biological mechanism.

## Scope reduction

The original rollouts and PPOReport stay in the pinned ticket 38 source. The new report stores full gradients and actual deltas, and its validator recomputes the source scalar report rather than duplicating it. No new CLI or training mechanism is needed.
