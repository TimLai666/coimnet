# Ticket 70：使用者可以在 OCR 輸入時拒絕超界像素

**Epic:** OCR 輸入契約
**User Story:** 手動建立的 Image 和檔案讀入的圖片遵循同一個 [0,1] 墨色範圍
**Blocked by:** 無
**Status:** done

## Root 決策（2026-10-13）與驗收

所有 OCR 圖片入口拒絕非有限值及 [0,1] 以外的像素，不靜默截斷資料。合法像素、舊輸入模式、已存模型及計分算式保持。

- [x] 超界／非有限像素經 Inputs 三模式、Columns、SegmentLines、TrainLine 的 RED 測試重現。
- [x] 修正後回錯且不改訓練狀態，0／1 邊界合法。
- [x] 完整工程驗證、證據、清理與交付完成。

減法審查：沿用既有形狀與錯誤優先順序，只擴充數值驗證。

驗證證據：[機制稽核](../../evidence/MOD-02/mechanism-audit-20261013/README.md)與[完整檢查](../../evidence/MOD-02/mechanism-audit-20261013/verification.json)。最後交付項已完成，暫存清理及提交 `768ab10`／`354b2be` 的推送與遠端核對見[清理回條](../../evidence/MOD-02/mechanism-audit-20261013/cleanup.json)與[交付回條](../../evidence/MOD-02/mechanism-audit-20261013/delivery.json)。
