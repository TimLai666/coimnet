# Ticket 84：失敗回饋重試不推進更新間隔

**Epic:** 機制與訓練狀態一致性
**User Story:** UpdateEvery 不得讓失敗回饋在重試時變成 skip 被消耗
**Blocked by:** 無
**Status:** completed

## Root 決策與驗收（2026-10-14）

沿用既有線上分數、共享參數與累積訓練契約，不增加公開欄位或更換保存格式。骨架及獨立測試責任見 [階段計畫](../../evidence/MOD-02/chemical-boundaries-20261013/online-sharing-plan.md)。

- [x] 公開合法 exhausted episode 個體重現第二次重試靜默消耗。
- [x] 連續三次重試保持同一錯誤，Consumed／GradientSteps為0。
- [x] 完整個體快照在拒絕後保持。
- [x] UpdateEvery 1／2 正常 skip／套用控制保持。
- [x] 完整工程驗證、證據、清理、提交及遠端讀回完成。

減法審查：重用化學 episode 建構與共享索引，直接補正既有回報與間隔提交，不增加平行訓練路徑或保存欄位。

驗收與交付：[verification.json](../../evidence/MOD-02/chemical-boundaries-20261013/verification.json)、[cleanup.json](../../evidence/MOD-02/chemical-boundaries-20261013/cleanup.json)、[delivery.json](../../evidence/MOD-02/chemical-boundaries-20261013/delivery.json)。
