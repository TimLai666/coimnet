# Ticket 80：研究者可以安全更換仍有讀出調節的受體設定

**Epic:** 化學狀態與快照接續
**User Story:** EnableChemistry 不得提交讓既有 ExpressionGain 指向不存在受體的宣告
**Blocked by:** 無
**Status:** completed

## Root 決策與驗收（2026-10-13）

沿用既有有限值與失敗原子性契約，不改合法運算或保存格式。骨架、檔案責任與完整流程見 [階段計畫](../../evidence/MOD-02/chemical-boundaries-20261013/plan.md)。

- [x] 公開設定可重現更換後的無效 ExpressionGain 與還原失敗。
- [x] 不相容更換回錯，完整舊快照及報告保持。
- [x] 拒絕後推論與獨立訓練、還原接續和 untouched twin 相同。
- [x] 合法更換及明確 ClearExpressionGain 後移除受體成功。
- [x] 完整工程驗證、證據、清理、提交及遠端讀回完成。

減法審查：補既有驗證缺口，不建立新的運算分支、通道抽象或自動清除狀態。

驗收與交付：[verification.json](../../evidence/MOD-02/chemical-boundaries-20261013/verification.json)、[cleanup.json](../../evidence/MOD-02/chemical-boundaries-20261013/cleanup.json)、[delivery.json](../../evidence/MOD-02/chemical-boundaries-20261013/delivery.json)。
