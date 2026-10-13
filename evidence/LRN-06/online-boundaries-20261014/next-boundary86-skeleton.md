# Ticket 86: root-owned repair skeleton

Current source remains frozen for ticket 85 full race and delivery. This document authorizes preparation only, not changes to formal Go files.

## Confirmed cause and result

OnlineLearner.Act currently releases Individual.mu between Advance and Snapshot. Outputs can use old parameters while ModelVersion names externally updated parameters. OnlineLearner.apply releases Individual.mu between episodePredict and TrainEpisode. An external update can make the weighted target use a different prediction from the training step. The public v3 diagnostic and its complete-state controls reproduced both defects. Its exact context call counts and runtime stack frames are diagnostic constraints, not a preferred permanent regression seam.

Expected result: each Act uses one parameter version for its output and label. Each weighted episode builds its effective target and runs training while other Individual operations wait. Preserve Individual -> Trainer lock order, public signatures, normal formulas, optimizer counters, accumulation, chemistry, frozen parameters, partial-success retry markers, and existing save formats. A zero score still runs the optimizer and increments the ordinary episode/update counters; it is not a no-op on all optimizer state.

## Implementation slots, after formal RED is frozen

1. Private versioned advance, proposed signature `advanceVersioned(ctx context.Context, input [][]float64) ([][]float64, string, error)`. Acquire Individual.mu once, run the existing advance body using a private held-lock helper, and derive parameterVersion before releasing the lock. OnlineLearner.Act calls this combined operation. Invalid input/cancellation returns an error and does not append an action record. No full snapshot is needed merely to hash parameters.
2. Private weighted training, proposed signature `trainWeightedEpisode(ctx context.Context, input [][]float64, target []float64, score float64) (StepResult, error)`. Acquire Individual.mu once, use the existing prediction, weightedTarget and TrainEpisode logic via held-lock helpers. Ordinary and chemical paths keep their current Trainer lock behavior. Do not reacquire Individual.mu within helpers. A prediction, weighted-target or training error must preserve the corresponding existing state/error contract.
3. Existing public Advance/AdvanceGated and TrainEpisode remain wrappers around their ordinary operation. Only helpers needed by these two combined operations may be extracted. Do not change the mixed retry bookkeeping or unrelated mechanism code.

## Root acceptance checklist

- Deterministic Act interleaving: output and ModelVersion both match the model version actually used, while an external TrainEpisode is queued. Assert public output/hash/version, not a private lock implementation.
- Deterministic weighted interleaving: compare the complete resulting IndividualSnapshot to the allowed serialized operation order. Cover scores 0, .5 and 1. Distinguish fresh zero-gradient behavior from Adam momentum and normal counters. Include ordinary, enabled chemistry, frozen masks and partial accumulation.
- Cancellation/error controls: waiting external operations can cancel without committing; invalid input/target and canceled combined operations preserve the existing pending/report behavior. No leaked goroutines, sleeps used for scheduling, new production hooks or test-only exported API.
- Normal control: the existing 32 public normal cases remain byte-equal, the mixed retry regressions remain green, and separate-process CLI continuation equals uninterrupted training.
- Full acceptance after adoption: gofmt, build, full unit/race, vet, dependencies, governance, root diff review, immutable input/source hashes, temp cleanup, commit/push/readback.

Tests must fail on the source before ticket 86 and pass on the adopted source. A diagnostic PASS that confirms the old defect is not repair acceptance. If a stable test cannot be obtained through existing seams, report the missing seam and a concrete minimal proposal; do not weaken assertions or invent certainty.

## Root test-seam review

The worker's proposed tests calling new private helpers directly are not adopted: a missing-helper compilation failure does not reproduce the defect, and a manually assembled ActionRecord would not verify the real Act wrapper. Its Act snapshot control also needs Advance before external training, because Advance changes persistent neural state.

Use the real public Act/Update/TrainEpisode paths with package-internal synchronization only. Snapshot first initializes the existing Individual mutex token. A test context pauses once at the first Err call that observes this token held. No external mutator starts until that pause, so it identifies the online operation holding Individual.mu without counting unrelated core Err calls. Queue the external TrainEpisode and use a minimal runtime stack probe solely to confirm its unique public wrapper is parked in select; Done evaluation alone is insufficient. Its context blocks on the first post-select Err after acquisition, detected by a Done-evaluated boolean, then resumes when released by the test. Public results and complete snapshots are the oracle, never private helper existence or lock fields.

The correct implementation should return the online operation before the blocked external update is released. Both old and new paths must have an explicit channel completion sequence and bounded deadlock guards, so the old path becomes a meaningful assertion failure rather than a hang. Only synchronization depends on the existing mutex/runtime probe; output, parameter version, optimizer, counters and persistent state are checked through public records/snapshots. This retains a documented small private synchronization seam without production hooks or the old 19/18 global context call counts.
