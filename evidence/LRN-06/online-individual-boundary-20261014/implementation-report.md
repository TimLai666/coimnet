# Ticket 86 implementation handoff

## Problem

`OnlineLearner.Act` released `Individual.mu` between the ordinary advance and
parameter-version hashing. A due weighted update also released `Individual.mu`
between episode prediction, weighted-target construction and training. These
split operations allowed an external individual operation to publish between
their dependent phases.

## Done

- `Act` now calls `Individual.advanceVersioned`, which derives the parameter
  version before the ordinary advance commits, while one `Individual.mu` lock
  covers both operations.
- Due weighted gradient updates now call `trainWeightedEpisode`, keeping
  `episodePredictLocked`, `weightedTarget` and `trainEpisodeLocked` under one
  `Individual.mu` lock. Ordinary prediction still uses `Trainer.Predict`; the
  chemistry path still takes `Individual.mu` before `Trainer.mu`.
- Public `advanceRows` and `TrainEpisode` wrappers acquire `Individual.mu` and
  delegate to their locked bodies without re-entering the mutex.
- The old lock-taking `episodePredict` wrapper was removed after the live draft
  inventory showed no remaining consumer. The weighted-target comment now states
  that zero score removes only the new residual; momentum, weight decay, update
  counters and partial accumulation can still affect parameters or optimizer
  state.

## Changed files and SHA-256

| File | SHA-256 | Change |
| --- | --- | --- |
| `implementation/online.go.txt` | `85dd936bef11d61de51535dd32c0c518cb25da8ee14ee5349c3b12718f21c7ef` | Atomic Act versioned path, atomic weighted episode call, corrected zero-score comment, removed obsolete wrapper. |
| `implementation/individual.go.txt` | `f5e626c5532df1a95d1abdae3bf836784030fedba8382d63b92f2f1c32bf80e2` | Extracted `advanceRowsLocked` and `trainEpisodeLocked`; public wrappers retain their contracts. |
| `implementation/individual_online.go.txt` | `c59af0997f55c43b84567d891e83ee06fbe916e575920219274abd823627a91d` | Added private versioned advance, weighted episode, and locked prediction helpers. |

## Compatibility basis

- Public method signatures, return values, error text from the existing locked
  bodies, overflow checks, candidate commit behavior, online interval state,
  mixed gradient/plastic marker lifecycle, schemas, and learning formulas are
  unchanged.
- `advanceRowsLocked` body is byte-equal to the original `advanceRows` body
  after the original lock acquisition: 4,718 bytes.
- `trainEpisodeLocked` body is byte-equal to the original `TrainEpisode` body
  after the original lock acquisition: 364 bytes.
- The ordinary prediction and chemistry prediction statements were moved from
  `episodePredict` into `episodePredictLocked`; chemistry retains the
  Individual-to-Trainer lock order.

## Actions

| Action | Target | Result |
| --- | --- | --- |
| `gofmt -w` | The three implementation drafts | Completed successfully. |
| Source/test/cache/Git mutation | None | No formal `learning` source, tests, evidence, cache, or Git state was modified. |

## Verified

- `diff -u` against the current formal `learning/online.go` and
  `learning/individual.go` shows only the listed lock-boundary edits and the
  comment/wrapper changes.
- Static symbol inventory shows `episodePredict` is absent from the three
  implementation drafts and all new private consumers resolve to the new
  helpers.
- No Go command, package build, test, race run, vet, or install-overlay check
  was executed, per the worker boundary. Root owns those checks.

## Notes

The drafts remain `.go.txt` overlays. The installation step must map them to
`learning/online.go`, `learning/individual.go`, and
`learning/individual_online.go` only after the root source-before and SHA gate.

## Next

Root should run the prescribed green overlay test and the full verification
suite, then review the installed diff against these draft hashes.
