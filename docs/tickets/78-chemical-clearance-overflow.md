# Ticket 78：研究者可以拒絕溢位的化學清除速率

**Epic:** 化學濃度與有限梯度
**User Story:** Step 與 Backward 一致拒絕不可表示的清除速率，避免成功回傳錯誤的零濃度與零梯度
**Blocked by:** 無
**Status:** completed

## Root 決策與驗收（2026-10-13）

沿用既有有限值與失敗原子性契約，不改合法運算或保存格式。骨架、檔案責任與完整流程見 [階段計畫](../../evidence/MOD-02/chemical-boundaries-20261013/plan.md)。

- [x] 有限 Tau、release、boost 可重現 rate + boost 溢位的 RED。
- [x] Step 與 Backward 一致回錯，原始狀態及輸入不變。
- [x] 合法零 boost、有限高值與傳輸保持原濃度及梯度。
- [x] 合法基準與修正版公開輸出逐位相同。
- [x] 完整工程驗證、證據、清理、提交及遠端讀回完成。

減法審查：補既有驗證缺口，不建立新的運算分支、通道抽象或自動清除狀態。

驗收與交付：[verification.json](../../evidence/MOD-02/chemical-boundaries-20261013/verification.json)、[cleanup.json](../../evidence/MOD-02/chemical-boundaries-20261013/cleanup.json)、[delivery.json](../../evidence/MOD-02/chemical-boundaries-20261013/delivery.json)。
