# 143 — 合成語音資料產生器先檢查容器與字元配置尺寸

Epic：音訊資料形狀與錯誤處理

User Story：使用者設定無法表示的樣本數或字長時，Generate 回傳錯誤，不會觸發配置 panic。

Blocked by：141 合成樣本尺寸修正的凍結驗收與交付。

Status：completed

相關需求：TSK-02 合成資料 helper 與失敗處理。

## Root 決策（2026-10-18）：公開重現與修正骨架

公開 Generate 的 MaxInt 樣本容器與單筆 MaxInt 字長在 64-bit 執行分別重現 makeslice cap／len panic。兩個要求的配置乘積均超過 uintptr，不需建立大型資料即可重現。正常單字元與零樣本、未使用的極端字長控制通過。現有 min/max 抽樣算式在已驗證範圍內不溢位，不改其亂數或抽樣語意。

先凍結公開失敗回歸，再在既有配置入口驗證 Utterance 容器與 rune 緩衝的位元組是否可用原生 int 表示。元素大小依平台型別。字元配置檢查實際抽到的字長，保留原本的抽樣規則，零樣本不驗證未使用的字長配置。這是算術檢查，不保證 runtime maxAlloc 或可用記憶體，不增加任意容量上限、公開 API 或模型欄位。

## 固定完成條件

- [x] 公開重現兩個配置 panic，與獨立整數乘積及正常／空集合控制核對。
- [x] Root 凍結先失敗的公開回歸，保持正常亂數與空集合行為。
- [x] 最小正式修正與專項一般／競態通過。
- [x] 格式、build、完整一般／競態、vet、固定模組及治理通過。
- [x] 保存證據、清除本票自有暫存、提交推送並核對遠端。

## 減法審查

檢查原有兩處配置的元素位元組乘積，不新增資料產生層、固定字長上限或音訊格式。

## 證據

[公開探測與 Root 核對](../../evidence/TSK-02/generate-capacity-audit-20261018/verification.json)。2,869 bytes 探測來源與工具快取已清除，必要來源保存在證據內，見[清理](../../evidence/TSK-02/generate-capacity-audit-20261018/probe-cleanup.json)。探測與隔離階段的 860 份來源保持，原版與隔離驗收已完成，正式驗收見後續紀錄。

公開原版的一般／競態各 3 fail／3 pass，兩個錯誤子案例與其父測試失敗，兩個正常控制及其父測試通過，見[失敗驗收](../../evidence/TSK-02/generate-capacity-audit-20261018/red-verification.json)。隔離候選各 6 pass，正式來源保持，12,898 bytes 專項暫存已清除，見[候選驗收](../../evidence/TSK-02/generate-capacity-audit-20261018/candidate-verification.json)與[清理](../../evidence/TSK-02/generate-capacity-audit-20261018/regression-cleanup.json)。

## Root 決策（2026-10-18）：共用最終工程驗證

本票與 ticket142 的候選改動不同檔案、回歸已各自先失敗再通過。ticket141 交付後，兩票一起採用，凍結同一份最終來源執行完整工程驗證，避免對相同來源重跑整套測試。兩票的原版失敗、專項驗收與各自固定五項完成條件分開保留。

兩項精確候選與三份凍結回歸已正式採用，見[共用採用紀錄](../../evidence/TSK-02/audio-boundaries-adoption-20261018/adoption.json)。863 份來源固定，正式專項與完整工程驗收均已通過。

正式專項一般／競態各 41 pass，含本票及另一票的 28 個新事件、原有合成控制 13 個事件，29,121 bytes 正常音訊與抽樣資料指紋保持，見[正式專項驗收](../../evidence/TSK-02/audio-boundaries-adoption-20261018/focused-verification.json)。完整一般 8,686 pass／23 skip、競態 8,685 pass／24 skip，各 58 套件通過，原有 8,681 個終端事件保持，只新增 28 pass。九項工程步驟、31 項治理、九份正常報告與中性增益控制均通過，見[正式驗收](../../evidence/TSK-02/audio-boundaries-adoption-20261018/verification.json)。最後 31 項治理通過，1,546,088,321 bytes 自有暫存已清除，原模型、資料、模組與其他工作保持，見[清理](../../evidence/TSK-02/audio-boundaries-adoption-20261018/cleanup.json)。修正 `9bce15d` 與主要證據 `66e6968` 已正常推送且遠端讀回相同，863 份提交來源與驗收指紋一致，見[交付](../../evidence/TSK-02/audio-boundaries-adoption-20261018/delivery.json)。本票 5／5。
