# Fixed scoring: implementation boundary

Root reproduced the fixed/scoring defect on the unchanged 846-file formal baseline. Three core kinds, receptor on/off and fast reset on/off all mutate fast state while scoring. The continuous no-receptor retained-fast case changes 0.25 to 0.03125 and changes the last output from the public frozen oracle's 0.9932714700698853 to 0.9910765290260315. With receptor gating it instead writes 1.0377794296456702. This contradicts the main specification 7.2 and ticket23's scoring-only inference contract. Severity P1.

## One implementation slot

Only `learning/individual.go` and `experiment/adaptive_run.go` may change. Tests are Root-owned, already compiled and observed failing. No worker may edit them, metadata, dependencies, snapshots, other source, or protected work.

Add `func (i *Individual) AdvanceFixed(ctx context.Context, input [][]float64) ([][]float64, error)` beside Advance. Reuse the existing locked advance validation, candidate computation and commit. The operation holds all fast state, including eligibility and pair traces, while still integrating base+slow+held-fast and advancing normal neural/chemical state. It must preserve the existing runtime freeze policy on success, refusal and cancellation, and serialize with native advances. No persisted flag or format changes. Nil/zero receiver, nil/canceled context, empty or malformed input return errors and publish nothing.

The smallest safe implementation can hold the individual lock, temporarily set the existing plastic runtime's freeze posture, defer restoring that same flag before unlocking, and invoke the existing locked native advance. This is internal to one locked operation: it must never call the public FreezePlasticity or public Advance while holding the lock, expose the temporary policy to another operation, or add a second stepping engine. The online130 gate-source path and native Advance/AdvanceGated remain unchanged.

`scoreItems` must call AdvanceFixed. Both fixed mode, adaptive scoring, and ShuffleInvariance already share this consumer. Update its comment and the adaptive-scoring comment to describe frozen fast state. Preserve current reset behavior; slow-layer/resource reset scope and adaptation feedback are separate findings, not authorized changes in this slot.

## Acceptance

- Fixed scoring agrees with the established public FreezePlasticity oracle across continuous/LIF/mixed, receptor on/off and reset on/off, including outputs and full snapshots.
- Adding scoring items after the same adaptation does not continue changing fast state.
- Per-call fixed inference preserves the pre-existing runtime freeze policy and later native trajectory.
- Invalid/canceled operations leave the complete snapshot unchanged and do not freeze the next native operation.
- Concurrent fixed/native zero-input calls retain their own fast-state policy.
- Disabled plasticity matches ordinary Advance. Native and online130 regression behavior remains unchanged.

Root runs all Go commands via the owned wrapper, performs full diff and evidence review, then completes project engineering and historical-state checks. No worker commits or invokes Go.

Subtraction review: share the current locked stepping engine and one scorer. Do not add evaluator configuration, another state representation, or a parallel simulation engine.
