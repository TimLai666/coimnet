# 91 — 研究者可以隔離並恢復受體驅動的個體宣告

**Epic:** 機制與模型品質稽核
**User Story:** 研究者可以修改輸入或快照副本而不改到個體，讀回時缺失或越界的受體宣告會立即被拒絕。
**Blocked by:** 90（已完成），不依賴 ticket 89 的增益決策
**Status:** not_started

- [ ] Root 凍結 New／Config／Snapshot／Restore 的受體所有權與既有交叉驗證契約。
- [ ] 公開失敗回歸先重現，Root 審查凍結後才修正輸入／輸出指標隔離及恢復驗證缺口。
- [ ] 三種 CPU 核心的正常、缺省與拒絕行為、完整工程及既有合法快照逐位相容通過。
- [ ] 同步需求與證據，清除本輪暫存並保留原件。
- [ ] 已驗證階段完成提交、推送與遠端讀回。

既有診斷確認四個宣告別名、四個個體在外部改動後越界、兩個正編號越界快照被接受後崩潰，以及兩個缺少 Chemical 的快照被接受後停用受體作用。完整 Root 證據見[ticket 90 稽核](../../evidence/MOD-06/slow-memory-composition-20261014/root-review.md)。這些診斷預期原 bug，不計為永久修正通過。

新修正沿用 immutable Model／independent copy 與 checkReceptorRule 的既有契約。保存格式、正常公式及可選欄位缺省保持。BaseL2 單位及 ticket 89 的增益合法性沒有由本票決策。
