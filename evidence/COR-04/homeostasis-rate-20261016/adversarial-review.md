# 唯讀審查與 Root 裁決

宿主 Codex，OpenCode 本輪 models 失敗，日誌無法開啟。依專案明確順序使用原生 gpt-5.6-luna / reasoning max，兩個唯讀分工；沒有派給宿主自己的 CLI。實際限制見 delegation.json。

兩位審查者核對共用 FMA 分支、合法域與真正溢位拒絕，沒有確認另一個正式程式缺陷。數學審查者指出正式 fresh 回歸的重算子項只查完成與零 upstream，不能單獨證明極端 Rate 跨 checkpoint 的數值相等。

Root 採納這個驗證缺口，保留正式失敗回歸，另用公開 API 診斷：eta=1e-308，在1022步加入放電門檻間的刺激。正確穩定狀態使事件為0，關閉後為1。11條公開輸出路徑、10條真溢位拒絕，以及分段反向重播通過 arm64 一般／競態與本機 Rosetta amd64。原始碼與結果見 observable-parity-verification.json 及 preserved-work.tar.gz。正式 Go 指紋沒有改動。

Root 已讀完整四行正式 diff、新測試、相關文件及每個數字的原始日誌。正常浮點路徑、API、模型格式與固定依賴沒有改動。初版測試的 nil 中性序列屬 Root 測試錯誤，已修正並用原程式 overlay 重跑紅燈，原錯誤完整保留。初次追蹤狀態值錯誤另見 initial-status-run.json，沒有更改或放寬測試。

未驗證：實體 Intel硬體、真實任務品質與全腦生物校準。完整工程結果以 engineering-verification.json 為準，不用此唯讀審查代替執行。

Root 也採納後續拒絕檢查建議：10條真溢位路徑須同時回錯且沒有部分 trace／outputs。加強後在三種執行模式重跑，結果相同，見 observable-check-strengthening.json。非零梯度的獨立證明留在下一個候選，不由本票零 upstream 的重播完成推論。
