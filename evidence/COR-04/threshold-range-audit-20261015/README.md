# LIF 閾值範圍與溢位驗收

本輪修正兩個已重現的數值缺陷。正常設定的模型在舊／新來源下，模擬、梯度、訓練與保存接續結果保持相同。原始來源為 `2f34aec4822059857fae3a5748a0eaeff6eaee9c`，最終 812 份來源指紋見 [source-final.json](source-final.json)。

## 兩個缺陷

1. **合法上下界的跨度溢位。** 設 `theta_min=-2^1023`、`theta_max=2^1023`、raw 為 0，原本會回報閾值不可表示。但獨立算式得到的閾值為 0、斜率為 `2^1022`，都可表示。現在只在跨度溢位時，先縮放有限端點再組合閾值與斜率。正常範圍沿用原算式。一般、持續狀態、分段重算、化學耦合與混合核心都涵蓋在修正內。
2. **最終閾值溢位仍回成功。** 有限基礎閾值加上適應或穩態偏移後可能成為無限大，其他狀態卻都有限，原本因此漏掉錯誤。現在於發布結果前拒絕，不提交呼叫端的輸入或持續狀態。化學耦合原本已有這項檢查。

這次沒有增加使用者參數、公開 API、依賴或模型格式。永久回歸先在修正前失敗，再固定指紋驗證修正，見 [原始失敗](root-red-final.json)、[最終閾值失敗](root-effective-red.json)、[跨度測試指紋](tests-frozen-final.json)及[最終閾值測試指紋](effective-tests-frozen.json)。第二組的兩個反向重算控制原本就會拒絕，沒有算成新增修復的失敗案例。

## 驗證結果

- 最終來源的兩組永久回歸，一般與競態各 40 個測試事件通過，見 [事件計數](final-target-summary.json)、[一般](root-final-green.json)與[競態](final-validation/root-green-race.json)。
- 完整一般測試 58 套件通過，5,818 個測試事件成功、22 個略過、0 個失敗，見 [計數](final-validation/full-unit-summary.json)與[回條](final-validation/full-unit.json)。
- 完整競態測試 58 套件通過，5,817 個測試事件成功、23 個略過、0 個失敗，見 [計數](final-validation/full-race-summary.json)與[回條](final-validation/full-race.json)。工程驗收見 [verification.json](verification.json)，COR-04 恢復 passed，最新 91 項工程需求皆為 passed。
- `go build ./...`、`go vet ./...`、`go mod verify`、模組清單、808 個正式 Go 檔格式及 CLI 的 `examples run lif-threshold` 通過。Insyra 維持 `v0.3.4`，`go.mod`／`go.sum` 不變。命令、環境與日誌指紋保存在 `final-validation/` 的同名 JSON 回條。

### 舊模型與保存相容性

[相容性結果](final-validation/normal-compatibility.json)包含普通及競態兩種建置，每種分別與相同建置的舊來源比較。每種建置有 16 組 LIF 設定、8 組獨立混合核心設定，完整模擬、梯度、狀態與新程序接續逐位元組相同。混合核心沒有 Workers 選項，重複跨越 LIF 的 worker 因子不另計為獨立設定。

32 組正常訓練涵蓋連續／LIF、視窗 1／3、損失縮放 1／7、重算開關與參數共用開關。新舊完整結果相同。64 次新程序讀取舊快照，分為舊來源及最終來源各 32 次，參數、最佳化器、後續更新與預測相同。Root 另外獨立讀回全部 206 個成功命令回條及日誌，指紋見 [封存索引](final-validation/compatibility-archive.json)，原始內容見 [封存檔](final-validation/compatibility-artifacts.tar.gz)。

合法大範圍另外實際訓練三次、保存並用新程序接續，最終 raw 為 `0.021880151981676275`。持續狀態下一步輸出為手算的 `0.5`，最終來源可恢復本版與第一版候選的保存物，見 [恢復結果](final-validation/wide-recovery.json)。這些是小型人工測試樣本，不是模型產品。原始版本在此範圍無法前向，不能宣稱它能產生相同的大範圍模型。

## 證據與重現

命令回條記錄實際 argv、Go 環境、輸入來源指紋及日誌 SHA-256。參考算式、公開重現與實作位置盤點保存在本目錄。Root 與委派者的測試審查修正分別保存在 `root-test-review.json`、`root-effective-test-review.json`，最終審查見 [root-review.md](root-review.md)及[獨立審查](adversarial-review-final.md)。[文件審查](evidence-doc-review.md)保留讀取當時的狀態，提出的計數與派工可追溯性缺口已分別補上直接回條；完整競態的最終結論以本頁與驗收 JSON 為準。

`before/` 保留六個原始核心檔，`final-validation/overlay.json` 記錄舊來源建置對應。`core-compat-reviewed.go.txt`、`trainer-compat.go.txt`、`wide-restore.go.txt` 及 `final-validation/compatibility.py.txt` 保留本輪公開重現程式。執行時須將其複製至專案內的忽略暫存目錄，依回條準備固定模組與快取。正式快取清除後不能直接重跑指向舊暫存絕對路徑的命令。

根目錄的第一版完整一般測試已被最終來源重跑取代，`full-race-interrupted.json` 記錄第一版競態中止，沒有列為通過。第一版候選可由 `candidate-v1.patch.gz` 解壓後配合指紋還原，解壓內容與原始 patch 逐位元組相同。Root 的試驗輸入／測試程式設定錯誤有獨立回條，不歸為框架缺陷。

四份完整工程日誌壓縮為 1,090,063 bytes，解壓後與原始 18,323,853 bytes 逐位元組相同。命令回條保持原內容，原始 `log` 欄位的路徑由 [壓縮索引](engineering-log-archives.json)對應到 `.gz`。最終驗收的 `test_log` 已指向存在的壓縮檔。

本輪主暫存已清除 1,707,216,011 bytes，見 [清理回條](cleanup.json)。清除前兩次核對完整內容與開啟狀態，18 個原模型檔、兩個原有暫存目錄及 41,797 個唯讀連結來源保持。清理後最後 11 項治理檢查、31 個測試事件通過，見 [回條](final-validation/governance-post-cleanup.json)。其 36,506,434 bytes 執行檔也已[清除](smoke-cleanup.json)，本輪兩個暫存目錄都已不存在，合計清除 1,743,722,445 bytes，沒有重建 Go 快取。

修正 `29dffdc` 與回歸／證據 `b433d26` 已正常推送到 `codex/receptor-chemistry-training`，遠端讀回相同，見[交付回條](delivery.json)與[完整檔案清單](changes.md)。812 份提交來源與完整驗證的指紋相同。本票完成 5／5，整體零 bug 與長期品質目標繼續追蹤。

## 執行環境與分工

宿主為 Codex，Go `1.26.5`、macOS arm64、Apple M3。執行工作與全部快取均位於本專案的 `runs/.tmp/`，使用唯讀連結的既有固定模組。沒有執行 SSH 或網路磁碟壓力測試。

OpenCode 的 `opencode models` 實際失敗，原文為 `Unknown: FileSystem.open (/Users/timlai/.local/share/opencode/log/opencode.log)`，見 [工具失敗](delegation-tool-failure.json)。依專案規則改用兩個原生 subagent，皆為 `gpt-5.6-luna`、reasoning effort `max`，分別處理指定測試與唯讀審查，見[實際派工參數與工具回覆](delegation-dispatch.json)。正式修正、測試裁決、驗證與交付由 Root 負責。沒有假稱呼叫到 OpenCode 模型。

## 限制與後續

執行證據限本機 macOS arm64。數值與恢復驗收不證明真實任務辨識率提升，也不證明生物機制正確或整個框架沒有 bug。LIF 的長期真實任務品質量測另行保留。

負向閾值位移在取設定下界前溢位時，普通與耦合入口的既有行為不同。原來源與本版相同，缺少共用契約決定，見 [待釐清紀錄](modulation-floor-follow-up.json)。已記錄在 `AGENTS.md`，本輪不改變這項行為。

減法審查：本次直接修正既有計算與拒絕條件，沒有新增設定或抽象。重複相容性證據合併封存；驗證完成後清除本輪可重建的快取與執行檔，保留必要指紋和壓縮日誌。
