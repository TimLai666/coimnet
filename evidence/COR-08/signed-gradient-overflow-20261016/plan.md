# Fixed-sign gradient finite-boundary audit

Status: confirmed P2 defect; independently frozen red regression followed by the common-producer finite guard. Completed 4/4: candidate, complete engineering, final governance, owned cleanup and remote readback passed. The original zero-bug objective remains active.

## Root reproduction skeleton

The shared learning/network.go lossGradientReverse multiplies a finite effective-weight gradient by its fixed-sign effective weight. Its trailing checks cover boundary/input gradients, but do not check that product. Investigate whether a legitimate public scalar trajectory can return Inf with nil error. All public loss/externally seeded/stateful and trainer paths use the same shared reverse producer.

Use a three-node tanh chain, raw log magnitudes 370, DT=1 and tau=0.001 so the membrane leak rounds to zero. Node 0 has a large finite bias and saturates to 1. Node 1 has bias equal to minus its incoming effective weight, so its later drive cancels to zero. Node 2 is the sole input/readout. Four zero input rows and a window of two let the final derivative reach node 1 while stopping before the earlier saturated source. Check direct core gradients first; they must all be finite. The subsequent fixed-sign chain-rule product is the candidate overflow.

## Expected results if confirmed

- Numerical failure returns a zero learning.Gradient plus an error through LossGradient, LossGradientFrom and LossGradientFromState. No caller parameter, saved state or chemical boundary is mutated.
- Trainer failures remain atomic for full/recompute and pending accumulation. Existing numerical safeguards are retained.
- Large but representable chain-rule products and ordinary finite controls remain unchanged. Test both edge signs and upstream signs. Explicit unsupported recomputation modes remain refused.
- Inventory every occurrence of the same chain-rule product before implementation. Minimal production change only; no public fields, API, dependencies or persistence-format changes.

## Test / implementation ownership

Root first runs a public reproduction. If confirmed, one worker writes only learning/signed_gradient_finite_test.go using this expected behavior. Root reviews/runs failing ordinary and race tests, records hashes and freezes them. A different worker may then edit only the verified production producer(s), never tests. Root independently validates the patch, controls, old model/state continuation and all required engineering checks.

## Acceptance and delivery

1. Public reproduction with finite input and direct core reference, true failure signal and preserved arguments.
2. Independent red tests followed by the minimal common-producer fix and unchanged finite controls.
3. Complete build/unit/race/vet, format, pinned-dependency, source/requirement/governance validation if formal Go changes.
4. Preserve necessary evidence, delete only runs/.tmp/signed-gradient-overflow-20261016, verify original models/data/global modules/foreign work, then complete authorized commits/push/readback.

## Limits and subtraction review

No quality improvement is inferred from an artificial numerical case. Do not add a new abstraction or change activation, clipping, masks, parameter constraints or supported modes. If the existing boundaries already reject the candidate before it reaches this product, record that fact and do not patch a speculative bug.
