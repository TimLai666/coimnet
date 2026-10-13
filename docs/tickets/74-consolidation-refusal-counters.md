# Ticket 74：鞏固拒絕報告保留既有計數

**Epic:** 機制契約稽核
**User Story:** 使用者可從被拒絕的鞏固報告判斷目前寫入次數與最近寫入回合
**Blocked by:** 無
**Status:** done

## Root 決策與驗收（2026-10-13）

`Individual.Consolidate(ctx, trigger)` 的三種正常拒絕目前未填入 `Used` 與 `LastEpisode`，與 `ConsolidationReport` 的公開說明不符。只在判斷拒絕前填入既有計數；成功時仍回傳更新後的計數。權重、預算、判斷順序與保存格式保持。

- [x] 獨立測試重現 gate closed、already consolidated、budget exhausted 三種報告漏計數。
- [x] 修正後三種拒絕保留非零計數、正確旗標與錯誤，完整快照不變；成功仍回更新後計數。
- [x] 完整工程驗證、證據保存、暫存清理與提交交付完成。

Root 修改 `learning/consolidation.go`；測試執行者只新增 `learning/consolidation_report_test.go`。驗收由 Root 執行。

減法審查：沿用既有報告欄位，不增加新報告或狀態。
