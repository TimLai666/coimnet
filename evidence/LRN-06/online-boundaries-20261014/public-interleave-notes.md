# online boundary concurrency v2 診斷筆記

## Confirmed（來源級，尚未宣稱實測）

- `OnlineLearner.Act` 在 `learning/online.go:260-271` 先持有 `OnlineLearner.mu`，呼叫 `Individual.Advance`，再呼叫 `Individual.Snapshot` 計算 `ModelVersion`。`Individual.Advance` 在 `learning/individual.go:462-469` 只持有 `Individual.mu`，而 `Snapshot` 在 `learning/individual.go:396-402` 重新取得同一把個體鎖。
- 公開的 `Individual.TrainEpisode` 在 `learning/individual.go:829-836` 只取得 `Individual.mu`。因此直接呼叫 `TrainEpisode` 不會取得 `OnlineLearner.mu`。這是合法公開插入點，但本輪未用 sleep 或 production hook。
- Act 草稿在持有 `Individual.mu` 的 `Advance` 內停在 `learning/individual.go:602-604` 的最終 `ctx.Err()`。3 列 fixture 的來源計數為 `OnlineLearner.LockContext` 2、`Individual.LockContext` 2、輸入驗證 3、`dynamics.Continuous.AdvanceModulated` 11、個體提交前檢查 1，因此 hold call 19。這個計數是來源耦合診斷條件，不是穩定公開契約。
- `OnlineLearner.Update` 的梯度路徑在 `learning/online.go:399-414` 依序執行 `episodePredict`、`weightedTarget`、`TrainEpisode`。`weightedTarget` 在 `learning/online.go:475-485` 明定 `w=clamp(score,0,1)`，score=0 時殘差為零，訓練 step 仍可能回報 `Applied`，但在此草稿使用 `learning/trainer.go:65-67` 的 `DefaultOptions`（`WeightDecay=0`），fresh-zero 的梯度與參數更新預期都為零。
- `episodePredict` 的一般路徑在 `learning/online.go:507-522` 先持有 `Individual.mu` 再取得 `Trainer.mu`。其 P0 產生後到 `weightedTarget`／下一個 `TrainEpisode` 之間，來源上存在外部 `Individual.TrainEpisode` 取得個體鎖的插入點。這只確認可插入的來源窗口，尚未確認執行結果。
- `ENG.md:199` 只明定 `Individual` 內部操作共用同一把鎖，沒有明定 `OnlineLearner` 的 `l.mu` 與呼叫者直接使用 `Individual` 的操作形成跨物件 atomicity。因此「可以合法並行插入」本身不是缺陷；只有實測證明答案 hash 對不上產生答案的參數，或 score=0 的 stale P0 造成非零額外訓練，才是契約違反。
- `cancellableMutex.LockContext` 的 select 位於 `learning/trainer.go:592-612`，Unlock 將 token 送回 `learning/trainer.go:615-618`。v2 不把 `queueContext.Done` 的 ack 當成排隊證據，而要求 `runtime.Stack` 同一 goroutine block 同時含 `[select]`、本測試 wrapper、`(*cancellableMutex).LockContext` 與 `(*Individual).TrainEpisode`。取得 token 後，`queueContext.Err` 第二次呼叫會暫停外部操作，並再以 stack 觀察第一操作的 public call 已等待個體鎖，或 Update 的 online `TrainEpisode` 已在 `LockContext` 等待，才釋放外部操作。這個診斷順序不依賴 FIFO 或公平性推論。

## Needs investigation

- 尚無 Root 的 Go 執行結果，故尚未把「ActionRecord 的 `ModelVersion` 指向更新後版本」或「score=0 仍產生額外梯度／錯誤加權」列為已確認缺陷。
- `runtime.Stack` 是診斷觀察，不是執行緒佇列 API。v2 要求 `[select]` 與三個 frame 同時出現，仍可能受 Go 版本、編譯器 inline 或 stack 格式影響。bounded deadline 到期時只能記為未證實，不能把 queue ack 解讀成已停泊。
- `holdContext` 的 call count 依目前 context.Err 呼叫數計算。任何新增或刪除檢查都可能讓 call 19／18 失效。失效應調查來源，不應調整測試來迎合預期結果。
- Act 的 post-Advance handoff 使用 `queueContext` 在外部 token receive 後持有 `Individual.mu`，再以 stack 證明 Act 已在 `Snapshot` 等待。這能觀察本次呼叫順序，但不建立 `cancellableMutex` 的 FIFO 或公平性公開保證。
- Score=0 的非 fresh optimizer 不應用「參數必定完全不變」作為判準。v2 以 `controlExternal` 的完整 optimizer state，再執行 `expectedExtra`，比較 `Parameters`、Adam state、更新次數、累積器，以及另行比較 Episodes，排除舊 Adam momentum 的替代解釋。

## 重現草稿

- 檔案：`runs/.tmp/online-boundary-audit-20261014/concurrency/public_interleave_v2_test.go.txt`
- build tag：`online_boundary_diagnostic`
- 測試一：`TestPublicInterleaveActAdvanceSnapshotVersion`。公開呼叫為 `OnlineLearner.Act` 與 `Individual.TrainEpisode`。驗證 V0、V1、Act output 與舊模型副本 output、外部 `StepResult`，並記錄 `ActionRecord.ModelVersion` 與 V1 的位置。
- 測試二：`TestPublicInterleaveOnlineScoreZeroAfterEpisodePredict`。公開呼叫為 `Act → SetTargets → Receive → Update`，外部只用 `Individual.TrainEpisode`。比較 fresh-zero、external-only、post-external expected extra 與實際完整訓練狀態；每測試以 `t.Logf` 記錄 V0/V1（score=0 另記 V2）、output、prediction、各結果 norm、完整 state 比較布林值與 Episodes，不印巨大 snapshot。
- 每個外部操作先等待 `queueContext.Done` ack，再由 `runtime.Stack` 看到 `[select]`、`diagnosticExternalTrainEpisodeAct`／`diagnosticExternalTrainEpisodeUpdate`、`(*cancellableMutex).LockContext`、`(*Individual).TrainEpisode`。此 ack 不是停泊證明，stack 檢查失敗就停止該重現。
- Root 應以其 overlay 執行：

  `python3 runs/.tmp/online-boundary-audit-20261014/check.py public-interleave-diagnostic go test -overlay <Rootoverlay> -tags online_boundary_diagnostic -count=1 -v -run '^TestPublicInterleave' ./learning`

  若需縮小診斷，可分別使用 `^TestPublicInterleaveActAdvanceSnapshotVersion$` 與 `^TestPublicInterleaveOnlineScoreZeroAfterEpisodePredict$`。本輪未執行 Go、build、race、cache 或上述命令。

## SHA-256

本輪讀取時工作樹來源與契約指紋（`learning/online.go`、`docs/individual-state.md`、`ENG.md` 在本輪期間已有父任務的未提交文件／混合重試變更，以下採目前讀取值）：

| 檔案 | SHA-256 |
| --- | --- |
| `learning/online.go` | `2f742e1c266e018d4964bfb7b92b6eabaa543fb7c4043108ee51dc51ba4dae25` |
| `learning/individual.go` | `e432bfdb794be69953a1d3b7f76ab4de61099f44f5934c407c678213b4dca4de` |
| `learning/individual_chemical_training.go` | `dcf14d76ede60f16e36f48f63fff48124d63c645dbde55355e43b617972f399e` |
| `learning/trainer.go` | `0a2f2cce3a72e00df4198767553f4a5c58785b3d3eaaae2af1b3731c95928943` |
| `docs/individual-state.md` | `5b404f809517a27a92eeeae56dd70e6d643d66e5563d30f655bab598902dabf7` |
| `docs/tickets/23-online-learning-replay-and-adaptive-evaluation.md` | `20870c35d8e8d244a1eea0d6273954bfa56c4911fff4646065093f1856efe41e` |
| `ENG.md` | `7d495442e2d578fe3ccfda6068ff69e4dcb8970ce7e0f00d1cfc6a8890a06efe` |
| `evidence/LRN-06/online-boundaries-20261014/plan.md` | `3a4ceff319c99028ca4dc27ca25fd4ff78f8a4ded53105d7530e1a1a943b2892` |
| `runs/.tmp/online-boundary-audit-20261014/concurrency/public_interleave_v2_test.go.txt` | `c1fafa69372da1b89bfd1dc830bc51dfae81605f64e639b0c7e31f377a646ff0` |

以上 SHA 只識別讀取或診斷草稿內容，不代表已完成 Go 驗證。

## 未驗證項

- 未執行 `go test`、race、build、vet、overlay check，未取得任何 runtime log、V0/V1 數值或 `StepResult` norm。
- 未證明目前 runtime stack 格式一定保留所有指定 frame，也未證明其他核心、化學訓練或非預設 optimizer 的結果相同。草稿只針對無化學、連續核心、3 列 fixture。
- 未提出 production 修正、正式測試、契約變更或缺陷修復；本檔僅供 Root 以 overlay 進行決定性診斷。
