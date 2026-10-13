# Ticket 67：研究者可以安全取得持續化學推進的有限報告

**Epic:** 持續個體的機制正確性
**User Story:** 合法逐步釋放的累計若無法表示，整次推進回錯且不提交部分狀態
**Blocked by:** 無
**Status:** done

## Root 決策（2026-10-13）與驗收

沿用既有 Advance／AdvanceGated 的候選計算與共同提交。化學濃度有限不保證報告加總有限。累計溢位須在候選內拒絕，保留神經、化學、來源記憶、可塑性及上一份報告。合法有限值及保存格式保持，不截斷總量或改成平均值。

- [x] 公開入口先失敗測試重現兩列有限釋放的報告溢位，確認失敗不提交。
- [x] Advance／AdvanceGated 共用修正，兩種核心、來源記憶與可塑性狀態均受保護。
- [x] 有限邊界、每次呼叫重新累計、凍結化學及 JSON 報告通過。
- [x] gofmt、build、完整一般與 race、vet、相依性及相關治理通過。
- [x] Root 審查、證據保存、暫存清理、提交推送與遠端核對完成。

## 減法審查

沿用既有候選與提交流程補一處數值檢查，不新增報告格式、儲存層或化學機制。全面機制稽核另保存已確認、待重現及證據限制，有限值回歸不當作整個框架無 bug 或模型品質改善的證明。

驗證證據：[機制稽核](../../evidence/MOD-02/mechanism-audit-20261013/README.md)與[完整檢查](../../evidence/MOD-02/mechanism-audit-20261013/verification.json)。最後交付項已完成，暫存清理及提交 `768ab10`／`354b2be` 的推送與遠端核對見[清理回條](../../evidence/MOD-02/mechanism-audit-20261013/cleanup.json)與[交付回條](../../evidence/MOD-02/mechanism-audit-20261013/delivery.json)。
