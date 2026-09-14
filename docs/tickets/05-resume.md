# 05 — 使用者可以中斷並接續參考訓練

Epic：狀態保存

User Story：使用者可以中斷並接續參考訓練

Blocked by：04

Status：partial（COR-01／STA-02／STA-04 fixture 通過，STA-01／STA-03 待完成）

## 交付

新增 `checkpoint.State`，保存獨立 episode trainer 的完整
`learning.TrainingSnapshot`、`delayed-pulse-splitmix64-v1` 資料 seed 與
sample cursor。`NewState` 要求 `NextSample == Training.Updates`，恢復時再由
`learning.RestoreTrainer` 驗證拓撲、形狀、最佳化器與有限值。

此 episode 格式不包含持續個體的神經狀態；下方新增的 individual profile 另行保存電位與延遲歷史。

## API

```go
func NewState(snapshot learning.TrainingSnapshot, dataSeed, next uint64) (State, error)
func Save(ctx context.Context, path string, state State) error
func Load(ctx context.Context, path string) (State, error)
```

檔案是帶 SHA-256 payload checksum 的嚴格 JSON envelope。Save 在目標同一目錄
建立 0600 暫存檔，寫入並同步後以排他 hardlink 發布，因此既有目標不會被覆寫。
暫存檔與目錄同步失敗會回報；公開後若目錄同步或取消失敗，錯誤會明確標示檔案
已發布但耐久性尚未確認，且不刪除該檔案。Load 與 Save 的 JSON 大小上限為
64 MiB，Load 會拒絕重複（含大小寫別名）、未知欄位、尾端資料、破損 checksum
與非有限／形狀錯誤資料。

此 atomic hardlink 與目錄 `fsync` 流程需要檔案系統提供相應能力；不支援的環境
會回傳錯誤，不宣稱已完成耐久發布。

## 驗收

- [x] 原子寫入與不可覆寫，未完成檔/破損/未知版本/形狀及快照內拓撲錯誤拒絕。
- [x] 真正新程序恢復後與連續執行比較輸出、參數、最佳化器、游標與資料序列。
- [x] episode trainer 狀態隔離，不互相共用可變切片。
- [x] 取消清理暫存，並行同一路徑寫入只有一方成功。

## 驗證

- `go test ./checkpoint`：pass
- `go test -race ./checkpoint`：pass
- `go test -count=10 ./checkpoint`：pass
- `go vet ./checkpoint`：pass
- subprocess helper 會在新程序 Load 中途快照、接續訓練並另存，再逐項比較完整 snapshot、Adam moments、predictions、cursor 與 SplitMix64 sample sequence。


## 本輪：分離狀態、安全保存與個體隔離

對應原始 COR-01、STA-02、STA-04。STA-01 的三種完整輸出與 STA-03 的含快速權重／化學／資料來源恢復另依原需求驗收。

公開流程：`NewIndividual(config, parameters, options, initial)` 建立獨立個體 → `Advance` 依持續神經狀態接續觀察 → `Snapshot` 在整次操作完成的同步點取出狀態 → `checkpoint.SaveIndividual`／`LoadIndividual` → `RestoreIndividual` 接續。`TrainEpisode` 明確使用既有獨立序列訓練，更新此個體的參數／最佳化器，保留持續推論的神經狀態，不宣稱跨段梯度學習。

共用契約：

- `dynamics.State` 保存版本、動態設定 SHA-256、步數、電位與延遲所需輸出史。歷史按時間排序，保留 `max(0, steps-maxDelay)` 至 `steps`，零時刻以前保持初始輸出。整段與任意分段推進必須相等。
- `learning.IndividualSnapshot` 的 Config、Parameters、Neural、Optimizer 分開擁有、可各自 JSON 保存。Optimizer 只含 Options、AdamState、Updates。完整 snapshot 另保存 profile／完整設定指紋，未知版本、精度或生命週期均拒絕。
- 個體提供 `ResetNeural`、`ResetParameters`、`ResetOptimizer` 三個明確操作，各自只改命名的部分。不可變 Config／原始 Graph 沒有就地重設 API，改接線須建立新個體。`ResetParameters` 保留最佳化器，若使用者也要清除動量須明確呼叫 `ResetOptimizer`。
- 目前 profile 僅含已實作的純量連續 CPU 模型，核心 float64，Insyra 輸入與讀出 float32，確定性運算不使用隨機流。快照不虛構尚未實作的放電、化學、快速權重、教師或重播欄位，未知 profile 不得讀成此模式。
- 新快照 schema 與 episode 格式分開。共用既有同目錄暫存、檔案同步、排他 hardlink 與目錄同步實作，保留原格式語意及錯誤。
- voltage、history 元素總量與每次輸出元素量各限制 1,048,576，新增歷史以前先檢查實際需求與整數溢位。JSON 沿用 64 MiB／64 層限制。這些是邏輯容量限制，不是程序 RSS 上限。

| 操作與需求 | 正常驗收 | 空值／錯誤及同步驗收 |
| --- | --- | --- |
| COR-01 狀態分離與重設 | 實際訓練後，各部分可分開保存；逐一重設只改指定部分；GraphStore 訓練前後 SHA-256 相同 | nil、空序列、形狀、非有限值、取消、延遲／步數溢位拒絕，失敗保持原狀 |
| STA-02 安全同步點與檔案 | Advance／TrainEpisode／Snapshot 競爭仍取得完整同步狀態；保存後新程序能接續持續神經輸出 | 截斷、中斷暫存、版本／profile／設定／形狀／校驗不符拒絕；並行保存只有一方成功；取消清理且不覆寫 |
| STA-04 個體隔離 | 同模型建立兩個個體，只訓練一方，另一方所有狀態保持不變；並行推進與學習通過 race | 建構輸入、匯出 snapshot 與恢復結果均無可變陣列共用；零值物件不 panic，修改操作回錯 |

- [x] COR-01：分離、選擇重設與解剖原件不變。
- [x] STA-02：同步快照、檔案拒絕與真正新程序接續。
- [x] STA-04：學習／推進隔離、快照深複製與 race。

驗證入口採公開 SDK 與 checkpoint 新程序模式。Mac／Ubuntu v25 同源 137 檔的完整檢查及 27 個相關頂層測試／範例均通過，Windows 交叉編譯通過。詳見 [驗證記錄](../../evidence/cpu-reference-20260914/verification-v25.json)。核心與 learning 留有實作前失敗證據，checkpoint 分工未遵守先測試順序，整合失敗記錄不得冒充實作前證據，詳見 [測試時序](../../evidence/STA-02/test-order.json)。

載入時，最新歷史輸出與電位經活化函數的計算值最多容許相鄰四個 float64 值的差異（4 ULP），不改寫保存的歷史。Go 1.26.5 的 Mac arm64／Ubuntu amd64 實測曾出現 tanh 1 ULP、softplus 2 ULP 差異；此容許範圍只用於狀態驗證，不保證跨處理器後續運算逐位元相等。

## 後續缺口：舊 episode 必填欄位

P1，下一個優先修正。`checkpoint.Load` 的舊 episode 格式會將缺失或明確 `null` 的 `data_seed` 等 scalar 解碼為零；重算 checksum 後仍會接受，可能改變後續資料序列。新 `LoadIndividual` 已拒絕這類輸入。修正時先加入重現測試，驗證省略／null 的 seed、cursor、updates、最佳化器設定與遮罩不得靜默變零，同時保留舊 Save 合法輸出的零邊 nullable 陣列、可省略 InputNodes／Delays，以及明確合法零值。此項不計為另一條原始需求。

重現：將 [probe](../../evidence/STA-02/episode-presence-probe.go.txt) 複製為 Git 外的 `.go` 檔，在 module 內以 `go run /ABS/PROBE.go` 執行。[實際日誌](../../evidence/STA-02/episode-presence.log) 確認 seed 1001 因缺失／null 被讀為 0。
