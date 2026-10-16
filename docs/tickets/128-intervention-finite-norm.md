# 128 — 研究者可以匯出有限介入差異的報告

**Epic:** 介入報告的數值可靠性
**User Story:** 合法有限狀態產生可表示的差異時，研究者能取得有限 L2 數值並匯出 JSON。
**Blocked by:** 127
**Status:** in_progress

- [x] 既有公開重現、上下溢位及真正不可表示結果的契約核對完成。
- [x] 最小修正與獨立數值／原子性控制通過。
- [x] 完整工程、舊正常行為及需求證據通過。
- [ ] 證據保存、暫存清理、提交與遠端讀回完成。

## Root 決策｜2026-10-16

ticket127 的正式來源已凍結且完整競態測試仍在執行，未擴大其實作。只補了一個隔離的公開 probe，確認 LIF + STDP 合法狀態在強制一次事件後 eligibility=5e307，真實 L2 可表示為 5e307，`l2Diff` 卻先平方造成 +Inf，`json.Marshal(InterventionLog)` 因此失敗。第一個 fixture 的 decay_pre=1 被既有 [0,1) 驗證正確拒絕，合法重現改為 .5，原始錯誤與有效輸入均保留。

問題位於 `learning/intervene.go` 的差異範數 helper，而非核心更新。後續先核對四個使用者、平方上溢／下溢與真正不可表示 L2 的失敗政策，再決定完整修正和提交順序。不得把部分數值修正當成整體零 bug。COR-11 在本票驗收前保持 specified。

重現、指紋與輸出見[下一項稽核](../../evidence/COR-11/frozen-intervention-20261016/next-audit.md)。

另已確認平方下溢：Continuous／Hebbian 控制的 eligibility=2.5e-201，介入後為零，但 PlasticDelta 回報 0。見同一份下一項稽核與 next-small-norm-probe 的命令回條。

本輪已接手，85 個獨立葉案例與七個父測試均成功執行，RED 為 14 pass／78 fail。正常位元控制保持，數值與失敗原狀斷言重現缺陷，正式來源尚未更動。範圍與判準見[本票計畫](../../evidence/COR-11/finite-intervention-norm-20261016/plan.md)。

第一候選 v1 的一般／競態各 92 個新專項、216 個相關事件及完整工程曾通過，但 Root 追加唯讀觀察者後確認 Snapshot 會與 rollback 寫入競爭，且可讀到最後被拒絕的候選，因此禁止交付。最初 overlay 的六案例兩種建置各七個事件全部失敗；凍結成正式第三份測試後，一般為六 fail／一 pass，競態為七 fail 並有 49 次競態警告。這是排程差異，斷言未改，見[紅燈核對](../../evidence/COR-11/finite-intervention-norm-20261016/red-readers-verification.json)。原 843 份來源與 v1 完整工程結果保留，不能當最終驗收。

最終 v2 使用同一把操作鎖保護 snapshot、twin、subject、報告、失敗還原及 hooks 清除。公開 Snapshot 的原複製內容僅抽成私有 helper，subject 沿用已持鎖的 advanceRowsLocked；既有驗證順序保持，不增加並行修改個體的保證。一般／競態各 99 個專項與 223 個相關事件通過，原 119 個事件保持。兩個公開 witness 分別正確回報 5e307 與 2.5e-201，大值 JSON 匯出成功。四種錯誤版本在兩種建置均被拒絕，18 種 preflight 控制與 v1 逐位相同，見[專項驗收](../../evidence/COR-11/finite-intervention-norm-20261016/diagnostic-v2-verification.json)。844 份正式來源的完整一般／競態各 58 套件通過，8,104／8,103 pass、23／24 skip，原有各 8,028 個終端事件保持，只新增各 99 pass。格式、build、vet、固定 Insyra v0.3.4、模組與治理通過，COR-11 恢復 passed。MOD-06 的另一個已確認報告缺陷由 ticket129 接續，工程追蹤 90／91。本票 3／4，待清理與交付，見[驗收](../../evidence/COR-11/finite-intervention-norm-20261016/verification.json)。
