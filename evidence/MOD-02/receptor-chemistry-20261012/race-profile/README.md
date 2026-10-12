# Coupled numerical and allocation checks

Ordinary builds check that optional chemistry adds no allocation to the disabled forward path. Both ordinary and race builds require each coupled core to match its own disabled and neutral reference bit for bit, including gradients. Cross-core comparisons use the existing Mixed/LIF contract: ordinary builds compare bits; race builds use the existing 1e-16-scaled tolerance for compiler contraction. Production arithmetic was unchanged by this test correction.

The race allocation failure also occurs between identical calls to the original core. Mixed and LIF format input labels with `fmt.Sprintf`; its printer cache uses `sync.Pool`. In Go 1.26.5 race builds, `sync.Pool.Put` randomly drops entries. Independent allocation counts therefore cannot be required to match in that build. Only the allocation-count blocks are disabled under race. Continuous allocation checks remain active because that success path does not format labels.

The ordinary `mixed-coupled-normal-green-v5` run contains 100 top-level pass events and no temporary diagnostic probe. The race `mixed-coupled-race-green-v4` run contains 100 pass events, including the probe used to establish the existing rounding and allocation behavior. The probe was removed before the final full-suite run. See the parent `verification.json` for acceptance of the current source.

Earlier green and diagnostic logs are historical investigations, not final-source acceptance. Archived source snapshots retain the original red Mixed comparison, the failed minimum-count attempt, and three diagnostic probe versions. Receipts record the exact command and the source hashes available at each run. Replay of a probe requires copying its text snapshot to the original test path and restoring the relevant archived test versions. The final source manifest is authoritative for the shipped implementation.

`artifacts.json` pins the archive bytes and, for compressed logs, the original event-stream bytes. This evidence establishes software arithmetic and test behavior; it does not measure biological fidelity or model quality.
