# Ticket 56 差異審查

Scope: CLEAN。基準是 `20527ae510c0dff9b825e2f8763717f9e3357426`，變更涵蓋連續與 LIF 的有限序列分段前向、訓練 observe、兩份新增回歸及對應文件。

Adversarial review: RUN。核心及訓練層共用契約有變更，使用原生 `gpt-5.6-luna`、`max` 唯讀審查。Root 親自讀取完整實作與回歸，獨立執行測試並核對指紋。

沒有確認的阻擋問題。

## 契約與相容性

`dynamics/forward_recompute.go` 的兩個新方法沿用既有段落運算。初始值、參數與全部輸入在配置段落前完成驗證，錯誤及取消回傳 nil 輸出。下一段保留實際已發生的延遲前史，段長先限於剩餘步數，避免 `start + segment` 溢位。所有輸出與檢查點配置獨立陣列，沒有寫入呼叫者參數或模型。

Continuous 使用與原前向相同的 Exp／Expm1 係數及電位更新順序。LIF 的既有 helper 保留電位、突觸跡、適應、不應期、活動率及慢速穩定偏移。重複邊與自我邊依原次序計算。

`learning/recompute.go` 的 observe 改呼叫有限序列入口，保持段長、核心支援、theta 形狀及空序列檢查。Network 的 Step／StepFrom 共用此入口，非零持續狀態的限制維持。NewState／Advance、反向公式、模型格式及相依性沒有變更。

此變更沒有新增 SQL、網路傳輸、權限設定或檔案寫入。輸入驗證與錯誤傳遞在新 SDK 入口及原訓練交易流程完成。並行修改呼叫者的參數不屬既有保證。

## 驗收範圍

Root 的先失敗日誌確認：349,526 節點、延遲 2 的人工圖在完整歷史訓練成功，兩種核心的重算模式因持續歷史超過 1,048,576 值失敗。固定回歸通過後，Step／StepFrom、最終參數及最佳化器逐位相同，持續個體仍在越界時拒絕。

28 個重算頂層測試涵蓋新 SDK、MaxInt 延遲及段長、跨段 LIF 狀態、非有限值、晚發溢位、取消、所有權、既有副作用控制及梯度等價。測試與實作由不同 worker 處理，實作前凍結兩份新測試；Root 再次核對完整 SHA-256 未變。

四個獨立程序的人工圖量測保持最終參數指紋、梯度量與更新量相同，重算尖峰 RSS 低於完整歷史。完整輸出矩陣占 T×N，LIF helper 每段另配置 O(T) 列指標，因此這不是整個訓練器的固定記憶體上限，也不是全圖效能保證。

## 審查來源指紋

| 檔案 | SHA-256 |
| --- | --- |
| dynamics/forward_recompute.go | 26636e3c2de10727261a8dadcd728855c63d3ee2b2f89e2fc65c40a40c27fa34 |
| learning/recompute.go | 4575b35303d0dce1dd29ad9633b48eaa4ececaaa7f274e29825dc6f201d5b6bb |
| dynamics/forward_recompute_test.go | ac1298bd2ca7e76259c79de8b01a65cf087689c223e8181b2c1c0cb1b776ef7e |
| learning/recompute_capacity_test.go | 64484a9dc274fbd2c32b6150459669269224350e5ac3b7f31b8e2308d9e205f1 |

## 已知限制

成功輸出所有權與執行中取消目前以配置及控制流程審查確認，新測試直接覆蓋輸入不變性及執行前取消。超長序列的累積配置成本、全圖與其他 CPU 平台沒有本輪實測。

既有連續 helper 逐邊活化的效能待辦保留。既有 RSS 腳本及資源文件的暫存路徑不符合目前 runs/.tmp 規則，已另記於 AGENTS.md，沒有擴大此票的實作範圍。
