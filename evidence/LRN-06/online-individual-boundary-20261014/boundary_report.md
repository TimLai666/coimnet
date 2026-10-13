# Online individual boundary tests

## Problem

The overlay draft started `Act`, `Update`, and external `TrainEpisode` goroutines, but a `Fatal` path only opened the pause contexts. It did not independently join every started goroutine, and its weighted interleaving covered only the ordinary fresh optimizer.

## Done

- Added `onlineBoundaryGoroutines`, which registers every started goroutine with an independent completion channel. Cleanup opens all pause contexts first, then joins completions without rereading result channels.
- Kept the bounded `Done`/`Err` queue signal, runtime stack probe, `//go:noinline` wrappers, public `Act`/`Update`/`TrainEpisode` calls, and complete `IndividualSnapshot` comparisons.
- Added an Act table for ordinary and enabled chemistry individuals.
- Expanded weighted interleavings to ordinary, enabled chemistry, frozen per-item masks, `AccumulateSteps=2`, and warmed Adam momentum, each at scores `0`, `.5`, and `1`.
- Chemistry cases use the serial public `Update` control and complete snapshot equality. The ordinary `Network.Predict` helper is used only for ordinary score-zero and stale-target controls.
- Score-zero norm and fresh-optimizer state assertions are limited to fresh optimizer cases. Warmed momentum cases compare the serial snapshot and do not require parameters to remain unchanged.

## Evidence

- Draft: `runs/.tmp/online-individual-boundary-20261014/tests/boundary_draft.go.txt`
- Draft SHA-256: `a32a7ce08d7c986fe528f760e6afa66781d9bc105fa06db7b2ba909c5d7bdce2`
- Static formatting check: `gofmt -d runs/.tmp/online-individual-boundary-20261014/tests/boundary_draft.go.txt` produced no diff.
- No Go command was run by this worker, per the prompt. Root owns RED/GREEN/race/build verification and the overlay execution.

## Not verified

Runtime behavior, baseline RED status, overlay compilation, chemistry fixture validity, and goroutine-leak behavior remain for Root's required Go verification.
