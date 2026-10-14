# GPU resident observer 審查

## Problem

確認新增的 `audit_gpu_resident_test.go` 是否真的讀回取消前後「目前選中的」GPU Adam bank，逐欄比較參數、兩個動量、計數器與設定，並確認取消後原物件重試等於不中斷對照。這份 observer 的 1,624 個取消點是既有取消點的補充，不能加入 1,916 個 distinct 點。

## Confirmed findings

- **CONFIRMED 缺陷：無。** 目前讀到的來源與 normal/race resident log 沒有證實缺陷。
- Observer 不是只看 host mirror。測試在 `runs/.tmp/gpu-cancel-audit-20261015/audit_gpu_resident_test.go:186-190,198-200,205-209,227-240` 直接呼叫同 package 的私有 `control.deviceAdam.Snapshot`／`live.deviceAdam.Snapshot`。正式來源的 `backend/webgpu/device_adam.go:173-189` 會持有 `DeviceAdamW.mu`，`backend/webgpu/device_adam_execute.go:346-351` 將 `o.currentBank` 傳給 `snapshotAdamBankLocked`。因此讀回的是當下 selected bank 的 device state。
- `AdamWSnapshot` 的 `Config`、`Parameters`、`First`、`Second`、`Steps` 欄位定義在 `backend/webgpu/device_adam.go:27-35`；`snapshotAdamBankLocked` 對這些欄位配置完整讀回在 `backend/webgpu/device_adam_execute.go:359-373`。測試以 `json.Marshal` 字串精確比較，位置在 `audit_gpu_resident_test.go:120-126`，所以 cfg、參數、第一動量、第二動量及每值計數器都在比較範圍內。
- 取消前先取 `Trainer.Snapshot` 與 resident snapshot，再以不中斷 `cc` 對照取得 `wantResult`、`wantHost`、`wantResident`，流程在 `audit_gpu_resident_test.go:186-200`。每個注入點都從 host snapshot 還原新 trainer，先確認還原後 resident snapshot 等於取消前 resident snapshot，再執行取消，位置在 `:202-213`。
- 取消結果的檢查完整涵蓋 `target reached`、`errors.Is(context.Canceled)`、零值 `StepResult`、host snapshot 不變，以及 selected resident snapshot 不變，位置在 `audit_gpu_resident_test.go:215-232`。之後同一個原物件重試，並將 retry 結果、host snapshot、resident snapshot 與不中斷 control 逐一比較，位置在 `:234-243`。
- GPU trainer 的提交邊界與 observer 的判斷相符。`learning/gpu_device_trainer.go:173-198` 以 `StepChecked` 取得並驗證候選，`backend/webgpu/device_adam_checked.go:45-61` 在最後 context check 後才設定 `o.currentBank`，而 host mirror 只在 `learning/gpu_device_trainer.go:201-206` 的成功返回路徑寫入。取消測試的零結果與兩份 snapshot 未變，沒有把成功提交後回滾當成必要契約。
- `resident-normal-hardware.json:1-15` 顯示 normal 執行 exit code 0；`resident-normal-hardware.log:1643` 的 report 是 `all_passed:true`、`cancel_points:1624`，包含 16 組 `mse`／`from`、direct／partial／full／zero-full、masked／unmasked case，裝置為 Apple M3 Metal。`resident-normal-hardware.log:3285` 為最終 `PASS`。
- race resident log 也已完成，`resident-race-hardware.log:1643` 顯示 `all_passed:true`、`cancel_points:1624`，`resident-race-hardware.log:3285` 為 `PASS`。目前沒有對應的 `resident-race-hardware.json` 執行封裝檔，只有 `resident-race-build.json:1-20` 的 race build metadata，因此 race 的實際通過依 log 判定，無法補報命令、exit code 或 log hash。

## Needs investigation

- **Physical bank index 未被觀察。** 測試比較的是 `o.currentBank` 所指向 bank 的完整內容，沒有輸出 `currentBank` 整數，也沒有比較兩個 bank。若需求是證明「取消後 bank 編號沒有切換」，目前證據不足；若契約是 selected state 的邏輯內容與後續結果保持，現有 observer 已覆蓋該契約。`RestoreGPUDeviceTrainer` 以 snapshot 建立新的 resident optimizer，`learning/gpu_device_trainer.go:79-105` 也沒有保存物理 bank 編號，因此 `initialResident == residentBefore` 是狀態等價，不是 bank identity 等價。
- **未觀察 shadow bank、scratch、metrics、queue drain 或 `unusable` 狀態。** `DeviceAdamW` 明確有兩個 resident bank 與可阻止後續重用的 `unusable` 狀態，定義在 `backend/webgpu/device_adam.go:64-73`。observer 只在取消後從 selected bank 做 Snapshot，不能單獨證明未選中的 bank、暫存映射、診斷 metrics、已提交 command 的 drain 及 map 清理全部符合內部資源契約。這是觀察範圍缺口，不是已確認的 runtime bug。
- **observer 本身會改變診斷計量與時序。** `DeviceAdamW.Snapshot` 會做完整 readback，並在 `backend/webgpu/device_adam_execute.go:346-350,374-377,407` 更新 readback duration／bytes metrics；測試又在 `audit_gpu_resident_test.go:248-249` 讀取 metrics。這不會把讀回資料寫回模型 bank，但會影響 metrics 觀察值與後續 GPU scheduling timing，尤其 mapping 重複 occurrence 本來就被標記為 timing-dependent。若 metrics 不變也是契約，仍需獨立 control evidence。
- **沒有獨立輸出「取消前 First／Second／Steps 非零」斷言。** fixture 在 `audit_gpu_resident_test.go:100-114` 設定非零參數與 learning rate，`audit_gpu_resident_test.go:169-185` 先做成功更新，但 report 只記錄 equality 布林值。若驗收要求證明兩個動量與計數器確實已經有歷史值，仍需從 snapshot 輸出非零摘要或明確斷言；目前只能確認這些欄位有被逐欄比較。
- `auditContext` 用 `runtime.Callers`／`CallersFrames`，不是固定 `runtime.Caller`，並在 `audit_gpu_resident_test.go:40-87` 依檔案、行號、phase 與 occurrence 觸發取消。`targets()` 在 `:89-98` 對 mapping 只保留每個 mapping 的第一次 occurrence，排除 scheduler-dependent 的 polling 重複；這是明示的探針範圍，不能宣稱涵蓋每次 map poll。
- `resident-race-hardware.log` 有完整 report／PASS，但目前沒有 race 執行的 JSON wrapper。後續若要把 race 結果納入可重播的正式 evidence，應補上命令、開始時間、exit code、log hash 與 source manifest；不需要因此否定目前 log 已提供的 test PASS。

## 取消／提交位置表

| 位置 | 觀察內容 | 預期結果與實際驗證 |
|---|---|---|
| `audit_gpu_resident_test.go:128-133` | 公開入口分別走 `Trainer.Step`／`StepFrom` | `learning/gpu_device_trainer.go:173` 進入 `StepChecked`；兩種 entry 都出現在 16 cases。 |
| `audit_gpu_resident_test.go:186-190` | 取消前 host 與 selected resident snapshot | 建立比較基準；resident snapshot 不是 `Trainer.Snapshot`。 |
| `audit_gpu_resident_test.go:191-200` | 同一 control 物件不中斷執行 | 保存 `wantResult`、`wantHost`、`wantResident`，作為正常對照。 |
| `audit_gpu_resident_test.go:202-213` | 每個 runtime observation point 還原原始 host state 後注入取消 | `RestoreGPUDeviceTrainer` 建立新物件，且 `initialResident` 必須等於取消前 resident。 |
| `audit_gpu_resident_test.go:215-232` | 候選計算期間取消 | 必須命中 target、回 `context.Canceled`、回零 `StepResult`，host 與 selected resident 都等於 before。 |
| `backend/webgpu/device_adam_checked.go:45-61` | 候選、readback、accept、最後 context check 與 device bank commit | 取消在 commit 前應不改 `currentBank`；commit 後才切換 bank。此 observer 沒有要求 commit 後 rollback。 |
| `learning/gpu_device_trainer.go:201-206` | host mirror commit | 只有 `StepChecked` 成功後寫入 parameters、optimizer、updates、accumulator；取消錯誤不會走這段。 |
| `audit_gpu_resident_test.go:234-243` | 同一原物件取消後正常重試 | retry result、host snapshot、resident snapshot 必須逐一等於不中斷 control。 |
| `audit_gpu_resident_test.go:40-87`、`:89-98` | 公開 context 包裝器與可達取消點 | `Err` 以 runtime frame 觸發，embedded `context.Context` 提供 `Done`；mapping 只取第一次 occurrence，重複 polling 未納入。 |

## 覆蓋與限制

- fixture 覆蓋 `mse`、`from`，direct window=1、partial、full、zero-full，masked／unmasked，共 16 cases。partial 主要覆蓋累積中的取消；direct/full/zero-full 覆蓋實際 GPU Adam update，zero-full 另覆蓋零 learning rate 的 checked path。這些模式不代表所有硬體容量或所有 driver 行為。
- `DeviceAdamW.Snapshot` 本身會在每個 chunk 前、encoder finish 後、submit 後檢查 `ctx.Err()`，並在提交後錯誤時 drain，來源在 `backend/webgpu/device_adam_execute.go:378-405`。observer 的 post-cancel Snapshot 使用未取消的 context，因而能讀完整 state，但不會把 Snapshot 自己的取消分支當成 update cancellation point。
- host `Trainer.Snapshot` 只複製 network config、host parameters、options、host optimizer、updates、accumulator，來源在 `learning/trainer.go:227-235`。resident observer 補上了 host mirror 無法證明的 device state；它不等同於比較所有 GPU allocation 或診斷資料。
- 既有 v2 evidence 的 distinct 取消點為 1,754，`normal-hardware-summary.json:1-15`；既有三 chunk normal/race 各 162 點，`multichunk-normal-hardware.log:169` 與 `multichunk-race-hardware.log:169` 都記錄 `all_passed:true,cancellation_points:162`。resident 的 1,624 點是同一類邏輯位置的 resident readback 補強，不應加入 1,916 distinct 總數。
- 新程序接續 evidence `new-process-hardware.json:1-12` exit code 0，`new-process-resume.json:1-17` 起的結果逐項標示 exit code 0 與 `snapshot_result_prediction_json_bytes_equal:true`，`new-process-hardware.log:51` 記錄 `CHILD_RESUMES_EXACT 50`。這支援接續等價，但不是 resident observer 自身的 bank identity 證明。

## 每項依據

| 判斷 | 依據 | 結論 |
|---|---|---|
| 是否讀 selected GPU bank | `backend/webgpu/device_adam_execute.go:346-372`；`audit_gpu_resident_test.go:187,198,205,227,238` | 是，直接從 `currentBank` 讀回完整 `AdamWSnapshot`。 |
| 是否比較 cfg、參數、兩個動量、計數器 | `backend/webgpu/device_adam.go:27-35`；`audit_gpu_resident_test.go:120-126,209,231,242` | 是，以 JSON exact equality 比較。 |
| 是否驗證取消後原物件重試 | `audit_gpu_resident_test.go:212-243` | 是，且與不中斷 control 的 result、host、resident 三者比較。 |
| 是否驗證 fresh restore | `audit_gpu_resident_test.go:202-210`；`learning/gpu_device_trainer.go:79-105` | 是，驗證 host snapshot 還原後的 resident logical state。 |
| normal hardware 結果 | `resident-normal-hardware.json:1-15`；`resident-normal-hardware.log:1643,3285` | 1624 點、16 cases、all_passed、PASS。 |
| race hardware 結果 | `resident-race-hardware.log:1643,3285`；build metadata `resident-race-build.json:1-20` | log 顯示 1624 點、all_passed、PASS；缺少執行 wrapper JSON。 |

## 採用判定

**可採用，但定位為補充證據。** 這份 observer 足以支持「Apple M3 Metal 上，取消發生於已觀察的 GPU trainer／DeviceAdam 路徑時，selected resident state 的 cfg、參數、First、Second、Steps 與 host mirror 維持不變，原物件重試等於不中斷 control」這個狹義結論。normal 與 race log 都已讀到完整 report 和最終 PASS。

它不能單獨支持 physical bank index、shadow bank、scratch／queue／map cleanup、metrics 或 `unusable` 的完整資源契約，也不能把 1,624 點計入 1,916 distinct 點。這些是證據邊界，不是已確認缺陷。

## Problem

本輪要確認 resident observer 是否真的補足 host `Trainer.Snapshot` 看不到的 GPU resident state，並避免重複計點或把未完成的 race metadata 當成新證據。

## Done

已唯讀核對 observer source、正式 WebGPU／learning 提交路徑、resident normal/race logs、v2／multichunk／new-process evidence。結論是無 confirmed defect，observer 證據可按上述狹義範圍採用。

## Changed

| 檔案 | 變更摘要 |
|---|---|
| `/Users/timlai/Developer/coimnet/runs/.tmp/gpu-cancel-audit-20261015/resident-review.md` | 新增本 resident observer 審查報告。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| 唯讀檔案核對 | CoImNet source、resident／v2／multichunk／new-process evidence | 完成。 |
| Go build/test、GPU 執行 | 本輪 reviewer | 未執行，依任務限制。使用的硬體結果均是已存在 evidence，沒有冒充本輪新執行。 |
| Git commit、stash、reset、正式來源修改 | 儲存庫與正式檔案 | 未執行。 |

## Verified

- `DeviceAdamW.Snapshot` 的 selected bank 讀回、`AdamWSnapshot` 欄位與 lock 邊界已由來源行號核對。
- cancellation、commit、host mirror、retry/control 的每個比較點已由 observer 行號核對。
- resident normal log 與 resident race log 都有 `all_passed:true`、`cancel_points:1624` 及最終 `PASS`。
- v2 1,754、multichunk 162、50 子程序接續 evidence 已分開核對，沒有把 resident 1,624 加入 distinct total。

## Notes

- race resident 執行有完整 log，但缺少與 normal 對稱的 JSON command wrapper，因此不能提供該次 race 的 exit code、開始時間或 hash metadata。
- observer 不輸出 physical `currentBank` 編號，也不檢查 shadow bank 或內部清理狀態。若這些是額外驗收條件，需另補直接可核對的 evidence。
- 本輪未使用 codebase-memory MCP，因指定的 local source 與 evidence 已足以完成核對；未讀取或寫入 memory。

## Next

若要封閉剩餘 evidence 缺口，補一份 race resident 執行 metadata wrapper，並在不改正式來源的 probe 中輸出 bank identity／雙 bank 或 cleanup 診斷；這不影響目前 observer 作為 selected resident logical state 補充證據的採用判定。

Skills：investigate、software-engineering-guidelines、diff-inspector
