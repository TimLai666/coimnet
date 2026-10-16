# Root 審查

範圍：ticket124 的兩個 learning 測試檔及追蹤文件。正式運算、設定合法域、模型格式、訓練目標與依賴保持。唯一 worker 格子為 `referenceForward` 函式體；Root 寫兩個 nominal 欄位與全部新檢查。函式外與凍結測試內容指紋已核對，見 `scope-review.json`、`frozen-checks.json`。

## 已確認結果

- 原獨立參考明確拒絕 enabled homeostasis。修正 Root 最初遺留的未定義變數名稱編譯錯誤後，先取得真實 unsupported panic，再派實作。初版與更正原因保留在 `initial-root-checks.go.txt`、`test-correction.json`，沒有調整驗收算式或容差。
- 參考保留 r/h 的前向值，h 在化學偏移與下限投影前加入閾值。h 的導數停止，adaptation 與活動來源的化學導數保持。LIF 正常與休止事件都更新 r/h，continuous 不更新，停用時新陣列是 nil。
- `chemical_homeostasis_reference_test.go:60` 起的手算逐行核對四步 r/h、公開輸出與第二步對首步輸入的 1/16 梯度。錯傳 dr/dh 的隔離對照得到 17/1296，漏更新休止步的對照把第二列 rate 從預期 .25 變成 0，兩者在一般與競態建置均被拒絕。
- 80 組完整輸出與梯度對照，含 mixed/LIF、controller/neural、Sum/Max、正負與上限／floor、明示／省略 InitialActivity。比較所有參數、核心初始狀態、核心輸入與外部輸入座標。16 組開關控制可觀察差異，8 組停用對照逐位相同。七筆舊記錄在各自建置內逐字相同，不要求不同編譯模式浮點結果逐字相等。
- 160 條訓練路徑檢查累積第一步的參數不變與完整梯度，再核對第二步第一次 Adam 的全部參數、一二階矩與計數。Mixed 只有全歷史，純 LIF 另有 segment 1／3。公式與 `trainer.go:528` 起公開路徑一致，容差由既定 float32 誤差與更新函式的變化上界推導。

沒有已確認的正式程式缺陷。Root 親自讀完整 diff 並執行一般／race 專項與隔離錯誤對照；結果見 `diagnostic-verification.json`。完整工程及最終交付只依各命令終端回條與 `verification.json` 判定。

## 適用限制

四步、兩節點、四邊、單一 source/channel 的普通有限範例。沒有本票新增的極端數值、正截斷視窗、持續神經狀態、多 source/channel、真實任務品質或生物校準結論。私有 nominal r/h 的逐行核對不等於公開持續狀態讀回。

首步 Adam 使用 accumulate=2，沒有 clip、weight decay、mask、schedule 或 sharing。其參數絕對容差下限為 5e-6，並未宣稱可單獨辨識所有極小更新；同一路徑另有完整 pending gradient 與一二階矩檢查。唯讀 worker 提出的兩點是驗證限制，未確認正式程式出錯，不擴張本票。

重新索引 MCP 被自動審查拒絕，理由是未驗證接收端與原始碼外傳授權。本票改採本機來源核對，沒有重試或改用其他管道外傳。OpenCode models 實際失敗，依專案規則使用 gpt-5.6-luna / max，見 `delegation.json`。

【減法審查】沿用同一個獨立參考與訓練入口，移除參考內不支援的分支；隔離錯誤只透過 overlay 驗證，來源與必要指紋保存後清除本輪編譯快取。沒有新增正式設定或另一套神經核心。
