# Insyra 整合查核

CoImNet 固定使用 Insyra `v0.3.4`，模組來源為
`github.com/HazelnutParadise/insyra`，對應提交
`90f3935d02b52ce0fd51a86ed95f340cff9030a8`。本文件記錄目前公開 API 查核與
Doctor 的實際探測結果，不把計畫中的能力當成已完成。

## 公開 API 與梯度邊界

已確認可用的 `nn` API 包括：

- `nn.NewTensor(shape []int, data []float32) (*Tensor, error)`、`Tape.Param`、`Parameter.Value` 與 `Parameter.Grad`：`nn/tensor.go:59`、`nn/autodiff.go:31,39,71`。
- `Tape.MatMul`、`Add`、`Mul`、`Div`、形狀操作與 `ReduceMean`：`nn/autodiff.go:86,101,113,125,278,297,323,376,392`。
- `Tape.MSELoss`、`BCEWithLogitsLoss`、`SoftmaxCrossEntropy`、`Backward`、`Grad`：`nn/autodiff.go:487,505,470,571,670`。
- `Tape.SGD`、`SGDMomentum`、`Adam`、`AdamW`：`nn/autodiff.go:681,704,771,778`。

固定版本提供 `Tape.Custom`（`nn/autodiff.go:529`），可把外部運算的結果與反向規則登錄到 tape。`Tape.BackwardFrom`（`:598`）可以用指定的上游梯度開始反向。`Tensor.Data()` 回傳 copy，外部建立的結果必須透過公開接合 API 才會把梯度送回輸入。

`nn.NewEdgeTopology`、`nn.EdgeSum` 與 `Tape.EdgeSum` 已提供 CPU 邊列表加總及梯度。它們將乘積精確加總後一次捨入為 float32。CoImNet 現有核心使用 float64 或依邊順序加總的 WebGPU 路徑，因此沒有用 `EdgeSum` 替換核心。

目前採兩個 tape 與明示的外部梯度。編碼器與讀出以 Insyra `float32` 執行，核心以 `float64` 手動沿時間回推。讀出 tape 用 `BackwardFrom(prediction, upstream)` 接收最後一步或每步的梯度。核心反向後，依輸入節點與向量分量的原有順序建立編碼器種子，再用 `BackwardFrom(encoded, seed)` 傳回編碼器與觀察輸入。

兩個種子都與輸出張量同形狀，沿用原有 float32 轉換及有限值檢查。核心輸出的 Insyra 張量是暫存副本，最佳化器不會更新它。平滑完整路徑以有限差分驗證，硬放電依宣告的替代梯度驗證。重構的數值參考與恢復驗收見 [ticket 31](tickets/31-direct-upstream-gradient.md)。

## Doctor 報告

`internal/cli/doctor.go` 提供：

```go
func Doctor(ctx context.Context) (DoctorReport, error)
```

報告的 `schema_version` 是 `coimnet-doctor/v1`，包含 runtime Go/OS/架構、
Go embedded build info 中的 Insyra module 與 replace、CPU 數、physical memory、
GPU 實際探測結果、Insyra 公開 API probe，以及分開的 core backend 能力。

GPU 探測使用帶兩秒 context timeout 的 `system_profiler SPDisplaysDataType -json`
（macOS）或 `nvidia-smi`（Linux/Windows）。工具不存在、命令逾時或輸出無法解析
會標示 `unknown`；工具成功但回報沒有裝置才標示 `unavailable`。沒有工具不會
被判成支援。physical memory 在 macOS 讀 `hw.memsize`，Linux 讀
`/proc/meminfo`，無法取得時 JSON 保留 `null`。

Doctor 將 CPU continuous/sparse forward、backward 與目前訓練路徑標示為
`implemented`，GPU sparse training 標示 `not_implemented`。Insyra 的矩陣能力
另外標示 `scope=2d_float32_matmul`、`status=available_in_dependency`、
`execution=not_probed`，`core_gpu_equivalent` 固定為 `false`。這表示固定版本
提供矩陣裝置介面，不代表目前 CoImNet 程式已啟用或執行該加速器。

上述 Doctor GPU 標示落後於目前可選的 WebGPU 純量零延遲連續核心與 episode trainer。實際裝置驗證見 [OPS-05 證據](../evidence/OPS-05/verification.json)，報告修正列於 [AGENTS.md Follow-ups](../AGENTS.md#follow-ups)。

## v0.3.4 相容性驗收

[2026-10-01 升級證據](../evidence/insyra-v0.3.4-20261001/verification.json)包含完整建置、一般與 race 測試、vet、模組校驗、CLI、Apple M3 Metal 稀疏前向／反向及訓練器恢復。`wgpu v0.34.5` 移除 `Limits`、`BindGroupLayoutEntry` 型別別名，CoImNet 改用 `gputypes v0.8.0` 中的原型別，運算與測試判準不變。

小型延遲範例在 v0.3.2 訓練 80 步、由 v0.3.4 接續 40 步後，快照與 v0.3.2 連跑 120 步逐位元組相同。停用 Insyra WebGPU 矩陣加速時也得到相同快照。這項結果不涵蓋所有模型或大型 dense MatMul。Linux／Windows amd64 交叉編譯通過，兩個平台的實際執行尚未驗證。

Insyra `Tape.Tanh` 的 float32 捨入規則有變更，CoImNet 目前沒有呼叫它。升級驗收完成後，梯度橋接依 ticket 31 採用 `BackwardFrom`，核心沒有改用 `Custom` 或 CPU `EdgeSum`。裝置常駐狀態與裝置更新仍追蹤 [#379](https://github.com/HazelnutParadise/insyra/issues/379)。

## 2026-09-13 歷史探測與限制

在 `/private/tmp/coimnet-insyra-probe-gndSae` 的獨立 probe 中，公開 tape 組成
三條稀疏邊，實際得到：

```text
output=[0 -1 3]
grad_weights=[4 16 12]
grad_input=[1 2.25 -0.5]
```

自訂運算的最小重現位於 `/private/tmp/coimnet-custom-vjp-probe.go`：`w=3`、
`x=2`、target=`0` 時， detached 結果的 loss 是 `[36]`、`dw=[0]`，同一運算改用
內建 `Tape.Mul` 則 loss 是 `[36]`、`dw=[24]`。這證明外部建立結果不會自動加入 tape
圖，不能在核心 detached 後宣稱仍具自動梯度。當時的自訂梯度接合缺口追蹤於
[#375](https://github.com/HazelnutParadise/insyra/issues/375)，v0.3.4 已提供上述公開 API。最佳化器保存需求另見
[#376](https://github.com/HazelnutParadise/insyra/issues/376)。

整合後的 `go test ./...`、race、vet、建置及 CLI 已在 macOS 通過，
見[完整日誌](../evidence/cpu-reference-20260913/macos/validation.log)與
[Doctor JSON](../evidence/cpu-reference-20260913/macos/doctor.json)。
CoImNet 自有 AdamW 可保存獨立序列訓練狀態，實際跨程序續訓測試已通過。
這不表示 Insyra tape 本身已有 optimizer state 序列化介面，也不涵蓋持續個體的
神經、快速權重或化學狀態。
