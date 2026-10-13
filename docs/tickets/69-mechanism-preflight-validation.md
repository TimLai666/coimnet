# Ticket 69：研究者可以在建模前拒絕不合法的機制宣告

**Epic:** 可選機制的設定正確性
**User Story:** 不支援的向量固定符號及來源通道錯位在建構時回錯，不等第一筆輸入才失敗
**Blocked by:** 無
**Status:** in_progress

## Root 決策（2026-10-13）與驗收

NewNetwork 沿用 NewTrainer 的既有向量固定符號限制，向量全零符號與未宣告符號保持合法。SourceSpec 的 timeline／replay 只能在宣告 Channel 有非零釋放，其他通道為零；Standalone 多通道來源保持可用。

- [x] Root 審查並執行公開 RED 測試，非零向量符號與兩種錯位來源均重現。
- [x] 公開建構入口在運算前回錯，合法向量、純量與來源宣告通過。
- [ ] 完整工程驗證、證據、清理與交付完成。

減法審查：把向量共用檢查移到 NewNetwork，移除 Trainer 的重複判斷。沿用 SourceSpec 驗證，不增加來源格式。

驗證證據：[機制稽核](../../evidence/MOD-02/mechanism-audit-20261013/README.md)與[完整檢查](../../evidence/MOD-02/mechanism-audit-20261013/verification.json)。最後交付項待本輪清理、推送與遠端讀回。
