# Ticket 68：研究者可以在重設最佳化器後繼續鞏固新經驗

**Epic:** 持續記憶的生命週期
**User Story:** 重設最佳化器保留慢速記憶及預算，新回合能鞏固，當下已寫入的回合不能重複寫入
**Blocked by:** 無
**Status:** in_progress

## Root 決策（2026-10-13）與驗收

ResetOptimizer 沿用既有 Episodes 歸零。慢速 Values／Used／Budget 保持，LastEpisode 轉成新時鐘中等價的已寫入狀態：原當下回合已鞏固則為 1，尚未鞏固則為 0。後續成功 TrainEpisode 才建立新回合，不靠重設重複消耗同一經驗。舊快照若已帶有較大的 LastEpisode，載入時保留，明確重設時保守保留當下已寫入狀態，見 ticket 73。

- [x] 公開回歸先重現新回合錯誤被當成重複鞏固。
- [x] 已寫入／未寫入重設、後續第一次／重複鞏固與保存恢復通過。
- [x] 非法／取消重設保留完整狀態，記憶與預算不被清除。
- [ ] 完整工程驗證、證據、清理與交付完成。

減法審查：沿用現有時鐘與標記，不新增快照欄位或第二份 episode 計數。

驗證證據：[機制稽核](../../evidence/MOD-02/mechanism-audit-20261013/README.md)與[完整檢查](../../evidence/MOD-02/mechanism-audit-20261013/verification.json)。最後交付項待本輪清理、推送與遠端讀回。
