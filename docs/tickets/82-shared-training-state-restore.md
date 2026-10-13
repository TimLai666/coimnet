# Ticket 82：共享訓練狀態安全恢復

**Epic:** 機制與訓練狀態一致性
**User Story:** Restore 不接受讓共享參數下次更新分歧的 optimizer 或 partial accumulator
**Blocked by:** 無
**Status:** completed

## Root 決策與驗收（2026-10-14）

沿用既有線上分數、共享參數與累積訓練契約，不增加公開欄位或更換保存格式。骨架及獨立測試責任見 [階段計畫](../../evidence/MOD-02/chemical-boundaries-20261013/online-sharing-plan.md)。

- [x] 合法訓練快照突變 First／Second／Steps／Sum 有公開 RED。
- [x] Trainer／Individual 恢復在提交前拒絕，原快照不變。
- [x] 正常完整／partial window 恢復後多步與原 twin 逐位相同。
- [x] weights／bias／log_tau 群組保持，singleton／未共享不同值可接受。
- [x] 完整工程驗證、證據、清理、提交及遠端讀回完成。

減法審查：重用化學 episode 建構與共享索引，直接補正既有回報與間隔提交，不增加平行訓練路徑或保存欄位。

驗收與交付：[verification.json](../../evidence/MOD-02/chemical-boundaries-20261013/verification.json)、[cleanup.json](../../evidence/MOD-02/chemical-boundaries-20261013/cleanup.json)、[delivery.json](../../evidence/MOD-02/chemical-boundaries-20261013/delivery.json)。
