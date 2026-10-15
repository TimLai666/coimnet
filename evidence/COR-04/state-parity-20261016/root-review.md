# Root audit review — 2026-10-16

## Result

No new confirmed production defect in this bounded state-continuation audit. Ordinary and race diagnostics are byte-identical. They each cover 96 scenarios, 3,744 continuation boundaries and 80 comparisons after relabeling nodes back to logical order. The comparison includes full persistent state and exact float bits, JSON continuation, caller ownership, raw callback activity, outputs/events and final voltage. All six mechanism/modulation ablations alter output in all 48 fixtures.

## Independent read-only review and Root disposition

LIF and Mixed paths retain the same timing and hidden-state evolution across Forward, Advance and applicable LIF recomputation. Mixed explicitly has no recomputation API (ENG.md); no unsupported path was introduced.

The reviewer identified two weaknesses in the initial diagnostic: relabelings were not compared across fixtures, and the callback previous activity was not checked. Root added numerical-state/output normalization across all six physical permutations and verified each callback row against initial raw activity or the preceding Advance output. Both were reviewed again without confirmed findings. The original ordinary/race logs and source remain as initial evidence; final logs correspond to probe.go.txt. This improves the audit; it is not a framework bug fix.

Continuous/vector read-only review found no confirmed normal-finite-value numerical or history-index defect. Two observations remain separate from confirmed defects:

- Fresh scalar/vector Forward lacks the same trailing context check used by some wrappers (dynamics/continuous.go:324-353, dynamics/vector_core.go:237-271). A Context based only on Err-call counts is not an independent cancellation schedule. Any final check also has a cancellation race before return. A real cancellation-responsiveness claim needs a valid context and a defined observation point; no numerical failure was reproduced or behavior changed here.
- ForwardCoupledFromState calls sequence steps from zero; recomputation offsets are relative to the current complete input sequence, not proof of a persistent lifetime clock (dynamics/coupled_state.go:8-49, dynamics/coupled_recompute.go:10-29). The neural Steps counter alone does not establish the external source clock. Public documentation does not explicitly settle all custom-sequence expectations. No existing supported caller failure was established; do not change model timing based on this observation.

Root inspected the final diagnostic and unchanged core sources, independently ran all runtime checks, verified receipts and positive controls, and accepted the evidence.

## Limits and subtraction review

These three-node public-API consistency checks do not independently establish all dynamic equations, learning gradients, biological validity, whole-brain performance or real-task quality. Full Forward exposes final voltage and output/events; adaptation/homeostasis/refractory are compared through persistent states rather than a new private Forward oracle.

Keep this diagnostic as reproducible evidence rather than expanding production interfaces or adding another framework. All 833 formal source entries remain identical to ticket 116, so its complete engineering validation is reused by exact fingerprint; this round runs new diagnostic ordinary/race/vet and current governance only. No all-suite rerun or quality improvement is claimed.
