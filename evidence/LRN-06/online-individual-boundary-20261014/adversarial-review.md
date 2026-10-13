# Ticket 86 adversarial static review

## Scope

**CLEAN for the requested production repair.** The implementation delta from baseline `5f82637a2c308c19f0278cefc305f93dbe6b01a2` is limited to the three intended learning sources: the Act boundary, the weighted episode boundary, and the two locked-body extractions. There is no public method, serialized field, dependency, optimizer formula, chemistry-training-file, or mixed-retry-marker change.

The worktree also contains root-owned status/docs changes, frozen boundary tests, and evidence/cache artifacts. They are outside this production review and were not modified here. If the acceptance scope is the whole worktree rather than the Ticket 86 implementation, classify those separately as worktree drift.

## Reviewed sources and SHA-256

| Source | SHA-256 | Review focus |
| --- | --- | --- |
| `learning/online.go` | `85dd936bef11d61de51535dd32c0c518cb25da8ee14ee5349c3b12718f21c7ef` | `Act`, `Update/apply`, interval state, mixed marker lifecycle, weighted-target contract |
| `learning/individual.go` | `f5e626c5532df1a95d1abdae3bf836784030fedba8382d63b92f2f1c32bf80e2` | public wrappers and locked advance/training bodies |
| `learning/individual_online.go` | `c59af0997f55c43b84567d891e83ee06fbe916e575920219274abd823627a91d` | versioned advance, weighted prediction/target/training, private lock order |
| `learning/online_individual_boundary_test.go` | `880f2bf18e2f2f03ea83b310363c9bf6dd26997702169b30c76613371d3d6d71` | frozen Act and ordinary/chemistry/mask/accumulation/warm-Adam boundary cases |
| `learning/online_individual_boundary_controls_test.go` | `adec4a483b0a3e849a01f94ef72f9fc2f42bc978c68619378c88606a58b7ed57` | frozen invalid-input, cancellation, retry and waiting-operation controls |

The hashes agree with `evidence/LRN-06/online-individual-boundary-20261014/source-final.json` and `frozen-tests.json`. The checked-in HEAD remains the stated baseline; these files are uncommitted worktree changes.

## Static findings

### CONFIRMED

None. No reproducible P0, P1, or P2 production failure was found in the requested scope.

The relevant invariants are supported by the source:

- `learning/online.go:253-287` now calls `advanceVersioned`; `learning/individual_online.go:11-27` acquires `Individual.mu`, hashes the current parameters before the advance, and runs `advanceRowsLocked` before releasing it. The output and version therefore share one individual boundary, and a hash refusal cannot publish an unlogged advance.
- `learning/online.go:389-441` still commits the gradient interval position and `gradientDone` marker only after successful gradient work, and clears the marker only after successful plastic completion. `learning/individual_online.go:33-50` keeps prediction, weighted-target construction, and training under one `Individual.mu` operation.
- `learning/individual_online.go:56-70` preserves ordinary `Trainer.Predict` and chemistry's `Individual -> Trainer` lock order. `learning/individual_chemical_training.go:10-110` remains the same isolated chemistry boundary and candidate commit path, so persistent neural/chemical/source state is not used as the training candidate.
- `learning/individual.go:462-473` and `833-844` retain the public mutex wrappers and delegate to bodies that do not re-enter the same mutex. Existing error, nil-context, cancellation, overflow and candidate-refusal paths remain in those bodies. `learning/trainer.go:301-573` validates the optimizer candidate before commit.
- `learning/online.go:463-493` only corrects the zero-score explanation. The implementation still allows Adam momentum, weight decay, counters, and partial accumulation to have their existing effects.
- `learning/intervene.go:70-187` continues to call the public advance wrapper and retains its documented non-concurrent-use contract; the extraction does not add a second mutation path.

The independent body comparison in `evidence/LRN-06/online-individual-boundary-20261014/extracted-body-equality.json` reports byte equality for the moved `advanceRows` body (`4719` bytes), `TrainEpisode` body (`365` bytes), and the removed prediction wrapper body (`300` bytes). This is consistent with the diff inspected here.

## NEEDS INVESTIGATION / not verified by this reviewer

- No Go command was run in this review, as required by `review-prompt.txt`. The recorded `green-boundary.json` and `green-race-stability.json` are root-owned readback evidence, not executions performed by this reviewer. Their exit status is recorded as zero, but this report does not independently certify those runs.
- Root still owns the full unit/race completion, 32-case byte comparison, separate-process CLI continuation, build/vet/module/dependency/governance checks, and final cleanup/commit gates. Their completion is required before declaring the ticket delivered.
- Static inspection cannot establish runtime scheduling coverage beyond the frozen tests or discover a defect that depends on an unreviewed external caller. No such caller or concrete failure is visible in the reviewed source inventory.

## Review boundary

Only `runs/.tmp/online-individual-boundary-20261014/review/report.md` was written by this reviewer. No Go source, test, evidence, cache, Git state, commit, stash, or reset was changed.
