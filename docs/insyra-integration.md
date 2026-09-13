# Insyra 整合查核

CoImNet 固定使用 Insyra `v0.3.2`，模組來源為
`github.com/HazelnutParadise/insyra`，對應提交
`1f1cdb949c51a10be104965cf4cf30f8f0aaa648`。本文件記錄目前公開 API 查核與
Doctor 的實際探測結果，不把計畫中的能力當成已完成。

## 公開 API 與梯度邊界

已確認可用的 `nn` API 包括：

- `nn.NewTensor(shape []int, data []float32) (*Tensor, error)`、`Tape.Param`、`Parameter.Value` 與 `Parameter.Grad`：`nn/tensor.go:57-66`、`nn/autodiff.go:30-44,69-81`。
- `Tape.MatMul`、`Add`、`Mul`、`Div`、形狀操作與 `ReduceMean`：`nn/autodiff.go:83-133,277-409`。
- `Tape.MSELoss`、`BCEWithLogitsLoss`、`SoftmaxCrossEntropy`、`Backward`、`Grad`：`nn/autodiff.go:466-578`。
- `Tape.SGD`、`SGDMomentum`、`Adam`、`AdamW`：`nn/autodiff.go:580-727`。

固定版本沒有公開 custom operation、external VJP 或 tape reset。`tapeOp` 與
`record` 是非公開成員，`Backward` 只能走已記錄的 VJP。`Tensor.Data()` 也只
回傳 copy，因此不能從外部塞回梯度或直接改 Tensor 內容。`Sub`、`Exp`、
`Clip`、比較與 `Where` 雖有 plain kernel，沒有相應的 Tape wrapper。

目前採兩個 Tape 與明示 VJP bridge：encoder/readout 以 Insyra `float32` 執行，
核心以 `float64` 手動 BPTT。readout loss 反向後取 core output gradient，核心
反向再將得到的 encoder gradient 建成 detached 常數，將 encoder output
`Reshape` 後與該常數以 `Tape.MatMul` 形成 scalar 內積，再對 encoder Tape
`Backward`。core output 的 Insyra Tensor 是暫存 copy，避免 optimizer 把它當成
真正核心參數更新。平滑完整路徑以 finite difference 驗證；硬 spike 仍必須依宣告
的 surrogate gradient 驗證，不能拿硬事件有限差分當答案。

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

## 實際證據與限制

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
圖，不能在核心 detached 後宣稱仍具自動梯度。相關缺口追蹤於
[#375](https://github.com/HazelnutParadise/insyra/issues/375) 與
[#376](https://github.com/HazelnutParadise/insyra/issues/376)。

整合後的 `go test ./...`、race、vet、建置及 CLI 已在 macOS 通過，
見[完整日誌](../evidence/cpu-reference-20260913/macos/validation.log)與
[Doctor JSON](../evidence/cpu-reference-20260913/macos/doctor.json)。
CoImNet 自有 AdamW 可保存獨立序列訓練狀態，實際跨程序續訓測試已通過。
這不表示 Insyra tape 本身已有 optimizer state 序列化介面，也不涵蓋持續個體的
神經、快速權重或化學狀態。
