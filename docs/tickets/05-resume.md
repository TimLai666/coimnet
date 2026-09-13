# 05 — 使用者可以中斷並接續參考訓練

Epic：狀態保存

User Story：使用者可以中斷並接續參考訓練

Blocked by：04

Status：partial（episode trainer 範圍完成）

## 交付

新增 `checkpoint.State`，保存獨立 episode trainer 的完整
`learning.TrainingSnapshot`、`delayed-pulse-splitmix64-v1` 資料 seed 與
sample cursor。`NewState` 要求 `NextSample == Training.Updates`，恢復時再由
`learning.RestoreTrainer` 驗證拓撲、形狀、最佳化器與有限值。

此 bounded 交付不包含 continuous individual 的 dynamics trace、神經狀態或
其他個體狀態；完整 continuous checkpoint 仍待後續 snapshot profile。

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
