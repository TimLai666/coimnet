# 存檔欄位與化學接續驗收

bundle 現在會在正規化前拒絕缺少必要狀態或設定的文件，合法含化學機制的任務存檔可以載入並接續。bundle 訓練的合法 null 空集合保持，單檔 episode 與個體快照的嚴格 null 規則保持。

## 已確認與修正

| 問題 | 修正 | 回歸 |
| --- | --- | --- |
| 必填化學步數、資源、回饋、訓練設定或證據清單被解碼成零值，無法辨識遺漏 | 四種公開 bundle 入口先檢查原始欄位，再解碼與還原陣列 | bundle_presence_test.go 的公開拒絕與空結果檢查 |
| 合法化學 task 因資源 map 無法通過通用檢查 | 任務訓練沿用化學專用檢查 | 保存、載入與下一次訓練對照 |
| 第一版候選誤拒舊 writer 寫出的合法 null 空集合 | 只有 bundle 訓練的已存在 resources／pending_feedback null 使用驗證副本，原始 nil 保留 | bundle_chemical_nil_test.go 的合法、缺欄位及單檔拒絕對照 |

第三列是在提交前抓到的候選回歸。原始失敗、取消與診斷修正紀錄保留，沒有把取消算成通過。

## 驗證

- 完整一般與 race 各 58 套件通過。一般 5,748 pass／22 skip，race 5,747 pass／23 skip，沒有 fail。跳過原因列在各份 summary。
- 格式、build、vet、固定相依性、資料與任務 CLI 通過。80 步保存加 40 步接續，與連續 120 步的快照位元組相同。
- 46 個永久公開回歸通過，14 個額外非法集合／時鐘案例全部拒絕、回傳空結果且原檔保持。
- 48 份舊個體在新程序中的輸出、化學報告、狀態及下一次訓練相同。三種舊保存物保持，八份原始壞存檔明確拒絕。
- 六個既有 KMNIST ON／OFF 模型的 SDK 推論與下一次 MSE 更新位元組相同。這不是新的 OCR 辨識率量測。
- 最後 11 個治理測試、31 個含子測試事件通過。809 份來源及兩份 Root 凍結回歸指紋相符。
- 清除 11,291 個本輪暫存檔，邏輯大小 2,122,375,284 bytes。41,797 個快取連結的原件、兩個原有暫存目錄及 18 個模型檔案保持。

來源與命令見 [verification.json](verification.json)、[最後一般測試](source-final-unit-summary.json)、[最後 race](source-final-race-summary.json)。原始大日誌以 gzip 無損保存，對照見 [archive-terminal.json](archive-terminal.json)。小型人工存檔與原始輸出逐位保存於 [fixture 回條](actual-fixture-archive.json)。清理見 [cleanup.json](cleanup.json)，每個變更檔案見 [changes.md](changes.md)。

## 範圍

沒有修改核心數值算式、公開 API、保存版本、依賴或合法舊檔省略規則。真實任務品質、生物定量正確性、新全腦效能及完整零 bug 尚未由這次驗收證明。

【減法審查】共用既有欄位檢查，移除未使用的函式，沒有新增保存格式或遷移工具。
