# Multichannel adapter review

Scope: new `examples/multichannel` against base `2c1524971dd7731ee5278571641e163359107183`, plus related guides, ticket and requirement evidence. Core and signal APIs are unchanged.

Adversarial review: Luna max, after Spark xhigh returned its usage-limit error. Root reviewed the complete implementation, source contracts, tests and documentation. No unresolved confirmed finding remains.

The review identified a handoff mismatch: Signal accepts finite float64 values that learning cannot convert to finite float32. The regression failed for continuous, interval, pulse and finite pulse sums before the fix (`range-red.log`). Adapt now validates final feature magnitudes against MaxFloat32 before returning (`adapter.go:164-172`), after pulse aggregation so huge opposite pulses may cancel to zero with presence=1. Luna rechecked this delta and confirmed it resolved; the root ran regression and full verification.

Compatibility checks cover independent source sequence namespaces, fixed clock rows and six feature columns, missing versus zero, complete Finish output, deterministic pulse sums, input ownership and error returns. The hand-derived matrix produces the same gradient, 50 update results and final training snapshot as the adapter. Inputs and Predict have no target argument; the example supplies its artificial target only to training.

Validation: same 115 Go/module/script files on Mac and Ubuntu in v19, full build, test, race, vet, module verification and cross-process resume comparison passed. Windows crosscompile passed. No Windows runtime, GPU, whole-brain or biological performance claim.

Simplification: use the existing signal and learning APIs with one example-local adapter. No new core adapter registry, shared source-sequence namespace, model artifact or media dependency.
