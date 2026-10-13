# Ticket 86 Root review

Scope: CLEAN. The production delta is the Act version/advance operation, the weighted prediction/training operation, and private helpers extracted from existing locked bodies. Five frozen source/test hashes and all 788 final source hashes were checked against disk. Existing tests remain unchanged.

## Confirmed repair

- `learning/online.go:267` invokes one versioned advance. `learning/individual_online.go:11` hashes parameters before advance publication and owns the individual lock through completion. Individual owns its isolated trainer (`learning/individual.go:129`). External public training therefore cannot change the parameters between answer computation and its recorded version.
- `learning/online.go:402` invokes one weighted episode. `learning/individual_online.go:33` holds the same individual lock through prediction, target construction and candidate training. Private ordinary/chemical prediction preserves the Individual -> Trainer lock order.
- `learning/individual_chemical_training.go:13` copies the episode boundary, while `:74` validates and publishes only the private candidate. Persistent chemical/source/neural state is retained. The static review was checked against these actual definitions rather than stale graph snippets.
- Existing public Advance/TrainEpisode bodies are byte equal after extraction; `extracted-body-equality.json` gives their lengths and hashes. Public signatures, optimizer formulas, checkpoint fields and dependencies are unchanged. Mixed gradientDone/eligible/pending lifecycle remains as delivered by ticket 85.

## Independent runtime evidence

Formal tests were Root reviewed and frozen before production repair. Baseline compiled and failed 17 behavioral leaves; the three refusal/cancellation controls passed. Fixed code passed five top-level tests / 31 pass events and 20 race repetitions / 620 pass events. The 32-case public API matrix and full snapshots are byte equal before/after. Separate-process delayed 80+40 and continuous 120 checkpoints are byte equal. These are root-run checks, not acceptance of worker claims. Full race and final governance/delivery gates remain separate prerequisites.

## Review disposition

No new confirmed P0/P1/P2 failure was found in this bounded repair. The adversarial review only wrote its permitted report and its five hashes match the reviewed source. No auth, migration, remote environment or new dependency change is present. This is not a zero-bug proof, new biological evidence, whole-brain validation or real OCR quality result. Large fixture skips remain disclosed in full-suite summaries.

## Subtraction review

The Act path no longer deep-copies an entire snapshot to hash its parameters. Weighted training uses existing formulas and locked bodies instead of a new public transaction API. Batch rollback and mechanism redesign remain outside this repair.
