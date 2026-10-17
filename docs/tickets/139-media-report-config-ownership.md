# 139 — 媒體報告獨立保存執行設定

Epic：實驗報告與可重現設定

User Story：研究者保留影像、音訊、影片及三種生成角色的報告後，可以調整下一次的種子與保留條件，不改到舊報告。

Blocked by：138 的凍結來源完整驗證結束後，才採用正式 Go 修正。

Status：completed

相關需求：TSK-04、TSK-05、TSK-06、TSK-07。既有能力及已序列化證據不因此撤銷，另補 SDK 報告設定的所有權驗證。

## 已確認問題

RunMedia 正常返回後修改原 c.Seeds 與 c.Holdout，舊 report.Config 跟著改變，但 ConfigHash 與 Runs 的 seed 保留原值。公開合法探測已重現，屬 P2 實驗紀錄缺陷，沒有模型權重或辨識品質改變的證據。

RunMedia 的影像／音訊、RunVideo 與 RunRoles 四條公開入口均在一般／競態重現設定共用。第一次探測的 red horizontal 不是合法提示，該次失敗只歸為探測案例錯誤，原始日誌保留。

## Root 決策（2026-10-17）：骨架與共用契約

沿用既有驗證順序，在計算設定 JSON／雜湊前，分別複製 Holdout 與 Seeds。RunMedia、RunVideo、runRoles 各兩行，不新增共用抽象、報告版本或欄位。GeneratorConfig、VideoConfig 沒有其他可變欄位。設定、種子順序、訓練、輸出、錯誤與取消契約保持。

驗收使用四條公開正常入口，先保存完整報告 JSON，再合法修改呼叫端兩個切片，舊報告及其設定雜湊須保持。反向修改 report.Config 也不能改到呼叫端。直接修改回傳報告本身後，沒有自動重算雜湊的保證。執行期間並行修改輸入不在此票範圍。

## 固定完成條件

- [x] 四種公開路徑原版失敗、雙向隔離回歸經 Root 核對與凍結。
- [x] 隔離候選通過一般／競態，修改前完整報告與原版逐位元組相同。
- [x] 前票驗收結束後採用三處正式修正及公開回歸，保留相容性。
- [x] 格式、build、完整一般／競態、vet、固定模組與治理通過。
- [x] 保存必要證據、清除本輪暫存、審查提交推送並核對遠端。

## 減法審查

只複製兩個既有切片，不建立不可變報告 API，不改生成器、訓練目標或解碼器。三個入口目前各兩行即可處理，不為這六行新增 helper。

## 證據

[合法公開探測](../../evidence/TSK-04/report-config-ownership-20261017/probe-verification.json)、[完整證據目錄](../../evidence/TSK-04/report-config-ownership-20261017/)、[既有導航同型態契約](49-nav2d-report-config-ownership.md)。

Root 的原版一般／競態各 5 個預期失敗事件，候選各 5 pass，四份修改前完整報告的 SHA-256 在各建置內與原版相同。候選 runner session74636 實際 terminal 0，詳見[候選驗收](../../evidence/TSK-04/report-config-ownership-20261017/candidate-verification.json)。探測工作 9 個檔案、68,295 bytes 已清除，ticket138 的共用快取也已於前票驗收後清除。正式三處修正及凍結回歸已採用，見[採用紀錄](../../evidence/OPS-03/report-ownership-adoption-20261017/adoption.json)。固定進度 5／5，與 ticket140 共用最終完整驗證。

正式最終來源完整一般 8,645 pass／23 skip、競態 8,644 pass／24 skip，各 58 套件通過。原有各 8,656 個終端事件保持，新增各 12 pass，九份正常完整報告及已核准中性增益各 21 個控制保持。九項工程步驟及 31 項治理通過，見[最終驗收](../../evidence/OPS-03/report-ownership-adoption-20261017/verification.json)。最後 31 項治理通過，1,556,834,694 bytes 共用自有暫存已清除。修正 `6eebf25` 與主要證據 `a685b80` 已推送且遠端讀回完全相同，見[交付](../../evidence/OPS-03/report-ownership-adoption-20261017/delivery.json)。本票 5／5。
