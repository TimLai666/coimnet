# 128 — 研究者可以匯出有限介入差異的報告

**Epic:** 介入報告的數值可靠性
**User Story:** 合法有限狀態產生可表示的差異時，研究者能取得有限 L2 數值並匯出 JSON。
**Blocked by:** 127
**Status:** ready

- [ ] 既有公開重現、上下溢位及真正不可表示結果的契約核對完成。
- [ ] 最小修正與獨立數值／原子性控制通過。
- [ ] 完整工程、舊正常行為及需求證據通過。
- [ ] 證據保存、暫存清理、提交與遠端讀回完成。

## Root 決策｜2026-10-16

ticket127 的正式來源已凍結且完整競態測試仍在執行，未擴大其實作。只補了一個隔離的公開 probe，確認 LIF + STDP 合法狀態在強制一次事件後 eligibility=5e307，真實 L2 可表示為 5e307，`l2Diff` 卻先平方造成 +Inf，`json.Marshal(InterventionLog)` 因此失敗。第一個 fixture 的 decay_pre=1 被既有 [0,1) 驗證正確拒絕，合法重現改為 .5，原始錯誤與有效輸入均保留。

問題位於 `learning/intervene.go` 的差異範數 helper，而非核心更新。後續先核對四個使用者、平方上溢／下溢與真正不可表示 L2 的失敗政策，再決定完整修正和提交順序。不得把部分數值修正當成整體零 bug。COR-11 在本票驗收前保持 specified。

重現、指紋與輸出見[下一項稽核](../../evidence/COR-11/frozen-intervention-20261016/next-audit.md)。

另已確認平方下溢：Continuous／Hebbian 控制的 eligibility=2.5e-201，介入後為零，但 PlasticDelta 回報 0。見同一份下一項稽核與 next-small-norm-probe 的命令回條。
