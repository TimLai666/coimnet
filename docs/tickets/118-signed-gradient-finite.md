# 118 — 固定正負號權重的梯度不能帶著無限大回報成功

**Epic:** 共用梯度數值稽核
**User Story:** 使用者直接取得模型梯度時，數值溢位會收到錯誤與零結果，避免把無效值交給後續更新。
**Blocked by:** 117（已交付）
**Status:** completed

- [x] 公開入口與直接核心參考重現，核對真正產生端。
- [x] 獨立失敗回歸經 Root 審查凍結，完成最小修正與有限值控制。
- [x] 完整工程、相依性、正常／恢復與需求證據驗收。
- [x] 清除自有暫存，完成提交、推送與遠端讀回。

## Root 決策｜2026-10-16

一般／競態都確認三組公開梯度回傳 Inf 而錯誤為 nil，直接核心梯度全數有限，另兩組有限值控制保持。共用 `learning/network.go` 在固定符號權重的梯度換算後缺少有限值檢查。COR-08 暫標 failed，舊證據保持。訓練器對啟用中的無效梯度已有後續攔截，本票修正公開梯度產生端。範圍、測試／實作分工與驗收見[計畫](../../evidence/COR-08/signed-gradient-overflow-20261016/plan.md)及[重現](../../evidence/COR-08/signed-gradient-overflow-20261016/reproduction-verification.json)。沒有變更保存格式或模型參數限制。

## Root 驗收｜2026-10-16

一般／競態各 58 套件通過，6,455／6,454 pass、22／23 skip、零失敗。原有各 6,405 個終端事件保持，只新增 72 個通過事件。格式、build、vet、固定 Insyra v0.3.4、模組校驗及 31 項治理通過。834 份正式來源及凍結測試指紋保持，正常核心、舊神經狀態與舊化學模型接續的 32／32／12 組結果保持。COR-08 已恢復 passed，最新 91／91 工程需求 passed。見[驗收](../../evidence/COR-08/signed-gradient-overflow-20261016/verification.json)。最後 31 項治理通過，本輪 1,726,991,821 bytes 暫存已清除，原模型、資料、原始模組與其他工作保持。修正 `29d611f` 與回歸／證據 `90cceb0` 已正常推送且遠端讀回相同，834 份來源與 96 個交付檔案的提交內容相符，見[清理](../../evidence/COR-08/signed-gradient-overflow-20261016/cleanup.json)及[交付](../../evidence/COR-08/signed-gradient-overflow-20261016/delivery.json)。本票完成 4／4，沒有新增模型品質或生物校準結論。
