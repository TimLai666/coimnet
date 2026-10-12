# Extreme receptor-gradient candidate

The existing Hill reverse expression returned zero for six representable, nonzero derivatives. Cases cover tiny positive concentration, multiplication underflow, saturated occupancy, and signed upstream. The test expectations use independent linear, quadratic, and cubic Hill limits; comparisons scale by the expectation so zero cannot pass for tiny slopes.

The candidate retains the ordinary arithmetic and finite-intermediate refusals. If that arithmetic loses a nonzero derivative, the reverse contribution is recovered in the log domain. The full modulation package passed 90 top-level tests after the fix. The selected dynamics and learning integration tests passed 64 top-level tests on the formatted candidate; the integration receipt fingerprints all 191 Go/module files in those three packages. These receipts establish numerical software behavior only. These are historical root-executed checks from ticket 65 implementation. Their original receipts and source hashes remain unchanged. See `../candidate-source-audit.json` for current-source differences and the parent `verification.json` for final acceptance.

The source snapshots are text evidence and are not compiled as additional Go packages. The original command, source hashes, elapsed time, and raw test events are preserved in each receipt and JSONL log.
