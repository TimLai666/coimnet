## Diff Inspector

Scope: CLEAN
Reviewed: main at `67a44556d322777781244ac0465bcfaf154a2a46` plus the complete ticket 34 SDK, new example, tests, documentation and evidence. The worktree was clean at intake. Added model outputs and raw data remain outside Git.
Adversarial review: RUN — source import, full-history learning, strict snapshots, output concurrency and autonomous evaluation span multiple components.

### Findings

No remaining confirmed findings. The independent review and final closeout are preserved separately.

The original optimizer-budget finding is closed by requiring all 144 after optimizer counters to equal 200 and rejecting an open accumulator. The current-row stimulus finding is closed by keeping the recorded raw row at decision zero and clearing only generated raw events. Both regressions failed before repair and passed afterward. The final actual CLI also rejects a forged untrained after snapshot.

The proposed partial-report finding was withdrawn. Its reproduction modified private history after source verification, giving an invalid out-of-arena start that the pinned public CLI cannot import. Valid unsuccessful walks are retained; contract errors and cancellation abort without publishing a partial report. The base-model digest records provenance from prepare. No authenticated-plan requirement was inferred.

### Contract and validation review

- SDK producer and consumers: the optional stimulus collector shares the existing bounded CSV stream and raw-byte SHA verification. Records align by trial/time. Legacy `Read` and `ReadWithReturnTargets` pass nil to that collector and retain their contracts. Root's actual source probe confirms 231,130 identical legacy/new rows.
- Data and learning: all five phases remain in source order; condition and LED establish instrument delivery, scheduled-only events stay in the separate audit. Six input features contain no ID, condition, phase or target. Next displacement appears only in the supervised loss. Invalid shape, ordering, values, missing labels and capacity are rejected. Complete `PredictAll`/`StepFrom` history and an independently referenced early-input gradient pass beyond 256 rows.
- Persistence: strict duplicate/unknown-field checks, required zero-valued contract fields, deterministic exact split reconstruction, matched seed initialization, fixed options/capacity and 200 optimizer steps are verified. Current source and module hashes equal the final full-check hashes.
- Evaluation: one zero-state individual per trial, actual starting observation consumed once, generated rows use zero raw stimulus, shuffled blocks use completed raw past history, and target coordinates affect scoring only. Every one of 24 groups retains all 13 legal trials and 200 decisions. Parameters and source optimizer remain frozen; final process reports match twice and match the pre-review study byte for byte.
- Output safety: existing directories remain untouched. Two concurrent writers yield one complete plan and one rejection. Temporary JSON publication is exclusive, cancellation leaves no bundle, and cleanup preserves unexpected files.
- Resources and dependencies: histories are bounded at 16,384 rows, outputs at 200 decisions and JSON input at 64 MiB. Insyra remains v0.3.4 with no dependency changes. Go build caches were reused after cleanup of only owned/finished transient caches. Full normal verification is fresh; complete race includes 52 validated cached packages and 5 executed packages, with all 3 affected packages separately rerun with `-count=1`.
- Documentation and governance: SDK description, example commands and schemas, engineering decisions, delivery status and requirement evidence match the implementation. All 91 requirement evidence paths exist. The seven handoff originals match their published SHA list. TSK-11 and OPS-05 remain specified at 89/91.
- SQL, authentication and migrations: not applicable to this local example and optional file importer. No database, account, service permissions or migration contract changed.

### Scientific result

Imitation MSE improved, while all model groups hit 0/13 and random control seeds hit 0/13, 0/13 and 1/13. No navigation-learning or biological-mechanism acceptance is claimed. No learning-rate selection or trial exclusion was made from these results.

### Reduction review

The example shares the existing trainer and import/scoring paths. No new reward, RL trainer, sensory simulator or model-product release was added. Further navigation work should first test the frozen model's response to past stimulus interventions rather than add more controls without a decision-relevant reason.
