# Ticket135 chemical evaluation reset

Root evidence: public RunAdaptiveEvaluation with energy=.75 and an internal-resource source succeeds without reset, but ChemicalAtItemStart=true returns `resource "energy" is not declared` and removes the task resource and queued feedback. A receptor-gated plasticity control unexpectedly succeeds because DisableChemistry refuses and its error is ignored.

Contract: ticket23 says reset chemical state. learning.EnableChemistry and ENG.md explicitly preserve task-declared resources and pending feedback while resetting concentrations and source memory. Disabling the whole mechanism is a different public operation. Preserve fixed/adaptive feedback policies, loss definitions, model format, flags and normal non-reset behavior. Do not add an all-evaluation rollback promise.

One production slot: experiment/adaptive_run.go resetForItem ChemicalAtItemStart branch. Obtain existing snapshot.Config then call EnableChemistry directly; omit DisableChemistry. Existing validated config must serve the same plastic rule and expression. No new API or abstraction.

Test slot: experiment/adaptive_chemical_reset_test.go, external experiment_test package, test-only worker writes candidate text in the assigned scratch file. Public API only. No production edits, no Go execution by workers.

Expected results:
1. Fixed and adaptive evaluations, reset on/off, preserve all declared resources and both available/future queued feedback exactly.
2. Internal-resource release remains usable and matches an independent first-step kinetics value; timeline-only control must not silently clear unrelated resources/feedback.
3. Reset really restarts concentration and source runtime, not merely skips the reset. Warm concentration before test. At neural clock t, a resource source has q=1 and one reset chemistry row yields tau*(1-exp(-DT/tau)), independent of warm concentration.
4. Gate-receptor and expression references do not alter the preservation decision.
5. No reset and chemistry-disabled controls keep existing behavior; missing resource remains a real error, not silently defaulted.
6. Multiple items and full reset shuffle control exercise the shared helper.
7. Root runs tests on unchanged code (red), implementation is supplied by a different worker without editing tests, Root reviews and runs green normal/race.

Acceptance flows: public resource probe and regression; timeline with metadata preservation; normal and old checkpoint compatibility; required full build/test/race/vet/mod verification. Preserve exact source fingerprints and foreign files. Save necessary inputs and logs to evidence/LRN-10/chemical-evaluation-reset-20261017 then delete only this owned scratch directory inode59353824.

Related slow consolidation reset and runtime freeze policy are separate investigations, not silently changed here. Scope reduction: reuse EnableChemistry rather than create a new reset API or duplicate runtime construction.
