# Ticket132 test slot, not part of the current frozen formal source

Public boundary: experiment.RunAdaptiveEvaluation(ctx, individual, declaration) (EvaluationReport, error), using AdaptiveEvaluation.FeedbackAvailable and EvaluationItem.AllowedFeedback. No new API or persisted field is proposed.

Root public normal/race probes already show a legal no-feedback declaration changing fast weights from zero to 2.4606582210702888 for target0.5 and retaining zero for target100. The declarations and observations are otherwise identical. A global score declaration without item permission and a correctness-only declaration behave identically to the explicitly allowed-score control. This is a P1 LRN-10 violation.

Expected behavior: the automatic runner produces score feedback only when score exists globally and is allowed for that adaptation item. Other names are retained as metadata and do not silently become score; do not invent correctness semantics or add a new refusal. An item without usable feedback still takes its ordinary closed-gate OnlineLearner.Act step, including eligibility tracking and natural decay, but its target cannot drive Receive/Update. Fixed scoring and fully allowed score retain their existing intended paths.

Test agent writes only two proposed text artifacts under the owned work root. The formal847 source stays frozen while ticket131's race gate runs. No source or formal test may change yet. Root reviews and later applies the tests after ticket131 delivery; a different implementation agent fills the fix.

Required behavioral checks:

1. Empty global and item feedback declarations: changing only adaptation targets cannot change outputs, neural/chemical/plastic/parameter/optimizer state. Use a nonzero seeded fast state so ordinary Act decay is distinguished from an accidental total freeze or skipped observation.
2. Score globally available but not permitted for the item has the same invariant. Correctness-only and score-globally-available/correctness-only-item also cannot trigger invented score updates.
3. Mixed item permissions: keep the first allowed-score item fixed, change only a following forbidden item's target, and require the same resulting model/output. No blanket disable of adaptation may pass.
4. Explicitly allowed score remains effective: useful and zero scores must produce different fast state and a later output when the fixture supports this. Base parameters remain unchanged.
5. Existing adaptation test that expects learning with empty permission is an invalid fixture for this contract: propose adding explicit score declarations without weakening its state-change assertion. Record the exact old/new lines as a patch rather than editing the formal test now.

Reuse established experiment_test fixtures where possible. No implementation mirrors, unrelated edge suites, dependency changes, timing changes or reset redesign. Each proposed case should expose the target-information leak or guard the required closed-gate/allowed-score behavior. Root runs the original and repaired public flow, normal/race, old state controls and full engineering.
