# Independent acceptance review

The read-only Luna reviewer found no confirmed false-positive acceptance or production defect. Root verified the reported counts, objective and process boundary against the actual sources and logs.

- Recomputed observation/initial-state derivatives are not exposed by the public Trainer API. Parameter accumulation and complete update snapshots are compared; public full-history observation gradients have independent Continuous references. This scope limit was declared before runtime and remains explicit.
- The initial evidence script reported the number of references rejecting a zero derivative without asserting that all 54 did. Root confirmed all 54 in both actual logs, then added an explicit gate and reran the evidence checks in `--check-only` mode. No diagnostic Go source, numeric tolerance or observed result changed.
- No further production repair or unresolved confirmed finding resulted from this review.
