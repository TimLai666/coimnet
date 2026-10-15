# Event slope ownership

Baseline 9a399f1 has exactly two production reciprocal-square expressions: LIF.event (lif.go:448) and Mixed.event (mixed.go:277). Root verified the codebase-memory search results against current source. Existing lifForwardTangent also squares the denominator and is excluded as an extreme-value oracle.

Callers are LIF ordinary (lif.go:404), segmented recomputation (lif_segment.go:211), chemical coupling (lif_coupled.go:275), Mixed ordinary (mixed.go:485), and Mixed coupling (mixed_coupled.go:221). Coupled checkpoint recomputation replays forwardLIFCoupled (coupled_recompute_lif.go:285). Persistent inference uses only the hard event comparison and stores no surrogate derivative, so its existing saved-state continuation is a compatibility control.

NewLIF validates a finite positive surrogate scale; Mixed delegates LIF validation. Finite input vectors have no magnitude cap. The rounded denominator is at least one, may be infinity, and its square can overflow before the reciprocal even when the reciprocal square is representable. The exceptional helper is justified for both event functions and leaves the ordinary expression in place.

The public LIFHomeostasis comment and model-and-mechanisms.md:50 incorrectly equate homeostatic detachment with adaptation. Ticket 11 decision 7, ENG's independent tangent policy, LIF/recompute and Mixed backward all preserve the existing adaptation recurrence. Root corrects the wording only; no mechanism or gradient policy is changed.

Read-only inventory worker: native gpt-5.6-luna/max, no Go execution or Git writes. Root separately confirmed the two formula occurrences and the caller source lines. No third production denominator-square expression was found by the scoped code search.
