# 混合線上更新重試查驗

日期：2026-10-14
基準：`5f26c932981319627e3b76a3e8d47a5ae741c05e`
範圍：只讀來源／契約與 SHA-256；未執行 Go、build、cache、正式測試、外部索引或 repo 傳送。

## Problem

`AllowGradient=true` 與 `AllowPlastic=true` 的同一筆回饋，若梯度半段已成功、塑性閘門半段第一次失敗，`Update` 會把回饋留在 pending，下一次只能重新呼叫同一個公開 `Update`。本查驗確認目前流程沒有保存「梯度半段已完成」的公開或私有階段狀態，因此 retry 可能重複梯度，或再次推進 `UpdateEvery`。

## 契約依據

- `docs/tickets/23-online-learning-replay-and-adaptive-evaluation.md:24-39` 定義公開 `Act → Receive → Update`、`UpdateEvery` 是每幾筆可用回饋做一次梯度更新、混合政策先做梯度再做塑性。
- `evidence/LRN-06/online-boundaries-20261014/plan.md:10-16` 要求部分成功不擴張成全交易回滾，且每個已完成階段只執行一次；本輪 `plan.md:20` 要求以小型呼叫序列取代大量訓練。
- `ENG.md:245-251` 明定 `GradientSteps` 只計 `StepResult.Applied`，成功但尚在累積的訓練仍消耗回饋，梯度失敗不提交間隔位置，並明寫混合梯度／局部可塑性的部分成功不做整體回滾。
- `docs/tickets/84-online-failed-interval-retry.md:8-18` 的修正範圍只涵蓋梯度失敗時不推進間隔，不涵蓋梯度成功後塑性失敗的階段重試。

## Findings

### [P1] [CONFIRMED] `UpdateEvery=1` 會在塑性閘門 retry 重跑同一筆梯度

觸發序列：

1. 建立可塑個體與 `OnlinePolicy{AllowGradient:true, AllowPlastic:true, UpdateEvery:1}`。
2. `RewardGate.Applied(0)` 回傳 `[]float64{0}, nil`，讓建構成功。
3. 公開呼叫 `Act`、`SetTargets`、`Receive`，回饋 `Score=1`。
4. 第一次 `Update` 中，`TrainEpisode` 成功，之後 `RewardGate.Applied(1)` 回傳一次性錯誤。
5. 用相同 `now` retry 同一個仍在 pending 的回饋。

程式證據：

- `learning/online.go:375-384` 只在 `apply` 成功後計入 `Consumed`，塑性錯誤時該回饋仍留在 queue。
- `learning/online.go:390-415` 先執行 `episodePredict`／`TrainEpisode`，成功後提交 `l.eligible`。
- `learning/online.go:419-433` 才呼叫 reward gate 與 `AdvanceGated`。因此第一次 gate 錯誤前，梯度及 `StepResult` 狀態已經保留。
- `learning/individual.go:823-851` 的 `TrainEpisode` 成功後保留參數、optimizer／accumulator 並遞增 `Episodes`；`learning/trainer.go:444-472,569-572` 顯示 `AccumulateSteps=2` 的 partial window 也會先寫入 accumulator。

可由目前程式直接推導的 deterministic 數值如下。這些是 Root 執行草稿前的預期值，不冒充已執行 log：

| 設定 | 第一次 `Update`（gate error） | retry 的目前結果 | retry 應保留的結果 |
| --- | --- | --- | --- |
| `UpdateEvery=1`, `AccumulateSteps=1` | `Consumed=0, GradientSteps=1, PlasticSteps=0, Deferred=0`; `Episodes=1, Updates=1` | 再次 `GradientSteps=1`，`Episodes=2, Updates=2`，再做塑性 | `GradientSteps=0, PlasticSteps=1, Consumed=1`; optimizer／參數與第一次失敗後相同 |
| `UpdateEvery=1`, `AccumulateSteps=2` | `Consumed=0, GradientSteps=0, PlasticSteps=0`; `Episodes=1, Accumulator.Count=1, Updates=0` | 同一梯度再加一次，`GradientSteps=1, Episodes=2, Accumulator=nil, Updates=1` | `GradientSteps=0, PlasticSteps=1, Consumed=1`; `Accumulator.Count` 仍為 1 |

這不是要求全交易回滾。第一次成功的梯度／累積器應保持，retry 只應完成尚未成功的塑性半段。重跑梯度違反 `plan.md:14` 的「每個已完成階段只執行一次」。

### [P1] [CONFIRMED] `UpdateEvery=2` 在同一筆部分成功回饋的 retry 會推進間隔，令下一筆提早做梯度

觸發序列：

1. 同樣建立混合政策，但 `UpdateEvery=2`。
2. 第一筆回饋的 gate 成功，作為間隔 skip，得到 `Consumed=1, GradientSteps=0, PlasticSteps=1`。
3. 第二筆回饋的梯度半段成功，gate 第二次正分數呼叫回錯。此時 `l.eligible` 已由 1 變 0，第二筆仍 pending。
4. retry 第二筆。現行程式再次計算 `nextEligible=1`，跳過梯度後 gate 成功，並把 `l.eligible` 寫成 1。
5. 第三筆回饋因此計算出 `nextEligible=0`，現行程式做梯度；若第二筆只完成一次，第三筆本應是 interval skip。

程式證據是 `learning/online.go:393-415`：`nextEligible` 在塑性 gate 前已寫入 `l.eligible`，而 `learning/online.go:419-433` 的 gate 錯誤不會回復它。這與 `UpdateEvery` 只按可用回饋位置計數及「同一已完成階段只執行一次」不相容。

預期數值（`RewardGate.Applied(0)` 成功，第二個正分數呼叫失敗，第三個正分數呼叫成功）：

| `AccumulateSteps` | 第一筆 skip | 第二筆首次失敗 | 第二筆 retry | 第三筆目前結果 | 第三筆應有結果 |
| --- | --- | --- | --- | --- | --- |
| `1` | `Consumed=1, GradientSteps=0, PlasticSteps=1` | `Consumed=0, GradientSteps=1, PlasticSteps=0`, `Episodes=1, Updates=1` | `Consumed=1, GradientSteps=0, PlasticSteps=1` | `GradientSteps=1` | `GradientSteps=0` |
| `2` | `Consumed=1, GradientSteps=0, PlasticSteps=1` | `Consumed=0, GradientSteps=0, PlasticSteps=0`, `Episodes=1, Accumulator.Count=1` | `Consumed=1, GradientSteps=0, PlasticSteps=1`，累積器仍 count 1 | `GradientSteps=1`，累積器關閉、`Updates=1` | `GradientSteps=0`，累積器仍 count 1 |

這個 finding 是 interval drift，不是把第一次成功的梯度回滾；修正設計仍需保留第一次梯度結果。

## 最小重現草稿與 exact tests

草稿：`runs/.tmp/online-boundary-audit-20261014/retry/online_mixed_retry_test.go.txt`。

草稿使用現有 `learning_test` fixture helper，公開流程只呼叫 `Act`、`SetTargets`、`Receive`、`Update`，並包含：

- `TestOnlineMixedRetryDoesNotRepeatGradientAfterGateFailure`：`UpdateEvery=1` × `AccumulateSteps=1/2`，預期在目前來源失敗。
- `TestOnlineMixedRetryDoesNotDriftUpdateEvery`：`UpdateEvery=2` × `AccumulateSteps=1/2`，預期在第三筆回饋的梯度計數斷言失敗。
- `TestOnlineMixedRetryControls`：無 gate error 的四格控制，預期梯度序列分別為 `[1,1]`、`[0,1]`、`[0,1,0,1]`、`[0,0,0,1]`，每筆 `Consumed=1` 且 `PlasticSteps=1`。

Root 可在不改正式測試的暫存副本執行：

```text
cp runs/.tmp/online-boundary-audit-20261014/retry/online_mixed_retry_test.go.txt learning/online_mixed_retry_tmp_test.go
gofmt -w learning/online_mixed_retry_tmp_test.go
go test -count=1 ./learning -run 'TestOnlineMixedRetry(DoesNotRepeatGradientAfterGateFailure|DoesNotDriftUpdateEvery|Controls)$'
rm learning/online_mixed_retry_tmp_test.go
```

這三個測試尚未由本 agent 執行；Root 應保存 stdout／stderr 與 exit code，並在執行後再把「source-confirmed」升級為「runtime-confirmed」或修正數值差異。

## Optional `AdvanceGated` 原子失敗路徑

目前未執行。可用 public `context` 控制設計：`RewardGate.Applied(score>0)` 第一次回傳合法 gate 前先 cancel 該次 `Update` 使用的 context，讓 `AdvanceGated` 走 cancellation；`learning/individual.go:617-820` 的 candidate `stepwiseResult` 與最後提交點 `605-614` 應使塑性／神經狀態不提交，但前面的梯度仍依部分成功契約保留。Retry 應只完成塑性階段。直接傳入錯誤長度 gate 的 `AdvanceGated` 或 exhausted episode 可作獨立 atomic controls，但不能取代本次 RewardGate 公開混合序列。

## Needs investigation

- `[NEEDS INVESTIGATION]` 需要決定公開契約是否要明確保存每筆 pending feedback 的梯度／塑性階段完成狀態。現有文件明定部分成功不回滾，但沒有公開的「只重試剩餘半段」操作；在未定案前不應直接加入整體交易回滾或新的保存欄位。
- `[NEEDS INVESTIGATION]` `AdvanceGated` cancellation／invalid gate／out-of-steps 的實跑報告尚缺，因本輪限制未執行 Go；不把它們列為 confirmed。

## SHA-256

來源與契約讀取指紋：

| 檔案 | SHA-256 |
| --- | --- |
| `learning/online.go` | `e4b67dbdb037e8ca4aebbb5ae6a4f6a8ecb5c0ff9ebbc7ab768e491470893e7d` |
| `learning/individual.go` | `e432bfdb794be69953a1d3b7f76ab4de61099f44f5934c407c678213b4dca4de` |
| `learning/individual_chemical_training.go` | `dcf14d76ede60f16e36f48f63fff48124d63c645dbde55355e43b617972f399e` |
| `learning/accumulate.go` | `f8abd731509d5a812c8ef61562b89ee5a1c8aae52e5e56807d6b9836464252c2` |
| `plasticity/plasticity.go` | `3e663a217f018266b09776416eefa5354ced8f6230bd62f8e01851a299a2a470` |
| `docs/tickets/23-online-learning-replay-and-adaptive-evaluation.md` | `20870c35d8e8d244a1eea0d6273954bfa56c4911fff4646065093f1856efe41e` |
| `docs/tickets/84-online-failed-interval-retry.md` | `90948355a1a19d9d2495a09d5ef74a334bab818e8e35f792c3625c57a638289e` |
| `ENG.md` | `eb38de811868d8e004513d0bc06ca44d62361af4b56e38263e1aa7c39ea75008` |
| `evidence/LRN-06/online-boundaries-20261014/plan.md` | `3a4ceff319c99028ca4dc27ca25fd4ff78f8a4ded53105d7530e1a1a943b2892` |

草稿指紋：

`online_mixed_retry_test.go.txt`
`d059091c56108e9a0d89926a395cb215559700823fa4770a6342bb3f71a49560`

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `runs/.tmp/online-boundary-audit-20261014/retry/online_mixed_retry_test.go.txt` | 新增只供 Root 暫存副本執行的 mixed retry RED/control 草稿。 |
| `runs/.tmp/online-boundary-audit-20261014/retry/report.md` | 新增來源證據、deterministic 預期值、exact tests、限制與 SHA-256。 |

## Actions

| 動作 | 對象 | 結果 |
| --- | --- | --- |
| 來源／契約讀取與 SHA-256 | learning、plasticity、ENG、ticket 18/19/20/23/84、online boundary plan | 完成；無外部索引、無網路、無 repo 傳送。 |
| Go/build/test/cache | 本專案 | 未執行，遵守本輪唯讀查驗限制。 |
| production／正式測試／docs／evidence 修改 | 本專案 | 未修改；只新建本 retry 暫存目錄內兩個檔案。 |

## Notes

目前兩項 finding 是由公開呼叫序列與目前 source path 確定的 deterministic failure；實際 runtime log 尚待 Root 依 exact tests 執行。未把 `AdvanceGated` optional 路徑或 retry 契約的 API 取捨列成 confirmed。

## Next

Root 執行上述三個 exact tests，保存失敗／控制 log 與 exit code；若數值相符，再建立 implementation ticket，先寫正式失敗測試並另行決定「每筆回饋階段完成狀態」的最小保存方式。
