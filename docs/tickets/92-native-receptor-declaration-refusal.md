# 92 — 原生模擬拒絕尚未綁定的受體學習宣告

**Epic:** 機制與模型品質稽核
**User Story:** 研究者不會得到默默忽略受體設定的原生模擬結果，正常的刺激閘門與固定衰退可以繼續使用。
**Blocked by:** 無，不依賴 ticket 89 增益決策
**Status:** in_progress

- [x] Root 固定原生 Protocol 的受體引用拒絕與現有正常公式契約。
- [x] 公開失敗回歸先凍結，實測修正前失敗、修正後拒絕。
- [x] 連續／LIF、gate／window／both、合法缺省與完整工程驗證，正常 Protocol／模型結果相同。
- [x] 同步 MOD-05 與證據，清除本輪暫存並保留原件。
- [ ] 提交、推送及遠端讀回，提交來源與驗證指紋相符。

Root 2026-10-14 決策：使用原生入口既有「閘門來自刺激通道」的宣告，在沒有化學／佔用率綁定的路徑拒絕兩個受體引用。沿用 Framework 的可選化學機制，不新增原生化學實作、不改正常 local learning 算式與模型格式。舊的未綁定宣告本來未生效，拒絕時須提供清楚錯誤；合法無受體欄位的 Protocol 保持。

Root 六個公開 Validate／Build／Run 案例接受不存在的受體 100，gate／window／both 對連續及 LIF 的局部變化與控制完全相同。診斷預期 bug 並非修正通過，見[原始診斷](../../evidence/MOD-05/receptor-rule-ownership-20261014/native-receptor-audit.json)。學習時間點公式的文件歧義另列 Follow-ups，不由本票決定。
