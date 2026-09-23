# Requirements report

Generated at 2026-09-23T19:54:06Z from docs/requirements-status.json. `evidence_present` reports only that the named record files exist.

## Completion

| Status | Count |
| --- | --- |
| passed | 88 |
| blocked | 0 |
| specified | 3 |
| total | 91 |

## Requirements

| ID | Status | Evidence present |
| --- | --- | --- |
| GOV-01 | passed | yes |
| GOV-02 | passed | yes |
| GOV-03 | passed | yes |
| GOV-04 | passed | yes |
| GOV-05 | passed | yes |
| GOV-06 | passed | yes |
| DAT-01 | passed | yes |
| DAT-02 | passed | yes |
| DAT-03 | passed | yes |
| DAT-04 | passed | yes |
| DAT-05 | passed | yes |
| DAT-06 | passed | yes |
| DAT-07 | passed | yes |
| DAT-08 | passed | yes |
| SIG-01 | passed | yes |
| SIG-02 | passed | yes |
| SIG-03 | passed | yes |
| SIG-04 | passed | yes |
| SIG-05 | passed | yes |
| SIG-06 | passed | yes |
| COR-01 | passed | yes |
| COR-02 | passed | yes |
| COR-03 | passed | yes |
| COR-04 | passed | yes |
| COR-05 | passed | yes |
| COR-06 | passed | yes |
| COR-07 | passed | yes |
| COR-08 | passed | yes |
| COR-09 | passed | yes |
| COR-10 | passed | yes |
| COR-11 | passed | yes |
| LRN-01 | passed | yes |
| LRN-02 | passed | yes |
| LRN-03 | passed | yes |
| LRN-04 | passed | yes |
| LRN-05 | passed | yes |
| LRN-06 | passed | yes |
| LRN-07 | passed | yes |
| LRN-08 | passed | yes |
| LRN-09 | passed | yes |
| LRN-10 | passed | yes |
| MOD-01 | passed | yes |
| MOD-02 | passed | yes |
| MOD-03 | passed | yes |
| MOD-04 | passed | yes |
| MOD-05 | passed | yes |
| MOD-06 | passed | yes |
| MOD-07 | passed | yes |
| MOD-08 | passed | yes |
| MOD-09 | passed | yes |
| MOD-10 | passed | yes |
| TCH-01 | passed | yes |
| TCH-02 | passed | yes |
| TCH-03 | passed | yes |
| TCH-04 | passed | yes |
| TCH-05 | passed | yes |
| TCH-06 | passed | yes |
| TSK-01 | passed | yes |
| TSK-02 | passed | yes |
| TSK-03 | passed | yes |
| TSK-04 | passed | yes |
| TSK-05 | passed | yes |
| TSK-06 | passed | yes |
| TSK-07 | passed | yes |
| TSK-08 | passed | yes |
| TSK-09 | passed | yes |
| TSK-10 | passed | yes |
| TSK-11 | specified | no |
| TSK-12 | passed | yes |
| STA-01 | passed | yes |
| STA-02 | passed | yes |
| STA-03 | passed | yes |
| STA-04 | passed | yes |
| STA-05 | passed | yes |
| STA-06 | passed | yes |
| OPS-01 | passed | yes |
| OPS-02 | passed | yes |
| OPS-03 | passed | yes |
| OPS-04 | passed | yes |
| OPS-05 | specified | no |
| OPS-06 | passed | yes |
| OPS-07 | passed | yes |
| OPS-08 | passed | yes |
| OPS-09 | passed | yes |
| OPS-10 | specified | no |
| NAT-01 | passed | yes |
| NAT-02 | passed | yes |
| NAT-03 | passed | yes |
| NAT-04 | passed | yes |
| NAT-05 | passed | yes |
| NAT-06 | passed | yes |

## Limitations

### GOV-01

- governance_test.go 驗證靜態結構與宣告，不自動重新執行所有 passed 需求的測試流程。
- 未執行的操作（未建立遠端、未發布、未刪原件）以本地 git log 與 release-checklist 狀態為證，依賴後續維護者自律遵守規範。

### GOV-04

- 能力表由人工維護，沒有自動同步測試。

### GOV-05

- 授權判定是固定文字規則、未知項需人工核對、發布授權等使用者。

### GOV-06

- 連結檢查是一次性指令，沒有進 verify.sh。

### DAT-01

- Abrupt process kill may leave a stale lock or inconsistent partial state; the downloader preserves and rejects those artifacts rather than claiming automatic crash recovery.
- Windows was crosscompiled only; real execution remains unverified.

### DAT-02

- This reader does not select neurons, normalize anatomical records, aggregate edges, or build a trainable graph. Those are separate tracked requirements.
- Arrow allocator accounting excludes other process memory; ReadAt itself cannot be forcibly interrupted by context.
- Windows crosscompiled only, not executed.

### DAT-03

- The graph is not persisted; a durable GraphStore and its read-back verification are a separate ticket.
- Selection is an engineering predicate; segment, neuron, pair and weight counts are reported separately and none is a synaptic contact or biological completeness claim.
- Executed on macOS arm64 only; Ubuntu and Windows real-data runs are not yet recorded for this requirement.

### DAT-04

- Index widths above 2^32 nodes are implemented (64-bit storage) but not exercised with a fixture of that size; the overflow paths are exercised through capacity errors.

### DAT-05

- Real MaleCNS duplicate semantics remain declared unknown; the real-data report in evidence/DAT-03 records the observed duplicate counts without aggregation.

### DAT-06

- 真實 FlyWire 檔案未取得：blocked_data。下載需要帳號、條款同意與使用者提供本機檔案路徑與授權欄位；本紀錄的所有數字都來自 fixture，不構成真實釋出資料的匯入證據。
- fixture 規模只有 3 顆神經元、3 列 connections、3 列 classification、3 列 neurons；真實釋出是十萬量級神經元與千萬量級連線，欄位變異、空值分布與資料量壓力都未驗證。
- 批次切分只驗到 3 列：Feather 寫出的批次大小為 250000 列（connectome/flywire/feather.go 的 featherBatchRows），fixture 全部落在第一批，跨批寫出與逐批讀回的路徑沒有被本次測試涵蓋。
- `data import --dataset flywire` CLI 尚未接上：internal/cli 沒有任何 flywire 參照，data import 只有 --manifest 與 --out-store 兩個旗標（internal/cli/import.go、internal/cli/data.go 的用法說明），目前只能以 Go API flywire.Convert 產生 manifest 後再交給 data import；票面第 8 點的 CLI 形式未實作。
- 欄位稽核未對照真實檔：docs/flywire-source-audit.md 的欄位名與型別整理自 Codex 公開下載頁，未逐欄對照實際下載檔；blocked_data 解除後須重新稽核。
- 一次執行 macOS arm64（darwin/arm64，go1.26.5），未在其他平台重跑。
- 本紀錄只涵蓋 adapter 轉換與拼接拒絕；FlyWire 圖上的模擬、參數推導或與 MaleCNS 的任何比較都未執行，也不在本票宣稱範圍。

### DAT-07

- This acceptance covers traceable selection without changing originals. Synthetic training has no held-out or biological acceptance.
- Full-graph training, GPU and Windows runtime remain unverified.

### DAT-08

- Chemical receptor records (ReceptorRecord) and action-sign modes are not implemented; this requirement only shows unknown values are preserved as unknown.

### SIG-01

- Artificial already-decoded numeric sources; media codecs, biological sensory mappings and modulation controllers are not validated by this fixture.
- Strict persistence contracts apply to DecodeSignal/DecodeMapping/DecodeClock/DecodeObservation/DecodeTarget/DecodeFeedback and their runtime JSON types. Plain Go builder specs and standalone MappingEntry use ordinary encoding/json semantics.
- SIG-04 remains unaccepted: full input/output mapping binding/replacement, selection modes and reconstruction provenance are outstanding.
- Windows crosscompile only; Windows runtime and GPU execution were not tested.
- JSON byte/depth limits are not process RSS bounds.

### SIG-02

- Synthetic scalar streams, fixed six-feature adapter and two-node artificial network; no biological performance claim.
- Source timestamps and ratios are separate from the model-step clock and wall time. Adapt returns only after completing the finite sources; it does not measure real-time latency.
- Interval values and durations must be known at their start. Media decoding, antialiasing, sensory anatomy and modulation controller are not part of this fixture.
- Windows crosscompile only; Windows runtime and GPU execution were not run.
- 32 samples per channel and 80 logical value slots per resampler do not bound caller-owned Observation allocations or process RSS.

### SIG-03

- The runtime taint test had no red state of its own: it asserts a property of code that already existed (the three separated types of ticket 03, the sources of this ticket), so its first run could only fail for an unrelated reason, which is what red-taint.log records. The two mutation logs are what stand in for a red state.
- Fixture scale only: three neurons, one episode, five input steps. No whole-brain or real-data run is claimed here.
- The static check reads the working tree it is run in. plasticity/ and learning/ were being edited by other agents while it ran, so their file counts and dependency lists in check-target-flow.log are a snapshot of 2026-09-16T10:45Z, not a stable fact about those packages.
- controller source not implemented, MOD-07 pending.
- concentration dynamics are ticket 21: this ticket stops at the release rate, so nothing here checks what a released rate does to a concentration.
- The evaluate-mode rule that a teacher is not called at all is the teacher ticket's; here only Feedback.Source records where an outcome came from.

### SIG-04

- Artificial graph fixture; no biological mapping efficacy, whole-brain training, GPU execution or Windows runtime claim.
- Candidates must be supplied in the actual dynamics graph index order. Framework validates consistency against that supplied identity/metadata list, not independent biological provenance.
- Projection neuron hash covers ordered IDs only; it is not a checksum of coefficients or the whole graph.
- Replacing projections replaces both encoder and readout with the supplied coefficients and starts a fresh optimizer. Trained projection coefficients must be explicitly exported from a snapshot.
- JSON limits are 16 MiB/depth 64; candidate/selected neurons and coefficients each have a 1,048,576 count limit. These are not process RSS bounds.
- Input/output channel rank is checked via ValidateChannelShape before flattening; BindProjections checks the model channel widths.

### SIG-05

- Synthetic scalar streams, fixed six-feature adapter and two-node artificial network; no biological performance claim.
- Source timestamps and ratios are separate from the model-step clock and wall time. Adapt returns only after completing the finite sources; it does not measure real-time latency.
- Interval values and durations must be known at their start. Media decoding, antialiasing, sensory anatomy and modulation controller are not part of this fixture.
- Windows crosscompile only; Windows runtime and GPU execution were not run.
- 32 samples per channel and 80 logical value slots per resampler do not bound caller-owned Observation allocations or process RSS.

### SIG-06

- Only zero-duration continuous/activity/modulation point samples with fixed per-stream metadata and common zero origin.
- Generic scalar/vector interpolation, not an audio codec, antialiasing filter, differentiable encoder, or biological time calibration.
- Channels align independently; combining them into one Observation requires a declared shared source-sequence mapping not implemented here.
- MaxValues bounds logical slots, not process RSS; no new throughput or training-resource measurement was performed.
- Windows crosscompile passed; Windows runtime and GPU execution were not run.

### COR-01

- Continuous CPU fixture profile only; no LIF, chemical state, local plasticity, whole-brain training, GPU execution or Windows runtime validation.
- TrainEpisode starts an independent zero-state sequence; no gradient crosses Advance calls. Data-provider cursor and external RNG are not part of this individual snapshot. STA-01 and STA-03 remain specified.
- Per-matrix/history element limits and the 64 MiB JSON limit are not process memory bounds.
- Cross-CPU loading is verified for the synthetic probes, not bit-identical future trajectories or arbitrary hardware.

### COR-03

- Executed on macOS arm64 (Go 1.26.5) only; Ubuntu and Windows runs for this requirement are not recorded yet, and Windows and Linux are covered only by cross compilation.
- One spike per step is the discrete-step representation limit of this model, not a measured biological firing rate. The synthetic three-neuron fixture is a numerical reference, not fly physiology.
- Only cancellation before the call is exercised; the per-step cancellation checkpoints mirror the continuous core and were not triggered in a test.

### COR-04

- tau_adapt, beta, tau_rate, target_rate, eta and h_max are fixed configuration, not trainable parameters. Only theta_raw is trained.
- All numbers come from synthetic fixtures: a five-step delayed-pulse generator with three neurons for the threshold task, a two neuron fixture for the hand-calculated homeostasis timing and a single unconnected neuron for the convergence run. Falling holdout loss and a rate converging to its target show the mechanisms work as declared on these fixtures; they are not generalization, real-connectome or biological firing claims.
- The three seeds of the threshold task only perturb the first edge weight, so the theta-only results are identical across seeds. They show the protocol reproduces, not initialization variance.
- The convergence run uses one parameter set. No sweep over tau_rate, eta or target_rate was made, and nothing here establishes a stability region or a convergence rate.
- Chemical modulation, the fourth term of the specification's theta_eff, does not exist. COR-04 is complete for the three terms that do.
- NAT-01's core_config_hash 61e0e242c1031775fedbd9e6c5aabe93d73595d1fbffe85aa19d24267d55a951 was not recomputed: it needs the 25,563,197 edge store and about 90 seconds of whole-graph execution. Its invariance is inferred from the omitempty encoding, which is pinned by a test, and from the recomputed protocol hash.
- A declared but disabled homeostasis block does not validate its numbers (matching the existing behaviour for a disabled adaptation block), but it does change the configuration fingerprint, because it is a different declaration.
- Executed on macOS arm64 (go1.26.5) only; Ubuntu and Windows runs for this requirement are not recorded.
- docs/requirements-status.json still records COR-04 as specified. That file was outside the file ownership of this stage and was not updated; the evidence for marking it passed is this record.

### COR-05

- No finite differences across hard events. The hard 0/1 event is not differentiated numerically anywhere; the surrogate is verified through the package-private smooth mode and through a hand computed gradient, exactly as ticket 11 requires.
- Fixture scale only. The largest model tested here has 4 nodes and 6 edges. Nothing in this record says anything about whole-brain memory, wall-clock time or numerical behaviour at 165,122 nodes, and no real connectome or derived parameter set was used.
- Vector nodes and shared parameters are stage two (COR-06) and recompute is stage three (LRN-02). Neither is implemented or measured here.
- The all-LIF bit identity is exact only in an ordinary build. Under -race the membrane-derived values agree to 1e-16 relative rather than bit for bit, because LIF.Forward's own fused-multiply-add choice changes between the two builds. See bit_identity.why_the_race_build_differs; this is a property of the existing core and is recorded, not worked around.
- checkpoint.ModelPackage does not carry the mixed core. Its topology fingerprint reads only the continuous and the spiking configuration, and the base64 node_rule array does not survive its required-field walk, so a mixed package would have been written with a fingerprint claiming no nodes and no edges and could not be read back. canonicalModelPackage now refuses it with a specific error and points at the persistent individual snapshot. Supporting it belongs to whoever extends the package format.
- The simulate package's native runner does not offer the mixed core; it still selects one rule for the whole model.
- stdp_pair is refused on the mixed core. A continuous node produces no event, and treating its absent event as "never fired" would be a silent wrong answer, so the rule is rejected rather than approximated.
- The mixed state's ConfigHash is the mixed configuration's fingerprint, so a single-rule assignment's state half is numerically identical to the corresponding single-rule state but does not carry that core's hash. A mixed state is deliberately not loadable as a Continuous or LIF state.
- Chemistry on a mixed individual is exercised only through the Modulation hook of the core (neutral modulation is bit-identical, a threshold entry on a continuous node is refused, a threshold entry on a LIF node suppresses its event). No end-to-end chemical fixture was added for the mixed core in this stage.
- One machine, one Go version, one architecture: go1.26.5 darwin/arm64. The contraction behaviour recorded above is measured there; the build-tagged constant will fail the contraction test rather than silently mislead if another toolchain chooses differently.

### COR-06

- 向量核心的持續個體狀態（NewIndividual）目前明確拒絕（TestVectorCoreRejectsPersistentStateForNow: NewIndividual returns an error stating persistent individual state is not yet supported for vector-state cores）。
- 共享只作用於 weights／bias／log_tau 且以「成員層 AdamW + 梯度和」實作（與 ticket 25 Root 決策 7 的 SharingIndex 寫法在數學上等價，未另建群組層最佳化器狀態）。
- 向量核心不接受固定邊號誌（TestVectorCoreRefusesEdgeSigns: nonzero EdgeSigns are refused with 'fixed edge signs are not defined for vector state edges'）。
- 容量數字是參數與乘加計數而非實測記憶體（Capacity reports ParameterCount, FreeParameterCount, and MultAddsPerStep based on topology and component dimensions, not runtime resident set size or allocator overhead）。
- 全部是 fixture（All verification results are derived from programmatic unit and integration fixtures under ./dynamics/, ./learning/, and ./checkpoint/; whole-brain or real-task training with vector cores is not evaluated in this profile）。

### COR-07

- The touchstone is a fixture; the whole-brain evidence is the NAT-01 run on macOS arm64 only.
- No dedicated N×N-allocation guard test exists beyond the accounting refusal test TestBuildRefusesOversizedGraphsInsteadOfDownsizing in simulate/simulate_test.go (line 375).

### COR-09

- The surrogate is a declared engineering rule, not a measured biological derivative; agreement with the tangent reference and the smooth model says the implementation matches the declaration, not that the rule is biologically correct.
- The smooth mode exists only in the dynamics package tests. The product hard mode keeps the detached reset, so its gradient is an approximation and must not be finite-differenced.
- Executed on macOS arm64 (Go 1.26.5) only; Ubuntu and Windows runs for this requirement are not recorded yet.

### COR-10

- Fixture scale only: two to four neurons. No masked, fixed-sign or range-constrained run exists on a real subgraph or on the whole 25,563,197-edge graph.
- The 1,000-step sign-flip run is one learning rate on one fixture; it shows that the parametrization makes a flip unrepresentable, not that any particular real training run is stable.
- The chain-rule check is on the continuous core only. The LIF surrogate gradient is not finite-differenced, by the rule ticket 11 declares.
- Ranges and the log-magnitude floor are verified as projections on parameters; no experiment shows what those bounds do to learning outcomes.
- COR-10 says nothing about whether the derived signs are biologically correct; that is ticket 13's evidence and carries its own unknown counts.
- One machine, one platform: macOS arm64, go1.26.5, with other work running on the same machine during the stage-two runs.

### COR-11

- Fixture scale only: one to three neurons, one channel, four rows or fewer in most calls, no whole-brain, real-data, cross-platform or performance run.
- The native runner has no chemistry, regions or delayed edges, so only the three node kinds (clamp_voltage, force_spike, silence) are supported there; block_channel, fix_concentration, remove_channel, swap_regions and shuffle_delays are refused on simulate (TestUnsupportedKindRejected covers block_channel only).
- The runner counts [Start, End) per Run call, not as global trajectory steps: AppliedSteps is the overlap of the declared window with the rows of that one call (overlapSteps clamps to [0, steps)); the mapping between a multi-Run intervention window and global steps is not tested.
- TaskDelta (task-result difference, specification 11.8) is not implemented: no current test produces a non-nil TaskDelta; only activity, plastic, base-parameter and concentration deltas are measured.
- Node interventions are refused on a mixed core ("not implemented yet"): the log only proves the refusal and that nothing changes, not a clamp on a mixed core itself.
- Real data were not run. The free-inference constraint is shown at the API level (no overwrite parameter, unauthorized refusal, unchanged protocol hash) on fixture graphs; no whole-brain protocol has been executed with an interventions block.
- shuffle_delays is verified on one permutation only (seed 7, [1 2] to [2 1]); the permutation's statistical properties (a preserved set of positive delays, no duplicates) have no test of their own.
- swap_regions is value-neutral on this stage's fixture because one channel releases the same rate into every region (ConcentrationDelta 0); per-region sources that would make the swap observable are a later decision.
- This log contains only the targeted -race run above; the full go test ./... and go vet ./... are outside the scope of this instruction.

### LRN-02

- Peak memory still grows with T x N: the network layer keeps the full-width core inputs (learning/network.go:570), the core outputs, the upstream rows (learning/network.go:404) and the input gradient for the whole episode. The measured saving (34.0% continuous, 23.9% LIF) is therefore smaller than the core-history reduction from 257 to 81 rows, and the O(T/S + S) bound of ticket 25 holds for the core history only.
- Because recompute is bit-identical, the permanent tests cannot tell whether Step recomputes; only the RSS comparison shows it.
- Recompute goes through the streaming state, so it also refuses graphs where nodes x (max delay + 1) exceeds dynamics.MaxStateValues (2^20); the full-history path has no such cap. The whole graph has delay 0 and is not affected (AGENTS.md follow-up).
- The continuous core's segment replay activates every edge on every step, so recompute costs time in proportion to edges rather than nodes (AGENTS.md follow-up).
- Only the continuous and LIF cores support recompute; mixed and vector cores are refused.
- ENG.md still says recompute is tracked; it is not updated here because that file is being edited in parallel work.
- One machine (macOS arm64); the Linux branch of scripts/recompute-evidence.sh was checked only against a synthetic time output.

### LRN-03

- Fixture scale only: two neurons, two edges, nine parameters, three episodes. No accumulation, schedule or loss-scaling run exists on a real subgraph or on the whole graph, and nothing here measures the memory or time cost of an accumulator on 25,563,197 edges.
- checkpoint.State still requires next_sample == updates, so a caller that consumes one episode per Step cannot save a checkpoint of a run with accumulate_steps > 1; the accumulating checkpoints in these tests are built with the cursor equal to the update count. learning.IndividualSnapshot has no accumulator field at all, so a persistent individual interrupted inside a window loses the partial sum. Both belong to ticket 18 or to root.
- Individual.ResetOptimizer clears optimizer moments and the update count but not the trainer's accumulator, because learning/individual.go is another agent's file this round. Step defends itself by starting a new window when the accumulator no longer fits the declared one, which is a fallback, not the fix.
- The loss-scale round trip was bit-identical for every scale tried on this fixture, so the 1e-12 tolerance in the ticket has not actually been exercised by a measured deviation.
- The schedules are pure functions of the update count and were verified on the update count alone; no long training run was executed to show that a schedule improves learning.
- One machine, one platform: macOS arm64, go1.26.5. The machine was shared with other work during the runs (load average up to 5.53), so the timings inside the logs are not a performance measurement.

### LRN-04

- Unit scale only: the largest fixture here is three neurons and three edges. No real graph, no whole-brain run and no learned task result.
- The red state is a build failure (the package had no non-test file yet) rather than a numeric mismatch, because the whole package was new. The behavioural red state for the integration is in evidence/LRN-05/.
- stdp_pair is verified against hand-computed spike patterns supplied directly to Step. Its behaviour on a real spiking core is only shown qualitatively in evidence/LRN-05/ (the sign of the fast change on a two-neuron LIF fixture), not with a hand-computed table of its own.
- The decay-then-add and read-before-own-update orders are pinned by test, so a change to them is a visible test failure; they are not independently derived from any published measurement.
- One machine, one toolchain: macOS arm64 with go1.26.5. No cross-platform run.
- The verification commands ran while other agents held uncommitted work in the same tree, so the full-suite log covers their packages as well as this one.

### LRN-05

- Unit scale only: two- and three-neuron fixtures, at most three edges. No real graph, no whole-brain run, no protocol and no learned task result.
- The gate is caller-supplied. MOD-05's receptor-derived gate and time window are not implemented here; only the gate and time-window behaviour of a caller-supplied sequence is shown.
- The runner and the three-cell comparison (before, after, off) of ticket 18 stage two are not done, so NAT-06 has no evidence yet.
- stdp_pair on a running individual is verified qualitatively (both traces persist, the fast change has the expected sign) rather than with a hand-computed table against the core's own events; the hand-computed pair tables are at the package level in evidence/LRN-04/.
- The B tables were produced by an independent Python implementation of the documented LIF and rule equations, not by a published reference implementation, and are compared to 1e-12 for the derived quantities and exactly for the readout.
- The red state for the plasticity package and for the learning integration is a build failure rather than a numeric mismatch, because both the package and the public methods were new. Only the checkpoint red state is behavioural.
- A plastic part written as JSON null is refused by the individual checkpoint's existing null scanner before the optional-part walk sees it, so "null means disabled" is only reachable through the in-process snapshot type, not through a saved document. The walk treats null as disabled for consistency with the neural union; the document-level rule is stricter.
- One machine, one toolchain: macOS arm64 with go1.26.5. No cross-platform run.
- The verification commands ran while other agents held uncommitted work in the same tree, so the full-suite log covers their packages as well as this one.

### LRN-06

- Fixture scale only: two neurons, two edges, one input and one output, three model steps per action. No full-graph run and no real connectome.
- One platform: macOS arm64, go1.26.5. No Ubuntu or Windows run.
- Teacher sources are not implemented (ticket 27). The rule that evaluate mode must not call a teacher is pinned here only as "Update refuses and changes nothing"; there is no teacher to call yet, so the test cannot observe a call that did not happen.
- Adaptive evaluation is stage two of this ticket (LRN-10). The Evaluate flag is the refusal only; there is no adaptation or scoring split, no reset policy and no contamination report here.
- NewOnlineLearner takes a learning.RewardGate rather than the *modulation.RewardMapper named in the ticket contract: learning cannot import modulation, which imports simulate, which imports connectome, whose own in-package test imports learning (go vet: "import cycle not allowed in test"). The declared mapping rule is unchanged and is wired in through learning.RewardGateFunc in the tests; the deviation is recorded in the ticket.
- The replay store is not carried by learning.TrainingSnapshot. It is an optional part of checkpoint.State instead; see evidence/LRN-07/ and the ticket deviations.
- An Update that fails partway leaves the updates it already applied in place. This is declared behaviour, reported through the partial UpdateReport returned with the error, not a rollback.
- Concurrency is covered only by the race detector over single-goroutine tests; no test drives one learner from several goroutines.

### LRN-07

- Fixture scale only: capacity 4 and at most ten experiences, each a one or two row matrix. No large buffer, no memory accounting and no throughput measurement.
- One platform: macOS arm64, go1.26.5. No Ubuntu or Windows run.
- Teacher sources are not implemented (ticket 27). Nothing here stores or replays a teacher response.
- Adaptive evaluation is stage two of this ticket (LRN-10). The scoring split is a declared name the store accepts; the counted refusal of scoring-split experiences belongs to the contamination check of that stage, and only the test split is refused here.
- The replay state is an optional part of checkpoint.State, not of learning.TrainingSnapshot as the ticket contract wrote it. The trainer owns no store, so a TrainingSnapshot field would have been written by nobody and lost on every Snapshot; the deviation is recorded in the ticket.
- Statistical properties are not measured. The reservoir is Algorithm R and the shuffles are Fisher-Yates over an unbiased bounded draw, but no test estimates the resulting distribution over many runs; the tests pin the exact sequence one declared seed produces.
- Concurrency is covered only by the race detector over single-goroutine tests; no test drives one store from several goroutines.

### LRN-08

- fixture 任務（三神經元延遲相關，兩個任務一次規則改變）與固定 seed 7/42/123，不是真實資料或實驗設計樣本。
- 小預算：CLI 用每 train_task 20 個 episode、每格 16 個評估 episode；分數是單一 fixture 個體在一個參數點上的樣本，seed 間離散度見 cells 的 std。
- 化學凍結是 evaluation 單獨的「跳過 kinetics、維持濃度」模式（FreezeChemistry），與 dynamics 完全關閉、可塑性路徑關閉是不同的事；fixed_chemistry 之外的其他凍結途徑未在此證據覆蓋。
- 控制器不含：本證據只涵蓋 fixture 個體的持續學習矩陣、遺忘、對照與事前比較；真實資料的持續學習、Imitation 與 PPO（LRN-09）及生物啟發協定（MOD-09）是 ticket 26 之後的階段。
- 真實資料未跑：set 是 fixture，無 MaleCNS 資料參與；每顆 seed 一個 fresh 個體，不是由真實接線建立。
- 一次執行（macOS arm64 only，upshot 由 environment 記錄）；矩陣的逐位重現由測試 TestContinualMatrixIsDeterministic 與 TestFreezeChemistryTwiceFromSameSnapshotIsBitIdentical 驗證，不是對這份 JSON 的第二次執行。

### LRN-09

- Synthetic corridor has only two goal-side cases; separate evaluation seeds do not establish generalization to a new task distribution.
- Three-seed aggregate evidence is not a significance claim; seed 1 remains below random.
- PPO and imitation use different synthetic model layouts and budgets; no algorithm superiority claim.
- PPO supports complete zero-state episodes, MiniBatch=1, no plasticity or chemistry. BurnIn masks direct losses but does not detach prefix gradients.
- Snapshot hashes include optimizer state but this report is not a resumable model artifact. CPU architectures produce different snapshot hashes; only within-platform exact repeat is asserted.
- No real connectome training, GPU backend execution, Windows execution or biological mechanism acceptance is claimed.
- Resource timings use two processes per host with warm system state, no cache flushing or isolation; not a full-brain estimate.
- Collector initial red was polluted by concurrent runner compile failures; isolated missing-implementation baseline was reproduced afterwards, see review.md.

### LRN-10

- 只在 macOS arm64 fixture 跑過。
- 梯度型適應被政策關閉。
- 沒有真實任務資料。
- 重播拒絕來自 replay 套件對 test 分割的固定規則。
- 全重設政策下 adaptive 與 fixed 的評分成績相同，成績差異未在本紀錄展示。

### MOD-01

- controller source not implemented, MOD-07 pending. The body and any other source kind named in the main specification are also absent; this ticket implements external, neural, internal-resource and replay only.
- concentration dynamics are ticket 21. A released rate is not yet consumed by anything, and the clearance boost the reward mapper produces has no equation to enter yet.
- No source is wired into a runner: nothing in simulate or learning calls Release, so there is no end-to-end run in which modulation changes a simulation.
- The activity a neural source averages is supplied by the caller. Nothing here verifies that the caller passes the activity of the same step, and the package cannot check it.
- Fixture scale only: three neurons, two channels, at most three recorded steps. No whole-brain or real-data run.

### MOD-02

- effects not yet wired into Individual (stage two): nothing calls Kinetics.Step in a runner or a trainer, no source of ticket 20 feeds it a release rate, and no snapshot carries a ChemistryState.
- Forward path unmodulated: the episode training path of both cores is untouched by this ticket, and the differentiable modulation path (MOD-07) is ticket 22.
- Fixture scale only: at most 3 regions and 2 channels over at most 1,000 steps. No whole-brain, real-data or cross-platform run.
- Transport mass conservation is exact only where the fractions are dyadic (the 0.5/0.5 case tested here). The asymmetric case is checked against the local-update bound with a 1e-12 relative slack, because a sum of rounded products can differ from the mathematical total in the last bits.
- Regions are an abstract index here. Nothing maps a region onto neuropil or onto node indices; that mapping is the RegionAssignment of stage two.
- The working tree was shared: other agents were editing learning/, checkpoint/, simulate/, replay/ and internal/cli/ while this work ran, and the full-repository commands failed to build several times for reasons outside this ticket before all four of them passed on the run recorded here. The uncommitted work of those agents is in the tree alongside this one.

### MOD-03

- effects not yet wired into Individual (stage two): nothing calls Occupancies or ApplyEffects inside a runner or a trainer, no ChemistryReport is returned by Advance, and no snapshot carries a chemical block. The chain test in modulation/effects_test.go assembles the layers by hand for one call; it is not the per-step order stage two owns.
- Forward path unmodulated: Continuous.Forward and LIF.Forward are untouched, the reverse pass is unchanged, and the differentiable modulation path (MOD-07) is ticket 22.
- The 100-step 'Parameters unchanged' check of the ticket is stage two. What is shown here is narrower: a modulation is an argument, not state, and the chain test asserts the parameters are unchanged after one four-step call on each core.
- Fixture scale only: at most four nodes, two regions, two channels and twelve steps. No whole-brain, real-data or cross-platform run.
- Two effects of the same kind that reach the same cell must declare the same coefficients; a disagreement is refused rather than resolved. The ticket's root decision describes mixing as occupancy times each effect's own ratio, and this stage instead mixes the raw occupancies and then applies one set of coefficients, which is what the per-cell formulas name. The two readings agree wherever the coefficients agree, which is every case a run can declare here.
- Receptor coverage is one region per node through the caller's region map. Nothing checks that the map is the same one the chemistry was stepped with; that binding is stage two.
- The working tree was shared: other agents were editing learning/, checkpoint/, simulate/, replay/ and internal/cli/ while this work ran, and the full-repository commands failed to build several times for reasons outside this ticket before all four of them passed on the run recorded here (gofmt -l . clean, go vet ./... clean, go test -count=1 ./... every package ok, go test -race over modulation, dynamics, learning and simulate all ok; uptime at the race run '19:17 up 11 days, 56 mins, load averages: 7.55 4.92 4.01'). The uncommitted work of those agents is in the tree alongside this one, so a rerun on a clean checkout of this ticket alone has not been done.

### MOD-04

- Sources are global to all regions. Every channel's release rate is added to every region, so regional structure enters only through transport and through the region map, never through where a source releases.
- Forward and backward are unmodulated. Continuous.Forward, LIF.Forward and the whole reverse pass are untouched, so an episode gradient step sees no chemistry; the differentiable modulation path (MOD-07) is ticket 22. TrainEpisode therefore trains a model the persistent path no longer runs exactly, which is the same split local plasticity already has.
- Fixture scale only: at most three nodes, two regions, one channel and one hundred steps. No whole-brain, real-data or cross-platform run, and no performance measurement.
- The three occupancy counts in ChemistryReport describe the last row's records, matching the Occupancy field, while the three clamp counts are summed over the rows of the call. That split is a decision of this stage, not a line of the ticket.
- The feedback queue is never drained. None of the four implemented sources reads a score, so keeping an arrived feedback queued changes no number, but a caller that offers feedback every step grows the queue and therefore the snapshot.
- SetResource and OfferFeedback require an enabled chemistry. Nothing else in an individual reads them and the snapshot has nowhere to keep them while the layer is off, so they are refused rather than silently dropped at the next save.
- A threshold effect on a continuous individual is refused at the step where the shift first becomes non-zero, by the continuous core, and not at EnableChemistry. Refusing it earlier would break the neutral-equals-off property, because a declared threshold effect at zero concentration is exactly the unmodulated run.
- The chemical part of the learning snapshot was implemented in the same change as the individual integration and its tests were written immediately after it, not before it. The checkpoint half was written test first and its red state is recorded in red-snapshot.log.
- The working tree was shared: another agent was editing simulate/, internal/cli/, scripts/, ENG.md and README.md (ticket 19) while this work ran, so a rerun on a clean checkout of this ticket alone has not been done. Every command recorded here passed with that uncommitted work in the tree.

### MOD-05

- Fixture scale only: two isolated neurons, one edge, one hypothesized receptor, three rows. No whole-brain, real-data, cross-platform or performance run.
- Only hebbian_rate is verified with a receptor gate on an individual; the stdp_pair path shares the same plasticity.Model machinery but no receptor-gate individual test runs it here.
- The window is verified with one receptor only; a rule declaring two receptors (one for the gate and one for the window) has no coverage in this log.
- The closed-form constants are the fixture's own hand tables (PLASTIC decay_c/decay_p halving rules of the chemistry fixture, reported literals), not a measurement or a biological parameter.
- TestStepStillMatchesStepWith was not selected by the -run expression, so Step == StepWith(nil-override) bit equality is not re-proven in this log.

### MOD-06

- Fixture scale only: one or two isolated neurons, one edge, one or three rows. No whole-brain, real-data, cross-platform or performance run, and no task-level metric.
- The consolidation gate reads the last advance's occupancy report only (ChemReport is refreshed by the preceding Advance/AdvanceGated call); a write requested without a preceding gated row reads whatever the previous call left, which the round-trip test compensates explicitly.
- No real task exercises either mechanism, and neither mechanism is composed with interventions or the controller (ticket 22 stages two and three, not yet dispatched).
- TrainEpisode advances the episode clock and leaves the slow counters intact (TestConsolidateBudgetExhausted); the gradient path's symmetry with 'Step does not touch slow' is asserted by the code and that counter invariant here, not by a gradient-difference measurement.
- All constants are the fixtures' own hand tables closed forms, not measurements; no biological claim is made about which write timing or which gain curve a fly uses.

### MOD-07

- The window state is not part of any snapshot in this stage; a restored controller restarts its window from zeros (documented in modulation/controller.go and memory_controller.go, asserted by construction rather than by a snapshot round-trip test).
- Only two objective proxies exist (reward_proxy and activity_target), and the controller can train only against declared objectives; real teacher or natural-reward targets are outside this ticket.
- No real-data or whole-brain run; all numbers come from the code-declared fixtures (three neurons at most) on one machine.
- The gradient is checked against finite differences of the fixtures' own hand-replayed losses, not on a trained task, and only for the tanh/softplus MLP shape; the concentration -> receptor -> core differentiable path is explicitly not implemented.
- The recorded log contains only the twelve tests the -run pattern selected; the shared-source tests of sources_test.go (feedback arrival, non-negative releases for every source) and the full go test ./... and go vet ./... runs are outside this command's scope.

### MOD-08

- concentration dynamics are ticket 21: ClearanceBoost is stored, not applied, because there is no concentration equation yet to add a clearance term to.
- controller source not implemented, MOD-07 pending, so no learned mapping from feedback to release is verified here.
- The mapper is not wired into any runner or trainer: nothing calls Map outside its tests, so no end-to-end run shows a reward changing a simulation.
- 'Raw is read-only' is shown for the paths that exist: Map copies the argument into the record and keeps its own history, and a record hands out no reference into the mapper. A caller that owns the raw value before calling Map can still change its own copy; nothing in this package can.
- Fixture scale only: five hand-computed values per combination, single-goroutine use. RewardMapper keeps mutable history and is not safe for concurrent use; the race run exercises independent mappers, not a shared one.

### MOD-09

- fixture 規模：三神經元的延遲相關任務（experiment/continual_fixture.go:16，Nodes 3、3 條邊、readout node 2），單一參數點、單一任務（delay 2、channel 0、width 2），不是真實資料或實驗設計樣本。
- 工程數值非定量重現：slow_magnitude 與 retest_score 都是這個 fixture 在這組工程係數（Kd 0.5、N 1、Consolidate 的 Threshold 0.1、Rate 0.5、Retain 0.5）下的輸出，不是 20E 或 NPF 的定量重現，也不是劑量反應關係；報告 assumptions 的兩句就是這個限制的原文。
- 脈衝時機只在一個 episode 的 4 列內：ecdysoneEpisodeRows = 4（experiment/bioinspired_run.go:20），pulse_step 必須落在 0..3，超出就整趟拒絕。所以「時機」只涵蓋一個學習 episode 內的前／中／後，沒有跨 episode 或訓練前後長距離時機的證據。
- 重測分數差只有 1e-7 量級（三個 seed 的末減首為 1.8403789113496938e-07、5.0651842484206178e-07、5.5983916738555628e-07）：只證明三個時機的分數方向一致且可分辨，不是效應量宣稱，也不足以支撐「某個時機比較好」的結論。
- 干預面只用到兩種機制：ticket 22 的八種干預 kind（clamp_voltage、force_spike、silence、block_channel、fix_concentration、remove_channel、swap_regions、shuffle_delays）在本階段一種都沒有用到，這兩個協定用的是 ExternalTimeline 脈衝（時機）與 SetExpressionGain 讀出增益（狀態）。ticket 26 決策 10 提到的「量測活動、快速權重、基礎參數與任務結果四種變化」在本證據裡只落實成 slow_magnitude、retest_score 與三個逐位不變旗標，沒有 ActivityDelta／PlasticDelta／BaseParameterDelta 的數值。
- npf 的抑制強度由 Validate 限制在開區間 (0, 1)（experiment/bioinspired.go:97-99），本次只測 0.4 一個值，沒有抑制強度的掃描；讀出增益的下界固定為 0.1，所以抑制永遠不會把讀出關到零。
- 真實資料未跑：全部是 fixture 個體與固定 seed 1/2/3，沒有 MaleCNS 或任何真實接線參與；三個 seed 是三個 fresh 個體，不是由真實接線建立。
- 一次執行（macOS arm64、go1.26.5、-race）：逐位重現由 TestEcdysoneInspiredIsDeterministic 與 TestNPFIsDeterministic 在同一次執行內驗證，不是對這兩份 JSON 的第二次獨立執行，也沒有跨平台或跨 Go 版本的重現證據。

### MOD-10

- The experiment is a fixture task (three-neuron delayed pulse) with a small budget: the recorded CLI run used 40 training and 32 held-out episodes per seed, the tests 6 and 8; the test.log mean/std lines come from that 6-episode config and differ from the 40-episode summary.json, which is why the two sources are quoted separately in this record.
- capacity_matched's offset is applied to the readout at scoring time only: the base parameters train against the unchanged loss, so the offset never enters their gradient and the corrected readout is used only for the metric this group reports.
- The controller window is per-episode, not per model step: in the ablation the controller is released once per training episode, its 3-row window spans three episodes, and the only feedback it can read is the previous episode's score before the first episode produces one.
- No real-data or whole-brain run; all numbers come from the code-declared delayed-pulse fixture on one machine, with three seeds and a fixed split, so the mean/std are descriptions of these runs, not significance statements.
- The recorded log contains only the thirteen tests the -run pattern selected; the full go test ./... and go vet ./... runs and cross-platform/device runs are outside this command's scope.

### TCH-01

- 只在 macOS arm64 fixture 跑過：沒有 Ubuntu 或 Windows 執行，全部測試輸入由測試內建，未觸碰真實標籤資料。
- 沒有真實標籤資料：教師回答都是測試內建的假紀錄，未驗證任何真實教師或真實輸入的紀錄品質。
- 蒸餾與學生評估是第二／三階段（TCH-03／TCH-04／TCH-05）：本證據只涵蓋契約、紀錄檔、離線與重播、HTTP 與封鎖教師。
- 本次只執行 /Users/timlai/Developer/coimnet 的指定 go test 指令；本輪未另行跑 gofmt、go vet 與其他套件測試。
- 無網路證明是換掉 http.DefaultTransport 的一次性替換，涵蓋 OfflineJSONL 與 Replay 兩條路徑；HTTP 教師本身依設計需要網路，其驗證在 TCH-02。

### TCH-02

- MaxCost 用盡路徑未直接測：TestHTTPBudget 只驅動 MaxRequests 用盡與零預算，依 cost 累積觸發拒絕（response 帶 Usage.Cost）的路徑在測試中沒有被走到。
- 3xx 不重試為保守解讀：測試沒有覆蓋 3xx；依 http.go request() 的分支，300–399 被歸為 unexpected status 立即失敗，是否重試是程式碼解讀而非測試證明。
- 真實遠端未驗證：全部 9 個測試都打本機 httptest 伺服器，沒有連過任何真實教師服務，具體服務 adapter 依官方文件另做。
- 限速測試證明的是「緊接的第二次呼叫被拒、伺服器只收到 1 次」，不是以計時量測的請求間隔。
- 只在 macOS arm64 fixture 跑過；本輪未另行跑 gofmt、go vet 與其他套件測試。

### TCH-03

- fixture 規模：受測學生為三神經元延遲玩具分類任務（DelayedEpisode），非真實果蠅神經接線全圖。
- 教師為 oracle：測試所用教師為硬編碼的分布或字串查表，非真實訓練過之大模型。
- 真實 HTTP 教師與真實任務未跑：本套件僅使用本機 fakeTeacher 與離線資料，真實遠端教師與具體下游任務尚未執行。
- 「參數保存」沒有 distill 專屬測試而是依賴 learning／checkpoint 既有測試（TestStepLabelMatchesMixZeroDistiller 用 trainer 快照比對確認狀態一致）。
- 一次執行 macOS arm64（darwin/arm64，go1.26.5）。

### TCH-04

- fixture 規模：受測學生為三神經元延遲玩具分類任務，非真實果蠅神經接線全圖。
- 教師為 oracle：測試所用教師為硬編碼的分佈，非訓練過的大模型。
- 真實 HTTP 教師與真實任務未跑：本套件僅驗證解析損失、梯度與本機 fixture，未連接真實 HTTP 教師。
- 可訓練的對齊轉換器未做（票面明列為可選）：目前僅支援 identical 與 declared_map 兩種靜態規則。
- 「參數保存」沒有 distill 專屬測試而是依賴 learning／checkpoint 既有測試（TestStepLabelMatchesMixZeroDistiller 用 trainer 快照比對）。
- 一次執行 macOS arm64（darwin/arm64，go1.26.5）。

### TCH-05

- fixture 規模：學生為三神經元延遲鏈、教師為查表 oracle（pulseLabel 建表），非真實果蠅接線全圖，也非訓練過的模型。
- 「獨立來源集」只是另一組 generator seed（DelayedEpisode 2003+seed 對保留集 1003+seed），不是獨立採集或獨立來源的資料。
- 真實教師與真實任務未跑：全部為本機 fixture 與假教師，無真實遠端教師與具體下游任務成績。
- 套件放在 experiment/studenteval 而不是 experiment：studenteval 需匯入 teacher，teacher/http.go 匯入 config，config/registry.go 又匯入 experiment，若併入 experiment 套件會形成 import cycle。
- 一次執行 macOS arm64（darwin/arm64，go1.26.5），無跨平台或跨程序重跑。

### TCH-06

- fixture 伺服器：教師端與誘餌端都是本機 httptest，只證明本機路徑，不代表任何真實遠端服務的行為。
- 工具註冊表尚未接進任何訓練或評估流程：teacher/tool 沒有 production 呼叫端，Invoke 目前只由測試呼叫，端到端「模型選工具→執行→回填」路徑未驗證。
- 真實遠端未跑：無真實 HTTP 教師服務、無真實外部工具，具體服務 adapter 依官方文件另做。
- 一次執行 macOS arm64（darwin/arm64，go1.26.5）。

### TSK-01

- 程式生成的偽字形不是真字型也不是掃描件：所有影像由 glyphs.Family 以 seed 產生的 8×8 點陣圖繪出，不含任何字型檔、排版引擎或真實文件影像。
- 只驗到單行、單欄、水平文字：SegmentLines 是規則式投影法，對多欄、傾斜、旋轉或混排版面沒有任何證據，套件註解也明寫不宣稱任意版面。
- fixture 字母表只有 3 個字元（a、b、c），每行 1..3 個字形：Unicode 與重複字的正確性由 tasks/ocr 的手算測試（台灣／臺灣、CTC 重複字需 blank）涵蓋，但 fixture 的 CER 數字本身沒有中文、大小寫或長行的證據。
- 真實授權資料 blocked_data：TSK-11 要求的真實資料匯入、訓練、推論、評估四項尚未執行，需要使用者提供有授權欄位的 OCR 資料集；在此之前本證據只能支持 fixture 範圍。
- 頁面區塊送進辨識器的流程只到切行為止：SegmentLines／Crop 產出 Block{Bounds, ReadingOrder}，Block.Text 由呼叫端填入，本票沒有把整頁區塊批次餵進辨識器再回填文字的端到端測試。
- fixture 訓練預算很小（3 個 epoch、40 行）：訓練後保留集 CER 仍在 0.62～0.73，只證明流程會學習與下降，不是辨識品質的宣稱；未見組合的樣本數只有 2～5 行，該欄數字的統計意義極低。
- 只跑過一次，單一平台：macOS arm64（darwin/arm64、go1.26.5），沒有跨平台或重複執行的變異數證據；固定 seed 的重現性由 TestRunOCRFixtureIsDeterministic 與 TestRecognizerIsDeterministic 在同一平台上證明。

### TSK-02

- Synthetic tones only; no natural or authorised speech (TSK-11, blocked_data).
- Generalisation to unseen speaker/session groups is weak: WER stays at 0.9 on the CLI fixture.
- Stream files are capped at 64 MiB; larger individuals need the directory bundle path.
- Run on one Mac; Linux and Windows were only cross-compiled in the earlier records.

### TSK-02

- 沒有使用有授權的自然語音資料；TSK-11 所需的真實匯入、訓練、推論和評估尚未驗收。
- ReadDataset 預設限制每檔 raw WAV 64 MiB、每檔處理緩衝估算 512 MiB、總額 2 GiB；可明確調整，但仍把全部處理後樣本留在記憶體，這些估算不保證程序 RSS 上限，也沒有串流語料讀取器。
- StreamState 可在記憶體中 JSON 往返，但沒有獨立嚴格檔案載入器、容量限制及原子落盤 API；完整保存介面尚未完成。
- 來源 SHA-256 與 WAV 解碼使用同一次讀到的位元組；若其他程序在匯入前後修改原檔，來源路徑本身仍不能當成不可變資料承諾。
- 只在 Mac CPU 上量測，Ubuntu 1 SSH 認證失敗，Windows PC 與 GPU／Metal 後端未驗證。
- 人工 fixture 保留集僅 10 筆、19 字元；WER 前後均 0.9，未證明自然語音品質或完整果蠅圖訓練能力。

### TSK-02

- 只驗證小型人工音調的檔案續跑，沒有自然語音辨識品質結論，也沒有完整果蠅圖個體保存或訓練結果。
- 單檔限制 64 MiB；此限制不等於保存／讀取時的程序 RSS 上限，超過上限的個體快照需要另一種容量契約。
- 尚未在 Ubuntu 1 或 Windows PC 執行此檔案恢復測試；交叉編譯不算平台執行。
- 真實授權語音上的匯入、訓練、推論與評估仍屬 TSK-11 的 blocked_data。

### TSK-03

- A three-word synthetic grammar; no claim of Chinese dialogue, reasoning or knowledge (the README and the report say so).
- The task check accepts any object followed by 。, so a model that always answers 米 is correct; it measures grammatical completion, not choosing the right object.
- Perplexities compare only runs with the same vocab_hash.
- Real authorised text corpora are TSK-11 and remain blocked_data.
- One machine (macOS arm64).

### TSK-04

- Nine 8x8x3 fixture images; no claim about natural images, resolution or perceptual quality.
- The held-out condition is generated wrongly in every seed (blue cross instead of blue diagonal); no compositional generalisation is shown.
- The frozen-core control matches the trained core, so the recurrent core's learning is not shown to matter on this fixture.
- Real authorised image data is TSK-11 and remains blocked_data.
- One machine (macOS arm64).

### TSK-05

- Nine synthetic tones of three pitches and three envelopes; no claim about speech, music or perceptual quality.
- The held-out condition is generated wrongly in every seed (high steady instead of high pulse).
- The frozen-core control matches or beats the trained core; the core's learning is not shown to matter on this fixture.
- Temporal consistency over three blocks is partial (2 to 6 of 9 prompts).
- Real authorised audio data is TSK-11 and remains blocked_data.
- One machine (macOS arm64).

### TSK-06

- One dot and one beep on 8x8 frames; no claim about natural video, resolution or perceptual quality.
- The held-out condition is never generated correctly at the default budget.
- At the default budget sync is weak (0 to 3 of 5 seen clips in sync). The core-versus-frozen comparison changes with the training budget (the control is ahead at 160 epochs, the core at 320), so no stable conclusion about the core's own contribution is drawn; the longer runs are post-hoc observations, not a chosen default.
- Packaged containers are not byte-reproducible because ffmpeg stamps run-specific metadata; the native files are.
- Real authorised video data is TSK-11 and remains blocked_data.
- One machine (macOS arm64).

### TSK-07

- Nine 8x8 fixture images; the three modes are compared on bookkeeping and on this fixture only.
- The external tool is a local stand-in that renders exactly the requested fixture image; no third-party model, network call, authorisation flow or real data policy is exercised beyond the allow-list, schema and call budget.
- The held-out success of the external-tool mode comes from the factorised request format, not from generation by the core.
- Time alignment is not measured: the fixed decoder is an image autoencoder.
- One machine (macOS arm64).

### TSK-08

- Fixture scale only: 9x9 maps, 16 hidden nodes, 60 training episodes, 20 evaluation episodes per split. Success rates are low and are not evidence of navigation competence.
- The feedforward control takes one optimizer update per observation, 25 to 28 times as many updates (mean 1508.7 to 1672.3 against 60) as the recurrent policy from the same demonstrations, and has 2664 parameters against 2728; it is not budget-matched in optimizer steps. The rewired control is (same parameter count, 60 updates).
- The report's config.env stores the zero fields the caller passed; the effective defaults are only visible in experiment/nav2d/env.go. Recorded as a follow-up in AGENTS.md.
- The CLI entry `examples run nav2d` (ticket 28 item 4) is not built yet; it edits internal/cli/run.go, which is on hold while parallel work on that file is coordinated.
- One machine (macOS arm64); no Linux run for this fixture.

### TSK-09

- Two small fixtures only (a length-7 corridor with two episode shapes and a five-row delayed pulse); no claim of transfer between tasks.
- The shared AdamW momentum moves an idle task's adapter, so adapter versions change even for a task that did not step at a given index.
- One seed per schedule; the report is deterministic, but no spread across seeds is measured.
- No CLI entry is part of this stage; the evidence runs through the env-gated test.
- One machine (macOS arm64).

### TSK-10

- The model does not generalize to the held-out combination (image-text retrieval 0 on all seeds); the evidence covers that pairing and unseen combinations can be built and evaluated, not that they are solved.
- Synthetic fixture only: one held-out combination, 18 unseen samples, three seeds.
- Retrieval inside the held-out subset alone is trivially 1 with one held-out label; the report's unseen_retrieval field, which searches the whole gallery, is the one to read.
- The CLI entry `examples run multimodal` is not built yet.
- One machine (macOS arm64).

### TSK-12

- The normal baseline did not learn under the pre-registered learning rate 0.05 once the encoder trains with the core, so the comparisons say little about the core's contribution. Choosing a rate or per-group rates would need a new pre-registration on other seeds before rerunning seeds 1 to 3 (AGENTS.md follow-up).
- Three seeds give wide intervals; a success difference of 0.05 is one evaluation episode per seed.
- Fixture scale only (9x9 maps, 16 hidden nodes, 60 episodes); not the fly connectome.
- The report's config.env stores zero fields that nav2d.New replaces with defaults (AGENTS.md follow-up).
- The CLI entry `examples run attribution` is not built yet.
- One machine (macOS arm64).

### STA-01

- Fixture only. Both models are hand-built test fixtures of two and three neurons; nothing here is a real-connectome, whole-graph or biological claim, and no package was built from a MaleCNS graph.
- The training snapshot's completeness is scoped to what the trainer actually has. No replay store, teacher signal, fast weights, chemical state or RNG stream exists anywhere in the framework yet, so 'complete' means parameters, optimizer options, Adam moments, per-parameter step counts, update count, and for the episode format the data seed and sample cursor. STA-03 therefore stays specified: it additionally requires fast weights and chemical state, which do not exist.
- The model package carries no data-provider cursor by design, so a package alone cannot resume a training job; that is the individual snapshot's and the episode checkpoint's role.
- The topology fingerprint is asserted to share simulate's encoding by the documented rule plus an independent recomputation in the test, not by calling simulate's own (unexported) implementation. The two implementations could drift without a test noticing.
- Load on a model package and on an individual snapshot is refused by the pre-existing schema check in checkpoint.go, which names the kind through its schema string rather than through a sentence; checkpoint.go was outside this stage's file ownership and was not changed.
- Evidence registry paths are stored verbatim and checked only for being relative and not escaping (no leading slash, no backslash, no .. segment). The framework never opens them and does not verify that the records exist or describe this model.
- Executed on macOS arm64 (go1.26.5) only; no Ubuntu or Windows run is recorded for this requirement.
- The recorded green run was produced in an isolated copy of the working tree with a concurrent agent's unfinished ticket 17 files removed and learning/ restored to commit 32a72fd, because those files did not compile at that moment. Every ticket 16 test also passes in the shared tree itself (21 of 21), where the only failure is that agent's own unimplemented red test.

### STA-02

- Continuous CPU fixture profile only; no LIF, chemical state, local plasticity, whole-brain training, GPU execution or Windows runtime validation.
- TrainEpisode starts an independent zero-state sequence; no gradient crosses Advance calls. Data-provider cursor and external RNG are not part of this individual snapshot. STA-01 and STA-03 remain specified.
- Per-matrix/history element limits and the 64 MiB JSON limit are not process memory bounds.
- Cross-CPU loading is verified for the synthetic probes, not bit-identical future trajectories or arbitrary hardware.
- Legacy episode Load still accepts some missing/null scalar fields as zero; this pre-existing issue is recorded in ticket 05. The new individual loader rejects them.

### STA-03

- Fixture scale only: three neurons, three edges, one region, one channel, one receptor, eight episodes of a synthetic delayed-pulse stream. Nothing here is a real-connectome, whole-graph or long-run training claim.
- Only the continuous core is resumed with the receptor-gated plasticity rule. The LIF path and the DecayEReceptor path are covered for save/load round trip (TestLoadIndividualAcceptsScalarPointerRuleFields) and for a plain subprocess resume (TestLIFIndividualCheckpointSubprocessResume), but no test resumes a spiking individual that has plasticity and chemistry enabled, and no resume test exercises DecayEReceptor.
- AdvanceGated (learning/individual.go:408) is not exercised: the fixture only calls Advance, which the implementation documents as AdvanceGated with a zero gate on every row, so the fast weight moves through the chemical receptor gate rather than through a caller-supplied learning gate. A resume across explicitly gated advances is unverified.
- No real data and no real training job: episodes come from experiment.DelayedEpisode, and the interruption is a clean SaveIndividual at an operation boundary, not a crash, a kill or a partial write.
- No independent random generator exists in this profile, so 'randomness' is verified only in the sense that the data seed and index fully determine the stream. A future stochastic source would need its own persistence and its own verification.
- WeightDecay is 0 in the fixture, so the AdamW decoupled-decay term is never non-zero during the resume; only the Adam moments and per-parameter step counts are actually compared across the break.
- The chemical layer is compared as part of the whole-snapshot reflect.DeepEqual, but unlike the fast weights there is no assertion that the concentrations are non-zero at the break, so this run does not independently prove the chemical comparison was not a comparison of zeros.
- One run, one platform: macOS arm64, go1.26.5, single -count=1 -race execution. No repeat-count stability run, no Ubuntu or Windows run, no cross-CPU bit-identity claim.

### STA-04

- Continuous CPU fixture profile only; no LIF, chemical state, local plasticity, whole-brain training, GPU execution or Windows runtime validation.
- TrainEpisode starts an independent zero-state sequence; no gradient crosses Advance calls. Data-provider cursor and external RNG are not part of this individual snapshot. STA-01 and STA-03 remain specified.
- Per-matrix/history element limits and the 64 MiB JSON limit are not process memory bounds.
- Cross-CPU loading is verified for the synthetic probes, not bit-identical future trajectories or arbitrary hardware.

### STA-05

- 模型包（coimnet-model-package/v1）與訓練快照（coimnet-episode-checkpoint/v1、coimnet-episode-training/v1）的跨版本遷移沒有實作，也未在這裡驗證；Migrate 對這兩種 schema 只做同 schema 的嚴格解碼與逐位複製。
- precision_mapping 目前恆為空（報告裡是 null）：沒有任何遷移規則在重寫時改變數值型別，所以沒有 PrecisionMapping 被填入。
- 只在單一 fixture 個體上跑過（sample-src.json，2 節點、1 邊的連續個體），沒有用真實全圖或訓練產生的個體檔案驗證遷移。
- 未驗證的重點：不變的舊版 episode 快照、壞 checksum 的 individual 文件、路由器或檔案系統層級的原子發布失敗（source 與 target 在不同檔案系統時）。

### STA-06

- CLI 日誌尚未全面經過 redact.Writer：internal/redact 只提供元件（Line 與 Writer 各五個測試），本輪沒有把套件接進所有 CLI 命令的 log 輸出或 report。
- 樣式清單有限：reJSON 只抓雙引號 JSON 字串、sk- 樣式要求至少 8 個字元、Bearer 只遮單個非空白 token；遮罩未覆蓋的樣式會原樣通過。
- internal/cli 的 net/http 偏差仍在：allow-list 為了 data sources 的 HEAD 檢查留了 internal/cli，尚未收緊到只剩 download 與 teacher（把 HEAD 檢查搬進 download 是待辦）。
- TestScanCommitScriptFlagsSecrets 只覆蓋了四條規則中的 secret 與 size，以及乾淨路徑的 PASS 案例；data 與 teacher_response 兩條規則沒有對應的 CLI 子測試，scripts/scan-commit.sh 的規則本體仍是唯一證據。

### OPS-01

- 票面驗證指令的 -run 樣式 'TestNoPanic' 在根套件裡沒有匹配到任何測試：`go test -list . .` 列出的測試是 TestOnlyNetworkPackagesImportHTTP、TestNoTelemetry、TestScanCommitScriptFlagsSecrets、TestCancelReleases、TestGovernance*（四個）、TestPublicAPINeverPanics、TestZeroValueGettersAreSafe 與八個 Example_*。'Example\|TestNoPanic\|TestCancelReleases' 實際只跑到 TestCancelReleases 與八個 Example（9 個頂層 PASS）；非法輸入不 panic 的證據是第二條指令補跑 TestPublicAPINeverPanics 得到的，不在原指令的輸出裡。
- 16.1 列的能力中，「載入資料」與「評估」沒有對應的 Example：資料載入只在 connectome／download 的既有測試裡驗證，評估留在 CLI 的 `examples run evaluate` 與 experiment 套件，SDK 層沒有可執行範例。
- 「註冊自訂編碼／動態／學習／調節／任務」只以既有介面的宣告式設定示範，沒有註冊 API：Example_enablePlasticity 與 Example_enableChemistry 傳的是 plasticity.Config 與 modulation.ChemistryConfig，票面 Root 決策 5 寫的 experiment.RegisterGenerator 在這個 build 不存在（`grep -rn 'func RegisterGenerator'` 無結果），任務產生器是 config/registry.go 裡的私有 map generators，外部套件無法新增自訂項目。
- 「不公開裸指標」沒有在本次證據裡直接驗證：這裡沒有跑專門的別名測試，該性質由 checkpoint 與 learning 既有的 alias-isolation 測試（例如 TestSaveLoadIndividualRoundTripAndAliasIsolation）支撐，不在 evidence/OPS-01/test.log 內。
- 取消釋放的 HeapInuse 比較以一個刻意保留的 4 MiB pad 當底線（cancel_release_test.go 的 baselinePadBytes），比的是 1.1 倍上限，不是逐位元組的洩漏證明；goroutine 數是 ±0。
- 只在 macOS arm64（go1.26.5 darwin/arm64）上跑過一次，沒有跨平台或重複執行；共用機器，量到的 HeapInuse 與耗時會隨負載變動。

### OPS-02

- 票面驗證指令的 -run 有兩個樣式沒匹配到任何測試：'TestExamples' 與 'TestResume'（`go test -list . ./internal/cli/ ./checkpoint/` 裡沒有以這兩個字串開頭或包含它們的測試名）。實際跑到的 46 個頂層測試是 internal/cli 的 38 個（TestBenchmark×3、TestCheckpointMigrateCLI×6、TestRunDryRun×5、TestRun 其他×9、TestContinualMatrix×3、TestDoctor×2、TestExport×3、TestReport×2、TestModel×5）與 checkpoint 的 8 個（TestMigrate×5、TestModelPackage×3）。
- 以下命令的測試存在但本次沒有執行，因此它們的端到端與失敗覆蓋不在 evidence/OPS-02/test.log 裡：data sources／download／inspect／import／derive／validate（TestData*）、simulate run／compare（TestSimulate*）、examples run evaluate（TestEvaluate*）、train delayed／resume／predict（TestTrainResumeAndObservationOnlyPrediction）。要涵蓋整張 16.2 表，-run 需要補上 'TestData\|TestSimulate\|TestEvaluate\|TestTrainResume'。
- 未實作的命令：`ablate` 完全沒有（票面說由 22 接上）；`evaluate` 沒有頂層命令，只有 `examples run evaluate`。兩者實跑都回 'unknown command; use coimnet --help' 並以狀態 1 結束。
- `run` 只實作 --dry-run：TestRunWithoutDryRunSaysWhichStageImplementsIt 釘住「不帶 --dry-run 時回報本階段未實作」，所以 16.2 要求的「依完整設定執行訓練／推論／評估」與「SIGINT 後返回非零並寫出部分報告」都沒有驗證。
- `resume` 沒有吃組態，也沒有 Root 決策 6 要求的 --allow-option-change 防護（`grep -rn 'allow-option-change'` 全專案無結果），所以「不因新預設改變原實驗」這條沒有實作也沒有驗證。
- 「JSON 輸出皆有 schema_version」有兩處例外：`examples list` 輸出的是沒有外層信封的 JSON 陣列；`model inspect` 與 `model validate` 的 schema_version 是被檢查檔案的 schema，不是報告自己的 schema 版本。
- `export` 沒有 --help 測試（export_report_test.go 只對 `report --help` 斷言），本次是手動實跑確認四段齊全。`doctor` 與 `data sources` 的 --help 沒有選項段，因為它們沒有旗標。
- benchmark 的數字是共用機器上的單次量測（load average 3.1–4.2），重跑會變動；rss_mib_after 來自 runtime.ReadMemStats 的 Sys，不是作業系統回報的 RSS；拓撲是合成的，不能代表真實接線圖的成本。
- 只在 macOS arm64（go1.26.5 darwin/arm64）上跑過一次，沒有跨平台或重複執行。

### OPS-03

- run without --dry-run is stage two. This stage refuses it with a message pointing at train and simulate; nothing here executes a configuration.
- The teacher package is ticket 27. The zero-call proof uses a counting fake that satisfies config.TeacherCaller, not a real client, and the configuration's teacher kind http/v1 is a declared name with no implementation behind it.
- The fixture is a three-neuron model package. Nothing here was run against a real graph store; the graph path of the model check is covered by a failure test only, and loading a whole-brain store inside a dry run has not been measured.
- The estimate is arithmetic over declared arrays and is not a resident set size. It excludes Go allocator slack, decoded JSON and the runtime, in the same way connectome.StoreLimits does.
- The command line evidence is one machine, macOS arm64, and one run of each command. Two other agents were editing learning/, dynamics/, checkpoint/ and simulate/ during the session, so go test ./... was run repeatedly until the tree built; the recorded log is a run in which every package compiled.

### OPS-04

- 其他平台未實際執行：linux/amd64、linux/arm64、windows/amd64 僅完成交叉編譯與 vet，未在目標平台環境實際執行測試。
- 並行模式只在 fixture 量過：Workers 分區與位元相同保證目前僅在 fixture 拓撲驗證，尚未在真實全圖（male-full）上實測並行表現。
- 熱路徑配置以 allocs/op 記錄而非 profile：熱路徑記憶體配置量以 go test -benchmem 的 allocs/op 記錄固定常數，未進行深入的 CPU/記憶體 profile。

### OPS-06

- The male-full number is arithmetic. No training run on the whole graph was executed, and nothing here measures resident set size, wall clock or temporary space for it.
- The 4.38 GB in the comparison is the NAT-01 simulation's measured maximum resident set size on macOS arm64, a different workload from the plan estimated here. The two are reported side by side only.
- StateDim 1 and the four-input, one-output peripheral counts are choices made in this record, not values ticket 24 fixed. Changing them moves the total by about a tenth of a percent.
- BufferFactor 0 means no working-buffer allowance is included. A real run will need scratch space that this total does not cover.
- The buffers item is computed in float64 before conversion, so a very large buffer factor loses precision below one byte; the overflow guard refuses anything that does not fit a uint64.
- run without --dry-run is stage two, and the teacher package is ticket 27; see evidence/OPS-03/verification.json for what the dry run does and does not do.

### OPS-07

- A single update step; multi-step whole-brain training and task accuracy are separate research results, not claimed here.
- The input drives one ALIN input unit and the target moves one readout; the gradient window is 8 of 16 rows.
- Under the default Go collector the process footprint reaches about 1.7 times the estimate; set GOMEMLIMIT (see AGENTS.md follow-ups) on machines near 16 GiB.
- CPU only on one Mac; no GPU backend (OPS-05 is blocked on the backend decision).

### OPS-08

- CPU only; the transfer column is always 0 until a device backend exists (OPS-05 is blocked).
- Two repeats on the full graph give one steady run per stage, so min, median and max coincide.
- rss_mib_after is the Go runtime's obtained memory, not the kernel's resident set; the /usr/bin/time log gives the process peak.
- One machine; timings are not portable.

### NAT-01

- Parameters are an engineering assumption, not biology; see assumption_statement. The report repeats the same statement in its assumptions field.
- No null model was run. Nothing here separates the contribution of the wiring from the contribution of the uniform parameter set; that comparison is ticket 14 (NAT-03).
- One protocol on one machine: macOS arm64 only, no Ubuntu or Windows real-data run, and an unrelated large download was competing for disk during both runs.
- The 8 GiB --max-memory-bytes limit is an accounting limit over the declared arrays; the measured 4.38 GB maximum resident set size includes the loaded store, Go allocator slack and runtime overhead that the accounting deliberately excludes.
- The whole-brain protocol was run twice with identical results, which is a reproducibility observation rather than the byte-identical guarantee the package and CLI tests verify directly; timing and resident set size are single-machine samples and varied between the two runs.
- Probe node sets were selected by annotation value only. descending_neuron and vnc_motor are official superclass labels; calling them behaviourally meaningful sets is not established here and belongs to the named-neuron protocol of ticket 14 (NAT-04).

### NAT-02

- Signs are rule-derived, not measured. The release provides predicted transmitter probabilities per presynaptic site; the mapping from a transmitter to +1 or -1, the inhibitory glutamate assumption and the two thresholds are engineering decisions recorded in the rules file, and a different rule file would produce a different network.
- The (x,y,z) identity between tbar-neurotransmitters and the presynaptic coordinates of syn-partners is confirmed only to the extent the matched ratio shows: every one of the 124,025,046 synapse rows between two selected neurons matched a usable T-bar key exactly, with no ambiguous key and no unmatched row, which is strong evidence that the two files share one voxel coordinate system, but it is a consistency observation, not a statement from the release documentation.
- matched_fraction uses the raw release weight of a pair as its denominator, so the min_matched_fraction 0.5 threshold rejected nothing here (every kept pair matched at ratio 1.0); the threshold is therefore untested against real data that would trigger it.
- Node scalars are still uniform engineering values and every delay is zero, so this is not a biological parameter set either; only the edge signs and strengths changed relative to NAT-01.
- No null model was run. The difference between the two runs separates two parameter assumptions on the same wiring; it says nothing about the contribution of the wiring itself, which is ticket 14 (NAT-03).
- One protocol, one policy and one machine: macOS arm64 only, unknown_sign exclude only, and the excitatory and inhibitory policies were exercised only in the package tests on the fixture.
- The whole-brain derived run was executed once at this build; byte-identical repetition is verified directly by the package and CLI tests, not by a second whole-brain run. Wall time and resident set size are single samples.
- Memory accounting and the 6 GiB and 8 GiB limits are accounted-array limits: the measured maximum resident set sizes include the loaded store, the parameter set arrays, Go allocator slack and runtime overhead that the accounting deliberately excludes.

### NAT-03

- Three seeds per kind. A quantile is one of at most three values a seed really produced and a percentile can only take the values 0, 1/6, 1/3, 1/2, 2/3, 5/6 and 1. It is a position in a distribution, not a test, and no test was declared before the run.
- No behavioural claim. alin, descending_neuron and vnc_motor are annotation labels chosen by the protocol; that they mean anything about fly behaviour is not established here and is not claimed.
- One protocol, one stimulus, one core, one unknown-sign policy, one weight_scale, one swap_factor and one machine (macOS arm64). The same matrix on the continuous core is evidence/NAT-05/.
- degree_preserving_rewire preserves every in and out degree but not the self loop count: it refuses to create a self loop and can destroy one the graph already has. The import report of this store records 101 self loops and 0 duplicate pairs among the 25,563,197 edges, so at most 101 edges are affected by that asymmetry, and the algorithm requires unique pairs, which this store satisfies.
- The rewire keeps the multiset of weights and signs but not their relation to the wiring, and the two shuffles keep the wiring but not the parameters: the three kinds therefore separate different things, and none of them is a control for all of the assumptions at once.
- Each cell was executed once. Byte-identical repetition of a compare matrix is verified directly by the package and CLI tests on fixtures, not by a second whole-brain matrix; the only whole-brain repetition here is cell 0 against the NAT-02 run, which matched exactly.
- Wall time and maximum resident set size are single samples from a shared machine whose load average was 3.03 before and 2.65 after; this session's own builds and tests ran on it shortly before the matrix.
- The 8 GiB accounted limit is an arithmetic bound over the arrays the simulate package allocates. The measured 5,909,626,880 byte maximum resident set size includes the loaded store, the parameter set arrays, Go allocator slack and runtime overhead the accounting deliberately excludes.
- The null model reports record topology and parameter hashes so a reader can rebuild each derivative from the store and the seed and compare, but that rebuild was not performed again here from a separate process.

### NAT-04

- A pass says only that the metric reached the declared value under the declared rule. Every one of the five thresholds also passes in all nine null model cells, so none of them distinguishes the real wiring here; reading a pass as evidence about the wiring or about behaviour would be wrong.
- The thresholds were never tightened or loosened, which also means they were never calibrated: they were guesses made before any whole-brain number existed, and the matrix shows most of them were loose.
- One protocol, one stimulus, one core and one machine. The same sets read out on the continuous core are evidence/NAT-05/.
- activity_ratio_vs_baseline uses the pulse window as its baseline, so it is a ratio of free-running to driven activity, not to rest. vnc_motor had no baseline spike at all, which is why its ratio is undefined on the original wiring.
- The sets are annotation values only. No selector combines fields here, and no set was checked against any independent definition of the population it names.
- Each cell was executed once; wall time and resident set size are single samples from a shared machine.

### NAT-05

- No metric kind is shared between the two matrices. mean_output is the only kind the continuous core can produce, and the spiking protocol declares the four event kinds instead, although mean_output is available on the spiking core too. The cross-core statements above therefore compare percentile patterns for the same set and the same window, not two measurements of one quantity. Declaring mean_output on the spiking protocol as well would have given a literally shared metric; the protocol was written before the run and was not changed afterwards.
- Three seeds per kind, so a percentile can only be 0, 1/6, 1/3, 1/2, 2/3, 5/6 or 1. It is a position in a distribution, not a test, and no test was declared.
- The cores disagree about the direction of the rewire and weight_shuffle effects on the two output sets. That disagreement is reported, not resolved: nothing here says which core is the better model of anything.
- One protocol pair, one stimulus, one unknown-sign policy, one weight_scale, one swap_factor and one machine (macOS arm64).
- No behavioural claim. alin, descending_neuron and vnc_motor are annotation labels, and mean_output is a core state average, not a motor output.
- degree_preserving_rewire preserves every in and out degree but not the self loop count: it refuses to create a self loop and can destroy one of the 101 the store's import report records among 25,563,197 edges.
- Each matrix was executed once. The cell-by-cell agreement between the two processes verifies that the null model derivation is reproducible from the seed, but not that a whole compare report is byte identical on repetition; that is verified directly by the package and CLI tests on fixtures.
- Wall time and maximum resident set size are single samples from a shared machine; the accounted memory limit is an arithmetic bound over the arrays this package allocates and excludes the loaded store, the parameter set arrays, allocator slack and runtime overhead that the measured resident set size includes.

### NAT-06

- Every constant of the rule and the gate is an engineering choice; see assumption_statement. The run report repeats them in its assumptions field.
- No null model was run alongside the learning variants, so this matrix does not separate how much of the change is the wiring's doing. That comparison needs the ticket 14 null models in the same protocol and is a different question with its own cost.
- Each matrix ran once. There is no repeat measurement, and the wall clock and resident set size include other agents compiling and testing on the same machine.
- The w_min floor held no edge in either whole-brain run (clamped_by_w_min 0), so the guarantee that a fixed sign edge never crosses zero is exercised on the fixture (simulate.TestPlasticFixedSignEdgeIsHeldAtWMin) and not on real data.
- The plastic_max bound held 33,391,125 and 34,549,873 entries, so a large minority of edge steps sat at the bound; the recorded deltas are the behaviour of a bounded rule, not of the unbounded law.
- stdp_pair was not run on real data. It is covered by the hand calculation on the fixture only.
- simulate run has no entry point that saves or restores the fast changes, so a plastic run cannot be continued across processes; --state-in and --state-out are refused together with the block rather than silently dropping the fast state.
- The matrix compares three cells over one stimulus. It says nothing about a task, a reward or whether the change is useful.

