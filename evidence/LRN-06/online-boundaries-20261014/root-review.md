# Root diff review

Scope: CLEAN. Baseline `5f26c932981319627e3b76a3e8d47a5ae741c05e`; production changes are limited to `learning/online.go`, SHA-256 `2f742e1c266e018d4964bfb7b92b6eabaa543fb7c4043108ee51dc51ba4dae25`.

The full diff preserves public signatures, persistence schemas, weighted-target arithmetic, interval selection, accumulation and partial-success behavior. The private pair-key marker is written only after successful completion of the gradient half, retained on every plastic error and removed before a fully successful feedback is consumed. Missing targets and legal skips complete the first half too. Independent pending stages survive non-monotonic update timestamps. All accesses are under OnlineLearner.mu, and marker count is bounded by pending feedback. No global state, new dependency or additional lock is introduced.

Root reviewed all three frozen regression sources and the worker's complete production draft before adoption. Runtime RED/GREEN, complete normal-state byte comparison and separate-process CLI continuation supply the independent evidence the static adversarial reviewer could not execute. The adversarial review's runtime uncertainty is resolved by `mixed-retry-green.json`; its production source fingerprint matches the adopted source.

No new confirmed P0/P1/P2 defect was found in this bounded diff. The pre-existing cross-Individual operation defects are separately CONFIRMED and assigned to ticket 86. They are not repaired by the marker. Full unit and race each passed 58 packages without race warnings. Cleanup and delivery remain separate acceptance gates, recorded in their receipts.

Security/authentication/SQL/migration checks are not triggered by this private in-memory learning-state change: no external request, authentication path, database, format migration or permission setting is changed. State lifetime, error paths, cancellation, resources, compatibility and test correctness were reviewed against their concrete producers and consumers.
