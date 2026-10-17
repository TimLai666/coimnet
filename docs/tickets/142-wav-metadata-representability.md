# 142 — WAV 輸出拒絕無法表示的欄位

Epic：音訊檔案完整性

User Story：使用者的取樣率、聲道數或檔案長度若超出 PCM16 RIFF 格式範圍，寫入入口會回傳錯誤，不會成功輸出欄位被截短的音訊。

Blocked by：139／140 的來源凍結驗收與交付；141 的合成尺寸修正先完成。

Status：in_progress

相關需求：TSK-02 音訊匯入／輸出與錯誤處理。

## Root 決策（2026-10-18）：公開重現與修正骨架

WriteWAV 在 wav.go:178-192 以原生 int 計算長度，再將取樣率、聲道數、byte rate 及 block align 轉成固定寬度整數，沒有檢查可表示範圍。Root 的公開一影格探測已確認三種成功寫入的錯誤：65,536 聲道變成 0 聲道，2^32+8000 Hz 變成 8000 Hz，兩聲道的 1,073,749,824 Hz 將 byte rate 4,294,999,296 截成 32,000。正常 8000 Hz 單聲道控制通過。

後續先凍結公開失敗回歸，再在配置與檔案建立前檢查所有固定欄位及衍生尺寸。合法輸出的位元組必須保持。聲道數仍依 Samples 長度決定，NaN 樣本仍依既有契約寫成靜音。讀取器對 byte rate／block align 的寬鬆處理不列為已確認缺陷，本票不變更讀取契約。

## 固定完成條件

- [x] 公開重現截短欄位，核對獨立整數參考與正常控制，保存探測並清理暫存。
- [x] Root 凍結先失敗的回歸，涵蓋欄位、衍生長度、拒絕前不建立檔案及正常控制。
- [x] 最小正式修正與專項一般／競態通過，正常 PCM16 位元組保持。
- [x] 格式、build、完整一般／競態、vet、固定模組與治理通過。
- [ ] 保存證據、清除本票自有暫存、提交推送並核對遠端。

## 減法審查

在既有寫入入口檢查格式限制，不新增音訊格式、任意取樣率上限或讀取修復層。

## 證據

[Root 核對](../../evidence/TSK-02/wav-metadata-audit-20261017/verification.json)、[原始輸出](../../evidence/TSK-02/wav-metadata-audit-20261017/probe.log)與[清理](../../evidence/TSK-02/wav-metadata-audit-20261017/cleanup.json)。公開探測、先失敗的回歸與隔離候選的各階段驗收已完成，正式採用與工程驗收見後續紀錄。

公開原版一般／競態各 6 fail／5 pass，五種非法 metadata 成功寫入的缺陷均重現，正常 golden 與既有路徑控制通過。另以未檢查算式的獨立私有函式骨架，一般／競態各 7 fail／4 pass，凍結不配置音訊的大整數尺寸界線測試。骨架不宣稱為既有正式實作，見[原版與骨架驗收](../../evidence/TSK-02/wav-metadata-audit-20261017/metadata-all-red-verification.json)。隔離回歸階段使用固定的 860 份正式來源，當時 WAV 正式檔尚未改動。

隔離候選的一般／競態各 22 pass，單聲道／雙聲道完整 bytes、合法空檔與既有路徑保留控制保持，見[候選驗收](../../evidence/TSK-02/wav-metadata-audit-20261017/metadata-candidate-verification.json)。11,606 bytes 暫存已清除，三份必要輸入保存，見[清理](../../evidence/TSK-02/wav-metadata-audit-20261017/regression-cleanup.json)。ticket141 已交付，兩項精確候選與三份凍結回歸已正式採用，見[共用採用紀錄](../../evidence/TSK-02/audio-boundaries-adoption-20261018/adoption.json)。目前 863 份來源固定，正式專項已通過，完整工程驗證進行中。

正式專項一般／競態各 41 pass，含本票及另一票的 28 個新事件、原有合成控制 13 個事件，29,121 bytes 正常音訊與抽樣資料指紋保持，見[正式專項驗收](../../evidence/TSK-02/audio-boundaries-adoption-20261018/focused-verification.json)。完整一般 8,686 pass／23 skip、競態 8,685 pass／24 skip，各 58 套件通過，原有 8,681 個終端事件保持，只新增 28 pass。九項工程步驟、31 項治理、九份正常報告與中性增益控制均通過，見[正式驗收](../../evidence/TSK-02/audio-boundaries-adoption-20261018/verification.json)。最後 31 項治理通過，1,546,088,321 bytes 自有暫存已清除，原模型、資料、模組與其他工作保持，見[清理](../../evidence/TSK-02/audio-boundaries-adoption-20261018/cleanup.json)。目前 4／5，提交與推送接續。
