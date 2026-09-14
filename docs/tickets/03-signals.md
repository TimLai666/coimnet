# 03 — 使用者可以提供與對齊具名訊號

Epic：訊號與時間

User Story：使用者可以提供與對齊具名訊號

Blocked by：無

Status：partial

## 交付

提供具版本/形狀/單位的訊號、整數時鐘、確定性同時事件排序、可驗證映射、連續值時間點重取樣及脈衝事件對齊。

## 已完成

- `signal.NewSignal` 建立帶 schema／encoder 版本、具名通道、訊號種類、整數時間、形狀、單位、品質與來源序號的訊號。連續、活動、脈衝、調節及干預模式各自驗證，干預另要求目標。
- `Observation`、`Target`、`Feedback` 是不同的具體型別。回饋同時驗證產生時間、可取得時間及行動因果關係，不能在可取得時間前消費。
- `OrderSignals` 對同一時間的事件依來源序號排序，拒絕時間倒退、重複來源序號、不同時間單位與非有限數值。`NewStream` 會保存排序後的私有副本。
- `Clock` 以整數步號及整數時間單位運作，檢查負步號、時間對齊與乘法／步號溢位。
- `Mapping` 只依 `namespace`／`external_id` 建立連續索引；JSON 保存順序與索引，可重建並用 `ValidateAgainst`／`ValidateShape` 檢查外部圖與維度，不依賴 connectome 套件。
- 公開解碼入口拒絕 JSON 未知欄位及尾隨資料；建構、讀取與匯出均不共享可變切片。
- 目前 schema 版本固定為 `1.0` 並拒絕未知 schema；encoder／model 版本是獨立的 `Version`，可使用任何正 major、非負 minor 的使用者版本。
- 嚴格 JSON 入口拒絕 Unicode 大小寫別名重複欄位，限制單一輸入為 `16 MiB`，巢狀路徑深度最多 64 層。掃描只保存路徑堆疊，遇到錯誤才組合訊息，避免深層長欄位名稱造成二次方配置。

- `NewStreamingResampler` 與 `ResampleOffline` 提供整數比率時間對齊、因果保持與離線線性插值。兩個入口共用數值路徑，結果保留模式。使用者指南見 [訊號取樣](../signal-resampling.md)。

## 驗收

- [x] 連續、活動、脈衝、調節與干預模式分別驗證。
- [x] 時間倒退、重複序號、負持續時間與不支援單位拒絕。
- [x] 相同時間依來源序號排序，空觀察以明確空陣列表示，缺少訊號陣列的 JSON 拒絕。
- [x] 觀察、Target 與 Feedback 型別分離，回饋取得時間不可提前。
- [x] 映射持久化後重建一致，未知神經元與維度不符拒絕。

## 驗證證據

- Red：在實作前執行 `go test ./signal`，因 `Version`、`Timestamp`、`SignalSpec` 等公開型別尚不存在而編譯失敗。
- Green：實作後 `go test ./signal` 通過（所有 signal tests）；`go test -race ./signal` 通過；`go vet ./signal` 通過。
- 重取樣：103 檔同源的 Mac／Ubuntu v14 建置、單元、race、vet、模組與跨程序恢復全部通過。13 組取樣測試及 Go example 的詳細結果見 [驗證](../../evidence/signal-resampling-20260914/verification.json)。SIG-06 附上 [fixture 證據](../../evidence/SIG-06/verification.json) 後標示通過。

## 本次取樣契約與驗收

輸入經 `Signal` 驗證、來源時間比率與 watermark 對齊後，輸出具神經時鐘的 `Signal`。持續時間為零的連續／活動／調節樣本才適用，同時群組以最大來源序號覆寫。以公開 API 與可執行 Go example 驗證，無資料庫或舊格式遷移。

| 路徑 | 規則與測試 | 狀態 |
| --- | --- | --- |
| 模式與取值 | 0／2 ms 的 2／6，半毫秒位置的因果值 2、離線值 3／4／5；即時建構器拒絕離線模式 | Mac／Ubuntu 通過 |
| 時間與形狀 | 44.1 比率、超過 float64 精確整數範圍、128 位元中間乘積，對照獨立大整數參考；時間戳溢位拒絕 | Mac／Ubuntu 通過 |
| 同時樣本與分塊 | 同時序號 3 先於 2 到達，所有 8 種切塊結果一致；watermark 可重複，舊時間與序號拒絕 | Mac／Ubuntu 通過 |
| 空值與結尾 | nil／空輸入產生空訊號，前段缺值省略，最後樣本保持至 EndStep；再次結束與結束後輸入拒絕 | Mac／Ubuntu 通過 |
| 失敗與取消 | nil context、取消、錯 metadata／模式／單位、容量與來源順序錯誤不提交候選狀態，可重試 | Mac／Ubuntu 通過 |
| 數值與資源 | 固定有效範圍不因插值擴張，正負極大有限端點不溢位；buffer、輸出步數與邏輯值數量先檢查 | Mac／Ubuntu 通過 |

測試來源：`signal/resample_test.go`、`resample_edges_test.go` 與 `resample_example_test.go`。初始失敗日誌及後續結果見 `evidence/signal-resampling-20260914/`。

## 脈衝事件契約與驗收

`NewPulseAligner` 接受單通道、固定 metadata 的 `pulse` 時間點，映射至 `ceil(sourceTime × SimulationSteps / SourceTicks)`，輸出保留每筆來源序號及值。同一步的事件按來源時間及序號逐筆輸出，沒有振幅覆寫、加總或保持。所有資料由呼叫者提供，不新增外部資料、資料庫或格式遷移。

| 路徑 | 驗收 | 狀態 |
| --- | --- | --- |
| 呼叫與下游 | `Push`／`Finish` 產生 Signal，可建 Observation；Go example 展示保留 10／20 序號的同一步事件 | Mac／Ubuntu 通過 |
| 碰撞與分塊 | 目的步來源位置嚴格早於 watermark 才輸出全組；5 筆事件所有 16 種切塊結果相同；未封閉同時群可補較小序號 | Mac／Ubuntu 通過 |
| 數值與排序 | 整數比率對照獨立大整數；128 位元乘積、商進位及 Clock 溢位拒絕；來源序號不連續仍保留 | Mac／Ubuntu 通過 |
| 空值與結束 | nil／空輸入不產生空白步號或 held tail；nil context／零實例拒絕；Finish 排空且只成功一次 | Mac／Ubuntu 通過 |
| 錯誤與重試 | 重複／舊序號、時間倒退、metadata／種類／持續時間／單位錯誤、取消及容量超限均不提交狀態，可重試 | Mac／Ubuntu 通過 |
| 容量 | pending＋incoming 受事件數上限；`(2 × (pending + incoming) + 1) × valuesPerSignal` 在 payload 驗證前檢查，保留候選輸入／輸出及一筆來源事件 | Mac／Ubuntu 通過 |

初始缺少公開 API 的失敗測試、8 組測試與可執行範例、107 檔同源的 Mac／Ubuntu v15 完整驗證見 [脈衝證據](../../evidence/pulse-alignment-20260914/verification.json)。本次只完成 SIG-02 的脈衝時間點部分，完整需求通過數維持 14／85。

## 限制與未完成

- 跨頻率的連續值取樣與分塊已實作。區間訊號、媒體解碼／濾波、自訂 adapter 的核心端到端範例尚未完成。SIG-02 與 SIG-05 保留未通過。通道各自輸出獨立 Observation，多通道共用來源序號的合併流程待補。
- `Mapping.ValidateAgainst` 需要呼叫者提供外部圖的 `NeuronID` 集合；本 ticket 不假造 MaleCNS 或 FlyWire 查詢。
