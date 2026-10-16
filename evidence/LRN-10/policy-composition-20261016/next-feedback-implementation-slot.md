# Ticket132 candidate implementation slot

This is a proposed follow-up fix in the owned temporary directory. Do not change the frozen847-file formal source or call it delivered. Ticket131's full race gate remains live.

Root read the complete simplified candidate tests and fixture patch. An overlay adding only those public tests compiled successfully against the unmodified formal source: six failing terminal events reproduce the forbidden target leak, while the explicitly allowed-score positive control passes. Existing AdaptiveFixture already declares score globally and per adaptation item. The historical basic adaptive test is missing those declarations; its proposed fixture-only patch retains the original learning assertion.

## Slot

Read experiment/adaptive_run.go and write one candidate copy to runs/.tmp/policy-composition-20261016/future-feedback-run.go.txt. This is the only allowed output file. Do not edit any formal code, test, fixture patch, evidence, dependencies, flags or snapshots.

The existing public signature and all declarations remain unchanged. In runAdaptiveEvaluation, preserve reset and OnlineLearner.Act for every item. After Act succeeds and before itemMSE or creating Feedback, continue to the next item unless that item explicitly allows score. AdaptiveEvaluation.Validate has already required every allowed kind to be globally available, so that single permission test enforces both layers without duplicating validation. Reuse the standard slices.Contains operation; no custom helper is needed.

When score is allowed, preserve the existing MSE/clamp, Feedback, Receive and Update path byte for byte. Other declared kinds remain in the report and cannot be silently converted to score. The runner only synthesizes score; do not invent a correctness metric or introduce a new rejection/schema. Add this restriction to the runAdaptiveEvaluation comment. No other behavior or adjacent reset/counting problem belongs to this slot.

## Expected results

- The six forbidden-target regression failures become passes and the existing allowed-score control remains passing.
- A forbidden item still performs normal closed-gate Act, including decay, rather than becoming skipped/frozen.
- A mixed sequence keeps permitted learning and excludes only forbidden feedback.
- The score-enabled path, fixed scorer, parameters, native receptor path and online130 remain unchanged.
- No formal source file changes while the current gate runs; Root later reviews the complete diff and runs both overlay modes plus relevant fixture controls before adopting this candidate in ticket132.

Root alone runs Go and owns the tests. The implementation worker must not modify tests or claim verification. Nil/invalid context and validation behavior outside the feedback permission branch remain in the existing shared code.

Subtraction review: one standard permission check in the existing loop, no feedback registry, callback API, new state or second evaluator.
