# Root review

Scope: CLEAN. Reviewed the complete three-line production diff in `learning/network.go`, its shared reverse callers, the frozen 513-line regression test, and actual red/green command receipts.

The confirmed P2 defect was a non-finite fixed-sign chain-rule product escaping the public gradient APIs with a nil error. The patch checks that exact product immediately and returns the existing empty gradient. All three public entry points propagate the failure, including zero loss for the loss-based call. This is a genuinely unrepresentable final derivative, as shown by the independent decimal product reference, rather than an avoidable intermediate overflow.

The multiplication order and all representable values are unchanged. No field, activation, mask, clipping rule, persistence format, dependency, supported-mode boundary or parameter constraint changes. The new check is shared by ordinary, stateful and recomputed reverse paths. Existing active-gradient Trainer norm rejection remains; this report does not claim the old Trainer silently committed invalid active gradients.

Root reviewed the independent tests before implementation. Ordinary and race red each had 17 failed parent/leaf events and 55 passes, including 12 failing public-API leaves. Root then froze the test hash. After the separate implementation worker's patch, both runs have 72 passing events. Trainer pending accumulation and argument/state ownership controls pass before and after the fix.

Root independently repeated the public probe: three nonrepresentable derivatives now fail with zero gradient, and both finite controls are unchanged. Within each build, all 16 normal core records, 16 saved-state continuations and six old chemical model continuations with a subsequent update are identical. Across ordinary/race this totals 32, 32 and 12 respectively; it is not a cross-build equality claim.

The user's neutral-gain compatibility decision is already adopted in ticket 89. Its production rule remains `0 < GammaMin <= 1 <= GammaMax` after zero defaults. This ticket does not repeat or reverse that authorized change; the complete suite must also retain both approved neutral-gain regression checks.

No remaining confirmed finding in the three-line patch. Full engineering, final governance, owned temporary cleanup and remote delivery are separate acceptance gates and are recorded only after execution. Artificial numerical controls do not establish real-task quality, biological calibration or absolute absence of bugs.

Subtraction review: keep the guard at the existing common producer; do not add a second validation layer, helper abstraction or migration path. Preserve the frozen test and old model inputs.
