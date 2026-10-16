# 129 — 研究者可以拒絕真正不可表示的記憶寫入報告

**Epic:** 記憶寫入的數值可靠性
**User Story:** 報告幅度無法用 float64 表示時，研究者會收到錯誤，且記憶狀態與寫入額度保持。
**Blocked by:** 128 完成交付
**Status:** specified

- [ ] 核對三種範數、最高端捨入與既有錯誤順序，凍結獨立失敗回歸。
- [ ] 最小修正、正常位元與原子性控制通過。
- [ ] 完整工程、既有模型相容性與需求證據通過。
- [ ] 證據保存、暫存清理、提交與遠端讀回完成。

## Root 決策｜2026-10-16

ticket128 第一候選 v1 完整工程期間的 843 份來源保持不變。本次只在專案暫存中的公開 probe 查證相同數值問題是否存在於 `Consolidate`，沒有更動其實作。

`learning/consolidation.go:244-249` 的 BaseL2 helper 與 `:161-162` 的 SlowL2／PlasticL2 累加皆使用逐項 `math.Hypot`。合法值 `[MaxFloat64, MaxFloat64*2^-26]` 的真實範數超過最近偶數捨入為無限大的界線，8192-bit 獨立參考為 +Inf。三個報告位置卻都回傳 MaxFloat64、Applied=true、Used=1，正常有限控制與明顯溢位拒絕控制則正確。一般與競態各九個公開案例一致。

嚴重度 P2，數值極端值的報告與寫入拒絕缺口，沒有一般辨識率下降的證據。MOD-06 在本票驗收前回到 specified。沿用既有正常動態、模型格式、鞏固公式與拒絕順序，後續先補失敗測試，再修正三種範數計算。BaseL2 的 raw rho／有效幅度單位是另一個未定義問題，這個 fixture 沒有固定符號，不涉及該疑慮。

證據見[公開驗證](../../evidence/COR-11/finite-intervention-norm-20261016/next-audit-verification.json)。本票尚未開始實作，不將 ticket128 的通過結果當作本票驗收。

最終 v2 的 844 份來源中，consolidation.go 與上述公開重現版本逐位相同，Snapshot 抽出前後的複製內容亦相同，見[來源核對](../../evidence/COR-11/finite-intervention-norm-20261016/next-audit-v2-source-parity.json)。此核對不是 ticket129 的修正驗收。
