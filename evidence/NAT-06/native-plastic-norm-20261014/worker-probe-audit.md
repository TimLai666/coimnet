# Native probe reduction audit

## Problem

本輪只審查 `simulate/runner.go` 的 native probe reduction。問題是 `mean_output` 先逐節點相加，再除以節點數；若每個連續輸出都是有限值，這個中間總和仍可能先成為 `+Inf`，使有限的真實平均被拒絕。依 `evidence/NAT-06/native-plastic-norm-20261014/followup-probe-plan.md`，本報告不把私有 `reduce` 的孤立輸入當成公開缺陷，也沒有執行 Go 或套件 runtime。

## CONFIRMED

無。依本輪限制，沒有可列為 runtime 已確認的 finding。

## NEEDS INVESTIGATION

### P2 candidate：`mean_output` 的 finite mean 可能被 sum-before-divide 拒絕

- **來源與契約**：`simulate/runner.go:377-396` 將 `mean_output` 設為 `scaleByOne`；`simulate/runner.go:807-836` 仍先以 `sum += outputs[node]` 累加，於 `:830-834` 才除以節點數並檢查 `finite(sum)`。`ReduceSumOutput` 也共用同一段累加，但不除法。
- **公開可達性**：`dynamics/continuous.go:99-104` 允許 `tanh` 與 `softplus`。`tanh` 的輸出有界，但 `dynamics/continuous.go:477-482` 的 softplus 對正的大值近似原值，沒有上界。`dynamics/state.go:83-96` 對 softplus history 只要求有限且非負，`dynamics/state.go:217-226` 對 drive、voltage、output 只拒絕非有限值。`simulate/protocol.go:254-264,327-334` 接受 continuous `mean_output` 與有限 uniform scalar，`simulate/parameters.go:87-118,324-345` 也只要求參數有限、形狀及 hash 正確。因此這條路徑不是只存在於私有 helper 的假想輸入。
- **公開 Build/RestoreState/Run recipe**：使用任何至少三節點、邊權有限且可由 `connectome.Graph` 公開載入／建出的 graph。套件內三節點 fixture（連續索引 `0,1,2`，有限邊權）可直接作為 graph 來源。建立：

  ```text
  protocol = Protocol{
    SchemaVersion: ProtocolSchemaVersion,
    Core: CoreContinuous,
    Continuous: &dynamics.Config{DT: 1, Activation: "softplus"},
    Injections: []Injection{{Channel: 0, Node: 0, Gain: 0}},
    Probes: []Probe{{Name: "mean", Nodes: []int{0, 1, 2}, Reduce: ReduceMeanOutput}},
    Stimulus: StimulusSpec{Pulse: &Pulse{
      Channels: 1, Steps: 1, Channel: 0, Onset: 0, Duration: 1, Amplitude: 0,
    }},
    ParameterSource: ParameterSourceUniform,
    Uniform: &UniformParameters{
      Gain: 0, Bias: math.MaxFloat64, LogTau: 0, ThetaRaw: 0,
    },
  }
  params, _ := UniformPositive(ctx, graph, *protocol.Uniform)
  first, _ := Build(ctx, graph, params, protocol, Limits{MaxMemoryBytes: 1 << 30})
  snapshot := first.State()
  second, _ := Build(ctx, graph, params, protocol, Limits{MaxMemoryBytes: 1 << 30})
  _ = second.RestoreState(snapshot)
  _, err := second.Run(ctx, second.Stimulus())
  ```

  `Build` injects the graph topology and constructs the softplus core (`simulate/runner.go:104-114,202-249,329-337`); the zero initial continuous state is valid and `RestoreState` validates and copies it (`simulate/runner.go:430-458`, `simulate/core.go:106-121`). The zero injection and zero pulse make every input finite zero (`simulate/runner.go:779-795`). With `alpha = -expm1(-1) = 0.6321205588285577`, each node computes `next = alpha * MaxFloat64 = 1.1363587890114286e308`; softplus evaluates to the same finite value because the negative exponential underflows to zero. The three-node mathematical mean is that same finite value, while sequential `sum` over the second node is `+Inf`; division by three cannot recover it.
- **Error and commit ordering**: `Run` calls `reduce` before appending a probe value (`simulate/runner.go:704-714`), so this recipe should return `simulate: probe "mean" produced a non-finite value` from `reduce`. `report`'s `checkFinite` guards are later at `simulate/runner.go:737-740,864-870`; they do not make this case safe because the reduction exits first. The state commit is later still (`simulate/runner.go:753-755`), so the failed call should leave the restored runner state unchanged. This is the required late-commit property, but it was not runtime-executed in this audit.

## Independent arithmetic evidence

Only Python standard-library arithmetic was used, independently of `reduce`:

```text
alpha = 0.6321205588285577
next/output = 1.1363587890114286e308  (finite)
2 * output = inf
3 * output = inf
true mean of three equal outputs = 1.1363587890114286e308  (finite)
```

The exact decimal product of the three represented outputs is approximately `3.409076367034285839432582792e308`, above `MaxFloat64`, while division by three returns the represented output. This establishes the arithmetic precondition, not public runtime execution.

## Root suggested tests

Root should run the recipe through the real public `Build` → `RestoreState` → `Run` path and record the result before classifying it as confirmed. The test should assert all of the following:

1. `Build`, `RestoreState` and input validation accept the finite setup.
2. `Run` returns the reduction error for the three-node `mean_output` probe.
3. `Runner.State()` is bitwise unchanged after the failed call, and a separately built runner accepts the same snapshot.
4. A control with one probed node, or with a finite lower bias such as `1e307`, succeeds and returns a finite mean, proving the failure is the intermediate sum rather than softplus state rejection.
5. A `tanh` control remains bounded; `dynamics/state.go:249-256` also caps a continuous model at `MaxStateValues`, so the bounded `tanh` sum cannot overflow through the permitted node count.

No implementation or API policy is proposed here. Whether the fix should use scaled accumulation, compensated accumulation, or another contract-preserving method is a Root decision after runtime evidence.

## Scope, compatibility and performance

- **Scope**：continuous native probes only; no change to LIF, plasticity, ticket 89, or pre/post timing policy.
- **Compatibility**：the static path preserves existing `mean_output` semantics for ordinary finite magnitudes; the candidate concerns only a finite softplus output whose sum is not representable.
- **Performance**：no benchmark was run. Any replacement accumulation strategy must be measured against the existing single pass and must keep late atomic commit semantics.
- **Uncertainty**：Go execution, `Build`, `RestoreState`, `Run`, and package tests were intentionally not run. The candidate is therefore `NEEDS INVESTIGATION`, not `CONFIRMED`; no runtime success or failure is claimed.

## Read fingerprints and working-tree boundary

- **HEAD**：`9dbc7aeda009c173c56dfd4b9e3446a402733bb7` (task-provided reference; no Git write was attempted).
- **Read SHA-256**:
  - `simulate/runner.go`: `22b2f0be883a8797540df3b502ebc0178a312d99e34a767874a4e33232660dba`
  - `dynamics/continuous.go`: `cc85e745832cffddfe7a41fa65c5c1eb485ce1f995776b815cf19fcb41c8a7d4`
  - `dynamics/state.go`: `71b3d63dfe06fda49eca4e0734fa5328b5c3dac30c004271e5e416ce0e4a0e88`
  - `simulate/core.go`: `60641d3f7cd49c7e7341262a70aed0a36b14e6152a5aa6492023c9425dc1fdd9`
  - `simulate/parameters.go`: `38bcdb81e807c24dd9f571cba48fd3f33a3c71196dbc87f8d4694f779f6fbc38`
  - `simulate/protocol.go`: `3f5e374b3811ff69bf90dd9b3f8548f511f4f973360e05114ab9ed3f0b037d3d`
  - `simulate/report.go`: `68c633d0d496c7024a18b7bad89f528edbaffeea0eee2a0c505f70eecc5d78bd`
  - `ENG.md`: `30fef6128de0caee5d918690eb5d7cf2e5dc6d000274cebe15996e2a212b1591`
  - `evidence/NAT-06/native-plastic-norm-20261014/followup-probe-plan.md`: `e2545ba5f33e7851efcd473eefb6c6d524cc71c059c75b035ab7fc6973addf5d`
- The working tree already contained unrelated parent changes (`ENG.md`, `delivery-status.md`, ticket 93, candidate sources/tests and evidence). They were read-only for this audit and untouched.

## Actual modification

Only this file was added:

`/Users/timlai/Developer/coimnet/runs/.tmp/native-plastic-norm-20261014/review/probe-audit.md`

No source, test, evidence, cache, Git index, commit, stash, reset, network, service, or Go execution was performed.

## Problem / Done / Changed / Actions / Verified / Notes / Next

- **Problem**：確認 native continuous probe 的 sum-before-divide 是否可能拒絕有限 mean。
- **Done**：依來源追蹤 activation、finite validation、Build／RestoreState／Run、reduction、report guard 與 late commit，並以獨立 Python 算術形成公開 recipe；候選嚴格標為 `NEEDS INVESTIGATION`。
- **Changed**：僅新增本報告檔。
- **Actions**：唯讀 `cat`／`sed`／`rg`／`shasum`、`git status --short`、`git diff --stat`；未執行 Git 寫入。
- **Verified**：來源行號、讀取 SHA-256、工作樹邊界與獨立浮點算術均已記錄；未驗證 Go runtime。
- **Notes**：真正 runtime 結果、錯誤文字與 state 不變性仍待 Root 執行公開 recipe 確認。
- **Next**：Root 先執行上述 Build／RestoreState／Run reproduction，再決定是否新增針對 sum-before-divide 的回歸測試與實作修正。

Skills：investigate, diff-inspector
