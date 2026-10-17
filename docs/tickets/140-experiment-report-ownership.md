# 140 — 實驗報告獨立保存可變宣告

Epic：實驗報告與可重現設定

User Story：研究者重用下一次實驗的種子、任務排程、保留組合與回饋宣告時，既有實驗報告的設定及結果保持。

Blocked by：138 的凍結來源驗收、清理與交付。

Status：completed

相關需求：OPS-03、TSK-09、TSK-10、TCH-04、LRN-10。這是 ticket139 同型態查核的其他 producer，保留兩票各自驗收條件，正式修正後共用一次完整工程驗證。

## Root 決策（2026-10-17）：骨架與共用契約

Root 已讀取 RunMultiTask、multimodaleval.Run、studenteval.Run、RunAdaptiveEvaluation 的輸入、驗證與報告 producer。前三者淺複製可變設定，後者的固定／適應模式直接保存 FeedbackAvailable。公開正常案例已在原版一般／競態重現，隔離候選通過，詳見本票證據。

沿用既有驗證與錯誤順序，在 producer 計算雜湊或派往模式分支前複製其保存在報告的清單：Schedule.Tasks、Holdout／Seeds、Seeds、FeedbackAvailable。巢狀元素都是純值。Adaptive 檔案已使用 slices，沿用 slices.Clone 保留 nil／空集合形式。報告本身仍可由持有人修改，沒有新增自動重算雜湊或執行期間輸入同步契約。

驗收包含三個正常實驗入口與兩種評估模式。先保留完整報告及設定 hash，再合法修改 caller 清單，原報告須保持。反向修改報告清單不能改到 caller。多模態兩個清單都要檢查，Adaptive 使用 score 加一個未使用的 metadata 回饋種類，返回後只改後者，原本的回饋權限保持。

OCR 與 Textgen 已在 producer 複製清單。ASR 私有 CLI helper 的設定由唯一 caller 建立並立即序列化，沒有目前公開重用入口，不列為此票需修改位置。既有導航、持續學習及其他已複製的入口不重做。

## 固定完成條件

- [x] Root 審查凍結公開回歸，原版各路徑在行為斷言失敗。
- [x] 隔離候選的一般／競態通過，正常報告指紋保持。
- [x] 前票結束後採用四個 producer 的正式修正與回歸。
- [x] 與 ticket139 的最終來源一起通過完整工程與治理。
- [x] 保存必要證據，清除自有暫存，提交推送並核對遠端。

## 減法審查

只在既有 producer 複製可變清單，不增加報告版本、設定解析層或不可變物件，不改學習／評分規則。與媒體同型態修正共用完整工程驗證，避免同份最終來源重跑兩套全套測試。

## 證據

[來源與靜態候選](../../evidence/OPS-03/experiment-report-ownership-20261017/baseline.json)、[媒體同型態修正](139-media-report-config-ownership.md)。

Root 已執行原版一般／競態各 7 個預期失敗、候選各 7 pass，五份正常完整報告的 SHA-256 在各建置內保持。原版 32 則失敗均是報告變動、設定指紋或反向共用的斷言，沒有編譯錯誤、panic 或資料競態。65,120 bytes 探測暫存已清除。見[本輪驗收](../../evidence/OPS-03/experiment-report-ownership-20261017/verification.json)。正式四個 producer 的修正及凍結回歸已採用，見[採用紀錄](../../evidence/OPS-03/report-ownership-adoption-20261017/adoption.json)。固定進度 5／5，與 ticket139 共用完整工程驗證。

正式最終來源完整一般 8,645 pass／23 skip、競態 8,644 pass／24 skip，各 58 套件通過。原有各 8,656 個終端事件保持，新增各 12 pass，九份正常完整報告及已核准中性增益各 21 個控制保持。九項工程步驟及 31 項治理通過，見[最終驗收](../../evidence/OPS-03/report-ownership-adoption-20261017/verification.json)。最後 31 項治理通過，1,556,834,694 bytes 共用自有暫存已清除。修正 `6eebf25` 與主要證據 `a685b80` 已推送且遠端讀回完全相同，見[交付](../../evidence/OPS-03/report-ownership-adoption-20261017/delivery.json)。本票 5／5。
