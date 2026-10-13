# Boundary 86 formal regression overlay

日期：2026-10-14

## Problem

目前 `OnlineLearner.Act` 在 `Individual.Advance` 和取 `Snapshot().Parameters` 之間釋放 `Individual.mu`，所以外部 `TrainEpisode` 可以改參數，讓 `ActionRecord.Output` 仍由舊參數產生，`ModelVersion` 卻指向新參數。`OnlineLearner.apply` 也在 `episodePredict` 和 `TrainEpisode` 之間釋放同一把鎖，外部更新可能使 weighted target 以 P0 計算、卻在另一個參數狀態訓練。

本交付只新增 overlay 測試草稿，沒有修改 production、正式測試、docs、evidence 或快取，也沒有執行 Go、build、race 或 CLI。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `/Users/timlai/Developer/coimnet/runs/.tmp/online-boundary-audit-20261014/boundary86/formal_regression.go.txt` | 完整 `package learning` overlay，含 public Act、Update、TrainEpisode fixture、private mutex-only 同步 seam、runtime stack 排隊診斷、Act 與 score 0/.5/1 weighted 回歸 oracle |
| `/Users/timlai/Developer/coimnet/runs/.tmp/online-boundary-audit-20261014/boundary86/draft-notes.md` | 記錄同步順序、預期完整狀態、舊碼失敗理由、限制、來源行號與雜湊 |

## Sync order

### Private test-only synchronization seam

`onlineBoundaryIndividual` 先呼叫 public `Snapshot`，把現有 `i.mu.token` 初始化為一個可取得的 token。`onlineBoundaryHoldContext` 持有 `*Individual`，在第一次觀察 `len(individual.mu.token) == 0` 時關閉 `entered` 並等待 `release`。它不計算 `context.Err` 次數，也不依賴核心行號。`entered` 發生後才啟動外部 public `TrainEpisode`，因此這個 pause 證明被測的 Act 或 Update 已取得同一個 `Individual.mu`。

外部使用 `onlineBoundaryQueueContext`。`Done` 只以 atomic bool 記錄 select 表達式已求值，不能單獨當作排隊證據。`onlineBoundaryWaitParkedTrainEpisode` 另外讀取 bounded `runtime.Stack`，要求唯一外部 wrapper 同時出現在 `[select]`、`(*cancellableMutex).LockContext` 和 `(*Individual).TrainEpisode` frame。收到 mutex token 後，queue context 的第一次後續 `Err` 關閉 `acquired` 並等待 `release`。這個 callback 不計固定次數，且不宣稱 channel FIFO。

同步順序固定為：

1. 建立 ordinary individual，public `Snapshot` 初始化 mutex token。
2. 啟動 `Act` 或 `Update`，hold context 在 Individual 鎖內回報 `entered`。
3. 只在 `entered` 之後啟動外部 public `TrainEpisode`。
4. 先確認 queue `Done` 已求值，再以 stack probe 確認外部 wrapper 確實停在 mutex select。
5. 開啟 hold，等待 online Act 或 Update 自己返回。此時不先釋放外部 queue，避免把舊碼的第二次 Individual lock 變成假成功，也避免要求修正後等待某個舊 SnapshotBlocked 狀態。
6. online 操作返回後，等待外部 `acquired`，開啟 queue，等待外部 `TrainEpisode` 結束。
7. 用 public `ActionRecord` 與完整 public `IndividualSnapshot` 比較結果。timeout 一律 `t.Fatalf`，不把 timeout 當作通過。

## Test cases and expected states

### `TestOnlineIndividualBoundaryAct`

來源的 Act 鎖與版本取值在 `/Users/timlai/Developer/coimnet/learning/online.go:253-291`。`Advance` 在 `/Users/timlai/Developer/coimnet/learning/individual.go:462-614` 取得並釋放 `Individual.mu`，`Snapshot` 在 `individual.go:396-426` 另取同一把鎖。`ActionRecord.ModelVersion` 的公開契約在 `online.go:91-108`，hash 必須是產生該答案的參數。

serial control 先 public `Act`，保存 `controlAfterAdvance`，再 public `TrainEpisode(.4)`，保存完整 `controlAfter`。因此 control 包含 `Advance` 後的 persistent `Neural`，不是只比較一次外部訓練的 snapshot。

interleaved case 使用同一初始 snapshot。新同步下，Act 必須在外部 queue 釋放前返回，且 `Output`、`OutputHash`、`ModelVersion` 三欄都與 serial `controlRecord` 相同。外部 `.4` 完成後，actual 的完整 `IndividualSnapshot` 必須等於 `controlAfter`，包含 `Parameters`、完整 `Optimizer`、`Neural` 與其餘 public snapshot 欄位。`controlAfterAdvance.Neural` 也必須保留到 final snapshot。

舊碼的 `Advance` 和 `Snapshot` 之間有公開可插入的 Individual lock 邊界。外部 wrapper 已在 `LockContext` select 等候時，舊 Act 在放開 hold 後會先面臨外部持鎖，再嘗試第二次 Snapshot lock，故 `online Act` 在 queue 釋放前不會返回，bounded wait 會失敗。這是預期 RED 路徑，不需要診斷舊碼的 Snapshot stack。

### `TestOnlineIndividualBoundaryWeighted`

來源的 weighted 流程在 `/Users/timlai/Developer/coimnet/learning/online.go:393-453`：先 `episodePredict`，再 `weightedTarget`，再 `TrainEpisode`。`episodePredict` 在 `online.go:507-533` 取得 Individual lock，`TrainEpisode` 在 `learning/individual.go:823-851` 再取得一次。score 權重及 clamp 契約在 `online.go:475-505`，ticket 23 的公開 replay／adaptive evaluation 契約在 `docs/tickets/23-online-learning-replay-and-adaptive-evaluation.md:24-39`。

每個 `score_0`、`score_half`、`score_1` 都建立相同的 public `before`：Act、SetTargets、Receive、Snapshot。serial oracle 是 weighted Update first，再 public external `TrainEpisode(.4)`。actual 走相同 input、相同 feedback，並在 Update 取得 Individual lock 後才啟動 external queue。

各 case 的 oracle：

| case | fresh weighted step | final actual state |
| --- | --- | --- |
| `score_0` | `Consumed=1`、`GradientSteps=1`；fresh score-zero `TrainEpisode` 必須 `Applied=true`、`GradientNorm=0`、`UpdateNorm=0`、Parameters 與 Neural 不變；`Optimizer.State` 可按來源更新，但 `Updates` 與 `Episodes` 各前進 1 | actual 完整 snapshot 等於 weighted-first → external control；只有在 stale control 完整 snapshot 可區分時，才拒絕 actual 等於 stale control |
| `score_half` | weighted target 是 P0 prediction 與 declared target 的中點，report 需一個 consumed/gradient step | `Parameters`、完整 Adam state、Updates、Accumulator、Episodes、Neural、Plastic、Chemical 與其他 public snapshot 欄位全部等於 weighted-first → external |
| `score_1` | weighted target 是 declared target，report 需一個 consumed/gradient step；不要求 stale-P0 target 一定不同 | 完整 snapshot 等於 weighted-first → external；stale negative control 僅作可區分時的反向檢查 |

stale negative control 先 external `.4`，再以 interleave 前保存的 P0 prediction 和 score 計算 stale effective target。它只在 `!reflect.DeepEqual(staleAfter, freshAfter)` 時成為可區分 negative control，避免把 Adam momentum、counter 或其他正常狀態差異誤報為順序缺陷。

`score_0` 的來源計數行為在 `/Users/timlai/Developer/coimnet/learning/trainer.go:392-573`：zero gradient 仍可能遞增 Adam steps、Updates；`Individual.TrainEpisode` 在 `individual.go:850` 遞增 Episodes。因此測試只要求 zero result 的參數、Neural、梯度 norm、update norm，及 Updates/Episodes 的明確前進，不要求整個 optimizer state 與 before 相同。

### 舊碼必敗依據

舊碼的 Act gap 是 `Advance` 解鎖後重新 `Snapshot`，weighted gap 是 `episodePredict` 解鎖後重新 `TrainEpisode`。兩個 gap 的來源鎖邊界與公開契約均已在上列行號核對。overlay 會在 queue 已由 stack probe 證明等待時，先放開 online operation 的 hold，再要求 online operation 在 queue release 前返回，因此目前舊碼至少 Act 與 score 0 會在 bounded deadlock guard 失敗，而不是因提早釋放 queue 而誤通過。這是來源級推導；本輪沒有執行 Go，沒有宣稱 runtime RED。

## Confirmed versus unknown

### Confirmed from source

- Act 目前分開呼叫 `Individual.Advance` 和 `Individual.Snapshot`，兩者間可有外部 `TrainEpisode`。
- `ActionRecord.ModelVersion` 公開定義為產生答案的參數 hash。
- weighted update 目前分開呼叫 `episodePredict` 和 `TrainEpisode`，兩者間可有外部參數更新。
- score 0 的 residual 為零，但 Trainer 仍走 optimizer step 路徑，不能用 Adam state 不變作 oracle。
- 測試沒有新 production API、沒有 sleep、沒有固定 `context.Err` call count；runtime stack 僅作 queue parked 的 bounded 診斷。

### Unknown / not runtime-verified

- 本輪沒有執行 overlay、普通 Go test、race、build、cache 或 CLI。Root 尚須驗證 compile、同步穩定性、完整 snapshot equality 及 race。
- Go channel waiter 的實作公平性不是測試契約。stack probe 能證明外部 wrapper 當下在 select，不保證語言層級 FIFO；測試不斷言 FIFO。若 root 實測出 scheduler 偶發未形成預期交錯，應保留失敗證據，不放寬 oracle。
- 這份 overlay 假定 ticket 86 修正會在同一個 Individual lock boundary 內完成版本化 Act 與 weighted prediction/target/training。若實作改用等價 private held-lock helper，測試不依賴 helper 名稱，但 root 的 overlay mapping 必須保留 public wrapper。
- 化學、frozen mask、partial accumulation 不在本 overlay fixture 內。這份回歸只覆蓋普通 continuous path 與完整 score 0/.5/1；其餘 contract 仍需 root 依 skeleton 續驗。
- Root 提供的 v3 ordinary/race PASS 及 V0/V1 證據不是本輪重跑，僅作既有診斷背景。

## Root exact verification command

Root 將把本檔映射到暫存 `learning` 測試路徑後執行：

```text
go test -overlay <Rootoverlay> -count=1 -json -run '^TestOnlineIndividualBoundary' ./learning
```

這是預定命令，本輪未執行。Root 另依專案流程執行 race 與完整工程驗證。

## SHA-256

| 檔案 | SHA-256 |
| --- | --- |
| `boundary86/formal_regression.go.txt` | `4c8e0dae2506f0b6f1088f22118b7d71db96f0cc3661b6f0226c1ac641be9dda` |
| `boundary86/skeleton.md` | `20798e692c2ad9ed205a58b103d13e79a63e13ddc024bda8b7c555549dbd7e84` |
| `concurrency/public_interleave_v3_test.go.txt` | `718980b1d6bb9514bd9bb250d1a25da17c27d842f0534f6a2757366aa98ba817` |
| `learning/online.go` | `2f742e1c266e018d4964bfb7b92b6eabaa543fb7c4043108ee51dc51ba4dae25` |
| `learning/individual.go` | `e432bfdb794be69953a1d3b7f76ab4de61099f44f5934c407c678213b4dca4de` |
| `learning/individual_chemical_training.go` | `dcf14d76ede60f16e36f48f63fff48124d63c645dbde55355e43b617972f399e` |
| `learning/trainer.go` | `0a2f2cce3a72e00df4198767553f4a5c58785b3d3eaaae2af1b3731c95928943` |
| `docs/individual-state.md` | `5b404f809517a27a92eeeae56dd70e6d643d66e5563d30f655bab598902dabf7` |
| `docs/tickets/23-online-learning-replay-and-adaptive-evaluation.md` | `20870c35d8e8d244a1eea0d6273954bfa56c4911fff4646065093f1856efe41e` |
| `ENG.md` | `7d495442e2d578fe3ccfda6068ff69e4cb8970ce7e0f00d1cfc6a8890a06efe` |

## Next

Root 先用 exact overlay 命令驗證兩個 top-level tests，再依結果決定是否需要調整 Root overlay mapping；不得以未執行的草稿宣稱修復已通過。

Skills：investigate
