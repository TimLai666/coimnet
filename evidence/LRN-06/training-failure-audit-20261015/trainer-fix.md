# Trainer overflow fix

## Problem

`learning/trainer.go` 的 `stepWithGradient` 在累積視窗中先將有效 mask 的梯度加入 `accumulator.Sum`，未立即拒絕有限值以外的結果。有限輸入相加溢位時，partial window 可能成功提交不合法的 accumulator。

## Done

在 `learning/trainer.go:460-466`，每個有效 mask 的 `Sum[i] += v` 後立即呼叫既有 `finite` 檢查。失敗時回傳 `zero` `StepResult`，錯誤包含參數索引。未新增 helper、公開 API/schema，也未改變平均、clip、Adam 順序。候選 accumulator 尚未寫回 `tr.accumulator`，因此拒絕不會保留部分累積狀態。

## Verified

- 修改前 `learning/trainer.go` SHA256：`0a2f2cce3a72e00df4198767553f4a5c58785b3d3eaaae2af1b3731c95928943`。
- 修改後 `learning/trainer.go` SHA256：`404e2d932e369c48ee15c69c0f249b860ec7732135d26616ce34a376922ea686`。
- `learning/accumulate_overflow_test.go` SHA256：`e1057f58c02821bb4ff2600f3a4cdf98c4a63f6d2b4a68f786c57d9c1c6ae134`，未修改。
- 執行 `gofmt -w learning/trainer.go`，成功。
- 執行指定兩個 `TestAccumulation` 測試，exit code `0`，輸出 `ok github.com/TimLai666/coimnet/learning 1.026s`。

## Unverified

- 倉庫內兩份 `tests-frozen-v3.json` 的 SHA 分別為 `ca3289c8c2114edc75d74e8014bf4cfd79d85518aaeb860c94b54dcd850c2ab7`（`evidence/LRN-06/training-failure-audit-20261015`）與 `c9ababe13dfda5518c74149f5f2c6027c47359cf1d08a685a7f515e2a3b75fec`（`evidence/LRN-09/stateful-continuous-ppo-20261006`），均非指定的 `e1057f58...`，未修改；指定 manifest 的實際路徑未知。
- 未執行完整測試矩陣、GPU 路徑、race、vet 或其他測試；本次只驗證指定的兩個測試。未對未執行項目作通過宣稱。
