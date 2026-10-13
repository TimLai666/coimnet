# Ticket 83：線上回報只計實際套用的梯度更新

**Epic:** 機制與訓練狀態一致性
**User Story:** 累積中間步不可冒充模型已更新
**Blocked by:** 無
**Status:** completed

## Root 決策與驗收（2026-10-14）

沿用既有線上分數、共享參數與累積訓練契約，不增加公開欄位或更換保存格式。骨架及獨立測試責任見 [階段計畫](../../evidence/MOD-02/chemical-boundaries-20261013/online-sharing-plan.md)。

- [x] 普通／化學個體 AccumulateSteps 2／3 的錯誤回報有 RED。
- [x] 前 k-1 次 GradientSteps 為0，optimizer update 不變。
- [x] 第 k 次回報1且實際更新，累積狀態清空。
- [x] AccumulateSteps 1控制及 partial restore 後接續均通過。
- [x] 完整工程驗證、證據、清理、提交及遠端讀回完成。

減法審查：重用化學 episode 建構與共享索引，直接補正既有回報與間隔提交，不增加平行訓練路徑或保存欄位。

驗收與交付：[verification.json](../../evidence/MOD-02/chemical-boundaries-20261013/verification.json)、[cleanup.json](../../evidence/MOD-02/chemical-boundaries-20261013/cleanup.json)、[delivery.json](../../evidence/MOD-02/chemical-boundaries-20261013/delivery.json)。
