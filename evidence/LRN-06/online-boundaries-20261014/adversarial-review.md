# Ticket 85 混合線上更新重試敵對審查

日期：2026-10-14
基準：`5f26c932981319627e3b76a3e8d47a5ae741c05e`
範圍：只審查基準相對目前 `learning/online.go` 的工作樹 diff；未執行 Go、build、cache 或測試。

## Diff Inspector

Scope：**CLEAN**。完整 diff 只有 `OnlineLearner` 的私人 `gradientDone` map、建構初始化，以及 `apply` 的階段讀取／寫入／清除。沒有公開簽名、保存 schema、正常算式或機制變更，與 `mixed-retry-skeleton.md`、ticket 85 及 `ENG.md` 契約一致。

Reviewed：`learning/online.go` 全部 diff；`gradientDone` 與 `pending` 的所有 producer／consumer；兩種 producer／consumer 的 action/source key；缺少及替換 target；`UpdateEvery`、多筆 pending、now 倒退、多來源與收到順序；gradient error、gate error、空 release、非有限 gate、plastic arithmetic、context cancellation；純 gradient、純 plastic、Evaluate、chemical online 路徑；map 與 pending 的清除生命週期；三份 `learning/online_mixed_retry*_test.go` 頂層測試及 ENG 新增契約。

Adversarial review：**RUN**。依來源與狀態轉移做靜態失敗情境審查，沒有把未執行的 runtime 結果當成通過證據。

## Confirmed

No confirmed findings. 本 diff 沒有可由目前程式與契約確認的 P0、P1 或 P2 缺陷。

## 生命週期與錯誤出口查驗

- map 的唯一寫入點是 `/Users/timlai/Developer/coimnet/learning/online.go:424-429`。只有 gradient 半段完整返回無錯後才標記，包括缺少 target、合法 interval skip、partial accumulation 及已套用更新。`episodePredict`、`weightedTarget` 或 `TrainEpisode` 在 `:406-416` 回錯時不會寫入 marker，`eligible` 也不會在失敗前提交。
- map 的 producer／consumer 配對完整。`/Users/timlai/Developer/coimnet/learning/online.go:396-400` 讀取 `(ActionID, Source)` marker，已完成 gradient 時略過重算；`:449-451` 只在 gate 成功、`AdvanceGated` 成功及 `PlasticSteps` 已增加後刪除。gate error、空 release、非有限 gate 或 plastic arithmetic／context error 由 `:434-446` 返回，marker 與 pending feedback 都保留。
- pending 的 producer／consumer 仍是原契約。`Receive` 在 `/Users/timlai/Developer/coimnet/learning/online.go:329-334` 以 action/source 去重後追加；`Update` 在 `:366-390` 依 `AvailableFeedback` 過濾、依收到順序呼叫 `apply`，只有成功處理的 prefix 才由 `remainingFeedback` 移除。因 mixed apply 成功前已刪 marker，正常路徑不會移除 pending 而留下已完成階段。
- `UpdateEvery` 的 `eligible` 只在 gradient 階段成功後於 `/Users/timlai/Developer/coimnet/learning/online.go:401-422` 推進一次。retry 讀到 marker 後不再執行這段，因此 skip、partial accumulation 與實際更新都不會二次計數。
- target 變更不會重做已完成階段。`SetTargets` 只更新 learner 自己複製的 target（`/Users/timlai/Developer/coimnet/learning/online.go:224-245`）；marker 存在時 `apply` 不再讀取該 action 的 target。缺少 target 的首次 gradient 半段也會在 `:424-429` 完成標記，之後補 target 只會重試 plastic。
- map 使用 `[2]string{ActionID, Source}`，多來源同 action 與不同 action 的階段互相隔離。`AvailableFeedback` 與 `remainingFeedback` 仍保留既有 pending 收到順序；now 倒退時未到期 feedback 不進 `available`，既有 marker 留在同一 pending feedback 上，之後仍能獨立重試。
- 純 gradient／純 plastic 不會寫入 marker，因為 `mixed` 在 `/Users/timlai/Developer/coimnet/learning/online.go:397-399` 為 false；Evaluate 在 `Update` 的 `:372-376` 於 apply 前返回，不消耗 pending 也不改 map。chemical online 仍透過相同 `apply` 階段界線，沒有另一份 marker producer／consumer。
- `gradientDone` 的活動數量在正常生命週期內受 pending 內容界定：每一筆 marker 都是在該 feedback 尚未成功消耗時建立，plastic 成功先刪 marker 再讓 `Update` 移除 feedback。沒有公開 restore 或 checkpoint 入口會另造孤立階段狀態。

## Needs investigation

- `[NEEDS INVESTIGATION] /Users/timlai/Developer/coimnet/learning/online.go:159-452`：靜態生命週期與錯誤路徑沒有發現缺陷，但本審查依指示沒有取得目前來源的 runtime exit code、報告或 log。缺少的事實是 Root 指定命令在這份工作樹上的實際結果；由 Root 執行 `python3 runs/.tmp/online-boundary-audit-20261014/check.py mixed-retry-green go test -count=1 -json -run 'TestOnline|TestIndividualTrain|TestChemicalOnline' ./learning` 後補足。這不是 confirmed finding，也不能由本報告宣稱 runtime 通過。

## Scope differences

無。Online 與 Individual 跨入口原子性依指定範圍列為獨立調查，沒有把既有未解議題算成這個 map diff 造成的缺陷。

## Problem

混合線上更新在 gradient 成功、plastic 後半失敗時，feedback 會留在 pending；本 diff 旨在保存每筆 action/source 的 gradient 完成事實，讓 retry 只執行尚未完成的 plastic 半段，同時保持 interval、partial accumulation、批次、延後回饋及正常公開結果。

## Done

完成基準相對工作樹 diff 的完整靜態敵對審查。確認 marker 的寫入只位於 gradient 成功邊界，讀取／清除與 pending 消耗對齊；所有指定錯誤出口及純政策分支都沒有發現新的狀態遺失、重複更新或格式變更。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `/Users/timlai/Developer/coimnet/runs/.tmp/online-boundary-audit-20261014/review/report.md` | 新增本次 ticket 85 diff 的敵對審查結果、狀態生命週期、範圍判定、未驗證事項與來源 SHA-256。 |

## Actions

| 動作 | 對象 | 結果 |
| --- | --- | --- |
| 唯讀來源／契約／測試審查 | `learning/online.go`、ENG、ticket 85、mixed-retry skeleton、三份 `learning/online_mixed_retry*_test.go` 及相關 Individual／signal 路徑 | 完成。 |
| Go／build／cache／正式測試 | 本專案 | 未執行，遵守本輪限制及 Root exact command 分工。 |
| production／tests／docs／evidence／其他暫存修改 | 本專案 | 未修改；只新增本報告。 |
| commit／stash／reset | Git 工作樹 | 未執行。 |

## Verified

- 基準 commit 與目前來源 diff：`5f26c932981319627e3b76a3e8d47a5ae741c05e` 相對 `learning/online.go` 的 21 行新增、3 行刪除，範圍只在私人 mixed-stage state。
- 基準 `learning/online.go` SHA-256：`e4b67dbdb037e8ca4aebbb5ae6a4f6a8ecb5c0ff9ebbc7ab768e491470893e7d`。
- 目前 `learning/online.go` SHA-256：`2f742e1c266e018d4964bfb7b92b6eabaa543fb7c4043108ee51dc51ba4dae25`。
- 凍結測試 SHA-256：`online_mixed_retry_test.go` `a166d225bf472ee9c440bccb785feb7c900ad2ca501074bbadd658b8e389ef15`；`online_mixed_retry_extra_test.go` `9c814874e018ad44bc2690a5f29fad2f48801ee95e33d4a5d74d919ac77779eb`；`online_mixed_retry_lifecycle_test.go` `939e0378c9058c9710e598265b77de5dc7f37d2154607243b8a51845f332a335`。
- 契約來源 SHA-256：`ENG.md` `7d495442e2d578fe3ccfda6068ff69e4dcb8970ce7e0f00d1cfc6a8890a06efe`；`mixed-retry-skeleton.md` `e2212494d32687028631fa24dde3a94ca98520fa6a2b241d1d34dd27d6395e20`；`docs/tickets/85-online-mixed-phase-retry.md` `e16513c3f689a767351ff68343f83e59192e88286e6ba76e063c9f59ff84cd86`。

## Notes

本報告是來源級審查，不是 runtime 綠燈證明。既有工作樹的其他 Root 變更與三份測試未被本 agent 修改；報告只對上述 `learning/online.go` diff 做結論。

## Next

Root 依指定 exact command 取得目前來源 runtime 結果，再把 runtime 證據與本報告的靜態結論合併判定。

Skills：diff-inspector、investigate
