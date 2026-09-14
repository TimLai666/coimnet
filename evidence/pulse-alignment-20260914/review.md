# Pulse alignment review

Scope: CLEAN. New `signal/pulse.go`, pulse unit tests and Go example, plus README, shared design, ticket 03, delivery status and signal guide. Existing continuous resampling and signal schemas remain unchanged.

Adversarial review: RUN. Spark xhigh returned a quota error; Luna max performed a read-only review of the implementation, stream and Clock contracts, and tests. No confirmed P1/P2 findings. The primary agent independently reviewed the complete implementation and documentation changes.

The initial contract review led to additional coverage of nonconsecutive source sequences, ceil overflow near MaxInt64, and capacity/overflow failures with pending events followed by successful retries. These tests pass; they were coverage improvements, not reproduced implementation defects.

Verified invariants: all accepted events remain separate, output retains source sequence and values, destination steps close using exact 128-bit comparisons, errors return no partial output and commit no state, and payload limits are checked before validation copies. The declared MaxValues formula is a conservative logical budget, not a process memory limit. Calls are serialized by the caller.

Producer/consumer check: PulseAligner uses existing Signal construction and Clock timestamps. Output is accepted by NewObservation in the tests. New SDK entry points are discoverable via Go documentation, a runnable Go example, README and the signal guide. Interval alignment and the multichannel core adapter remain on ticket 03.

Verification: eight pulse tests and one example pass. The frozen 107-file v15 source matches the workspace and both platform manifests. macOS and Ubuntu complete verification scripts, including race, vet, build, module checks and cross-process resume comparison, passed. Windows cross-compilation passed; Windows runtime and GPU training were not tested. See verification.json and ../cpu-reference-20260914/verification-v15.json.

Subtraction review: reuse immutable Signal and the existing stream ordering/validation; no new event envelope, aggregation policy or batch-only API is needed for this slice.
