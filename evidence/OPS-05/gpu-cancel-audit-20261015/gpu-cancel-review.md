# GPU 取消與恢復來源審查

日期：2026-10-15

審查範圍：`docs/tickets/100-gpu-cancel-preservation.md:1-17`、`evidence/OPS-05/gpu-cancel-audit-20261015/plan.md:1-21`，以及目前工作樹的 DeviceAdamW、GPUDeviceTrainer、GPU sparse forward/backward 與 Trainer 提交路徑。`baseline.json` 記錄的來源基準為 `52044729ad1922dcab1a88bbff05764e7658b0e0`。

## Problem

票 100 要確認 GPU 更新在有效提交前取消時，裝置參數、第一／第二動量、計數、Trainer 累積窗與原物件重試都保持一致；需逐點核對 `Step`、`StepChecked`、裝置讀回和 `GPUDeviceTrainer` 更新路徑。提交完成後的取消依票 100 的固定決策不要求回滾。

## CONFIRMED 缺陷

無。目前沒有能由目前來源與本輪允許的證據同時確認為缺陷的路徑。既有 `evidence/STA-03/training-cancel-audit-20261015/root-review.md:17` 明確記錄上一輪只有 GPU 來源／既有取消控制查驗，沒有完整候選逐階段取消的新硬體實測；不能把它當成本票的 GPU 證據。

## NEEDS INVESTIGATION

1. **Legacy `Step` 的最後檢查與提交之間沒有第二次檢查。** `backend/webgpu/device_adam_execute.go:161-164` 完成候選狀態檢查後，`executeAdamStepLocked` 在 `backend/webgpu/device_adam_execute.go:16-22` 直接把 `currentBank` 換成 `shadowBank:20`。取消若落在 `161` 返回與 `20` 執行之間，預期依固定契約是成功提交後不回滾，但目前沒有逐點硬體觀察。這是覆蓋缺口，不是已確認錯誤。

2. **候選驗證用的 sparse GPU readback 清理尚未由本票硬體證明。** `GPUDeviceTrainer` 的 `StepChecked` callback 在 `learning/gpu_device_trainer.go:189-194` 呼叫同一個 GPU-backed `Network.Predict`。Sparse forward 在提交後於 `backend/webgpu/sparse_drive.go:654` 檢查取消，Map 失敗在 `:659-665` 只做 `Unmap`；backward 對應為 `backend/webgpu/sparse_backward.go:712-723`。需在取消期間、提交後、Map 等待中觀察原物件下一次 `Predict`／`Step` 是否可重用，以及是否需要進入明確的不可重用狀態。來源沒有足夠依據把這點升級為 bug。

3. **DeviceAdamW 取消 Map 的清理契約需要真實裝置觀察。** `mapAdamBuffer` 在 `backend/webgpu/device_adam_execute.go:204-220` 取消後走 `cancelAdamMap:223-251`，依序 `Unmap`、`WaitIdle`、`PollPoll`、`pending.Release`；等待或清理失敗才以 `markAdamUnusable:254-259` 拒絕重用。需記錄每個步驟的實際結果，並以同一物件重做 `Step`／`Snapshot`。既有來源測試與上一輪證據不能替代本票的新硬體執行。

4. **逐元素讀回期間沒有 `ctx.Err`。** DeviceAdam 的 `snapshotChunk` 在 `backend/webgpu/device_adam_execute.go:420-446` 解析 mapped bytes 時沒有中途檢查；sparse forward/backward 分別在 `backend/webgpu/sparse_drive.go:673-688`、`backend/webgpu/sparse_backward.go:730-758` 解析結果時沒有中途檢查。取消只能在下一個外層檢查或公開操作返回時被觀察。需確認這些未到達位置不會造成已選模型 bank、Trainer host mirror 或 map 狀態的部分提交。

5. **建構／恢復有明確的檢查間隔，而非每個資源建立呼叫都檢查。** DeviceAdam 在 `backend/webgpu/device_adam_buffers.go:128-139` 只在 pipeline 編譯後檢查，`planAdamChunks`、buffer 建立與 bind group 建立之間沒有檢查；初始 state 的每 chunk 檢查在 `:434`，全部上傳後再檢查 `:456`。SparseDrive 的 topology upload 在 `backend/webgpu/sparse_drive.go:526,532` 檢查 offsets／sources，但 `:538-539` 的 edgeOrder upload 後沒有同段檢查。`NewGPUDeviceTrainer`／`RestoreGPUDeviceTrainer` 的 host 驗證本身在 `learning/gpu_device_trainer.go:17-39,53-93` 沒有 context 檢查，取消由下游 GPU 建構器觀察。這些是建構取消覆蓋的待驗證條件，不能用初始化前取消取代更新中取消。

6. **以 `runtime.Caller` 定位的觸發點需要記錄實際 frame 與次數。** `validateAdamContext` 把 `Step`、`StepChecked`、`Snapshot` 的入口檢查共用到 `backend/webgpu/device_adam.go:225-230`，包裝器第一個非 wrapper frame 會是 `:229`，不是公開方法的呼叫列。每 chunk 的同一行也會重複；若只比檔案／行號，無法分辨第一個與最後一個 chunk。這是探針設計風險，不是產品缺陷。

## context.Err／Done 檢查位置

以下列的是此審查路徑中實際執行的檢查。`learning/gpu_device_trainer.go` 沒有直接呼叫 `ctx.Err` 或 `ctx.Done`，只在列出的公開入口與 callback 把 context 往下傳。

### DeviceAdamW

| 檔案及行號 | 所在階段 | 取消後預期 |
|---|---|---|
| `backend/webgpu/device_adam.go:229`，由 `NewDeviceAdamW:97-116`、`RestoreDeviceAdamW:120-138`、`Step:143-170`、`Snapshot:173-189` 及 `StepChecked:12-25` 間接呼叫 | nil／已取消入口檢查 | 不開新操作；已存在物件的模型 bank 不變 |
| `backend/webgpu/device_adam_buffers.go:50,69,86,128` | instance、adapter、device、pipeline 建構後 | 清理已建立資源並回傳取消；不返回物件 |
| `backend/webgpu/device_adam_buffers.go:156` | warmup 出錯分支 | 清理資源；若是 context 錯誤回傳該錯誤 |
| `backend/webgpu/device_adam_buffers.go:434,456` | 初始 state 每 chunk、全部初始上傳後 | 建構失敗時清理；不返回部分建構物件 |
| `backend/webgpu/device_adam_execute.go:43,48,59` | Adam input／uniform／status 上傳前、每 chunk 前、上傳 encoder 前 | 尚未提交 GPU command；不換 bank |
| `backend/webgpu/device_adam_execute.go:80,90` | upload encoder 完成後、upload submit 後 | 已 submit 時 defer 走 `drainAdamSubmission:261-276`；不換 bank |
| `backend/webgpu/device_adam_execute.go:128,137` | dispatch encoder 完成後、dispatch submit 後 | 已 submit 時 drain；不換 bank |
| `backend/webgpu/device_adam_execute.go:144,161` | 每個 status 讀回前、所有 status 驗證完成後 | error 時保留 `currentBank`；已提交工作先 drain |
| `backend/webgpu/device_adam_execute.go:187,198,204,208` | map 入口、`MapAsync` 錯誤、poll 前後 | 取消 map 走 `cancelAdamMap:223-251`；清理失敗才標記不可重用 |
| `backend/webgpu/device_adam_execute.go:302` | upload staging mapped range 釋放後、unmap 前 | 只清理 upload staging；尚未 submit，不換 bank |
| `backend/webgpu/device_adam_execute.go:379,393,401` | snapshot 每 chunk encoder 前、submit 前、submit 後 | submit 後取消先 drain；回傳空 snapshot，不修改 `currentBank` |
| `backend/webgpu/device_adam_checked.go:56` | callback 成功後、唯一 device commit 前 | 回傳取消，不執行 `currentBank = shadowBank` |
| `backend/webgpu/device_adam.go:229`（`validateAdamContext`）後的 `Step` commit `backend/webgpu/device_adam_execute.go:20` | Legacy `Step` 唯一 bank commit | `executeAdamCandidateLocked` 成功才換 bank；commit 後取消依契約不回滾 |

此範圍沒有 `ctx.Done`；DeviceAdam 以 `ctx.Err` 輪詢。整個公開 GPU 訓練路徑唯一的 `ctx.Done` 是 `learning/trainer.go:604`。

### GPUDeviceTrainer 與其 GPU core

| 檔案及行號 | 所在階段 | 取消後預期 |
|---|---|---|
| `learning/trainer.go:318,352` | `Trainer.Step`／`StepFrom` 入口 | 不取 Trainer lock，不改任何持久狀態 |
| `learning/trainer.go:599,604-610` | `cancellableMutex.LockContext` 入口、`Done` 等待、token 取得後 | 等待時回傳取消；token 與 Done 同時 ready 時 select 可能任一分支，但 token 分支再檢查 `ctx.Err` 並釋放 token |
| `learning/network.go:524,671` | forward 入口、讀出 finite check 後 | gradient／candidate 是局部資料；不改 Trainer 狀態 |
| `learning/network.go:498` | reverse／Insyra gradient 完成後 | gradient 是局部資料；不改 Trainer 狀態 |
| `dynamics/gpu_adapter.go:55` | GPUContinuous 建構入口 | 不開 GPU 或清理已開資源 |
| `dynamics/gpu_adapter.go:173,205,232` | Forward 入口、輸入檢查、每時間步執行前 | trace 是局部資料；不改模型參數 |
| `dynamics/gpu_adapter.go:313,344` | Backward 入口、每反向時間步前 | gradient 是局部資料；不改模型參數 |
| `backend/webgpu/sparse_drive.go:224,241,260,275,329` | SparseDrive 建構的 instance／adapter／device／pipeline 階段 | 清理已建資源，不返回半成品 |
| `backend/webgpu/sparse_drive.go:526,532` | topology offsets、sources 上傳間 | 建構失敗清理資源 |
| `backend/webgpu/sparse_drive.go:556,593,609,654` | SparseDrive Execute 入口、上傳前後、dispatch submit 後 | error 不寫入持久 Trainer；submit 後需由硬體驗證 staging／queue 重用 |
| `backend/webgpu/sparse_backward.go:175,192,210,273` | SparseBackward 建構入口、instance／adapter／pipeline 階段 | 清理已建資源，不返回半成品 |
| `backend/webgpu/sparse_backward.go:568` | topology 每 buffer 上傳前 | 建構失敗清理資源 |
| `backend/webgpu/sparse_backward.go:591,616,635,712` | Backward 入口、上傳前後、dispatch submit 後 | error 不寫入持久 Trainer；submit 後需由硬體驗證 staging／queue 重用 |
| `learning/trainer.go:470` | 部分累積窗、`tr.accumulator` 提交前 | 回傳取消時局部 accumulator 不寫回 |
| `learning/gpu_device_trainer.go:173,192` | DeviceAdam `StepChecked`、候選 GPU `Predict` | callback／StepChecked 失敗時不換 device bank；host mirror 不寫回 |
| `learning/gpu_device_trainer.go:203-206` | StepChecked 成功後的 host mirror commit | `parameters`、`optimizer`、`updates`、`accumulator=nil` 一起寫入；此段沒有 context check，符合提交後不回滾 |

`learning/trainer.go:568` 是 CPU optimizer 路徑的最後檢查，不會在 `tr.deviceAdam != nil` 的 GPU 路徑執行；列出它是為了避免把 CPU 提交檢查誤算成 GPU 覆蓋。

## 取消／提交位置表

| 提交或清理位置 | 公開入口 | 取消前後的狀態界線 | 預期觀察方法 |
|---|---|---|---|
| `backend/webgpu/device_adam_execute.go:20` | `DeviceAdamW.Step` | `executeAdamCandidateLocked` 的 shadow bank 成功後換 `currentBank`；之前錯誤不換，之後不回滾 | 逐點取消，讀回完整 parameters／First／Second／Steps；將已成功提交結果與不中斷 control 比對 |
| `backend/webgpu/device_adam_checked.go:61` | `DeviceAdamW.StepChecked`，由 `learning/gpu_device_trainer.go:173` 使用 | callback、候選 snapshot、最後 `ctx.Err` 全通過後才換 bank；唯一 device commit | 取消於每個前置檢查、readback、callback `Predict`、line 56 前後；錯誤時完整 DeviceAdam snapshot 應等於 before |
| `backend/webgpu/device_adam_execute.go:366-373,412-446` | `DeviceAdamW.Snapshot` 或 `StepChecked` candidate readback | 建立與填入的是 fresh host snapshot；不是模型 bank commit。讀回錯誤時回傳零 snapshot，`currentBank` 不變 | 檢查錯誤時回傳 snapshot 是否為零值，並另外讀回 current bank |
| `learning/trainer.go:473-474` | `Trainer.Step`／`StepFrom`，`AccumulateSteps>1` 且未滿窗 | 這是明示的部分成功：只提交 `tr.accumulator`，不改 parameters／Adam／updates；不是 GPU Adam full update | 取消於 `:470` 前後；比對 accumulator、parameters、optimizer、updates，並確認 `Applied=false` |
| `learning/gpu_device_trainer.go:203-206` | `Trainer.Step`／`StepFrom` 的 GPU full update | Device bank 已在 `StepChecked:61` 提交後，host mirror 一起更新；不設提交後回滾 | 比對 Trainer `Snapshot` 的 parameters／Adam／updates／accumulator；取消只可觀察到提交後成功或下一次操作結果 |
| `backend/webgpu/device_adam_execute.go:223-251` | DeviceAdam status／snapshot／map 讀回 | map 取消先清理；清理失敗設定 sticky `unusable`，這是安全拒絕重用，不是模型 bank 變更 | 記錄 `Unmap`、`WaitIdle`、`PollPoll`、`pending.Release` 結果，再測同物件 Step／Snapshot；清理失敗時預期後續回 `unusable` |
| `backend/webgpu/device_adam.go:216-220`、`learning/trainer.go:241-258` | DeviceAdam／Trainer `Close` | 等待 GPU idle 後釋放資源；closed 後不允許再 Step，Close 可重入 | 取消操作後呼叫 Close，確認等待、釋放、第二次 Close 與後續 `ErrClosed` |

## 可變的診斷／scratch 與不可變的模型資料

取消或候選失敗後可以改變的內容：

- 裝置 `shadowBank`、每 chunk 的 status、input／uniform／upload staging、snapshot staging，以及 GPU sparse 的 input／result／gradient staging。這些不是 `currentBank`。
- 局部 `metrics`。Legacy `Step` 在 `backend/webgpu/device_adam_execute.go:22` 儲存量測；`StepChecked` 以 `device_adam_checked.go:43-44` 的 defer 保存量測，即使 callback／context 失敗也可能留下診斷差異。
- `deviceGradient`、`originalParameters`、`originalOptimizer`、`candidateParameters`、`candidateOptimizer`、`updateNorm`，位置見 `learning/gpu_device_trainer.go:151-172`；callback 的 candidate slices 是 owned readback，見 `:173-196`。
- GPU sparse 的 queue、metrics 與局部 trace／gradient。它們不應被拿來判斷模型 bank 是否提交。

有效提交前應保持不變的模型資料：

- DeviceAdam 的 selected `currentBank`，包括 parameters、First、Second、Steps。`executeAdamCandidateLocked` 明確只寫 opposite bank，見 `backend/webgpu/device_adam_execute.go:26-35,112-164`。
- GPU Trainer host mirror 的 `tr.parameters`、`tr.optimizer`、`tr.updates`、`tr.accumulator`。完整更新只在 `StepChecked` 成功後 `learning/gpu_device_trainer.go:203-206` 寫回；部分累積的 `tr.accumulator` 是已授權的成功提交，例外為 `learning/trainer.go:447-475`。
- `Trainer.Snapshot` 在 `learning/trainer.go:227-236` 只持有 Trainer lock 並複製 host mirror，沒有重新從 DeviceAdam 讀回；因此 GPU 稽核需另外讀 DeviceAdam snapshot，不能只看最後 Trainer snapshot。

## lock、map 清理、等待、重用與最後 snapshot

- `DeviceAdamW.Step`、`StepChecked`、`Snapshot`、`Close` 全程持有 `o.mu`，見 `backend/webgpu/device_adam.go:147-189,208-218` 與 `device_adam_checked.go:16-17`。候選 callback 不能再呼叫同一 optimizer 的 locking method，來源在 `device_adam_checked.go:8-11` 已明示。
- Trainer 更新持有 `tr.mu`，再呼叫 DeviceAdam；`GPUAdamWMetrics` 也是 Trainer → DeviceAdam，見 `learning/gpu_device_trainer.go:111-120`。`Trainer.Close` 同樣是 Trainer → DeviceAdam／GPU core，見 `learning/trainer.go:241-258`。目前來源未見反向 DeviceAdam → Trainer lock，沒有靜態 deadlock 證據。
- `cancellableMutex.LockContext` 的 `Done` 分支在 `learning/trainer.go:603-608`，token 分支在 `:609-614`。兩者同時 ready 時 Go `select` 的分支選擇不是固定的，但 token 分支會再檢查 `ctx.Err`，必要時把 token 還回去；需在探針記錄分支，不應把這種 nondeterminism 誤報為狀態損壞。
- DeviceAdam 在 command submit 後的 error／取消由 `drainAdamSubmission:261-276` `WaitIdle` 加 `PollPoll`；map 取消另做 `Unmap`、`WaitIdle`、`PollPoll`、`pending.Release`。若等待失敗，`unusable` 會黏住，之後 `Step`／`Snapshot` 應拒絕重用。這符合票 100「無法證明可重用就拒絕重用」的固定決策。
- `snapshotAdamBankLocked:359-409` 每 chunk 都建立 fresh host slices，submit 後取消會 drain 並回傳零 snapshot；它不換 `currentBank`。`Snapshot` 的 metrics 會更新，但 snapshot 的模型內容不應部分回傳。
- `Close` 先 `WaitIdle` 再 release。即使 `WaitIdle` 回錯也會釋放資源並把物件標為 closed；此後不能再用原物件，需以 `ErrClosed`／close 結果記錄，不要求回滾已成功提交的更新。

## runtime.Caller 取消包裝器的覆蓋判斷

目前來源搜尋沒有找到本產品的 `runtime.Caller` 取消 wrapper；唯一命中是無關的 `examples/realocr/main.go`。因此以下是對固定探針契約的來源判斷，不是已執行的探針結果。

- 對直接 `ctx.Err()` 的位置，wrapper 可以在 `Err` 被呼叫時以第一個非 wrapper frame 觸發取消；但 `validateAdamContext` 會把多個公開入口集中到 `device_adam.go:229`，必須保留完整 call stack 或公開入口標記。
- 對每 chunk／每 timestep 的同一行，檔案／行號只能識別迴圈，不足以識別第幾次；必須同時記錄呼叫次數、chunk／timestep 與是否已 submit。
- `ctx.Done` 只有 `learning/trainer.go:604`。若 wrapper 的 `Done()` 觸發，取消時點落在 lock wait；不能當作 DeviceAdam update 中的檢查。`select` 於 token 與 Done 同時 ready 時也不是固定先後。
- 沒有 `ctx.Err` 的同步區段無法用 line trigger 直接命中，例如 Adam pipeline 的 `planAdamChunks`／buffer／bind group 建立、`snapshotChunk` 逐元素解析、sparse readback 逐元素解析，以及 `StepChecked` line 56 與 line 61 之間的實際 bank assignment。探針必須標示「未到達」，不能把它算成通過。
- `ctx.Err` 可能只在錯誤分支出現，例如 `device_adam_execute.go:198` 的 MapAsync error 後檢查與 `device_adam_buffers.go:156` 的 warmup error 分支。正常硬體執行不應把未命中的分支誤標成功。
- 取消探針每次應使用新物件或新程序。DeviceAdam 取消後若 cleanup 失敗會永久 `unusable`；把同一物件的拒絕重用當成資料保持失敗是不正確的分類。

結論：runtime.Caller 位置觸發可以覆蓋大部分已明示的 `Err` 檢查，但不能覆蓋沒有檢查的同步區段，也不能單靠檔案／行號區分共用 helper、迴圈次數或提交前後。這足以規劃探針，尚不足以宣稱 GPU 取消已通過。

## 探針必須覆蓋的驗收

1. 先以非零梯度做 control，且 before snapshot 已有非零 First／Second 與 Steps；使用多 chunk fixture，並分別保留 frozen mask、零 rate `StepChecked`、`AccumulateSteps=2/3` 部分窗與滿窗。
2. `DeviceAdamW.Step` 覆蓋 `device_adam_execute.go:43,48,59,80,90,128,137,144,161`，並獨立觀察 `:20` 前後；`StepChecked` 覆蓋 `device_adam_checked.go:24,45-56,61` 與 `snapshotAdamBankLocked:379,393,401,404`。
3. 覆蓋 `mapAdamBuffer:187,198,204,208` 的入口、MapAsync error、poll 前後，記錄 `cancelAdamMap` 的四個清理動作及是否標記 `unusable`；取消錯誤後以同一物件做合法 retry，若清理失敗則依契約期待拒絕重用。
4. 覆蓋 `learning/trainer.go:318,321,352,355,470` 與 `learning/gpu_device_trainer.go:173,192`。取消錯誤時比較完整 `DeviceAdamW.Snapshot` 與 Trainer host `Snapshot` 的 parameters、First、Second、Steps、updates、accumulator；metrics 與 scratch 分開報告。
5. `Step`／`StepFrom` 的完整窗要確認取消失敗不增加 `updates`、不清空既有 accumulator、不改 host mirror；部分窗要確認只有明示的 accumulator 提交。原物件 retry 與不中斷 control 要逐欄位相同。
6. 以同一來源的一般／race 建置、新程序 JSON restore 與相同建置 control 重複上述檢查。跨建置 float64 差異沿用既有 follow-up，不能改寫成 GPU 取消缺陷。
7. 最後再呼叫 `Trainer.Snapshot`、DeviceAdam `Snapshot`、`Close`、第二次 `Close`，分別記錄 host snapshot、裝置 snapshot、等待／釋放及 closed／unusable 結果。不要把成功提交後的取消擴張成回滾驗收。

## Changed

| 檔案 | 變更摘要 |
|---|---|
| `runs/.tmp/gpu-cancel-audit-20261015/gpu-cancel-review.md` | 新增本次 DeviceAdamW／GPUDeviceTrainer 來源審查、取消位置、提交界線、清理契約與硬體探針驗收清單。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| 只讀來源與票據查驗 | `docs/tickets/100-gpu-cancel-preservation.md`、`evidence/OPS-05/gpu-cancel-audit-20261015/plan.md`、指定 Go 來源 | 完成；沒有修改正式來源、測試或共享文件。 |
| codebase-memory-mcp 查詢 | CoImNet 索引中的 DeviceAdamW／GPU trainer 呼叫圖 | 作為導覽；所有結論再以目前來源逐行核對。 |
| 只讀 `git status --short` | 工作樹 | 看到既有 `delivery-status.md` 修改及票據／OPS-05 證據未追蹤項；本輪未改它們。 |
| 寫入審查檔 | 指定 `runs/.tmp/.../gpu-cancel-review.md` | 只新增本檔。 |

## Verified

- 已核對 DeviceAdamW 公開 `Step`、`StepChecked`、`Snapshot`、`Close`，candidate／shadow bank／current bank 的實際提交順序，以及所有指定 DeviceAdam source files 的 `ctx.Err` 位置。
- 已核對 `NewGPUDeviceTrainer`、`RestoreGPUDeviceTrainer`、`applyDeviceAdamUpdate`、`Trainer.Step`／`StepFrom`、部分累積窗、GPU forward／backward、候選 `Predict` 的 context 傳遞與 host commit。
- 已核對 DeviceAdam mutex、Trainer mutex、`cancellableMutex` 的 lock order，Map cancellation 的 unmap／wait／poll／pending release，submit error 的 drain，以及 Close 的 WaitIdle／release。
- 已讀取 STA-03 歷史審查；它只能證明先前 GPU 新逐階段硬體證據缺少，不能作為本票實測通過證據。
- 本輪沒有確認錯誤。所有硬體行為、位置觸發是否實際到達、正常／race／新程序結果仍待 root probe。

## Notes

- 依本輪限制只做唯讀 `git status --short`、來源／文件讀取與 codebase-memory-mcp 查詢；沒有執行 `go build`、`go test`、`go test -race`、`go vet`、硬體 probe、額外暫存或任何其他 CLI 驗證。
- 宿主已嘗試 `opencode models` 與免費 `big-pickle` plan 呼叫，60 秒逾時並終止；本輪沒有重試，也沒有假稱取得該結果。依 AGENTS.md 的回退設定，後續若需 agent 應使用 `gpt-5.6-luna/max`，但本輪不再派工。
- 既有工作樹的 `delivery-status.md`、票據檔與 OPS-05 證據檔不是本輪修改目標，未改動、提交或清理。

## Next

由 root 執行固定硬體探針，逐點記錄實際 caller frame／次數、submit／map／drain 狀態、完整 DeviceAdam snapshot、Trainer snapshot、原物件 retry、一般／race 及新程序結果；在有直接反例前維持「無 CONFIRMED 缺陷」。

Skills：investigate、software-engineering-guidelines、diff-inspector
