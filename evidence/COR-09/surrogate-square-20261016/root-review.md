# Diff Inspector

Scope: CLEAN. Root read the complete two production diffs, private helper, all 585 frozen test lines, affected mechanism documentation and ENG addition. The sole behavior change recovers the already declared local fast_sigmoid slope when its denominator square overflows. No public API, model format, dependency, parameter bound or learning rule changes.

Adversarial review: RUN. The read-only project fallback gpt-5.6-luna/max independently reviewed the established diff, caller assumptions, rational rounding, underflow bound, test references and public wording. No confirmed unresolved finding was returned. Root separately checked source and runtime records.

## Findings

No unresolved confirmed source finding. The reproduced P2 extreme-value failure is fixed in both event functions. The public one-step positive and negative gradients match an independent rational reference; all 51 frozen events pass in both builds. The 32 ordinary core records are byte-identical, and 32 saved neural continuations plus 12 saved chemical-model continuations remain identical within each build. A chemical continuation includes inference and one actual training update.

## Numerical and state checks

- The finite positive scale contract gives den >= 1. A zero reciprocal can only require recovery after the square overflows; ordinary finite squares retain the original expression and evaluation order. Infinite or NaN denominators preserve the old branch behavior.
- The helper has only local big.Rat state. It rounds the exact reciprocal square of the stored denominator once. For den >= 2^538, the slope is at most 2^-1076 and must round to zero, so the early return avoids unnecessary allocation for true underflow.
- The two-row fixture cancels synaptic carry with an explicit -5e307 upstream at row zero. This isolates the membrane path rather than silently omitting a large derivative. Root requested this correction before freezing and before any production edit.
- Direct tests compare subnormal slopes by Float64bits, including both sides of the last-nonzero neighborhood. Normal slopes retain the old expression. Public checks cover LIF ordinary, both segment sizes, coupled replay, all-LIF Mixed and a genuine continuous-plus-LIF model. Existing smooth, refractory, adaptation and truncation tests remain unchanged in the full suite.
- Homeostasis remains detached and adaptation keeps its existing cross-time derivative. The public type comment, mechanism guide and individual-state guide now match the implementation and ticket 11. Root's related-wording search found the third mismatch in individual-state.md:15; it is corrected without changing state or gradient behavior.

## Limits

Runtime verification is on macOS arm64. These numerical and compatibility fixtures do not establish a real-task accuracy increase, complete biological fidelity, an arbitrary-precision global derivative or the absolute absence of bugs. Full engineering acceptance and delivery are tracked separately.

## Subtraction review

Share only the exceptional reciprocal-square helper. Keep normal arithmetic in place and reuse the existing coupled fixtures. The true-underflow early return removes unnecessary big-number work. No user setting, clipping policy or generic arithmetic framework is added. Remove the owned work directory after preserving necessary evidence.
