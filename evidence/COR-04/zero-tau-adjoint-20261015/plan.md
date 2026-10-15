# Zero membrane-time derivative audit

Base: `99b2a5cc2c0d82ac05abb77f1a66e1668102f74e`.

## Scope and expected results

1. Reproduce through public LIF calls: a legal reset of -1e308, DT=log(2), inputs [3, 1e308], and one refractory step. Forward must return spikes [1, 0] and synaptic outputs [1, 0.5]. The held row contributes exactly zero to every direct membrane derivative. The final-output loss has log-tau derivative -3*log(2)/16 from the first row alone.
2. Freeze regression tests before implementation. Cover ordinary, coupled and recomputed LIF, mixed LIF, and the same derivative expression in continuous, GPU-adapted and vector cores where reachable. Also cover finite nonzero derivatives recovered after subtraction overflow: a continuous tanh node at initial=-1e308, input=1e308, DT=log(2), upstream=1 has a finite derivative approximately -1e308*log(2). Upstream=16 has a genuinely unrepresentable derivative and must be rejected. Preserve the normal evaluation order. Root owns the expected values; test and implementation workers have disjoint file lists.
3. Complete relevant public acceptance, normal bitwise controls, full project engineering checks, source and protected-data fingerprints. Preserve evidence and remove only this audit's `runs/.tmp/zero-tau-adjoint-20261015` directory. Commit and verify the authorized remote delivery.

The public LIF reproduction confirmed that `(previous - drive)` overflows even though both terms and the accepted forward candidate are finite. Multiplying that result by an exactly zero adjoint then yields NaN. The same subtraction also loses nonzero derivatives whose final product is representable. The finite result contract applies to the full derivative.

## Implementation skeleton

Private `membraneTauGradient(adjoint, previous, drive, slope float64) float64` recovers the existing `adjoint * (previous-drive) * slope` expression. Return it unchanged when finite or when an operand is already non-finite. Only a non-finite result from four finite operands uses an exact rational difference and product followed by one float64 rounding. Genuine overflow remains non-finite and existing public validation rejects it. This also returns zero for exact zero products without accepting malformed operands.

Keep the original expression at each caller. For direct accumulation, remember the previous accumulated gradient, execute the unchanged original accumulation, and only if its result is non-finite replace it with previous plus the recovered contribution. Coupled paths already name and validate the contribution: keep that original expression, recover only if non-finite, then retain their existing checks and accumulation.

The initial candidate moved every expression into a helper. It passed all 27 regression events but changed 80 scalar values in 16 LIF/Mixed records and 16 scalar values in four Continuous/Vector records, all in log-tau gradients. That candidate is rejected. The caller-preserving form must match both old datasets byte for byte within each build. This is a rounding observation, not proof of a particular compiler transformation.

The second candidate restored direct accumulation but still reassigned the coupled contribution. It retained 40 coupled scalar differences in the LIF/Mixed records and one in Continuous/Vector. Root preserved that candidate and separated the recovered value from the unchanged normal value in three coupled branches. The accepted ordinary-build datasets now match the original bytes exactly. Race-build controls and old-model continuation are separate acceptance gates.

Replace the ten occurrences in continuous.go, lif.go, mixed.go, recompute.go, lif_recompute.go, coupled_reverse.go (two), mixed_coupled.go, vector_core.go and gpu_adapter.go. A shared helper avoids ten copies of rare recovery logic. GPU float32 validation may make the trigger unreachable there; retain that restriction and verify its existing device controls.

## Boundaries

Do not change refractory input validation, chemical clock ordering, model configuration, saved formats, surrogate rules, dependencies, unrelated files or existing work directories. This audit does not establish task-accuracy improvement or absence of every bug. Keep the ordinary expression and use the existing standard-library rational approach only for a non-finite intermediate result. Do not introduce a general arithmetic framework.

## Delegation

OpenCode model enumeration failed this turn with `Unknown: FileSystem.open (/Users/timlai/.local/share/opencode/log/opencode.log)`. The project fallback is native gpt-5.6-luna with max effort. A read-only worker checked the refractory contract; Root verifies source and runs all acceptance commands.
