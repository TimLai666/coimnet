# Max mixing and clamp gradient audit

Status: investigate supported CPU scalar Continuous/LIF combinations. No confirmed production defect. The prior goal turn made progress: ticket119 evidence, cleanup and delivery completed. The original all-bugs objective remains active.

## Fixed contract before runtime

Use the exact ticket119 two-node, five-source fixture and its verified public Trainer comparison helpers. The new case key adds Mix and Profile. Each same-kind overlapping effect keeps identical coefficients. Add a second LIF threshold effect on receptor 2 (beside receptor 0) so max selection also reaches threshold. SourceStep=7, kinetics Steps=3, delay=5, eight rows, existing controller partial state/resources/transport/location/expression/feedback stay fixed.

Profiles: Continuous uses open-positive, open-negative, gain-upper, gain-lower, offset-upper, offset-lower; LIF also uses threshold-upper/threshold-lower. Open sensitivity gamma/beta scales are +/-0.05 and +/-0.02, bounds [0.5,2], beta cap 1. Open threshold scale +/-0.1, cap 0.8. Gain-upper uses gamma scale 1 and bounds [0.75,1.005]; gain-lower uses scale -1 and [0.995,1.25]. Offset caps use beta scale +/-0.1 and cap 0.0005. Threshold caps use theta scale +/-0.1 and cap 0.0005. Other coefficients retain the corresponding signed open profile. Exact profile construction is Root-owned before implementation/runtime.

- 336 matrix cases per build: 14 core/profile combinations x MixSum/MixMax x Filled 1/3 x reverse window 0/3 x segment 1/5/11. Compare five alternating StepFrom/Step operations, every StepResult, full snapshot excluding only Options.Recompute, predictions, optimizer and pending sums. Require two updates, pending Count=1 and actual parameter changes. Keep exact full/recompute bits; direct gradient-to-zero-initialized-sum accepts both zero signs.
- 28 mix controls: each core/profile and fill changes MixSum to MixMax, requiring output or complete public gradient to differ. Eight signed open controls compare positive/negative at both mixes/cores/fills.
- 56 initial chemical-step proofs using only public SourceSpec.Build/Controller.RestoreState/Source.Release/Kinetics.Step/Receptors.Occupancies/ApplyEffects. Use the exact initial activity, source states, resources and clock 7; pending feedback is not yet available. Require no clamps for 16 open cases and the declared kind of clamp for 40 capped cases. These prove the first chemical step, not every later row. Do not equate exact-bound zero derivative with ClampReport holding a value back.
- 432 independent Continuous parameter/input finite differences: six profiles x two mixes x two fills x 18 coordinates. Epsilon=0.002 and tolerance=0.002+0.002*abs(reference), fixed before observations. Require at least one reference to reject a zero derivative per 24 group; report the total. No LIF hard-spike finite differences.
- 28 new-process pending-snapshot continuations per build: MixMax, both cores, fills 1/3, reverse window 3, segments 1/5. Common profiles open-negative/gain-lower/offset-upper; LIF additionally threshold-lower. Require four subsequent alternating operations to equal full-history reference. SDK JSON, not bundle migration or cross-build equality.
- Run existing direct max/tie/zero/exact-clamp/finite-difference modulation tests ordinarily and with race. Those public rules are authoritative; do not central-difference nondifferentiable ties/endpoints as a unique derivative.

## Source and delivery

Root verifies every diagnostic line and executes all runtime. Workers only fill named diagnostic slots; no production repair without a minimal public failing reproduction and frozen regression. The coefficient explanation in ticket21 is mathematically wrong for negative max scales; correct the explanation without changing the already explicit raw-occupancy mixing contract or handoff originals.

If combined coverage warrants compact permanent integration tests, freeze and run those tests and all required framework format/build/unit/race/vet/module checks. Otherwise retain bounded diagnostic evidence with exact source identity before reusing prior full-engineering results. No claim of biological or real-task quality improvement follows from these checks.

Acceptance: (1) source/contract/read-only review, (2) ordinary/race comparisons and references/resumes, (3) applicable full engineering/governance plus requirement evidence, (4) preserve evidence, delete only owned temporary root, verify protected work/data/modules, commit/push/remote readback.

Subtraction: reuse the exact verified fixture and comparison path; retain only used helpers. Add no production abstraction, dependency, unsupported core mode, model rewrite or real-task tuning job.
