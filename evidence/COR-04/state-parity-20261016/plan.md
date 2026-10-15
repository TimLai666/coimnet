# Mixed persistent-state audit

Scope: one bounded audit of the existing Mixed continuation contract. There is no confirmed new bug. Production Go, API, dependencies and persistence schemas remain unchanged unless a concrete counterexample establishes a defect.

## Acceptance flows

1. Construct three-node Mixed graphs with one continuous and two LIF nodes under all six node permutations, both supported activations and fixed ordinary finite parameter seeds. Include delay 0/1/2, self edges, nonzero initial voltage, adaptation, homeostasis and refractory steps. Compare ordinary Forward with one complete Advance.
2. Under nil and nonneutral per-step modulation, compare one 24-step AdvanceModulated with three chunkings (including one step at a time and repeated 2/1/3 boundaries). Compare every output/event float bit and every persistent state field. Compare every boundary to a complete-prefix call. Round-trip states through JSON before continuation. Normalize public outputs and all numerical persistent fields back to logical C/L1/L2 order and compare across all six physical permutations.
3. Fixed external modulation sequence: compare public ForwardCoupled outputs, events and final voltage with AdvanceModulated. Check each callback previous activity against the initial raw output and the preceding Advance output. No activity-dependent reverse approximation or claim about gradients is permitted.

## Checks and evidence

- A diagnostic program lives only in the owned ignored work root and is preserved as probe.go.txt. It calls public APIs. Failures exit nonzero and name the fixture and differing field.
- Snapshot state/parameters/initial/inputs/modulation before calls and verify ownership after calls. Validate every returned state. Float comparisons use math.Float64bits, including signed zero. Mapping, hashes and counters are compared as exact structured values.
- Require nonzero spiking, adaptation, homeostasis, rate and refractory counters in the one-step trajectory. Require modulation and mechanism ablations to change observable outputs in the cohort, to exclude inactive controls.
- Run ordinary and race builds of the diagnostic and compare output receipts. Preserve source, configuration fingerprints, counts, command/environment/log hashes and any failure output.
- The prior immutable 833-entry formal-source manifest is verified before and after. If unchanged, reuse ticket 116 full engineering validation explicitly; do not claim a new full-suite run. If a production defect requires a fix, first freeze independently authored red tests and use a different implementation author, then run required full engineering checks.
- Only runs/.tmp/state-parity-20261016 is owned temporary work. Verify original data/models, foreign files and global modules, preserve the diagnostic source, then delete that work root. Do not alter existing work roots.

## Interpretation

Equality confirms continuation consistency only in these artificial graphs. It is not an independent physiological oracle, a gradient/learning proof, a whole-brain result, a task-quality improvement or proof of zero bugs. Mixed recomputation is explicitly unsupported and is not a defect.

## Subtraction review

Keep the audit outside production sources. Reuse exact-source full validation. Avoid additional abstractions, speculative contract changes and duplicated basic controls. A final context cancellation race or an undefined callback clock is not automatically a confirmed numerical defect.
