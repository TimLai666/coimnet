# Partial chemical source-history / recomputation audit

Status: coverage investigation. No confirmed defect and no production repair authorized from a hypothesis alone.

## Root scope and expected results

Keep the delivered 834-entry formal source manifest unchanged unless a real mismatch is reproduced. Audit public CPU scalar Continuous and LIF episode training. Mixed, vector, GPU and stateful chemical recomputation remain explicitly unsupported.

Use the existing two-node chemical recomputation fixtures as the starting point, extending to five channels because ChemistryConfig requires exactly one source per channel, in channel order. Keep two regions, nonzero concentration, transport, receptor locations, expression gain, feedback and resource inputs. Use Controller/ExternalTimeline/NeuralActivity/InternalResource/Replay at channels 0/1/2/3/4 respectively. Extend Tau to [2,2.5,3,3.5,4] and concentration rows to [0.21,0.13,0.07,0.04,0.09] and [0.16,0.24,0.11,0.08,0.05]. Add channel-3/4 receptors with Kd 0.8/1, N 1.2, all sensitivity effects uniformly GammaScale=0.2 and BetaScale=0.02, because same-kind receptors overlap on the same cells (modulation/effects.go:136-137). Their resource/recorded rates are fixed episode inputs, not neural activity derivatives. Preserve SourceStep=7 and the distinct kinetics Steps=3.

Construct controller windows of four oldest-to-newest rows. Filled=0/1/3/4 has exactly four rows with the unfilled prefix zero. Compare delays 2/5, truncation windows 0/3/5, and recomputation segments 1/2/5/11 over eight input rows. This is 192 supported comparisons. Truncation is local fixed-window reverse truncation and does not reset forward history at segment joins.

Multiply the inherited fixture's external upstream rows by ten before any run. This strengthens the independent derivative signal without changing forward activity. Compute the first update mean norm from the first StepFrom gradient plus the next public MSE gradient, divided by two; compare it to ClipNorm=0.7 to report actual clipping eligibility. Do not infer clipping from the per-call GradientNorm. Keep the reference epsilon/tolerance fixed; do not choose them from observed test errors. Report the number of reference coordinates that would reject a zero derivative and whether clipping was actually active.

For every case, compare five alternating public StepFrom/Step calls with AccumulateSteps=2. Compare every StepResult, the full snapshot (only Options.Recompute may differ), predictions, parameters, Adam moments/counts, update counters and pending accumulation. Main cases train every parameter group so the first pending Sum captures the full flattened parameter gradient. For this direct gradient-to-sum check, numerical zeros are equivalent because adding to a zero-initialized accumulator can canonicalize signed zero. All full/recompute comparisons remain bit-exact. Require final Updates=2 and pending Accumulator.Count=1 explicitly. Verify caller-owned config/parameters/inputs/upstream remain unchanged. Repeated public Network gradients must also be stable and finite. This Trainer API does not publish recomputed observation/initial-state derivatives; do not claim a direct comparison of those outputs. Full-history observation gradients are independently checked below.

For each core, run explicit ablations of chemistry, expression, transport, each source, resource amount, feedback availability and controller history. A source ablation replaces just that indexed source with a zero-release ExternalTimeline (same channel, ChannelCount=channel+1, no entries), and clears only its SourceStates entry. It retains the five-channel shape and the one-source-per-channel contract. A control must change outputs or gradients; unchanged spiking output alone is not proof that a LIF modulation was inactive. Only require a nonzero effect for controls actually declared in the fixture. Do not weaken assertions after a failure without first establishing that the assertion or fixture is wrong.

For Continuous with Filled=0/1/3 and full reverse history, independently finite-difference all parameter and input coordinates against the public scalar objective sum(PredictAll * upstream). Use epsilon=0.002 and the existing boundary-aware tolerance 0.002 + 0.002*abs(reference). LIF surrogate derivatives are not claimed to equal finite differences of discontinuous spikes.

Save public TrainingSnapshot JSON at a pending-accumulation boundary and restore through RestoreTrainer in genuinely new processes for selected core/Filled/segment/window combinations. Compare all subsequent results and normalized snapshots. Clearly label these as SDK snapshot JSON checks, not a new checkpoint bundle format or old-model migration test.

## Diagnostic slots and ownership

The diagnostic lives only in runs/.tmp/partial-source-recompute-20261016/probe. The worker may write diagnostic fixture/check code there, never formal production or test files. Root owns this contract, independently reviews every diagnostic line and runs ordinary/race commands with project-local caches. Root independently verifies result counts, nonzero controls, finite-difference tolerances, source identity and cross-process evidence. A confirmed mismatch requires a separate minimal reproduction and frozen failing regression before any production implementation.

## Acceptance and delivery

1. Current source/contract and independent read-only review completed; classify existing coverage correctly.
2. Public ordinary/race matrix, mechanism controls, independent Continuous reference and new-process continuation checked. Any mismatches remain visible and are investigated before conclusions.
3. Verify formal source identity against the delivered full engineering gate; run diagnostic vet and current governance. Only reuse full engineering evidence when all formal source bytes and source paths are identical.
4. Preserve necessary evidence and diagnostics, remove only this owned work root, verify original models/data/modules/foreign work, then complete authorized commit, push and remote readback.

## Subtraction and limits

Prefer one shared fixture and one comparison path. Do not add production abstractions, change supported modes, relax existing tests, run a new real-task training job, or infer biological correctness or absolute zero bugs from this bounded audit.
