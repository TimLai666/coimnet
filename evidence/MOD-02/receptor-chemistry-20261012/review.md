# Diff Inspector

Scope: CLEAN. Ticket 65 changes are compared with base commit `0720762ecdcfd12563ed0172b4032ef80ded7816` on `codex/receptor-chemistry-training`.

Reviewed: receptor location and provenance, local concentration selection, optional episode chemistry, continuous/LIF/mixed coupled dynamics, chemical recurrence and controller-window reverse flow, private state ownership, checkpoint presence validation and fresh-process chemical recompute. The existing episode lifecycle is retained. Unsupported GPU/vector/PPO, mixed chemical recompute and stateful chemical recompute combinations reject the declaration before update.

Adversarial review: RUN. The change spans neural, chemical, training and persistence paths. The adversarial reviewer found a representable Hill derivative lost to multiplication underflow. Root reproduced six nonzero cases and retained the ordinary arithmetic while adding a log-domain recovery. Root's independent modulation and integration runs passed after the fix. The dedicated regression source and red/green events are preserved in `extreme-gradient-candidate/`.

## Findings

No unresolved confirmed P0, P1 or P2 finding in this change.

The final test-only correction follows the repository's existing Mixed/LIF compiler-rounding contract. Same-core disabled/neutral outputs and gradients still require exact bits. Ordinary builds retain allocation comparisons; race builds omit only Mixed/LIF allocation-count blocks because the existing fmt cache randomly discards pool entries. Root inspected `fmt/print.go`, `sync/pool.go` and `testing/allocs.go` and reproduced allocation differences between identical original calls. The removed diagnostic source and repeated normal/race events are preserved in `race-profile/`.

## Needs investigation

The existing persistent `Advance` report sums `ReleaseTotal` without a separate finite-total check. This code is present in the base commit and is not read by the new training bridge. The extreme-release reproduction has not been run, so this remains an investigation item in `AGENTS.md`, not a confirmed regression.

## Evidence review

Root independently verified the latest full-suite source manifest against 772 Go/script/module files. Two existing Go main packages under `evidence/`, excluded by the script's manifest but included in `./...`, are pinned in a supplemental source record. The final verification record reports completed runtime commands separately.

Root audited historical candidate hashes. Earlier training candidates differ from the final source in 31 or 32 files. The later extreme-gradient integration and fresh-process receipts differ in only the two allocation/comparison test files. Original receipts are unchanged. `candidate-source-audit.json` records those distinctions; no historical candidate is presented as final acceptance. The source example green receipt also predates formatting of source_test.go; source/README.md identifies the current v2 source execution and preserves the original receipt unchanged.

The normal allocation control reports 100 top-level passes across five tests repeated twenty times, without the removed probe. Current-source full-suite acceptance is in `verification.json`. These are software checks, not measurements of biological fidelity or OCR quality.

## Delegation

Host: Codex. The project-specific fallback used the collaboration agent `chemical_recompute_fresh_process_tests` with `gpt-5.6-luna`, reasoning effort `max`, for read-only source/receipt review. It made no changes or Go calls. Root checked its file counts, hashes and assertions against the actual files.

The OpenCode model-discovery attempt in this turn failed before a model ran:

```text
Error: Unexpected error
Unknown: FileSystem.open (/Users/timlai/.local/share/opencode/log/opencode.log)
```

No successful OpenCode model execution is claimed. The project's explicit fallback configuration takes precedence over the general delegation skill's effort default.
