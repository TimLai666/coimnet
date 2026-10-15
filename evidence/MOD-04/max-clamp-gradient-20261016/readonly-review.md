# Read-only source review

Both Luna reviewers found no confirmed production defect. Root independently checked the actual max selection, strict interior derivative guard, existing direct tests and primary specification 11.4.

MixMax selects the largest raw occupancy before applying signed coefficients; the first positive maximum wins a tie in declaration/record order. Exact clamp endpoints have zero derivative by the explicit BackwardEffects contract, while ClampReport only counts values actually held back. Therefore an exact endpoint need not increment ClampReport. Duplicate declarations contribute once per declared effect in Sum and preserve the first winner in Max.

The main specification requires an explicit mixing choice and does not prescribe weighted max. Ticket21 already records raw-occupancy max; its assertion that equal coefficients always make weighted/raw interpretations identical was wrong for negative scales. The explanation is corrected without changing that contract. Original handoff files stay unchanged.

Existing modulation direct oracles cover max/tie/zero/endpoints and smooth finite differences; the missing combination is signed/clamped learning plus explicit MixMax and recomputation. The public Trainer does not expose recomputed observation/initial-state derivatives, despite one review suggestion. Root checks complete trainable pending sums, snapshots and updates instead, with independent full-history Continuous parameter/input references. No hard-spike finite difference is claimed for LIF.

## Root disposition of script review

The final835 manifest is intentionally pending while the new test writer owns its file. Its absence is not a production P1 or a false success: no engineering success has been claimed. To allow independent public diagnostics, audit-driver now pins original834 source files and the exact path set plus that one new test; regression-driver/engineering require final835. The filled diagnostic does not compile test files. The diagnostic verifier separately checks all resume JSON results and exact key/coordinate sets; the current resume binary exits2 on any failed equality, so the review's hypothetical exit0/false case is not a reproduced failure.

The cleanup gate now explicitly requires both mutations to pass, and archives pin the mutation contents and original module manifest. Baseline834 is the correct count before the new test, not an evidence mismatch. Hash preservation claims concern exact original bytes; no filesystem identity claim is made for arbitrary protected regular files. Owned roots have explicit inode/no-symlink checks. No formal source, model or data mutation is authorized by these scripts.

The matrix count is14 core/profile pairs x2mix x2fill x2window x3segment=336; a review line accidentally wrote an extra leading2 while reporting336. LIF hard-spike finite differences are expressly excluded. Direct modulation hand oracles and existing LIF gradient tests supplement parity; this ticket cannot establish a new independent full LIF surrogate oracle or all-bugs proof. Capped Continuous finite differences use small fixed perturbations and a predeclared tolerance; they are bounded references, not a claim about all nonsmooth points.
