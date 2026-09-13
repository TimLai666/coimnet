# 09 — 使用者可以保存並讀回標準化接線圖

Epic：真實接線資料

User Story：研究者可以把 08 產生的 `annotated_neurons` 視圖與報告以固定格式落盤，
之後不必重跑 151M 列掃描就能驗證並讀回同一張圖。

Blocked by：08 標準化接線圖

Status：draft（規劃中，未實作，驗收項目不得勾選）

## 交付

以 08 的 canonical stream（節點 ID 順序、邊順序 `(source, target, absolute row)`、
nullable weight、來源位置）與 `GraphReport` 為輸入，定義固定位元組格式的
GraphStore：manifest hash、converter version、predicate hash、node index hash、
edge order hash 與每個區段的校驗碼一起寫入；同目錄暫存檔寫入並同步後以不覆寫
方式發布；讀回時逐段校驗並與報告 hash 比對，任一不符拒絕載入。

## 未決決策

- 位元組格式：固定寬度區段加 JSON header，或直接沿用 Arrow IPC；需比較讀回校驗成本與
  版本相容性。
- 是否同時保存 `raw_segments` 統計（只有報告）或整個 raw stream（1.05 GB 原件已存在，
  暫不複製）。
- 讀回後接上 `dynamics`／`learning` 的介面屬 DAT-07（真實子圖訓練），不在本票。

## 依據

- [08 標準化接線圖](08-connectome-graph.md)：canonical stream、報告與 hash 定義。
- [主規格 5.2、15.2](../handoff/CoImNet_Implementation_Plan.zh-TW.md#chapter-05)：衍生物可重建、快照原子性。
- [共用工程設計](../../ENG.md)：不覆寫、原子寫入、版本變更保存舊原件。
