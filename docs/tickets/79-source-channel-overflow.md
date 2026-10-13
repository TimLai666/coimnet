# Ticket 79：研究者可以安全拒絕無法形成通道寬度的來源

**Epic:** 公開化學來源
**User Story:** NeuralActivity、InternalResource 與 SourceSpec.Build 回錯，避免通道編號加一溢位造成 panic
**Blocked by:** 無
**Status:** verifying

## Root 決策與驗收（2026-10-13）

沿用既有有限值與失敗原子性契約，不改合法運算或保存格式。骨架、檔案責任與完整流程見 [階段計畫](../../evidence/MOD-02/chemical-boundaries-20261013/plan.md)。

- [x] 兩種 Release 與兩種 SourceSpec.Build 有 MaxInt 通道 RED。
- [x] 所有入口在配置前回錯，沒有 panic 或負寬度成功結果。
- [x] 零、一般正通道及零釋放依舊正確。
- [x] 合法基準與修正版公開釋放逐位相同。
- [ ] 完整工程驗證、證據、清理、提交及遠端讀回完成。

減法審查：補既有驗證缺口，不建立新的運算分支、通道抽象或自動清除狀態。
