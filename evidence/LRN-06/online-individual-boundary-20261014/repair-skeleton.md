# Ticket 86 repair skeleton

Baseline: 5f82637a2c308c19f0278cefc305f93dbe6b01a2. Ticket 85 is delivered; its mixed retry marker is unchanged. This phase repairs both confirmed split-lock operations, not a partial rollback transaction.

## Expected operation and private slots

- Act holds OnlineLearner.mu then Individual.mu. A private advanceVersioned(ctx, input) (output, parameterVersion, error) derives the version and runs the ordinary Advance body within the same Individual lock. Factor only advanceRowsLocked from the existing body. Version hashing precedes any state commit so hash refusal cannot leave an unlogged advance. Public Advance and AdvanceGated retain their ordinary wrappers and result contracts.
- Each due weighted update calls private trainWeightedEpisode(ctx, input, target, score) (StepResult, error). Hold Individual.mu across private episode prediction, weightedTarget and trainEpisodeLocked. Preserve Individual -> Trainer ordering and ordinary/chemical paths. Existing public TrainEpisode acquires its ordinary lock and delegates to the same unchanged body.
- Production responsibility: learning/online.go, learning/individual.go and new learning/individual_online.go only. Remove the old private prediction wrapper only if the live consumer inventory proves this refactor leaves it unused. No public API, schema, dependency, learning formula or plastic retry changes.
- Zero score requires zero residual/gradient on a coherent prediction. Adam momentum, weight decay, update counters and partial accumulation are legitimate optimizer effects; a zero score does not promise zero parameter movement in all prior states. Correct the affected weightedTarget comment accordingly.

## Failure and interaction coverage

| Operation | Error/edge | Expected result | Owner |
| --- | --- | --- | --- |
| Act | invalid/nil/empty input or canceled waiting/context | no action appended; existing state refusal retained | boundary regression |
| version derivation | parameter encoding error | before advance state commit; no action appended | private versioned path review |
| weighted prediction/target | invalid input/target or canceled context | no completed feedback, no interval/phase commit, pending retained | boundary controls |
| weighted training | optimizer/episode overflow or refused candidate | existing atomic refusal and pending feedback retained | existing + boundary controls |
| external individual operation | queued/canceled/multiple callers | waits for complete operation; cancellation commits nothing | boundary controls |
| enabled chemistry/frozen masks/accumulation | independent boundary and prior optimizer state | full serial snapshot equality, not merely norm equality | boundary regression |
| mixed retry | plastic half fails after gradient success | successful gradient runs once, marker/pending lifecycle unchanged | ticket 85 regressions |

## Root-owned verification and test seam

Use public Act/SetTargets/Receive/Update/TrainEpisode and complete public snapshots for result oracles. Existing package-internal mutex token and bounded runtime stack probe are synchronization only, with no production hook, sleep, fixed Err-call count or missing-helper compile RED. Snapshot initializes the token before concurrent use. The online context stops at the first lock-held Err. Start external training only afterward; confirm its wrapper is parked at the existing mutex select before releasing the online context. It blocks at the post-acquisition context check. Correct online operations must finish before this external callback is released. Old split-lock operations fail that return-order assertion. Do not assume Done acknowledgement alone proves queueing or language-level FIFO.

Every started goroutine needs an independent completion channel. Cleanup first releases all pause contexts, then waits for completion even on Fatal paths, without rereading an already-consumed result channel. The old timeout is a failed assertion and must release/join; it is never a PASS. Verify runtime stability before adopting this synchronization.

Cases: Act versus external train; weighted scores 0/.5/1 with ordinary, enabled chemistry, partial accumulation, frozen masks and warmed Adam momentum. Serial controls use weighted-first then external training, and stale-target external-first negative controls where distinguishable. Invalid input/target, nil/canceled contexts, waiting cancellation, error-preserved pending reports and successful retries remain covered.

Baseline checks and RED run before production edits. Root freezes reviewed tests, dispatches implementation without test access, and runs GREEN plus an adversarial review. Keep the 32-case normal byte comparison, existing mixed-retry controls and separate-process CLI continuation. Complete gofmt/build/full unit/full race/vet/module verification/governance, source/receipt fingerprints, owned temp cleanup and commit/push/readback. No new real-task accuracy or biological conclusion is claimed by these engineering checks.
