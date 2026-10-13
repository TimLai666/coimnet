# Ticket 86 boundary controls

## Problem

The repaired online/individual lock boundary needs compact public-behaviour
controls for rejected input and targets, nil and canceled contexts, an external
`TrainEpisode` canceled while an online update owns the individual lock, and a
failed `Update` whose feedback and interval position must remain retryable.

## Done

`boundary_controls.go.txt` contains three self-contained tests with the
`TestOnlineIndividualBoundaryControl` prefix:

- invalid empty or wrong-width action input preserves the empty public log and
  complete `IndividualSnapshot`; wrong-width and non-finite targets are refused
  without changing that evidence, then a corrected target completes one public
  gradient update;
- canceled `Act`, nil or canceled `Receive`/`Update`, and nil or canceled
  `TrainEpisode` return errors without state publication; an `UpdateEvery=2`
  interval skip followed by a canceled update leaves a zero report and pending
  feedback, and one background retry consumes exactly one gradient update;
- a public online `Update` is paused after acquiring the existing
  `Individual.mu` token, an external public `TrainEpisode` reaches the existing
  cancellable mutex select, and cancellation returns `context.Canceled` with a
  zero `StepResult`; after the online operation completes, the victim's full
  snapshot equals a serial public control, proving the canceled caller did not
  commit state.

All started goroutines have independent completion channels. Deferred cleanup
opens every pause and cancels the external context before joining, and never
reads a result channel a second time. Synchronization uses the existing private
mutex token and context `Done` acknowledgement only; no production hook, sleep,
fixed `Err` call count, or manual action oracle is used.

## Changed files and SHA256

| File | SHA256 | Change |
| --- | --- | --- |
| `runs/.tmp/online-individual-boundary-20261014/tests/boundary_controls.go.txt` | `49d070689d7d6b5ef129e800c07c0f3d79251d4a9331bc766aaa271e64145c1a` | Added the controls above. |
| `runs/.tmp/online-individual-boundary-20261014/tests/controls_report.md` | reported to Root after save | Added this evidence summary. |

## Evidence and basis

The controls were written against `learning/online.go`, `learning/individual.go`,
`learning/trainer.go`'s existing `cancellableMutex`, the ticket 86 skeleton,
`trainer_cancel_test.go`, and the existing online retry tests. Assertions use
public `ActionRecord`, `OnlineLearner.Log`, `UpdateReport`, `StepResult`, and
complete `IndividualSnapshot` values. `errors.Is` is used for
`context.Canceled`; validation cases require a non-nil explicit operation
error.

## Uncertainty and not run

- Go compilation and all Go tests were intentionally not run, as required by
  the worker prompt. Root must run the prescribed overlay command and resolve
  any integration or implementation RED result.
- This file does not duplicate the other worker's score, chemistry, frozen-mask,
  accumulation, warmed-Adam, or broad lock-order matrix. Those controls remain
  outside this worker's scope.
- The target file is an overlay source ending in `.go.txt`; its package and
  helper names are self-contained but still require Root's `overlay.json`
  mapping for Go execution.

## Next

Root should run the required red overlay test, review any failure against the
public contract, then run the green and full verification suites.
