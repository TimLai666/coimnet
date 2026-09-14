# 多通道自訂 adapter 範例

這個範例把三條不同頻率的訊號轉成固定步數的矩陣，再交給既有的 `learning.Network` 與 `learning.Trainer`。資料與兩個神經元的接線都是人工測試資料，目的是示範外部資料如何接入框架。

在 repo 根目錄執行：

```sh
go test ./examples/multichannel -run ExampleAdapt -count=1 -v
go test ./examples/multichannel -count=1 -v
go doc ./examples/multichannel.Adapt
```

第一個命令執行 [端到端範例](example_test.go)，建立觀察、取樣、預測及 50 次訓練更新。第二個命令也驗證相容性。範例資料建構在 [adapter_test.go](adapter_test.go)，所有資料都在記憶體產生，不下載資料或輸出模型。

## 固定範例契約 `multichannel/v1`

`Adapt(ctx, Inputs, chunkSize)` 接受同一個 experience 的三個獨立 `signal.Observation`。各自的 stream ID 與 channel 必須和下表相符。來源序號只在各自串流內比較，輸出列使用共同的神經步號。

| 通道 | 輸入 | 每個神經步的來源刻度 | 取值規則 |
| --- | --- | --- | --- |
| `level` | `continuous`、持續時間 0 | 2 | 因果保持，同時樣本以較大序號覆寫，尾端延續至步號 7 |
| `gate` | `activity`、正持續時間 | 3 | 起點包含、終點不包含，較新區間優先，到期後恢復仍有效的舊區間 |
| `pulses` | `pulse`、持續時間 0 | 4 | 對齊當下或下一步，同一步按來源時間及序號加總振幅 |

來源使用 `normalized_time`，數值單位為 `1`、形狀 `[1]`、encoder 版本 `1.0`。輸出 Clock 使用 `model_step`，步長 1，固定包含步號 0 至 7。三個來源與輸出共用零起點，這些比率是人工設定。實際執行耗時不參與取樣，也不等於神經步數。

每列固定六欄：`[level, level_present, gate, gate_present, pulse_sum, pulse_present]`。缺少樣本時 value 與 present 都是 0。觀測到零值時 present 是 1，脈衝互相抵銷也保留 present=1。空白步號會保留，避免把沒有事件的時間壓縮掉。

每條串流最多 32 個純量樣本，`chunkSize` 為 1 至 32。來源起點不得晚於步號 7 對應的來源位置，超出就回傳錯誤。區間可以延伸超過輸出範圍，輸出只取到步號 7。空通道必須建立具身份的空 Observation，零值 Observation 會失敗。任何錯誤或取消只回傳 nil 矩陣，輸入維持不變。

## 接到核心與更換資料

[adapter.go](adapter.go) 只依賴公開的 `signal` API，依次收集各通道 `Push` 與 `Finish` 的結果。回傳矩陣可直接傳入 `Predict`、`LossGradient` 或 `Step`，模型的 `InputSize` 設為 6。

範例的 `syntheticModel` 明列六個特徵到兩個人工神經元的初始投影係數。讀出只看第 1 號人工神經元的核心活動，答案只傳給 `Trainer.Step`。相容性測試把 adapter 結果與手算矩陣分別交給同一組初始模型，逐一比對完整梯度、50 次更新結果、參數與最佳化器狀態。

要接入其他資料，可在自己的專案沿用這個函式結構，改寫來源通道、形狀、時間比率與取值規則，並為新契約補上手算矩陣測試。核心與訓練器不用因資料格式而改寫。通道數或形狀改變時，也要同步更新欄位順序、模型 `InputSize` 與編碼器參數形狀。

## 適用限制

這是有限序列的整合範例，每次 `Predict`／`Step` 依 learning 契約從零神經狀態開始，只讀最後一步輸出。取樣使用因果保持與已知持續時間的區間，但函式會等待整段來源處理完才回傳，不能用它量測即時延遲。由事後資料推算的區間持續時間不符合這裡的起點已知假設。

輸出矩陣的數值本體為 `8 × 6 × 8 = 384 bytes`，這不是程序記憶體用量。Observation 在呼叫前已由使用者建立，函式還會使用 metadata 副本、取樣緩衝及 Go 配置。各取樣器的邏輯值數量上限是 80，詳細計數方式見 [取樣指南](../../docs/signal-resampling.md)。訓練核心另有運算與歷史成本。

此範例使用 float64 矩陣，adapter 回傳前會檢查最終特徵值符合 Insyra 編碼器的有限 float32 範圍。脈衝依宣告順序加總，途中溢位就失敗，最終抵銷為零則保留。範例沒有一般媒體解碼、抗混疊濾波、真實感官映射或生物脈衝動態，也沒有新增 GPU 後端。
