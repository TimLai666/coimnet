# Ticket 71：使用者可以在語音前處理時拒絕無效數值

**Epic:** ASR 輸入契約
**User Story:** 公開 STFT／LogMel 成功時只回傳有限特徵，無效輸入或無法表示的計算回錯
**Blocked by:** 無
**Status:** in_progress

## Root 決策（2026-10-13）與驗收

保留既有視窗、取樣率與分塊語意。Floor 必須有限且大於零。非有限音訊在前處理拒絕，頻譜／mel 計算無法保持有限時不回部分結果，正常結果逐位保持。

- [x] 非有限樣本及有限大值的 RED 回歸重現。
- [x] STFT／LogMel 回錯且結果為 nil，合法輸入與既有整檔／串流測試通過。
- [ ] 完整工程驗證、證據、清理與交付完成。

減法審查：共用 STFT 的輸入檢查，LogMel 不重複掃描音訊，不新增前處理版本。

驗證證據：[機制稽核](../../evidence/MOD-02/mechanism-audit-20261013/README.md)與[完整檢查](../../evidence/MOD-02/mechanism-audit-20261013/verification.json)。最後交付項待本輪清理、推送與遠端讀回。
