# Ticket 85 混合線上更新重試修正草稿

日期：2026-10-14
範圍：只產出 `implementation/online.go.txt` 與本報告；未修改正式來源、測試、docs 或 evidence。

## Problem

`AllowGradient=true` 且 `AllowPlastic=true` 時，原流程先完成梯度階段，再執行塑性階段。塑性閘門或 `AdvanceGated` 失敗會讓 feedback 留在 pending，但沒有保存梯度階段已完成的事實。下一次 `Update` 因而可能重跑梯度、重複部分累積，或再次推進 `UpdateEvery`。

`Update` 的 `now` 可以倒退。先前塑性失敗的 feedback 可能暫時不在 `available`，之後另一筆較早到期 feedback 仍可處理，因此單一失敗槽不足以保存所有尚未完成的混合階段。

## Done

在 `online.go.txt` 建立最小私人 `map[[2]string]bool`。key 是 `(ActionID, Source)`，避免以字串串接造成碰撞。只有混合政策使用此階段記錄，公開 API、保存格式、既有 queue 及 `remainingFeedback` 都保持不變。

- 梯度半段成功完成後才記錄，包括沒有 target、合法 `UpdateEvery` skip、部分累積與實際套用的梯度。
- 梯度錯誤不記錄完成狀態，保留既有純梯度失敗重試及間隔位置不前進的行為。
- 塑性錯誤保留 feedback 與階段標記，重試直接進入塑性半段。
- 塑性成功後回報 `PlasticSteps++` 並刪除該 `(action, source)` 標記，feedback 才由既有 `Update` 流程消耗。
- 多筆失敗階段可同時存在。時間倒退時，未到期的 feedback 與其標記都保留，其他已到期 feedback 依收到順序處理。

## 根因與修正依據

- `evidence/LRN-06/online-boundaries-20261014/mixed-retry-skeleton.md` 要求每個已完成階段只前進一次，塑性失敗只重試塑性，且不引入整個 `Individual` 複製或回滾。
- `docs/tickets/85-online-mixed-phase-retry.md` 限定正式責任在 `learning/online.go`，不改公開簽名、保存 schema、規則公式或機制。
- 原 `apply` 在梯度成功後才進入 gate；修正把梯度完成標記提交點放在所有梯度操作成功之後，把標記刪除點放在 `AdvanceGated` 成功及報告遞增之後。
- `UpdateReport.GradientSteps` 仍只在 `StepResult.Applied` 時遞增。部分累積標記為已完成階段，但本次回報仍為零，符合既有報告契約。

## 標記生命週期與鎖順序

`Update` 先取得 `OnlineLearner.mu`，再呼叫 `apply`。`gradientDone` 只在這把鎖保護下讀寫；`Receive`、`SetTargets`、`Act` 也沿用同一把 learner lock，因此標記不需要額外 mutex。

混合 feedback 首次進入 `apply` 時，先依既有 `eligible` 計算梯度位置。沒有 target、合法 skip、partial accumulation 或成功梯度都在梯度操作無錯後寫入 map。任何梯度錯誤都在寫入前返回。若已有 map 標記，梯度半段完全略過，直接執行塑性半段。gate 錯誤、空 release 或 `AdvanceGated` 錯誤都不刪除標記。塑性成功後刪除標記。

持續個體的鎖順序維持原契約：`OnlineLearner.mu → Individual.mu → Trainer.mu`。`episodePredict` 與 `TrainEpisode` 沿用 `Individual → Trainer` 的內部順序，map 不在個體鎖外被存取。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `runs/.tmp/online-boundary-audit-20261014/implementation/online.go.txt` | 完整正式 `learning/online.go` 草稿，新增混合 feedback 梯度階段 map 及 apply 的提交／清除生命週期。 |
| `runs/.tmp/online-boundary-audit-20261014/implementation/report.md` | 本修正依據、標記生命週期、鎖順序、指紋與驗證限制。 |

## Actions

| 動作 | 對象 | 結果 |
| --- | --- | --- |
| SHA-256 基線查驗 | `learning/online.go` | 與要求基線相同。 |
| 正式來源修改 | `learning/online.go` | 未修改。 |
| 測試／build／cache／Go 指令 | 本專案 | 本 agent 未執行，交由 Root 依 exact 命令執行。 |
| commit／stash／reset | Git 工作樹 | 未執行。 |

## Verified

- 基線來源 `learning/online.go`：`e4b67dbdb037e8ca4aebbb5ae6a4f6a8ecb5c0ff9ebbc7ab768e491470893e7d`。
- 修正草稿 `implementation/online.go.txt`：`24c1261d419d214235fc55a40c0a6f271677bb2a0c60746148828c920c7d3128`。
- `diff -u learning/online.go implementation/online.go.txt` 顯示只有 `OnlineLearner` 的私人 map、建構初始化及 `apply` 階段邏輯變更。
- 尚未驗證 Go 語法、格式、一般測試、race、build、vet、依賴性或 Root 的 32 組相容性比較；不得據此宣稱通過。

## Notes

- `remainingFeedback` 的既有 action/source 字串 key 依任務要求保持不動；本修正的私人階段 key 使用 `[2]string`，兩者範圍分開。
- 階段 map 與 pending queue 同一 `OnlineLearner` 生命週期，沒有 checkpoint 或保存欄位；程序重建不會假設可恢復線上 queue。
- `AdvanceGated` 本身若在錯誤前產生部分內部效果，仍沿用既有個體錯誤處理契約；本修正只決定混合階段是否完成及是否重跑梯度。

## Next

Root 執行指定命令：

```sh
python3 runs/.tmp/online-boundary-audit-20261014/check.py mixed-retry-green go test -count=1 -json -run 'TestOnline|TestIndividualTrain|TestChemicalOnline' ./learning
```

接著執行 32 組相容性比較及完整 build/unit/race/vet/deps；若發現草稿格式或行為差異，再回到這兩個允許檔案修正。
