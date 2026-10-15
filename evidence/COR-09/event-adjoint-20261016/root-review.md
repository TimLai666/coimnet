# Diff Inspector

Scope: CLEAN. Reviewed the complete five existing caller diffs, new private event_adjoint.go, all 539 lines of the final test and the directly related ENG/ticket wording. No API, model format, dependency or biological-rule change.

Adversarial review: RUN. The project fallback gpt-5.6-luna/max performed two read-only reviews; Root independently checked all source and runtime evidence. OpenCode model discovery failed with the exact error retained in opencode-models.log. Workers did not run Go or make Git writes.

## Findings

No unresolved confirmed source finding. The original event overflow is reproduced in eight leaf cases across all five implementations, then resolved by the frozen regression in both builds. The first coupled fixture's true gain-gradient overflow was rejected as a test premise error and corrected without weakening the expected core gradients. The first implementation's eight normal ThetaRaw differences per build were rejected; final LIF/recompute keep the normal du immutable, and all 32 normal records now match exactly.

## Contract and behavior checks

- Adaptation retains its existing cross-time path; only homeostasis is detached. Corrected contradictory ticket wording matches ticket 11 decision 7, ENG and the existing independent tangent reference.
- All five callers use the original same-row synaptic/adaptation/voltage values. Held rows still carry synaptic and adaptation recurrence. Truncation and threshold-floor masks keep their existing placement.
- The helper includes only enabled mechanisms. Non-finite relevant original operands stay non-finite; exact final overflow is still rejected. No shared mutable state or new allocation occurs in the normal path.
- Final candidate has 23 passing regression events per build, 32 byte-equal normal records, 32 old-state and 12 old chemical-model continuations. Full engineering acceptance is tracked separately in engineering-verification.json.
- Private smooth fallback and invalid internal operands have source-level review; existing smooth finite-difference and invalid public-input tests run in the full suite. The new tests exercise public behavior with independent rational hand calculations, not a copy of the production reverse loop.

## Limits

Synthetic numerical and compatibility controls cannot establish improved recognition accuracy, complete biological fidelity or absolute absence of bugs. Full engineering must pass before the ticket is closed.

## Subtraction review

One private exceptional arithmetic helper handles all five occurrences. Retain the two explicit normal/recovery ThetaRaw branches because the measured one-unit rounding difference makes that separation necessary. Add no new user settings, clipping, parameter bounds or general arithmetic framework. Remove the owned work tree only after preserving the necessary inputs and evidence.
