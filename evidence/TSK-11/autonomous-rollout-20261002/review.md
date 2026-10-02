# Ticket 32 verification and review — 2026-10-02

## Diff Inspector
Scope: CLEAN
Reviewed: owned worktree changes against 94cfb8751145d77da0e971bf27aa2b44515d0eb2, including trajectory metadata, realnav CLI and rollout, tests, documentation and evidence.
Adversarial review: RUN — Trigger: multi-component evaluation and concurrent output creation. Spark was rejected as Unknown model gpt-5.3-codex-spark. Actual collaboration model: gpt-5.6-luna / max. Root reviewed the findings and repairs.

### Findings
No confirmed unresolved findings in the new production rollout.

### Resolved findings
- Extreme finite actions overflowed Hypot and lost direction during clipping; normalization before the norm preserves direction (examples/realnav/rollout.go:642-659; rollout_regression_test.go:171-181).
- Repository/symlink outputs were accepted; the shared guard resolves symlinks and rejects repository ancestors. Exclusive final directory creation admits exactly one concurrent writer (examples/realnav/main.go:406-427; concurrent-output.json).
- Initial hits bypassed context/start/strategy validation; common validation now precedes both early success and simulation (examples/realnav/rollout.go:521-551,588-596; rollout_regression_test.go:126-145).
- The random assertion compared a value to itself; it now compares the engine result to a separately simulated strategy. The reordered-trial comparison remains intact (examples/realnav/rollout_test.go:211-223). Readonly Luna/max final review confirmed the sorted first-trial correspondence and all final checks; rollout.go SHA-256 7e55b6bea46857e028fd126563a0e07a1a9a2d323df0de826c68a794af364245, rollout_test.go SHA-256 c10279d09d0e7b17f058c5d536587f8730e89ab0d39042f159024ce61dc44a27.

### Contract review
Future source rows validate saved split/sample counts; subsequent actions consume generated observations. Snapshot hash identifies the complete neural training snapshot, including optimizer state. Evidence separately fingerprints the full saved-model file. README and ticket clarification resolved the adversarial contract uncertainties without changing model behavior or scoring.

The fixed-action seam is internal test support, absent from CLI strategies. Invalid fixture actions after an initial hit have no CLI impact.

## Test Report ticket32 2026-10-02

### Summary
Nine final verification commands passed. Full normal and race suites each passed 56 packages. The changed tests run with count=1; unchanged packages may reuse Go's valid cache. The earlier executed full race and exact source manifests are retained as first-race.log and first-source-*.json. Three production failure classes were repaired with red/green regressions; one empty assertion was replaced.

### Changes tested
- Metadata invalid-input rejection and compatibility with original Read; target centers stay separate from model inputs.
- Nonzero neural actions match a restored-trainer oracle across the 256-decision reset; complete saved snapshot remains unchanged.
- Generated actions ignore target and future source changes; initial hits, first hits, tangency, zero motion, collisions and extreme finite clipping are covered.
- Strict source/preprocessing/split checks, old snapshot loading, rejected paths and invalid limits, and concurrent single-writer output.
- Two independent real-data train/rollout processes reproduce identical models/results apart from runtime. All four strategies hit 0/13, with no exclusions or initial hits. Independent start and persistence references agree for all 13.
- Handoff originals retain their checksums. Insyra stays at v0.3.4. Current execution is Mac CPU only.

### Issues found (not yet fixed)
- [P2] Legacy infer accepts overlapping train/test IDs in a tampered model. Reproduced in legacy-infer-overlap.json and recorded in AGENTS.md Follow-ups. New rollout rejects overlap. Blocking this ticket: no.

### Regression tests added
- examples/realnav/rollout_regression_test.go: chunk oracle and snapshot freeze, causal action isolation, canceled/invalid initial hit, output path rejection, extreme clipping, first off-origin hit.
- examples/realnav/rollout_test.go: independent random result, strategy/trial isolation, protocol and geometry cases.
- tasks/nav2d/trajectory/return_targets_test.go: compatible import and missing/nonfinite/inconsistent centers.

### Recommendation
SHIP the engineering evaluation. TSK-11 and OPS-05 remain specified. No navigation-capability, device-resident/full-graph, or new remote execution claim.
