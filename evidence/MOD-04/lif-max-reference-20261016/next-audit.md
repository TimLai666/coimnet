# Next bounded audit candidate

No new production defect is confirmed by ticket121. The next useful independent check is the slow activity stabiliser's declared stopped-gradient rule, before extending the chemical reference to that mechanism.

The public declaration at dynamics/lif.go:43-54 updates activity estimate r and bounded threshold offset h after the event, uses the previous h for the current threshold, and explicitly stops reverse gradients through h while retaining adaptation. The ordinary reverse at dynamics/lif.go:552-603 and coupled reverse at dynamics/coupled_reverse.go:257-318 carry voltage/adaptation adjoints but no r/h adjoints. Existing lifForwardTangent at dynamics/lif_test.go:207-283 only includes base+adaptation in its threshold. The hand fixture/table at dynamics/lif_test.go:843-933 already exercises the stabiliser's timing, upper limit and decay; reuse it rather than invent a second fixture. Ticket121's chemical reference explicitly refuses enabled homeostasis at learning/chemical_mixed_reference_test.go:644-645.

A minimal next protocol should independently carry r/h primal values and their clamps, but carry zero r/h tangents under the declared learning rule. Compare all weights, bias, tau, threshold, initial-voltage and input derivatives with public Backward at full window, using nonzero upstream. First freeze a case in which incorrectly propagating through r/h would change the gradient. Preserve the existing hand table and all disabled-mechanism results. Only a failing public reproduction with a verified root cause can justify production repair.

This is an unfilled independent-reference check, not an observed model defect or a new requirement that the stabiliser improve task quality. No new ticket or implementation has started. Source fingerprints are in this audit's formal-source.json. Other existing native-plasticity timing questions remain governed by the project Follow-ups and are not resolved by this candidate.

Subtraction: extend the existing forward tangent and hand table; no new training API, parameter field or task tuning.
