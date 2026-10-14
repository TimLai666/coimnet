# LIF Adam 浮點第一分歧審查

## Problem

`STA-03` 的 `lif-w1-rfalse-sfalse-cfalse` 案例在普通建置與 `-race` 建置間觀察到 `optimizer.second[7]` 不同：普通為 `9.812042275199087e-06`，race 為 `9.812042275199089e-06`。兩者同一建置的新程序恢復都 exact，但目前沒有 `gradient`、`mean`、`clipScale`、`d` 或 Adam 中間乘積的 bit pattern，因此不能把 FMA 或某個編譯器重排宣稱為根因。

## Done

唯讀核對了既有 `/Users/timlai/Developer/coimnet/evidence/STA-03/training-cancel-audit-20261015/float-build-review.md`、ordinary/race assembly、child-resume diagnostics、人工 before/expected archive、`learning/trainer.go` 的 Adam 資料流，以及本機 Go 1.26.5 官方規格與 `math.FMA` 實作。沒有建置、測試、修改正式 Go、修改探針、建立額外暫存或寫入 Git。

## CONFIRMED 觀察

### 1. 現有證據支持跨建置數值差異，不支持 same-build restore 損壞

- `/Users/timlai/Developer/coimnet/evidence/STA-03/training-cancel-audit-20261015/child-resume-environment-diagnostic.json:1-121`：普通建置在 `GOMAXPROCS` 未設定及 `2` 各三次都 JSON 相同；race 建置在兩種設定各三次都只報告 `.snapshot.optimizer.state.second[7]`。
- 同一檔的差異是普通 `9.812042275199087e-06`、race `9.812042275199089e-06`。以 IEEE-754 float64 bits 讀取，分別為 `0x3ee493ccd25ca90b` 與 `0x3ee493ccd25ca90c`，是相鄰兩個可表示值。
- `/Users/timlai/Developer/coimnet/evidence/STA-03/training-cancel-audit-20261015/child-resume-same-build-diagnostic.json:1-15`：普通建置 `second_before=4.802241627766806e-06`、`second_expected=second_resumed=9.812042275199087e-06`；race 建置同一 `second_before`、`second_expected=second_resumed=9.812042275199089e-06`。兩邊 `same_build_resume_json_equal` 都是 `true`。
- `/Users/timlai/Developer/coimnet/evidence/STA-03/training-cancel-audit-20261015/resume-check-correction.json:2-7` 已確認先前失敗是拿普通建置 snapshot/control 比對 race binary，並非同一建置的恢復錯誤。

因此目前可確認的是「建置模式改變了成功更新產生的 float64 結果」，不能確認「取消後保存或載入把 state 改壞」。

### 2. 可觀察到的最早 stored divergence 早於這次 `second[7]` 更新

對 `/Users/timlai/Developer/coimnet/evidence/STA-03/training-cancel-audit-20261015/cancellation-resume-fixtures.tar.gz` 內相同 `lif-w1-rfalse-sfalse-cfalse` 的 `v2` 與 `race` JSON 做唯讀 bit-level 比較：

| 時點與欄位 | 普通 bits／值 | race bits／值 | 結論 |
|---|---|---|---|
| `before.parameters.core.bias[2]`，flat index 5 | `0xbf80b36ee8b784ca` / `-0.008154741742091427` | `0xbf80b36ee8b784cb` / `-0.008154741742091428` | race 數值更負一 ULP |
| `before.optimizer.state.first[5]` | `0x3fa6a0758b6cbb3a` / `0.04419295622789439` | `0x3fa6a0758b6cbb3b` / `0.044192956227894396` | 一 ULP |
| `before.optimizer.state.second[5]` | `0x3f29993d27bf3de7` / `0.0001953017380160592` | `0x3f29993d27bf3de8` / `0.00019530173801605923` | 一 ULP |
| `before.optimizer.state.second[7]` | `0x3ed4245e20956a29` / `4.802241627766806e-06` | 相同 | index 7 的 old second 尚未分歧 |
| `expected.optimizer.state.first[5]` | `0x3fb5bae73452a005` / `0.08488316562090865` | `0x3fb5bae73452a006` / `0.08488316562090867` | 一 ULP |
| `expected.optimizer.state.second[5]` | `0x3f3a1f48acc5c7bc` / `0.00039859318055512536` | `0x3f3a1f48acc5c7bd` / `0.0003985931805551254` | 一 ULP |
| `expected.optimizer.state.second[7]` | `0x3ee493ccd25ca90b` / `9.812042275199087e-06` | `0x3ee493ccd25ca90c` / `9.812042275199089e-06` | 一 ULP |

`before` 已是第一次 `TrainEpisode` 完成後的 archive，第二次更新才產生 `expected`。所以：

- 就 index 7 而言，這次更新的 `oldSecond[7]` 兩邊相同，分歧是在該次更新的 `d`、乘積或乘加結果中某處產生，或來自未觀察的輸入。
- 就整個案例而言，最早已觀察到的 stored divergence 是第二次更新之前的 `parameters.core.bias[2]`、`first[5]`、`second[5]`。它已經把第一個跨建置分歧推回第一次更新或更早的梯度計算；現有 archive 沒有更早的中間值，不能再往前定位。
- `expected` 的參數、`StepResult`、neural state 與 plastic state 其餘內容相同；`snapshot_json` 的差異只是上述 state 差異的序列化反映，不是額外的數值分歧。
- archive-wide 比較中，before/expected 的跨建置差異出現在 LIF 案例；continuous 與 mixed 案例沒有同樣差異。這是範圍線索，不是 LIF 根因證明。

### 3. 實際 Adam 更新資料流與 index 對應

- `/Users/timlai/Developer/coimnet/learning/individual.go:849-864` 顯示 `TrainEpisode` 在沒有 chemistry 時直接呼叫 `i.trainer.Step`。本案例 `cfalse`，所以不是 `StepFromState` 的 stateful continuous API。
- `/Users/timlai/Developer/coimnet/learning/trainer.go:301-337`：`Step` 先以私有 `network.lossGradient` 算 gradient，再交給 `stepWithGradient`。
- `/Users/timlai/Developer/coimnet/learning/trainer.go:401-413`：`flatParameters`／`flatGradient` 後，才可能做 sharing reduction 與 mask freeze。
- `/Users/timlai/Developer/coimnet/learning/trainer.go:414-427`：loss scale 乘回除回；`/Users/timlai/Developer/coimnet/learning/trainer.go:429-443`：計算本 step gradient norm。
- `/Users/timlai/Developer/coimnet/learning/trainer.go:444-482`：累積窗；window 1 直接使用 gradient，較大 window 才建立 `mean`。
- `/Users/timlai/Developer/coimnet/learning/trainer.go:483-504`：取得 `clipNorm`，必要時 `scale = ClipNorm / clipNorm`。
- `/Users/timlai/Developer/coimnet/learning/trainer.go:521-541`：每個 trainable index 先遞增 step，`d := mean[i] * scale`，再更新 first、second、bias correction 與參數。
- `/Users/timlai/Developer/coimnet/learning/trainer.go:661-673` 的 flatten 順序是 `Core.Weights`、`Core.Bias`、`Core.LogTau`、`ThetaRaw`、`Encoder`、`Readout`。本 fixture `/Users/timlai/Developer/coimnet/evidence/STA-03/training-cancel-audit-20261015/cancel-probe-v2.go.txt:55-59` 的 LIF core 有三個 bias 與三個 log tau，因此 flat index 5 是 `Core.Bias[2]`，index 7 是 `Core.LogTau[1]`。
- Adam 原始式在 `/Users/timlai/Developer/coimnet/learning/trainer.go:529-534`：

  ```go
  d := mean[i] * scale
  state.First[i] = Beta1*oldFirst + (1-Beta1)*d
  state.Second[i] = Beta2*oldSecond + (1-Beta2)*d*d
  ```

### 4. 兩份 assembly 都是兩次 `fmul` 加一次 `fmadd`

- `/Users/timlai/Developer/coimnet/evidence/STA-03/training-cancel-audit-20261015/ordinary-adam-second.asm.txt:7-13`：`fsub`、`fmul`、`fmul`、`fmadd`、store。
- `/Users/timlai/Developer/coimnet/evidence/STA-03/training-cancel-audit-20261015/race-adam-second.asm.txt:7-26`：同樣兩次 `fmul` 與一次 `fmadd`，另有 stack spill 及 `runtime.racewrite`。
- `/Users/timlai/Developer/coimnet/evidence/STA-03/training-cancel-audit-20261015/root-assembly-verification.json:32-34` 也明確記錄 `both_builds_use_two_fmul_then_fmadd=true` 與 `float_difference_root_cause_confirmed=false`。

因此不能說「只有其中一份使用 FMA」。目前只知道兩份 machine code 都把 `((1-Beta2)*d)*d` 放進 `fmadd` 的加法輸入，race 另有儀器化與不同的 register/stack 配置；沒有 source-line mapping 或中間值，仍不能把差異歸因給該段 FMA。

### 5. Public API 能讀到什麼

**可以直接取得 raw gradient，但不能直接讀出 optimizer 內部完整向量。**

- `learning.Gradient` 是公開型別，欄位在 `/Users/timlai/Developer/coimnet/learning/network.go:75-81`。
- `(*Network).LossGradient` 是公開入口，在 `/Users/timlai/Developer/coimnet/learning/network.go:258-263`，契約是以 `Parameters`、input、target、truncation window 算 MSE 與 raw gradient，不做 optimizer update。它也明示 core 使用 float64，而 Insyra encoder/loss/readout 有 float32 邊界。
- 對本案例的 no-chemistry、no-recompute、no-sharing、window 1，可以從相同 `snapshot.Config` 建 `learning.NewNetwork`，呼叫：

  ```text
  network.LossGradient(ctx, snapshot.Parameters, input, target,
      snapshot.Optimizer.Options.Truncation)
  ```

  再依 `flatGradient` 順序讀 index 5／7。fixture 的 input、target 在 `cancel-probe-v2.go.txt:74-75`。這能重算 raw gradient，並在這些前提下把 `mean` 重建為該 gradient；但它不是 trainer 已保存的「actual post-sharing/post-mask」向量。
- `StepResult.GradientNorm` 只有本 step gradient 的 norm，且明定在 clipping 與 accumulation average 前，見 `/Users/timlai/Developer/coimnet/learning/trainer.go:97-103`。它不能證明 gradient vector 相同，因為不同向量可能有相同 norm。
- `TrainingSnapshot` 可從 `/Users/timlai/Developer/coimnet/learning/trainer.go:227-235` 取得 Parameters、Options、Optimizer 與部分 accumulator；`GradientAccumulator` 的公開欄位在 `/Users/timlai/Developer/coimnet/learning/accumulate.go:5-14`，但 window 1 不會保留 accumulator。
- 沒有公開欄位或方法直接提供：post-sharing gradient、mask 後 gradient、`mean`、`clipNorm`、`clipScale`、`d`、每一項 old/new first/second 或 Adam 乘積。`Trainer.Sharing()` 只回報群組統計，見 `/Users/timlai/Developer/coimnet/learning/sharing.go:272-287`。
- `LossGradientFromState`／`StepFromState` 僅支援 CPU scalar continuous core，見 `/Users/timlai/Developer/coimnet/learning/stateful.go:9-16,41-48`，不能用來重算 LIF persistent state。但本案例 `TrainEpisode` 走獨立 zero-state `Step`，所以這個限制不妨礙上述 no-chemistry fixture 的 raw-gradient 重算。
- 一般有 sharing、mask、LossScale 或 accumulation 時，公開 raw gradient 不能等同 trainer 內部 gradient；sharing reduction 是私有的 `/Users/timlai/Developer/coimnet/learning/sharing.go:118-135`。本案例的前提可使它接近 exact，但仍沒有 API 把 clip scale 或 Adam 中間值報出來。

## 補充：OPS-05 public gradient 把可觀察分歧推到 LIF reverse

### CONFIRMED 觀察

- `/Users/timlai/Developer/coimnet/evidence/OPS-05/gpu-cancel-audit-20261015/public-gradient-comparison.json:1-4` 表示兩個 binary 的初始 snapshot 相同、此次 probe 沒有插入正式來源，且第一次更新的參數與 Adam state 都和原保存結果相同。這把 archive 內容與 probe 重算接上，但不等於跨建置逐位一致是支援契約。
- 第一次更新最早可觀察的不同已在 raw gradient flat index 5：ordinary `0x3fee65bc866308b6`、race `0x3fee65bc866308b7`，`norm_equal=true`，見同檔 `:7-15`。同一筆 index 5 的 `d` 是 `0x3fdc4892ee47ea0a` 對 `0x3fdc4892ee47ea0b`，`product2` 與 new second 也各差一 ULP，且兩邊 `actual_second_equals_fma_reference=true`，見 `:16-34`、`:56-74`。因此目前證據把首次分歧推到 Adam 之前，沒有證明是哪一個 LIF 或 float32 邊界操作先不同。
- 第二次更新 raw gradient 的不同轉到 index 7：ordinary `0xbfc3c5d7a5976b04`、race `0xbfc3c5d7a5976b05`，norm 仍相同，見 `:97-106`。index 5 的 raw gradient 此時相同，但 race 帶著第一次更新後不同的 old first/second；index 7 的 old moments 相同而 raw gradient 不同，見 `:107-184`。這符合「前一更新已改變參數，後一更新在另一項 raw gradient 顯出差異」的觀察，不能直接命名為 Adam bug 或品質下降。
- `:189` 仍為 `root_cause_confirmed=false`。本段只把最早的**已觀察資料值**定位為 step 1 的 `g.Core.Bias[2]`，不把它當成最早的 machine instruction。

### Network 到 `LIF.Backward` 的 index 5 資料流

- flat 順序在 `/Users/timlai/Developer/coimnet/learning/trainer.go:661-673` 是 weights、bias、log tau、theta、encoder、readout；LIF 有三條 edge，因此 flat index 5 正是 `Core.Bias[2]`。本案例 no chemistry、no recompute、no sharing、window 0，`TrainEpisode` 會走 `/Users/timlai/Developer/coimnet/learning/individual.go:849-864` 的一般 `Trainer.Step`。
- `Network.LossGradient` 在 `/Users/timlai/Developer/coimnet/learning/network.go:258-320` 先將 target、encoder input、readout input 送入 Insyra 的 float32 tensor，再由 MSE VJP 產生最後一步的 readout upstream。`lossGradientReverse` 在 `/Users/timlai/Developer/coimnet/learning/network.go:390-431` 以 readout tape 的 `dh.Data()`（float32）填入 core `up`，然後 `/Users/timlai/Developer/coimnet/learning/network.go:432-437` 在本案例直接呼叫 `n.core.backward`。
- LIF core adapter `/Users/timlai/Developer/coimnet/learning/core.go:171-215` 將該呼叫轉成 `l.model.Backward(ctx, trace, upstream, window)`。`encoderTape.BackwardFrom` 在 `/Users/timlai/Developer/coimnet/learning/network.go:455-483` 之後才計算 encoder/input gradient，不會回寫或改動已產生的 `cg.core.Bias[2]`。
- `dynamics.LIF.Backward` 先在 `/Users/timlai/Developer/coimnet/dynamics/lif.go:518-532` 建立 `gx` 與零值 `g.Bias`，再由 `:535-588` 反向走每個 step。對本 fixture 的 adaptation disabled、smooth false、window 0，node 2 的核心項可寫成：

  ```text
  dspike_t = gx[t+1][2]
  dcand_t  = gv_t[2] * trace.reset[t][2] + dspike_t * trace.dspike[t][2]
  input_t  = trace.alpha[2] * dcand_t
  Bias[2] += input_t                 // t = T-1 ... 0
  ```

  實際對應是 `du`、`dcand`、`g.Inputs`、`g.Bias` 的 `/Users/timlai/Developer/coimnet/dynamics/lif.go:564-577`。`gx` 還會在 `:558-563` 經過 synaptic recurrence，並在 delayed edge reverse `:589-603` 由 `driveGrad * parameter.Weight` 加回來源節點；`gv` 則在 `:585-587` 傳到上一個電壓。故不能只用最後一步 readout upstream 代數推出 `Bias[2]`。
- Forward 端先在 `/Users/timlai/Developer/coimnet/dynamics/lif.go:359-370` 依 edge 宣告順序形成 drive；serial helper 的乘加在 `/Users/timlai/Developer/coimnet/dynamics/continuous.go:208-214`。加入 bias、candidate、event derivative、reset、synaptic trace 的值在 `/Users/timlai/Developer/coimnet/dynamics/lif.go:381-420`，fast-sigmoid derivative 在 `/Users/timlai/Developer/coimnet/dynamics/lif.go:436-452`。這些 trace 值是 `Backward` 的輸入，因而是 index 5 可能更早分歧的資料。

### 可用公開 `dynamics.LIF` API 的最小重現

下列只重現 core 的 `g.Bias[2]`，不宣稱可單獨重現完整 `learning.Gradient`：

```text
model, _ := dynamics.NewLIF(dynamics.LIFConfig{
    Nodes: 3, Sources: []int{0, 1, 1}, Targets: []int{1, 1, 2},
    Delays: []int{0, 1, 0}, DT: 1, TauSyn: 1,
    ThetaMin: .05, ThetaMax: 1, VReset: -.5, RefractorySteps: 1,
    Surrogate: dynamics.LIFSurrogate{Kind: "fast_sigmoid", Scale: 2},
    Workers: 0,
})
params := dynamics.LIFParameters{Weights: ..., Bias: ..., LogTau: ..., ThetaRaw: ...}
trace, _ := model.Forward(ctx, params, []float64{0, 0, 0}, encodedInputs)
up := make([][]float64, len(encodedInputs))
for t := range up { up[t] = make([]float64, 3) }
up[len(up)-1][2] = float64(dh32)
got, _ := model.Backward(ctx, trace, up, 0)
bits := math.Float64bits(got.Bias[2])
```

入口與契約是 `/Users/timlai/Developer/coimnet/dynamics/lif.go:275-280`、`:492-500`。`encodedInputs` 必須是 Network 的 `z.Data()` 每個 float32 轉成 float64 後的 rows，而不是原始 float64 input；初始 probe 的 exact config、parameters、input/target 見 `/Users/timlai/Developer/coimnet/evidence/OPS-05/gpu-cancel-audit-20261015/float-probe.go.txt:28-41`。初始參數時，encoder `[1,0,0]` 使六列 encoded input 形如 `[float64(float32(.7)),0,0]`、`[float64(float32(.8)),0,0]` 交替；後續 step 必須使用各 binary 自己的 `before.Parameters`，不能把兩邊 step 1 後的 parameter 混在一起。

要與原 `Network` 的 `Bias[2]` 等價，還必須滿足：

1. `tensor` 在 `/Users/timlai/Developer/coimnet/learning/network.go:685-704` 對 input、encoder、readout、target 逐項做 float64→float32；encoder MatMul 的輸出在 `/Users/timlai/Developer/coimnet/learning/network.go:577-582` 再 float32→float64 才交給 LIF。
2. `h` 是 LIF `trace.Outputs()` 的最後一步 readout node 2，經 `network.go:647-663` 再轉成 float32；readout parameter 也須用 `float32(p.Readout[0])`。Insyra 的 2-D MatMul 是 `/Users/timlai/go/pkg/mod/github.com/!hazelnut!paradise/insyra@v0.3.4/nn/kernels.go:1078-1118` 的 float32 `sum += a*b`，不是 float64 dot product。
3. `dh32` 必須是 readout tape 的 float32 VJP 結果，而不是直接把 MSE scalar 當作 core upstream。對本 fixture 的單一輸出、最後一步 readout、MSE Backward seed 1，等價契約是先按 `/Users/timlai/go/pkg/mod/github.com/!hazelnut!paradise/insyra@v0.3.4/nn/autodiff.go:485-500,1052-1064` 計算 `mse32 = float32(scale * (pred32-target32))`，其中 `scale = float32(1) * (2/float32(1))`，再按 MatMul VJP `/Users/timlai/go/pkg/mod/github.com/!hazelnut!paradise/insyra@v0.3.4/nn/autodiff_attention.go:8-82` 計算 `dh32 = float32(mse32 * readout32)`。最後只把 `float64(dh32)` 放在 `up[5][2]`，其他五列與其他兩個 node 必須為零。較一般的 readout 或 `OutputSize` 則須完整重現同一個 float32 tape，不能套用這個 scalar 簡化。
4. LIF `Forward` 的 initial zero、`window=0`、`Workers=0`（或兩邊相同的固定 Workers）、同一 topology/parameters、同一 input rows 必須相同。`lif.go:33-38` 說明 0/1 是 serial reference；`Workers>1` 雖宣稱維持 edge order，仍不應在最小隔離診斷混用。

這個最小重現可回答「給定相同 float32 boundary 與 core upstream，兩個 binary 的 LIF `g.Bias[2]` 是否仍不同」。它不能在沒有 `encodedInputs`、`dh32` bit pattern 的情況下，聲稱 Network 的 raw 差異一定來自 LIF。

### 可能首次融合位置與最小可區分觀察

- **encoder boundary**：`/Users/timlai/Developer/coimnet/learning/network.go:577` 呼叫 Insyra MatMul；source kernel `sum += a.data[...] * b.data[...]` 在 `/Users/timlai/go/pkg/mod/github.com/!hazelnut!paradise/insyra@v0.3.4/nn/kernels.go:1107-1114` 可能受普通/race codegen 影響。先比較每一列 `z.Data()` 的 float32 bits。
- **readout/MSE upstream boundary**：比較 `/Users/timlai/Developer/coimnet/learning/network.go:411-429` 的 `dh.Data()` 與送入 LIF 的 `up` rows。若 encoded bits 相同而 `dh32` 不同，差異在 readout MatMul、MSE VJP 或其 float32 conversion，而非 LIF reverse。
- **LIF Forward**：`/Users/timlai/Developer/coimnet/dynamics/lif.go:383` 的 `lambda*voltage + alpha*drive`、`event` 的 `den*den`（`/Users/timlai/Developer/coimnet/dynamics/lif.go:444-452`）、以及 edge drive `:370` 是 trace 內的候選首次融合或取整位置。比較 `lambda`、`alpha`、`drive`、`cand`、`reset`、`dspike`、`syn` 的 per-step bits。
- **LIF Backward**：`/Users/timlai/Developer/coimnet/dynamics/lif.go:564-577` 的 `du`、`dcand`、`g.Inputs[t][2]` 與 reverse `g.Bias[2]` 累積，以及 `:599-602` 的 delayed-edge contribution，是 raw index 5 之後仍可首次分歧的位置。需按 reverse step 輸出每一項與累積後 `g.Bias[2]`，不能只輸出最後值。

最小區分判準是依序比較 `encoded` → `dh/upstream` → Forward trace → 每個 reverse step 的 `input_t`／`Bias[2]`。第一個不同 bits 才能縮小資料流；若這些都相同而 final `Bias[2]` 不同，才需要未插入 binary 的 source-line/DWARF/machine-code 證據。任何 logging、`math.Float64bits`、額外 local 或 `defer` 都可能改變 register allocation、spill、fusion 和 race instrumentation，沿用本報告 `:116-118` 的 overlay 限制，診斷版本必須與未插入 binary 分開保存。

## 最小診斷契約

### 需要先重建的起點

不要只用現有 `before` archive 比較第二次更新。它已含 index 5 的 cross-build divergence。用 fixture 的共同初始狀態，讓普通與 race 各跑第一次 `TrainEpisode`，再各跑第二次；兩個 build 都保留未插入診斷的 binary 與 source manifest，並把診斷輸出標記為另一份 build。

### 最小插入位置

若只能用只讀原始來源加 Go overlay，overlay `/Users/timlai/Developer/coimnet/learning/trainer.go` 的 `stepWithGradient`，不改工作樹正式檔案。每次成功 `Step` 對 index 5 與 7 輸出下列 stage：

1. 緊接 `/Users/timlai/Developer/coimnet/learning/trainer.go:401-403`：`parameter_before`、`raw_gradient`、`mask`。
2. 緊接 sharing/loss-scale 區段 `/Users/timlai/Developer/coimnet/learning/trainer.go:411-427`：`post_share_gradient`、`post_loss_scale_gradient`、`loss_scale`。本案例應確認 sharing=nil、mask=true、LossScale=1。
3. 緊接 norm `/Users/timlai/Developer/coimnet/learning/trainer.go:429-443`：`raw_norm`。
4. 緊接 mean 形成位置 `/Users/timlai/Developer/coimnet/learning/trainer.go:447-482`：`accumulator_sum_before`、`accumulator_count`、`mean`。本 fixture `AccumulateSteps=1`，也要明確輸出 `mean == post_loss_scale_gradient` 的 bits；這裡的 truncation window 仍是 0。
5. 緊接 `/Users/timlai/Developer/coimnet/learning/trainer.go:483-504`：`clip_norm`、`clip_norm_limit`、`clip_scale`。
6. 在 `/Users/timlai/Developer/coimnet/learning/trainer.go:525-534` 的每個 index 更新前後：`old_first`、`old_second`、`mean`、`scale`、`d`、`Beta1`、`Beta2`、`1-Beta1`、`1-Beta2`、`new_first`、`new_second`、以及可由獨立參考重算的兩個乘積。

每個 float64 欄位同時輸出十進位與 `math.Float64bits` 的 16 位十六進位字串；每筆還要有 `build_mode`、Go/toolchain、GOOS/GOARCH、`GOMAXPROCS`、binary SHA、source manifest、scenario、update number、flat index 與 symbolic name。至少保留 index 5 與 7，因為 index 5 已在 `before` 先分歧。

### 最小可區分判準

- `raw_gradient` 首次不同：差異在 LIF forward/reverse、surrogate、float32 boundary 或更早輸入，不是 Adam update 可單獨解釋。
- raw 相同但 `post_loss_scale_gradient`、`mean` 或 `clip_norm/scale` 不同：差異在 mask、sharing、loss scale、accumulation、norm 或 clip path。
- `d` 與 old moments 相同，但 `new_first`/`new_second` 不同：才可把焦點縮到 Adam 乘加的取整、register/spill 或 compiler codegen。
- old moment 已不同：目前 update 不是最早分歧；必須回到前一次 update 的 raw gradient 與 Adam stage。
- 所有 Adam inputs 與兩次乘積 bits 都相同，只有 new second 不同：需要未插入 binary 的 compiler/DWARF/機器碼證據；overlay 輸出本身不足以判定原始 binary。

### Overlay 的可靠性界線

`logging`、`math.Float64bits` 呼叫、額外 local variable 或 `defer` 都可能改變 register allocation、spill、排程、FMA 選擇與執行路徑，尤其 race build 還會增加 race instrumentation。因此 overlay 只能用來找資料流的第一個可觀察分歧，不能把 overlay 的 machine code 當成未插入 binary 的 codegen 證據。必須保存未插入版本的 source hash、binary hash、assembly 與結果，並把 overlay 產物另行標示。

## 獨立算術參考的適用限制

本機 `/usr/local/go/doc/go_spec.html:5048-5074`（Go 1.26.5）明定實作可跨 statement 合併浮點運算而得到不同於逐項取整的結果；明確浮點轉型才會建立阻止該融合的 precision boundary。`/usr/local/go/src/math/fma.go:93-95` 定義 `math.FMA` 是只做一次 rounding 的 `x*y+z`，可作為獨立 reference，但不表示原始 source 一定呼叫 `math.FMA`。

依 ordinary/race assembly 的兩次 `fmul` 加一次 `fmadd`，可用 Decimal 以每個 `float64` 邊界取整模擬：

```text
b2  = float64(0.999)
q   = float64(1 - b2)                 // bits 0x3f50624dd2f1aa00
p1  = round64(q * d)
p2  = round64(p1 * d)
v_fma = round64_exact(b2 * oldSecond + p2)  // FMA-style final rounding
v_sep = round64(round64(b2 * oldSecond) + p2) // separate final add
```

這個 reference 只能回答「已觀察的 operands 若相同，哪種 rounding model 會得到哪個 bits」，不能代替缺少的 `d`。以 archive 的 `oldSecond[7]`、`Beta2=.999` 與兩個 stored new values 做 Decimal 反推，只能得到假設性 `d²`：普通約 `0.0050146028890600432900`，race 約 `0.0050146028890600449841`；這不是實際 gradient 或 `d` 的觀察。

一個可區分但不能當成根因的 scalar example：使用 binary float constants、`oldFirst[7]=-0.006929820796937537`、`oldSecond[7]=4.802241627766806e-06`，並以兩次 `round64` 加 final FMA-style rounding：

- `d` bits `0xbfb220db7078c3e4` (`-0.07081386085407321`) 產生 `newFirst[7]` bits `0xbf8b4695dfbecc01` 與 ordinary `newSecond[7]` bits `0x3ee493ccd25ca90b`。
- 下一個 `d` bits `0xbfb220db7078c3e5` (`-0.07081386085407322`) 仍產生相同的 `newFirst[7]` bits，但 `newSecond[7]` 變成 race 的 `0x3ee493ccd25ca90c`。

這只證明一個 ULP 的 post-scale `d` 變化可能在 first moment 看不出來、卻讓 second moment 變一 ULP；沒有 gradient/mean/scale 就不能把這個 example 反推成實際成因。

對本 fixture，`StepResult.gradient_norm` 兩邊都為 bits `0x4001738f45e8cf53`，`ClipNorm=1`、`AccumulateSteps=1` 且 truncation window 為 0，因此若接受該 stored norm 為 exact input，可重建候選 `clipScale=1/norm`，float64 bits 為 `0x3fdd56af48aba1ca`（約 `0.4584158143684617`）。這仍是由公開結果推導的 scale，不是 trainer 直接輸出的內部欄位；norm 相同也不證明 gradient vector 相同。

對 LIF reverse 的獨立參考，必須先捕獲每一個 `float64` operand，再對候選融合逐項比較。例如 `dcand` 可比較 `math.FMA(gv, reset, du)` 與 `round64(round64(gv*reset)+du)`；Forward candidate 可比較 `math.FMA(lambda, voltage, round64(alpha*drive))` 與兩次明確 `round64` 的乘加。`g.Bias[2]` 還要依 `t = T-1 ... 0` 模擬每次 `g.Bias += g.Inputs[t][2]` 的累積順序。這些 reference 只能在 operands、float32 boundary、trace 與 upstream 都已取得時區分 rounding model，不能由目前 final raw bits 反推出實際的 LIF operand，也不能證明原始 source 使用 FMA。

## NEEDS INVESTIGATION 缺少的事實

- 第一次 `TrainEpisode` 的 raw gradient、post-transform gradient、norm、clipScale、mean、`d`、old/new moments 尚未有普通/race bit pattern；因此 causal earliest divergence 未定位。
- 第二次更新 index 5 的 old parameter/moments 已不同，index 7 的 old second 相同；需要同時收集兩個 index，不能只收集 `second[7]`。
- OPS-05 public probe 沒有保存 encoder `z.Data()`、readout `dh.Data()`／core `up`、LIF Forward trace 或每個 reverse step 的 `du`、`dcand`、`g.Inputs[t][2]`／`g.Bias[2]` bits；所以目前只能把 step 1 raw `Bias[2]` 視為最早已觀察值，不能再前推到某一個融合或取整指令。
- 尚未以相同 boundary bits 實際呼叫公開 `dynamics.LIF.Forward`／`Backward` 做 ordinary/race core-only 對照；因此 Network raw gradient 差異仍可能在 Insyra encoder、readout/MSE upstream 或 LIF Forward/Backward 任一段。
- 尚未證明 ordinary 與 `-race` 之間存在 bit-for-bit 公開契約。主規格 `/Users/timlai/Developer/coimnet/docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:428-432` 要求相同硬體、編譯器與版本的單執行緒參考可重現，對跨 CPU/GPU 與不同加總順序只承諾容差；它沒有在現有文字中明定 race instrumentation 與普通 binary 的 optimizer state 必須逐位相同。
- 尚未做跨建置 snapshot 讀取的相容性專項驗證；same-build exact 不能代替該檢查。
- 尚未量測後續 loss、參數軌跡或模型品質，不能宣稱此 ULP 差異無影響，也不能宣稱已造成品質退化。

## 修改清單

| 檔案 | 變更摘要 |
|---|---|
| `/Users/timlai/Developer/coimnet/runs/.tmp/gpu-cancel-audit-20261015/float-first-divergence-review.md` | 保留原 float64 first-divergence review，補上 OPS-05 public gradient bits、Network→LIF `Bias[2]` 資料流、公開 Forward/Backward 最小重現、float32 等價契約與 LIF 分段診斷判準。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| 唯讀讀取 | `STA-03` evidence、archive、ordinary/race assembly、正式來源 | 完成，未修改。 |
| 唯讀讀取 | 本機 Go 1.26.5 spec、`math.FMA` source、compiler intrinsic source | 完成，未下載或修改。 |
| 診斷規劃 | `learning/trainer.go` overlay 插入位置 | 僅記錄於本報告，未建立 overlay、未建置。 |
| Git | 工作樹、索引、提交、遠端 | 未執行任何寫入。 |

## Verified

- ordinary/race repeated environment diagnostics 與 same-build resume diagnostics 已逐筆核對。
- artificial before/expected archive 已做 JSON 結構與 float64 bit-level 比較，確認 earliest available stored divergence 在 index 5，而非本次 index 7 的 old second。
- `learning/trainer.go` 的 gradient、mask、loss-scale、norm、accumulation、clip、Adam 順序與 flat index mapping 已逐行核對。
- ordinary/race assembly 已逐指令核對，確認兩邊都是兩次 `fmul` 後接 `fmadd`，race 另有 spill／racewrite。
- public `Network.LossGradient`、`Gradient`、`StepResult`、snapshot、accumulator 與 stateful API 的資料契約已核對。
- OPS-05 public gradient comparison 已逐欄核對：共同 initial snapshot、step 1 index 5 raw bits、step 2 index 7 raw bits、norm/scale bits、Adam scalar references，以及 `root_cause_confirmed=false`。
- `learning/network.go` 的 `lossGradientReverse`、`learning/core.go` 的 LIF adapter、`dynamics/lif.go` 的 Forward/Backward 與本機 Insyra v0.3.4 的 float32 MatMul/MSE VJP source line 已核對。
- Decimal reference 只在記憶體內做純量算術；沒有 Go build/test、race test、外部下載或額外暫存。

## Notes

根因仍是 NEEDS INVESTIGATION。現有 public probe 已把 step 1 的最早已觀察值推到 raw `Core.Bias[2]`，但沒有 per-step LIF 或 float32 boundary bits。最小可判斷方案是先比較 encoded → readout `dh`/core upstream → LIF trace → reverse `Bias[2]`，再接 raw gradient → mean → clipScale → `d` → old moments → two multiplies → final FMA；沒有這條鏈的 operands，不得把 FMA、spill、`-race` 或 LIF 本身寫成已確認根因。

## 未驗證項目

- 未取得原始未插入 binary 的 gradient、mean、clipScale、d 或中間乘積。
- 未取得原始未插入 binary 的 float32 encoder output、readout VJP／core upstream、LIF trace 或 per-step Bias[2] reverse operands。
- 未執行公開 `dynamics.LIF` core-only Forward/Backward 對照。
- 未取得 Go source-line 對應的 DWARF/objdump 指令資訊。
- 未確認普通與 race 跨建置逐位一致是否為專案支援契約。
- 未執行任何新增診斷、Go build、Go test 或 cross-build restore test。

## Next

先由 Root 依「最小診斷契約」建立隔離 overlay，且保留未插入版本的 hash 與結果；優先保存 encoder `z`、readout `dh`／core upstream、LIF trace 與每個 reverse step 的 `Bias[2]` 累積，再接 Adam chain；若所有 Adam inputs 相同才追 compiler rounding/codegen。不要先改正常 Adam 式。

Skills：software-engineering-guidelines、investigate
