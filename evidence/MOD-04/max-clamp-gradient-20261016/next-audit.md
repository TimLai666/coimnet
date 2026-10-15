# Next bounded audit

Ticket120 does not establish an independent full LIF max-mixing surrogate reference. No production defect is confirmed. The next useful check is to extend the existing independent mixed-core reference, rather than create another production implementation or tune the OCR task.

Current source evidence: learning/chemical_mixed_reference_test.go:14-18 explicitly prohibits production dynamics/modulation helpers in its reference. Its fixture at173-196 has one sensitivity receptor and one threshold receptor, so there is no same-kind competition. referenceEffects at1108-1160 sums records per individual receptor and then assigns the cell effect. That helper is valid for its current fixture; merely changing the fixture to overlapping effects would require generalizing the independent helper first. This is a bounded test limitation, not an observed model defect.

Before runtime, define one independent aggregation per effect kind/cell, raw-occupancy max with first-positive tie handling, signed coefficient application, and the declared zero clamp derivative. Add overlapping sensitivity and threshold receptors with identical same-kind coefficients. Use the existing dual-number surrogate rule for the hard event; central finite differences of hard spikes are not an oracle for that rule. Require the original reference cases to stay identical, new full public gradients to match the independent path, effective sum/max controls, and mutations that expose a shared max/clamp error.

Freeze the exact cases and acceptance thresholds before observations. A failing public result needs root-cause evidence before any production repair. Keep the normal model format, five replaceable framework layers and original data unchanged. Any new formal Go tests require the full project gate and owned temporary cleanup.

Subtraction: reuse the independent reference and its existing hand-checked primitives. Add no alternate training API, extra dependency or speculative model mechanism.
