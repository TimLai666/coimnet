# Trainer 與裝置 AdamW 取消保留稽核

- 基準：`HEAD 2ff6864f854cd3129ac0ac17065bdf54f73047c3`。
- 來源指紋：`evidence/STA-03/training-cancel-audit-20261015/source-before.json`，808 份 Go 加 `go.mod`、`go.sum`，共 810 份；本次引用的核心來源 shasum 已核對符合該指紋。
- 範圍：`learning/trainer.go`、`learning/stateful.go`、`learning/gpu_device_trainer.go`、legacy GPU trainer／adapter，以及 `backend/webgpu` 的 AdamW 候選、讀回、取消清理與提交路徑。
- 方法：以目前實檔行號和 codebase-memory graph/search 交叉核對候選 copy／clone、實際 mutate／commit、`context.Err`／`Done` 和返回值。只做來源審查，未執行 Go 或測試，也未推測特定 GPU runtime 行為。

## CONFIRMED

### 1. 未確認參數、Adam、累積窗或裝置 current bank 在取消後偷偷提交

- CPU `Step`、`StepFrom` 先以 `LockContext` 和 context-aware gradient/recompute 路徑取得梯度，之後 `stepWithGradient` 在 `learning/trainer.go:401-403` 使用平坦化副本。sharing 只改副本 gradient 與 mask（`learning/trainer.go:408-413`）。
- 累積窗在 `learning/trainer.go:450-480` 先深拷貝 accumulator。未滿窗時只有通過 `ctx.Err()`（`470-474`）才寫回 `tr.accumulator`，參數、Adam、`updates` 不變。滿窗後平均值仍是區域資料，失敗或取消會保留原 accumulator。
- CPU AdamW 在 `learning/trainer.go:512-553` 改區域 `p`、`state`，候選以 `unflatten` 複製（`554`、`675-682`），前向驗證後於 `learning/trainer.go:568-574` 才做最後 context 檢查和單一 host commit。參數、`First`、`Second`、`Steps`、`updates`、累積窗不會在該檢查前寫回。
- `StepFromState` 在 `learning/stateful.go:48-79` 先鎖定、拒絕 `Recompute`、從固定 saved state 求梯度，再經同一個 `stepWithGradient`。`stepWithGradient` 在 `learning/trainer.go:398-400` 也拒絕 device Adam 與 stateful initial 的組合，因此沒有繞過 CPU 交易邊界的 stateful GPU 路徑。
- GPU device AdamW 在 `learning/gpu_device_trainer.go:151-168` 建立區域 float32 gradient、host 參數與 Adam snapshot；`validateDeviceAdamCandidate` 在 `438-470` 產生新參數／moments／counters，逐項要求 frozen 欄位完全相等、可學欄位 counter 只前進一步。`StepChecked` 成功後才在 `learning/gpu_device_trainer.go:201-206` 同時更新 host mirror、`updates` 和 accumulator。
- backend 的真正 device commit 只有 `backend/webgpu/device_adam_checked.go:53-62` 的 `o.currentBank = shadowBank`。候選執行、完整 device readback、accept callback 和最後 `ctx.Err()`（`43-58`）都通過前，`currentBank` 不變。取消或 callback rejection 不會提交參數、moments 或 counters。
- legacy `DeviceAdamW.Step` 也只在 `backend/webgpu/device_adam_execute.go:15-23` 的候選執行成功後換 bank；候選執行在 `30-165` 對 upload、dispatch、每個 status readback 及最後 context 皆檢查。依本任務界線，最後提交後才發生的取消不列為 bug。

### 2. 取消後的裝置 scratch bank 不等於 Adam 狀態變動

`executeAdamCandidateLocked` 對 `1-currentBank` 寫入候選（`backend/webgpu/device_adam_execute.go:30-41`、`105-114`），取消或錯誤後由 `drainAdamSubmission` 等待 GPU 工作排空（`261-275`），但不換 `currentBank`。WGSL 每個元素都會把 frozen 或 active 結果寫入 shadow bank（`backend/webgpu/device_adam.wgsl:49-191`），下一次候選會完整覆寫所有元素。因此取消時可能留下物理 scratch 內容，但可觀察的 Adam 狀態仍是舊 current bank，host mirror 也未變。

### 3. 已確認會變動、但不屬模型訓練狀態的欄位

- `backend/webgpu/device_adam_checked.go:43-47` 以 defer 將本次 transfer／dispatch／readback metrics 寫回 `o.metrics`，所以候選取消、拒絕或執行錯誤後，`Metrics()` 仍可能反映已發生的裝置工作。這不會改參數、Adam moments、counters、accumulator 或 `currentBank`，屬診斷 telemetry 的預期可觀察變動。既有 `learning/gpu_device_trainer_test.go:278-294` 也明確要求候選失敗後 metrics 累加但完整 trainer snapshot 相同。
- 若已提交 GPU 工作而 `WaitIdle` 或 map cleanup 失敗，`backend/webgpu/device_adam_execute.go:223-251`、`261-275` 會設定 `o.unusable`（欄位定義在 `backend/webgpu/device_adam.go:64-73`）。這是取消後無法證明裝置可安全重用時的 fail-safe，會讓後續 `Step`／`Snapshot` 失敗，但仍可由 `Close` 釋放資源；目前只有來源條件路徑證據，沒有本輪 runtime cleanup failure 的實測。

### 4. 裝置資源生命週期沒有找到取消洩漏的靜態路徑

- 新建／restore 先驗證、建立 device Adam，再 attach GPU；attach 失敗會呼叫 `deviceAdam.Close()`，見 `learning/gpu_device_trainer.go:17-49`、`53-106`。
- map 取消會 unmap、`WaitIdle`、`Poll` 並釋放 pending handle，見 `backend/webgpu/device_adam_execute.go:182-251`。
- trainer close 以 trainer mutex 序列化並呼叫 device Adam close，`backend/webgpu/device_adam.go:202-223` 設為 idempotent；所有 bind group、pipeline、staging、state／upload buffers、device／adapter／instance 由 `backend/webgpu/device_adam_buffers.go:472-526` 釋放。
- legacy GPU trainer 的 AdamW 仍在 host，`learning/gpu_trainer.go:74-77`；GPU sparse forward/backward 只建立 local trace／gradient，取消返回前沒有 trainer 或 Adam 寫入，見 `dynamics/gpu_adapter.go:156-265`、`294-409`。

## NEEDS INVESTIGATION

### N1. 缺少真正 GPU Adam 候選各階段的取消覆蓋

- 缺少事實：目前沒有 trainer 層在 `learning/gpu_device_trainer.go:173-198` 的 `StepChecked` 執行期間，分別於 upload map／submit、compute dispatch submit、status readback、候選完整 snapshot readback 取消的實際測試證據。現有 backend 測試只涵蓋 callback 在完整候選讀回後取消（`backend/webgpu/device_adam_checked_test.go:76-99`），以及獨立 map helper 取消後重用（`backend/webgpu/device_adam_transaction_test.go:49-72`）。
- 可驗證步驟：在多 chunk 的真實裝置 fixture 中加入只供測試的可控 context／階段 hook，逐一讓 `ctx.Err()` 在 `backend/webgpu/device_adam_execute.go:43-163`、`snapshotAdamBankLocked` 的 `378-408` 各邊界返回 `context.Canceled`；每次比較取消前後 trainer `Snapshot()`、backend `Snapshot()` 和 `GPUAdamWMetrics()`，再以同一合法 gradient 重試並和未取消 reference 比對。需把 metrics 與參數／Adam／accumulator／currentBank 分開判定，不能把 scratch bank 或 telemetry 誤判為邏輯提交。

### N2. 公開 `Snapshot` 的最後讀回後取消返回語意尚未定義

- 缺少事實：`backend/webgpu/device_adam_execute.go:378-409` 在每個 chunk 前、submit 後與 `snapshotChunk` 內檢查 context，但最後一個 `snapshotChunk` 成功後沒有再做一次 `ctx.Err()`；因此若取消恰好發生在最後讀回完成與函式返回之間，`DeviceAdamW.Snapshot`（`backend/webgpu/device_adam.go:173-189`）可能仍返回成功。這不會寫入 Adam 或 device bank，且 `StepChecked` 在 callback 後的 `56-58` 仍有最後檢查，所以目前不能列為訓練狀態污染。
- 可驗證步驟：用單 chunk 與可計數的測試 context，將取消安排在最後一次 `snapshotChunk` 完成後、`snapshotAdamBankLocked` 返回前，確認 API 契約要的是「呼叫完成前取消必須報錯」還是接受最後讀回邊界；若契約要求報錯，再只補最小的最後 context check 與對應測試。此輪不修改程式或測試。

## 候選、copy 與 commit 對照

| 檔案與行號 | 候選／copy | 真正 mutate／commit | 取消時的影響 |
|---|---|---|---|
| `learning/trainer.go:401-413` | flat parameter／gradient、sharing plan、mask | 尚未寫 trainer | 參數與輸入 gradient 不變；sharing 不走原始 gradient |
| `learning/trainer.go:450-480`、`learning/accumulate.go:34-39` | 深拷貝 accumulator，滿窗才產生 mean | `473` 只提交 partial accumulator | partial 取消不改參數、Adam、updates；full 失敗保留舊窗 |
| `learning/trainer.go:512-575`、`655-682` | CPU Adam、projection、unflatten 都在 local copy | `571-574` 原子式 host 欄位替換 | 最後 `ctx.Err` 前任何錯誤都不提交 |
| `learning/stateful.go:48-79` | saved state 只作 forward/reverse boundary | 仍回到同一 `stepWithGradient` | `Recompute` 拒絕；不會以 stateful 路徑繞過 commit |
| `learning/gpu_device_trainer.go:151-196`、`438-470` | float32 gradient、host snapshot、candidate arrays | `203-206` 更新 host mirror | `StepChecked` 失敗時 host 全不變 |
| `backend/webgpu/device_adam_execute.go:30-165` | shadow bank、upload／dispatch／status local metrics | legacy `Step` 在 `15-23` 換 bank | 取消後 drain；current bank 不換，scratch 可留候選 |
| `backend/webgpu/device_adam_checked.go:43-62` | local metrics、完整 candidate readback | `61` 是唯一 checked device commit | callback／context 失敗不換 bank；metrics 仍可能變 |
| `backend/webgpu/device_adam_execute.go:182-276` | map pending／submitted GPU work | cleanup 失敗時設定 `unusable` | 資源進入不可重用 fail-safe，`Close` 仍可清理 |
| `backend/webgpu/device_adam_execute.go:354-447` | fresh host snapshot slices | 無 device state commit | readback 取消不改 resident bank；最後讀回後取消的返回語意待定 |
| `learning/trainer.go:241-259`、`backend/webgpu/device_adam.go:202-223` | — | close flags／resource release | Close 序列化、可重複；本身非 Step 取消路徑 |

## 既有測試缺口與尚未驗證

- 已有 CPU lock cancellation：`learning/trainer_cancel_test.go:56-107`，只涵蓋取消前或等待 trainer lock，沒有 gradient、candidate validation 與最後 commit 間取消。
- 已有 CPU Recompute 失敗保留 snapshot：`learning/chemical_recompute_test.go:291-329`；沒有中途 cancel 的細分階段測試。
- 已有 CPU accumulation 正常 partial/full、snapshot/resume：`learning/accumulate_test.go:146-230`；沒有 partial assignment 前取消與 full window 候選取消。
- 已有 GPU device accumulation／mask／schedule、非 finite candidate rollback 與 continuation：`learning/gpu_device_trainer_test.go:193-305`；沒有 trainer-level context cancel。
- 已有 backend `StepChecked` callback cancellation、invalid candidate atomicity：`backend/webgpu/device_adam_checked_test.go:76-145`，以及 cancelled map drain：`backend/webgpu/device_adam_transaction_test.go:49-72`；尚缺 upload／dispatch／status／snapshot 各邊界的整體交易測試。
- 本輪未執行 Go、測試、GPU runtime 或硬體壓力驗證，因此不能宣稱實際裝置在每個取消時序都已驗收。靜態結論只涵蓋目前來源與上述既有測試內容。

## 結論

目前沒有 `CONFIRMED` 的「取消後 Adam、模型參數、累積窗或 device current bank 偷偷變動」缺陷。已確認的取消後變動只有診斷 metrics，以及 cleanup 無法同步完成時的 `unusable` 保護狀態。真正需要補證的是 GPU 各提交階段的實體取消時序和公開 `Snapshot` 的最後讀回返回契約。

Skills：software-engineering-guidelines、investigate、human-writing、project-memory
