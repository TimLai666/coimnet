# 129 — 研究者可以拒絕真正不可表示的記憶寫入報告

**Epic:** 記憶寫入的數值可靠性
**User Story:** 報告幅度無法用 float64 表示時，研究者會收到錯誤，且記憶狀態與寫入額度保持。
**Blocked by:** 128 完成交付
**Status:** completed

- [x] 核對三種範數、最高端捨入與既有錯誤順序，凍結獨立失敗回歸。
- [x] 最小修正、正常位元與原子性控制通過。
- [x] 完整工程、既有模型相容性與需求證據通過。
- [x] 證據保存、暫存清理、提交與遠端讀回完成。

## Root 決策｜2026-10-16

ticket128 第一候選 v1 完整工程期間的 843 份來源保持不變。本次只在專案暫存中的公開 probe 查證相同數值問題是否存在於 `Consolidate`，沒有更動其實作。

`learning/consolidation.go:244-249` 的 BaseL2 helper 與 `:161-162` 的 SlowL2／PlasticL2 累加皆使用逐項 `math.Hypot`。合法值 `[MaxFloat64, MaxFloat64*2^-26]` 的真實範數超過最近偶數捨入為無限大的界線，8192-bit 獨立參考為 +Inf。三個報告位置卻都回傳 MaxFloat64、Applied=true、Used=1，正常有限控制與明顯溢位拒絕控制則正確。一般與競態各九個公開案例一致。

嚴重度 P2，數值極端值的報告與寫入拒絕缺口，沒有一般辨識率下降的證據。MOD-06 在本票驗收前回到 specified。沿用既有正常動態、模型格式、鞏固公式與拒絕順序，後續先補失敗測試，再修正三種範數計算。BaseL2 的 raw rho／有效幅度單位是另一個未定義問題，這個 fixture 沒有固定符號，不涉及該疑慮。

證據見[公開驗證](../../evidence/COR-11/finite-intervention-norm-20261016/next-audit-verification.json)。本票尚未開始實作，不將 ticket128 的通過結果當作本票驗收。

最終 v2 的 844 份來源中，consolidation.go 與上述公開重現版本逐位相同，Snapshot 抽出前後的複製內容亦相同，見[來源核對](../../evidence/COR-11/finite-intervention-norm-20261016/next-audit-v2-source-parity.json)。此核對不是 ticket129 的修正驗收。

本輪已接手，ticket128 的最終 844 份來源與交付已重新核對。Root 已固定範圍、正常位元、最高端參考及失敗原狀判準，測試與唯讀契約分工進行中，正式實作尚未改動。本票 0／4，MOD-06 保持 specified。見[本票計畫](../../evidence/MOD-06/consolidation-norm-20261016/plan.md)。

RED 已由 Root 執行並核對：129 個終端事件，74 pass、55 fail，失敗均在最高端邊界。一般位元與前置拒絕控制通過，公開重播另確認三種層都可能誤拒合法值或誤接受不可表示值。原有 844 份來源保持，唯一新測試已凍結，見[RED 驗證](../../evidence/MOD-06/consolidation-norm-20261016/red-verification.json)。本票 1／4，實作由獨立 worker 接續，MOD-06 保持 specified。

三種報告共用少見最高端的精確補正，正常路徑沿用逐項 Hypot。一般／競態各129個新專項與224個相關事件通過，兩種隔離錯誤均被拒絕，公開各24案例一致。845份來源凍結，完整工程驗收進行中。本票2／4，MOD-06保持specified，見[診斷驗證](../../evidence/MOD-06/consolidation-norm-20261016/diagnostic-verification.json)。

845份來源的完整一般／競態各58套件通過，8,233／8,232 pass、23／24 skip，原有各8,127個終端事件保持，只新增各129 pass。格式、build、vet、固定Insyra v0.3.4、模組及治理通過，MOD-06恢復passed，工程追蹤90／91，LRN-06由ticket130接續。本票3／4，待清理與交付，見[驗收](../../evidence/MOD-06/consolidation-norm-20261016/verification.json)。

本票完成4／4。1,882,889,920 bytes自有暫存已清除，18份原模型、六份資料、原始模組與其他工作保持，見[清理](../../evidence/MOD-06/consolidation-norm-20261016/cleanup.json)。主要提交`0e08198`已正常推送且遠端讀回相同，845份提交來源核對完成，見[交付](../../evidence/MOD-06/consolidation-norm-20261016/delivery.json)。整體零bug與任務品質改善沒有由本票證明。
