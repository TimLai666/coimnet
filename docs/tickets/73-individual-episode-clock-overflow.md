# Ticket 73：訓練與鞏固回合時鐘不回繞

**Epic:** 持續記憶的生命週期
**User Story:** 從合法最大計數快照恢復後，不會把新回合誤當舊回合或重複鞏固同一回合
**Blocked by:** 無
**Status:** done

## Root 決策（2026-10-13）

保留 uint64 欄位及舊快照的接受規則。TrainEpisode 在計數已最大時，先回錯而不更新模型；Consolidate 在無法表示 episodes+1 時，先回錯而不消耗預算。ResetOptimizer 仍是明確建立新最佳化器生命週期的入口。舊版重設可能留下大於目前回合的鞏固標記，不追溯拒絕這些模型；明確重設保留當下寫入禁令並重新對齊標記，下一個訓練回合才可鞏固。

- [x] 公開快照恢復的 RED 證明 TrainEpisode 回繞及 Consolidate 可重複寫入。
- [x] 修正後在最大值拒絕，完整快照保持，最大值前一回合與明確重設可執行。
- [x] 舊標記快照仍可載入，重設保留預算／慢速記憶，下一回合可鞏固。
- [x] 完整工程驗證、證據、清理與交付完成。

減法審查：不新增時鐘、保存版本或追溯資料遷移。

驗證證據：[機制稽核](../../evidence/MOD-02/mechanism-audit-20261013/README.md)與[完整檢查](../../evidence/MOD-02/mechanism-audit-20261013/verification.json)。最後交付項已完成，暫存清理及提交 `768ab10`／`354b2be` 的推送與遠端核對見[清理回條](../../evidence/MOD-02/mechanism-audit-20261013/cleanup.json)與[交付回條](../../evidence/MOD-02/mechanism-audit-20261013/delivery.json)。
