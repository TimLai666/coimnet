# Ticket 42 regression review

Root personally read both test-worker files before implementation and requested missing equal-extreme CTC, temperature-before-normalization, mixed-CE shift and result-overflow cases. Root reviewed the added cases and ran both red passes. The final test paths and hashes are fixed in frozen-tests.json. Implementation workers cannot modify them.

## Expected values

- Equal two-class logits yield probability 0.5 independently of their common magnitude. Uniform teacher KL is zero. For label 1, mixed CE loss is (1-Mix)*ln(2), gradient is (1-Mix)*[0.5,-0.5].
- Distillation reference uses probabilities and their logarithms on small base scores; it does not reconstruct log probability by subtracting a large log-sum-exp. Mapped and partial distributions retain the declared mass.
- CTC reference enumerates all 3^4=81 paths. It collapses consecutive repeats before removing blank, sums only paths matching the target, and computes conditional frame/class occupancy. Both loss and gradients are checked, with finite checks before tolerances.
- CTC equal-score one-frame loss is ln(2), gradient [0.5,-0.5]. Empty, repeated and distinct targets are independently enumerated.
- Extreme normalization and output overflow must return errors and clear partial outputs. ZeroOnImpossible only covers targets whose alignment cannot fit, not numerical errors.
- The Root posterior regression demonstrates that finite outputs alone cannot establish correctness: two frames [0,-1e16] and target [1] returned [[0,-1],[0,-1]]. Each gradient row sums to -1 instead of zero. This violates conditional posterior mass conservation. An explicit precision error is required for that case; supporting arbitrary extreme score differences would require a different precision contract.

## Baseline results

Existing distill and CTC tests passed. The completed worker regression run had 8 failing and 1 passing top-level tests; Root posterior regression adds one failure. Failure logs are red-complete.log and posterior-red.log. The earlier incomplete-coverage red.log is retained, not used as complete acceptance evidence.

Scope is public numerical correctness. These tests add no evidence that a navigation model learned a direction or that a biological mechanism was reproduced.
