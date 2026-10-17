# 135 — 化學評估重設保留任務輸入

**Epic:** 評估與機制一致性
**User Story:** 使用者可以在每題重設化學狀態，保留任務已宣告的資源與排隊回饋。
**Blocked by:** 無，ticket134 已完成
**Status:** in_progress

## Root 決策｜2026-10-17

公開 RunAdaptiveEvaluation 已重現 P1 缺陷。啟用內在資源來源並宣告 energy=0.75 後，不重設時評估成功。ChemicalAtItemStart=true 卻移除資源與排隊回饋，回傳 resource is not declared。受體閘門控制因 DisableChemistry 拒絕卸載而意外成功。原始日誌與來源指紋見[重現](../../evidence/LRN-10/chemical-evaluation-reset-20261017/reproduction.json)。

根因是共用 resetForItem 先卸載整個化學模組，忽略卸載錯誤，再重新啟用。既有 EnableChemistry 已提供重設濃度與來源記憶並保留任務輸入的契約，移除前置卸載即可沿用它。不新增 API、保存格式、學習規則或整次評估回滾保證。慢速記憶與凍結旗標的重設政策另行調查，不混入本票。

## 驗收

- [x] 以公開 SDK 重現資源遺失、錯誤及受體依賴差異。
- [x] 固定先失敗的公開回歸，最小修正通過一般與競態測試，濃度確實重設，沿用既有來源初始化。
- [x] 正常與舊模型控制、完整工程與需求證據通過。
- [ ] 保存必要證據、清除本輪暫存、提交及遠端讀回完成。

驗收範圍與測試責任見[骨架](../../evidence/LRN-10/chemical-evaluation-reset-20261017/fix-skeleton.md)。軟體回歸不等於真實任務辨識率或生物機制證明。
