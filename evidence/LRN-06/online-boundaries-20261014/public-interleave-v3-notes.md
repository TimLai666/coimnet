# online boundary concurrency v3 診斷

日期：2026-10-14

## Problem

查驗公開 `OnlineLearner.Act`／`Update` 與同一 `Individual` 的外部
`Advance`／`TrainEpisode` 交錯時，是否會把輸出、`ModelVersion` 或 score=0
損失權重綁到不同的參數版本。這份 v3 仍是人工 overlay 診斷草稿，不是正式
測試，也沒有 production hook。

## Confirmed

- `learning/online.go:100-102` 對 `ActionRecord.ModelVersion` 的公開註解要求
  它是「產生這個答案的參數」的 SHA-256。`OnlineLearner.Act` 在
  `learning/online.go:267-281` 先 `Advance`，再在 `271` 由
  `Individual.Snapshot().Parameters` 產生版本。`Individual.Snapshot` 在
  `learning/individual.go:396-402` 只保護自己的單次快照；外部
  `TrainEpisode` 在 `learning/individual.go:829-850` 另一次取得同一把
  `Individual` 鎖。這留下 `Advance` 完成與 `Snapshot` 取版本之間的合法公開
  插入點。
- Root 已執行 v2 的 Act 診斷。`runs/.tmp/online-boundary-audit-20261014/logs/public-interleave-diagnostic.log`
  記錄 `V0=3a0ef20e20f448e675a20615778ce840b3914dc396408fd7eb8ac47110a94140`、
  `V1=da8d93f7c150eb2e2d5a70c3b90eafee1daaa7325ba4f4525bdda65d4b6d8d76`，
  Act `output` 與先前模型的 `oldOutput` 逐值相同，但
  `recordModelVersion=V1`。外部更新的數值是
  `gradient_norm=0.49764385010005635`、`update_norm=0.029999852819557984`、
  `updates=1`。因此「ActionRecord 的 ModelVersion 對不上產生該答案的
  參數」是已確認的來源級與執行級缺陷，不能只歸類為未承諾的跨物件原子性。
- `learning/online.go:475-485` 與 ticket 23 `:24-37`／`:33-36` 定義
  `weight=clamp(score,0,1)`，`weight=0` 時 weighted target 應等於 episode
  prediction，形成零殘差；`learning/trainer.go:517-572` 的實作即使
  `mean=0` 仍會遞增每個可訓練參數的 `AdamState.Steps`（`:522-528`）、提交
  `tr.updates++`（`:568-572`），而 `learning/individual.go:850` 再遞增
  `Episodes`。預設 `WeightDecay=0` 時，來源可推得參數不變、
  `GradientNorm=0`、`UpdateNorm=0`，但 optimizer／episode 計數會變；這是
  v3 修正 v2 錯誤 baseline 的依據。

## Needs investigation

- `OnlineLearner.Update` 在 `learning/online.go:355-390` 持有自己的 `l.mu`，
  `apply` 在 `:406-414` 先用 `episodePredict` 取得 P0，再用同一個 stale
  `prediction` 呼叫 `TrainEpisode`。外部 `TrainEpisode` 可在這兩個公開操作之間
  取得 `Individual` 鎖。`ENG.md:199` 與 `docs/individual-state.md:11-13`
  說明 Individual 內部操作共用鎖與各自的完成邊界，但沒有寫出
  `OnlineLearner` 與外部直接 mutator 的跨物件原子性承諾。因此目前只能確認
  插入點與加權公式的前提不一致，不能在 Root 實測前把 score=0 的併行結果
  單獨列為 confirmed。
- 若 Root 執行 v3 後看到 `externalFirst` 先套用 `.4`，再由
  `staleP0Extra` 產生非零梯度／更新，且實際完整快照等於
  `externalThenStaleP0` 控制，便得到決定性的公開呼叫序列。這表示 score=0
  的名義權重在跨操作插入下沒有保留零殘差；是否視為現有 loss-weighting
  契約違反，仍須先決定該契約是否包含跨 `OnlineLearner`／外部 mutator 的
  序列化要求。若沒有，這一項至少是文件未定義的競態邊界。
- v2 的 score=0 診斷在 fresh-zero baseline 以「參數或 optimizer state
  完全不變」失敗。該斷言與 `Trainer.stepWithGradient` 的零梯度 counter
  行為矛盾，不能作為 production failure 證據。

## v3 診斷草稿

`public_interleave_v3_test.go.txt` 延續 v2 的公開操作 fixture、
`holdContext` 呼叫計數、`queueContext` 與 `runtime.Stack` 探針。`Done` ack
只表示 `LockContext` 求值到 `ctx.Done`，不是 receiver 已入等待隊列；
`waitParkedTrainEpisode` 只有在同一 goroutine stack 同時出現 `[select]`、
本測試 wrapper、`(*cancellableMutex).LockContext` 與
`(*Individual).TrainEpisode` 時才繼續。它使用 bounded deadline 與
`runtime.Gosched`，沒有 sleep、FIFO 斷言或 production hook。

v3 的 score=0 區段有三組資料：

1. `freshzero`：從 `before` 還原，對同一個 P0 做 score=0 的
   `TrainEpisode`。只要求 `Applied=true`、兩個 norm 為 0、Parameters 相同，
   同時記錄 `AdamState` 是否相同、Adam Steps、Updates、Episodes 的前後值。
2. `externalFirst`／`staleP0Extra`：先外部 `.4`，再以 interleave 前的 P0
   重播 stale score=0 episode，建立完整 `externalThenStaleP0` 控制。實際
   interleave 必須與這個完整 optimizer／parameter／episode snapshot 相同。
3. `atomicFreshZero`／`atomicExternalAfterFresh`：先以 P0 做 fresh score=0，
   再做外部 `.4`。實際完整 snapshot 也與此控制比較，避免把 Adam 動量、
   Steps 或 Episodes 的正常變化誤判成 interleave 原因。

每個 score=0 診斷 log 會記錄 V0/V1/V2、輸出與 P0、兩個控制的 gradient／
update norm、Applied、Adam Steps、Updates、Episodes，以及 Parameters／Adam
state／完整 snapshot equality；不輸出巨大 snapshot。測試保留 `UpdateReport`
的 `Consumed=1`、`GradientSteps=1` 檢查。

Root 驗證命令：

```text
python3 runs/.tmp/online-boundary-audit-20261014/check.py public-interleave-v3-diagnostic go test -overlay <Rootoverlay> -tags online_boundary_diagnostic -count=1 -v -run '^TestPublicInterleave' ./learning
```

精確測試名稱是：

- `TestPublicInterleaveActAdvanceSnapshotVersion`
- `TestPublicInterleaveOnlineScoreZeroAfterEpisodePredict`

本輪沒有執行 Go、build、race、cache 或上述命令。

## SHA-256

| 檔案 | SHA-256 |
| --- | --- |
| `public_interleave_v3_test.go.txt` | `718980b1d6bb9514bd9bb250d1a25da17c27d842f0534f6a2757366aa98ba817` |
| `evidence/LRN-06/online-boundaries-20261014/plan.md` | `3a4ceff319c99028ca4dc27ca25fd4ff78f8a4ded53105d7530e1a1a943b2892` |
| `learning/online.go` | `2f742e1c266e018d4964bfb7b92b6eabaa543fb7c4043108ee51dc51ba4dae25` |
| `learning/individual.go` | `e432bfdb794be69953a1d3b7f76ab4de61099f44f5934c407c678213b4dca4de` |
| `learning/individual_chemical_training.go` | `dcf14d76ede60f16e36f48f63fff48124d63c645dbde55355e43b617972f399e` |
| `learning/trainer.go` | `0a2f2cce3a72e00df4198767553f4a5c58785b3d3eaaae2af1b3731c95928943` |
| `docs/individual-state.md` | `5b404f809517a27a92eeeae56dd70e6d643d66e5563d30f655bab598902dabf7` |
| `docs/tickets/23-online-learning-replay-and-adaptive-evaluation.md` | `20870c35d8e8d244a1eea0d6273954bfa56c4911fff4646065093f1856efe41e` |
| `ENG.md` | `7d495442e2d578fe3ccfda6068ff69e4dcb8970ce7e0f00d1cfc6a8890a06efe` |
| `logs/public-interleave-diagnostic.log` | `0e0dc1571856415ac53ded709e9966257c6ab08e9c9cdb1ff65bc180b54607e3` |

## 未驗證與限制

- v3 尚未由本輪執行；fresh-zero 的實際 norm、Adam state、Steps、Updates、
  Episodes，兩個控制的完整 equality 與 score=0 stale-P0 現象都待 Root log。
- `runtime.Stack` frame 名稱與 goroutine 狀態依 Go runtime 格式。草稿要求
  `[select]` 與完整 frame 集合，若 Root 的 overlay 編譯或 runtime 符號格式
  不同，診斷應回報無法證明 parked，而不是退回 Done ack 或 sleep 時序。
- 本草稿不是正式 RED regression，也沒有提出 production 修正；需先由 Root
  核對跨物件 atomicity 是否屬公開契約，再決定 score=0 結果的分類。
