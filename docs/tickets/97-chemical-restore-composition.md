# 97 — 研究者可以核對化學來源狀態恢復後的接續及拒絕原子性

**Epic:** 機制與模型品質稽核
**User Story:** 研究者可以確認個體保存後的神經、化學與來源活動歷史按宣告規則接續。
**Blocked by:** 91、96（皆已完成）
**Status:** done

Root 骨架見[計畫](../../evidence/STA-03/chemical-restore-audit-20261014/plan.md)。本票的四項里程碑不是完整零 bug 目標的分母。

- [x] 核對來源狀態、不同時鐘、快照所有權與失敗原子性契約。
- [x] 公開入口與新程序保存恢復有非零對照，壞狀態拒絕且原個體不變。
- [x] 只有確認缺陷才以失敗測試及最小修正處理，完成相稱驗證。
- [x] 保存證據、清除本輪暫存及完成已授權提交與遠端核對。

## Root 決策 2026-10-14

ResetNeural 保留化學濃度與來源視窗。來源時間使用新的 neural step，化學更新計數各自保留。FreezeChemistry 是不保存的暫時旗標，恢復後解除。bundle 在正規化前沿用既有原始欄位檢查，正常模型、版本及數值算式保持。

## 驗收與交付

完整一般／race 各 58 套件與其餘工程通過，11 項治理、809 份提交來源、合法舊保存物及六個既有模型的接續已核對。2,122,375,284 bytes 本輪暫存已清除，原有資料及模型保持。修正 `0f6c87d` 與回歸／證據 `6e811e0` 已正常推送且遠端讀回相同，見[驗證](../../evidence/STA-03/chemical-restore-audit-20261014/verification.json)、[清理](../../evidence/STA-03/chemical-restore-audit-20261014/cleanup.json)及[交付](../../evidence/STA-03/chemical-restore-audit-20261014/delivery.json)。本票完成 4／4，工程需求為 91 項 passed。整體零 bug 與真實任務品質提升尚未證明。
