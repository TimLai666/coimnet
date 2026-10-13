# Ticket 81：線上回饋使用同一化學 episode 的預測

**Epic:** 機制與訓練狀態一致性
**User Story:** 化學／expression 開啟時，分數零與半權重不得被普通預測帶偏
**Blocked by:** 無
**Status:** completed

## Root 決策與驗收（2026-10-14）

沿用既有線上分數、共享參數與累積訓練契約，不增加公開欄位或更換保存格式。骨架及獨立測試責任見 [階段計畫](../../evidence/MOD-02/chemical-boundaries-20261013/online-sharing-plan.md)。

- [x] 公開 continuous／LIF 的錯誤加權先重現失敗。
- [x] 同參數普通／化學及 expression 控制確實產生不同預測。
- [x] score 0、.5、1、-1、2 與獨立明示 episode twin 相同。
- [x] 凍結化學、來源視窗與回報保持，普通路徑合法控制保持。
- [x] 完整工程驗證、證據、清理、提交及遠端讀回完成。

減法審查：重用化學 episode 建構與共享索引，直接補正既有回報與間隔提交，不增加平行訓練路徑或保存欄位。

驗收與交付：[verification.json](../../evidence/MOD-02/chemical-boundaries-20261013/verification.json)、[cleanup.json](../../evidence/MOD-02/chemical-boundaries-20261013/cleanup.json)、[delivery.json](../../evidence/MOD-02/chemical-boundaries-20261013/delivery.json)。
