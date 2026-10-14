# 96 — 研究者可以核對多區域記憶讀出調節的完整訓練梯度

**Epic:** 機制與模型品質稽核
**User Story:** 研究者可以確認記憶讀出增益對多個細胞、不同受體區域與活動來源的梯度符合宣告機制。
**Blocked by:** 88、89（皆已完成）
**Status:** completed

固定範圍及 Root 骨架見[計畫](../../evidence/MOD-06/expression-gradient-audit-20261014/plan.md)。原始完整零 bug 目標保持，本票不是整個框架的完成條件。

- [x] Root 與獨立審查核對中性值、區域平均、讀出種子、來源回授與夾限契約。
- [x] 用非零控制、不同區域與刻意錯誤的參考驗證公共梯度，保存實際差異或一致結果。
- [x] 確認缺陷先建立失敗測試再修正，完成相稱驗證，區分未確認疑點及模型品質證據。
- [x] 同步需求證據、清除本輪暫存，完成已授權提交／推送與遠端讀回。

## Root 決策 2026-10-14

160 組獨立參考一般與 race 通過，80 組有實際不同區域的佔用率。四種錯誤參考各自出現預期的數值失敗，原有化學回歸一般／race 各 100 個事件通過，build、vet 與相依性通過。正式 806 份來源及既有測試不變，前次同來源完整 58 套件一般／race 證據已核對，沒有宣稱本票重跑全套。沒有新確認缺陷。先前 mixed-status 疑慮在公開 producer 不可達，已撤回。詳見[驗證](../../evidence/MOD-06/expression-gradient-audit-20261014/verification.json)及[Root 審查](../../evidence/MOD-06/expression-gradient-audit-20261014/root-review.md)。

本票已完成 4／4。891,799,195 bytes 本輪暫存已清除，原有資料與暫存保留。提交 `4b1e209` 已正常推送並核對遠端，806 份提交來源與凍結指紋相符，見[清理](../../evidence/MOD-06/expression-gradient-audit-20261014/cleanup.json)與[交付](../../evidence/MOD-06/expression-gradient-audit-20261014/delivery.json)。小圖驗證不證明真實品質提升、生物校準或完整零 bug。
