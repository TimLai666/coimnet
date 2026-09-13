# 03 — 使用者可以提供與對齊具名訊號

Epic：訊號與時間

User Story：使用者可以提供與對齊具名訊號

Blocked by：無

Status：partial

## 交付

提供具版本/形狀/單位的訊號、整數時鐘、確定性同時事件排序及可驗證映射。

## 已完成

- `signal.NewSignal` 建立帶 schema／encoder 版本、具名通道、訊號種類、整數時間、形狀、單位、品質與來源序號的訊號。連續、活動、脈衝、調節及干預模式各自驗證，干預另要求目標。
- `Observation`、`Target`、`Feedback` 是不同的具體型別。回饋同時驗證產生時間、可取得時間及行動因果關係，不能在可取得時間前消費。
- `OrderSignals` 對同一時間的事件依來源序號排序，拒絕時間倒退、重複來源序號、不同時間單位與非有限數值。`NewStream` 會保存排序後的私有副本。
- `Clock` 以整數步號及整數時間單位運作，檢查負步號、時間對齊與乘法／步號溢位。
- `Mapping` 只依 `namespace`／`external_id` 建立連續索引；JSON 保存順序與索引，可重建並用 `ValidateAgainst`／`ValidateShape` 檢查外部圖與維度，不依賴 connectome 套件。
- 公開解碼入口拒絕 JSON 未知欄位及尾隨資料；建構、讀取與匯出均不共享可變切片。
- 目前 schema 版本固定為 `1.0` 並拒絕未知 schema；encoder／model 版本是獨立的 `Version`，可使用任何正 major、非負 minor 的使用者版本。
- 嚴格 JSON 入口同時拒絕大小寫別名重複欄位，並限制單一輸入為 `16 MiB`。

## 驗收

- [x] 連續、活動、脈衝、調節與干預模式分別驗證。
- [x] 時間倒退、重複序號、負持續時間與不支援單位拒絕。
- [x] 相同時間依來源序號排序，空觀察以明確空陣列表示，缺少訊號陣列的 JSON 拒絕。
- [x] 觀察、Target 與 Feedback 型別分離，回饋取得時間不可提前。
- [x] 映射持久化後重建一致，未知神經元與維度不符拒絕。

## 驗證證據

- Red：在實作前執行 `go test ./signal`，因 `Version`、`Timestamp`、`SignalSpec` 等公開型別尚不存在而編譯失敗。
- Green：實作後 `go test ./signal` 通過（所有 signal tests）；`go test -race ./signal` 通過；`go vet ./signal` 通過。

## 限制與未完成

- 尚未實作完整媒體重取樣、不同頻率的媒體／神經時間對齊、分塊尾端處理，亦未提供因果與離線非因果前處理 adapter。`OrderSignals` 僅處理已在同一整數時間單位的事件排序，因此 SIG-02／SIG-05／SIG-06 不標示完成。
- `Mapping.ValidateAgainst` 需要呼叫者提供外部圖的 `NeuronID` 集合；本 ticket 不假造 MaleCNS 或 FlyWire 查詢。
- 待修：`signal/json.go` 的重複鍵掃描（`scanJSONValue`）沒有巢狀深度上限，且每層以字串串接保存路徑，深層巢狀輸入會造成二次方配置（16 MiB 上限下可達數十 GB）。`connectome/manifest.go` 已改為深度上限 64 加路徑堆疊，同一做法應套回 `signal`，並加深層巢狀的回歸測試。2026-09-13 對抗性審查發現。
