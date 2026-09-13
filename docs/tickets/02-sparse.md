# 02 — 使用者可以計算稀疏傳播與梯度

Epic：數值參考

User Story：使用者可以計算稀疏傳播與梯度

Blocked by：無

Status：passed（稀疏算子範圍）

## 交付

提供 O(N+E) 稀疏算子，保留固定邊順序並支援 batch、向量與共享參數。

## API

```go
New(nodes int, sources, targets []int) (*Operator, error)
(*Operator).Forward(weights, input []float64, batch, channels int) ([]float64, error)
(*Operator).Backward(weights, input, gradOutput []float64, batch, channels int) (gradWeight, gradInput []float64, err error)
(*Operator).WithParameterIndex(parameterIndex []int) (*Operator, error)
```

輸入與輸出採 `[batch][node][channel]` 扁平順序。每條邊使用一個對所有 channel 廣播的純量權重；`WithParameterIndex` 讓多條邊共享權重並按固定邊順序累加梯度，權重長度由最大參數索引加一決定。建構與共享參數設定都複製可變切片，`batch` 與 `channels` 必須為正整數。

## 驗收

- [x] 規格第9.2節手算值全部吻合。
- [x] 隨機小圖與獨立稠密參考、平滑有限差分一致。
- [x] 零邊、孤立、自我連線、重複邊有明示語意。
- [x] batch、向量、共享梯度加總正確。
- [x] 非法形狀/索引/非有限值/容量溢位返回錯誤，不修改輸入。

## 驗證紀錄

- `go test ./internal/sparse`：通過，含手算、N×N 稠密參考、有限差分、batch、向量、共享參數、零邊、孤立、自我與重複邊。
- `go test -race ./internal/sparse`：通過。
- `go build ./...`：通過。
- `go test ./...`、`go test -race ./...`、`go vet ./...`：工作區目前被 `learning/` 僅含 `network_test.go` 且沒有非測試 Go 檔阻擋；`dynamics` 與 `internal/sparse` 測試本身通過。
- `go list -m all`：通過；未修改 `go.mod` 或 `go.sum`。
