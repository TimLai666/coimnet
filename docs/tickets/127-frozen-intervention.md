# 127 — 凍結個體的介入實驗保留正確對照與失敗原狀

**Epic:** 介入實驗可靠性
**User Story:** 研究者在凍結化學／可塑性時執行介入，能取得只歸因於介入的差異，且失敗不會偷偷前進個體。
**Blocked by:** 22、126（已交付）
**Status:** complete

- [x] 公開重現、完整契約與獨立控制測試 RED 完成。
- [x] 最小修正、專項一般／競態、隔離回退及合法舊行為通過。
- [x] 完整工程與治理通過，需求證據更新。
- [x] 審查、證據保存、暫存清理、提交與遠端讀回完成。

## Root 決策｜2026-10-16

已由實際公開 probe 確認未生效 plan 回報非零 ActivityDelta，以及凍結可執行的 fresh neural source 因對照解凍而回錯卻前進狀態。範圍、固定控制與取消判準見[計畫](../../evidence/COR-11/frozen-intervention-20261016/plan.md)。只修正內部對照及提交順序，不改公開載入旗標或 frozen channel phase 優先序。

專項一般／競態各 90 個 terminal pass，涵蓋 85 個公開控制案例。原有與新介入／凍結測試各 119 pass。隔離遺漏旗標版本各 45 fail，先提交 subject 的版本各 19 fail，皆已成功編譯並實際執行。841 份正式來源的完整一般／競態各 58 套件通過，8,005／8,004 pass、23／24 skip，原有各 7,938 個終端事件保持，只新增各 90 pass。格式、build、vet、固定 Insyra v0.3.4、模組及治理通過。COR-11 因另已重現的範數溢位保持 specified，工程追蹤 90／91，下一項由 ticket128 修正，見[驗收](../../evidence/COR-11/frozen-intervention-20261016/verification.json)。清理與交付完成後才勾選最後一項。

本輪 1,740,939,604 bytes 自有暫存已清除，18 個原模型、六份必要資料、原始模組與其他工作保持，見[清理](../../evidence/COR-11/frozen-intervention-20261016/cleanup.json)。主要提交 `0af230b` 已正常推送且遠端讀回相同，841 份來源與提交內容核對完成，見[交付](../../evidence/COR-11/frozen-intervention-20261016/delivery.json)。本票完成 4／4。
