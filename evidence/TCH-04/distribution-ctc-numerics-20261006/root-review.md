# Root numerical and contract review

Scope: CLEAN. Base: `4c7ff4879c6b9f1db61e9ce8c19d1ca3f90925c1`. Reviewed complete diffs for both implementations and all three new tests. Source identity is frozen in `source-manifest.json`; test identity is in `frozen-tests.json`.

## Correctness and failure handling

- Distribution KL and CE use shifted log probabilities. Temperature is applied after subtracting the maximum. The coefficient for the KL derivative is `Mix*T*Scale`; the loss retains `Mix*T*T*Scale`. Disabled KL skips temperature arithmetic while teacher/alignment validation remains active. Partial mass and declared maps are unchanged.
- CTC normalizes each frame without adding its maximum back. Forward/backward tables preserve unreachable negative infinity. Numeric overflow returns cleared outputs. Posterior occupancy is accumulated once per state and is checked against unit mass with absolute tolerance `1e-8`; the independently reproduced two-frame precision failure returns a numeric error even with `ZeroOnImpossible` enabled.
- Existing `TrainLine` and `TrainUtterance` callers return CTC errors before applying updates. Impossible targets still use the existing explicit skip policy. Distillation callers propagate Loss errors. No model persistence or external writes are introduced by either pure Loss calculation.
- Inputs are not written. New tests compare their original float64 bits on representative success and failure paths. Helpers and allocations are local to each call; no mutable shared state was added.

## Root verification

`green.log`: 10 top-level numerical tests passed. `related-tests.log`: 124 top-level tests passed across distill, CTC, OCR and ASR. `full-test.log`: the complete ordinary suite passed. Build and vet passed. The frozen tests retain their pre-implementation hashes; only the two owned implementation files changed among existing Go files.

`public-probe-green.log` independently confirms uniform distillation loss 0 and single-frame CTC loss ln(2), gradient [0.5,-0.5], both before and after a common shift of 1e16. `gap-probe-red.log` records the old finite but wrong CTC gradient; `gap-probe-green.log` records explicit refusal after the fix.

## Compatibility and limits

Public Loss signatures and options retain their shapes. Normal finite cases, partial top-k, mappings, repeats, empty targets and impossible policies pass their existing and new tests. No dependency, model or task changes are included. This is a bounded float64 correctness fix; the mass guard detects the demonstrated loss of precision and does not prove arbitrary dynamic-range CTC accuracy. The stage provides software evidence, not navigation learning or biological evidence.

No confirmed remaining in-scope defect was found in this Root review. Full race and independent adversarial review have separate receipts; acceptance requires those results before delivery.
