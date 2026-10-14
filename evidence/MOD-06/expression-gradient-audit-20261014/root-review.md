# Root review

Scope: CLEAN. Formal Go source, tests, formulas, dependencies and model formats are unchanged from 318b7518eddef07c9925f7a8b97a3513123786c4. All 806 baseline fingerprints match.

No confirmed findings in this bounded expression-gradient audit. Public PredictAll and LossGradientFrom agree with the independent forward-mode reference for 160 cases in normal and race runs. The matrix has 80 cases with observed distinct regional occupancies, one/two expression nodes, positive/negative/zero scale, clamp/point bounds, implicit/explicit initial activity, controller/neural sources and full/window-1 gradients. Correct-reference maximum absolute gradient difference is 1.11022302e-16 within the established float32 boundary tolerance.

Four deliberately wrong independent references fail on numeric core.weights[0] comparisons: missing average divisor, expression occupancy seed, second-node seed, and clamp derivative guard. These are expected diagnostic failures, not production failures or compiler failures. Original chemistry tests separately pass 100 test events in each normal/race run, including supported continuous/LIF recomputation.

The native review's mixed known/unknown-skipped hypothesis is withdrawn after producer verification: learning/memory.go:15-20 selects one receptor record index; modulation/receptors.go:219-241 assigns one record-level status to every cell of that index. Its denominator cannot combine statuses. All skipped occupancies are zero, so including them gives mean zero and excluding all records also gives zero under learning/memory.go:102-117; neither changes gain 1 or concentration derivatives. No new policy decision or production change is justified by that concern.

Build, vet, module verification and fixed module list are newly run. The preceding full unit/race runs cover exactly the same 806 source files: 58 packages each, receipts and raw log hashes reverified in comparison-summary.json. They are prior executions, not full-suite reruns during this diagnostic audit. This phase adds evidence and documentation only.

Limits: the four-step two-node fixture does not prove real task quality, biological calibration, whole-brain performance or absence of every bug. Existing unresolved Follow-ups are preserved. Avoid adding production validators or changing correct formulas solely because an audit ran.
