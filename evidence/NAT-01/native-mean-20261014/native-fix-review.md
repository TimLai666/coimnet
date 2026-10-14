# Ticket 94 candidate fix review

## Problem

審查 `simulate/runner.go` 的 native mean 修正與新增 `simulate/mean_range_test.go`，確認有限但中間加總溢位的 softplus 均值可表示、真正不可表示的 sum 仍失敗且不提交，並確認追蹤用的 `OutputSum` 維持原始總和契約。

本次只做來源與既有 evidence 的唯讀審查，未執行 Go、編譯或大量資源操作。

## Done

### Confirmed Findings

無。依目前程式碼、指定 evidence 與公開流程，未確認可列為 P1/P2 的缺陷。

- `reduce` 在 `/Users/timlai/Developer/coimnet/simulate/runner.go:820-865` 仍先依既有索引順序累加。只有 `scaleByOne` 且中間結果為無限值時，才改用最大絕對值縮放與補償加總。正常有限加總仍走原本的除法路徑，因此沒有證據顯示一般輸出位元會改變。
- `ReduceSumOutput` 在 `/Users/timlai/Developer/coimnet/simulate/runner.go:821-825` 不設定 `scaleByOne`，仍保留 raw sum 語意。`scaleByOne` 的設定位置 `/Users/timlai/Developer/coimnet/simulate/runner.go:395` 只涵蓋 mean 與 spike fraction，沒有用 mean 偷換 sum。
- `ReduceSpikeFraction` 雖共用 `scaleByOne`，但 `/Users/timlai/Developer/coimnet/simulate/runner.go:826-834` 只對每個 0/1 事件做 `sum++`，不具備此次 softplus 大輸出總和溢位的觸發條件，因此不會因 fallback 改變正常 fraction 結果。
- `observe` 在 `/Users/timlai/Developer/coimnet/simulate/runner.go:574-594` 逐步累加原始輸出，`trackedMeasurements` 在 `/Users/timlai/Developer/coimnet/simulate/runner.go:598-620` 將該值寫入 `WindowMeasurement.OutputSum`，而 `/Users/timlai/Developer/coimnet/simulate/metrics.go:346-348` 仍把它作為 `MetricMeanOutput` 的 raw numerator。候選修改只在 `/Users/timlai/Developer/coimnet/simulate/runner.go:753-760` 檢查是否可表示，未改成平均值。
- `Run` 仍把本地 `state`、`fast` 與 tracker 保留到最後，候選的 tracked sum 檢查位於 `/Users/timlai/Developer/coimnet/simulate/runner.go:753-760`，提交欄位在 761-763 行，因此檢查失敗時不會提交神經狀態、plastic state 或舊 measurements。
- softplus 的公開輸出在 `/Users/timlai/Developer/coimnet/dynamics/continuous.go:477-482` 為非負，tanh 有界；LIF readout 在 `/Users/timlai/Developer/coimnet/dynamics/lif.go:455-463` 是 synaptic trace。現有公開 core 沒有可觸發縮放補償之後混合符號、無界大數精度問題的具體路徑，因此不能把該假設列為確認缺陷。

### Evidence and test coverage

- `/Users/timlai/Developer/coimnet/simulate/mean_range_test.go:56-120` 以 Build → State → RestoreState → Run 驗證 equal MaxFloat、上界 MaxFloat、異質初始狀態、子集合與 selector，並用獨立 `math/big` 均值作比較。它也比較 mean runner 與 control runner 的 state，避免修正改變神經動力學。
- `/Users/timlai/Developer/coimnet/simulate/mean_range_test.go:122-134` 驗證真正 `ReduceSumOutput` 溢位回錯、回傳空 report，且 State、PlasticState、Measurements、輸入與參數未被提交。
- `/Users/timlai/Developer/coimnet/simulate/mean_range_test.go:136-184` 驗證追蹤輸出的 raw sum 可以序列化，並在連續 core 的下一次溢位時拒絕提交；LIF 分支至少覆蓋一次成功的追蹤與序列化。
- `/Users/timlai/Developer/coimnet/simulate/mean_range_test.go:186-205` 覆蓋 ordinary mean 與 allow-empty mean。指定 evidence `/Users/timlai/Developer/coimnet/evidence/NAT-01/native-mean-20261014/green.json` 記錄候選測試 13 個 pass，`red-v2.json` 記錄修正前同一測試集合有 8 個 failure，能區分這次缺陷與修正後行為。`normal-fixed.json` 與 `serialized-resume.json` 另記錄正常輸出及新程序恢復相容性通過。

## Needs investigation

- **[NEEDS INVESTIGATION] 普通輸出的精確位元回歸保護。** `/Users/timlai/Developer/coimnet/simulate/mean_range_test.go:186-203` 只檢查 ordinary mean 為 finite、empty mean 為零，沒有在永久單元測試中對普通輸出做 bit-for-bit baseline 比較。現有 `/Users/timlai/Developer/coimnet/evidence/NAT-01/native-mean-20261014/normal-fixed.json` 有相容性 evidence，但若要確認後續重構不誤入 fallback，需把既有正常輸出基準或 exact-bit assertion 接到持續測試流程。
- **[NEEDS INVESTIGATION] LIF 的真正 tracked raw-sum 溢位拒絕。** `/Users/timlai/Developer/coimnet/simulate/mean_range_test.go:157-164` 的 LIF 分支只做一次成功 Run 與 JSON marshal，沒有建立 LIF `OutputSum` 真正溢位後的 no-commit 案例。要完成這個覆蓋，需先以公開 Build → RestoreState → Run 路徑構造可達的 LIF sum overflow，再比對失敗前後的 State、PlasticState 與 Measurements；若受核心輸出與 limits 限制而不可達，應留下該不可達性的明確證據。

## Changed

| 檔案 | 變更摘要 |
|---|---|
| `/Users/timlai/Developer/coimnet/runs/.tmp/native-mean-20261014/review/fix-review.md` | 新增本次候選修正的唯讀審查報告。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| 唯讀 diff/status/source/evidence 審查 | CoImNet 工作樹 | 完成；未執行 Go 或編譯。 |
| Git 寫入、commit、stash、reset | 無 | 未執行。 |

## Next

若要補強測試，優先把普通 mean 的 exact-bit 相容性與 LIF tracked raw-sum overflow/no-commit 加入持續驗證。候選修正本身目前沒有需要阻擋合併的已確認缺陷。

## Reviewed files

- `/Users/timlai/Developer/coimnet/simulate/runner.go`
- `/Users/timlai/Developer/coimnet/simulate/mean_range_test.go`
- `/Users/timlai/Developer/coimnet/simulate/metrics.go`
- `/Users/timlai/Developer/coimnet/dynamics/continuous.go`
- `/Users/timlai/Developer/coimnet/dynamics/lif.go`
- `/Users/timlai/Developer/coimnet/evidence/NAT-01/native-mean-20261014/green.json`
- `/Users/timlai/Developer/coimnet/evidence/NAT-01/native-mean-20261014/red-v2.json`
- `/Users/timlai/Developer/coimnet/evidence/NAT-01/native-mean-20261014/normal-fixed.json`
- `/Users/timlai/Developer/coimnet/evidence/NAT-01/native-mean-20261014/serialized-resume.json`

## Unverified items

- 本次審查未重新執行 Go 測試、race、build、vet 或序列化恢復；上述結論中的測試結果取自指定 evidence。
- 尚未對 LIF 真正 raw-sum overflow 的可達性做執行驗證。

Skills：diff-inspector
