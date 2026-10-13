# Boundary 86 regression seam report

日期：2026-10-14

## Problem

Ticket 86 需要一個可長期保留、能證明修正有效的並行回歸測試。舊診斷依賴固定 `context.Err` 次數與 `runtime.Stack` 字串，不能作為正式同步方式。查驗範圍是 `Act` 的輸出／`ModelVersion` 配對，以及線上梯度更新在 `episodePredict` 與 `TrainEpisode` 之間被外部訓練插入時的加權一致性。

## Changed

只新增本報告：

| 檔案 | 變更摘要 |
| --- | --- |
| `/Users/timlai/Developer/coimnet/runs/.tmp/online-boundary-audit-20261014/boundary86/test-seam-report.md` | 記錄公開 seam 查驗、最小內部 seam、正式測試流程、預期完整狀態與限制 |

沒有執行 Go、build、race、CLI、下載或快取操作，也沒有修改 production、正式測試、docs、evidence 或既有 v3 草稿。

## Confirmed evidence

### 1. 兩個插入點是實際的鎖邊界

- `OnlineLearner.Act` 在 `/Users/timlai/Developer/coimnet/learning/online.go:260-291` 持有的是 `l.mu`。它在 `:267` 呼叫 `Individual.Advance`，在 `:271` 才從另一個 `Individual.Snapshot` 取參數版本。
- `Individual.advanceRows` 在 `/Users/timlai/Developer/coimnet/learning/individual.go:462-469` 取得 `i.mu`，並在 `:602-614` 完成輸出計算與持續狀態提交後解鎖。`Snapshot` 又在 `:396-402` 另行取得同一把鎖。外部 `TrainEpisode` 在 `:829-850` 再次取得 `i.mu`，所以舊碼確實允許 `Advance` 完成後、版本快照前的外部參數寫入。
- `ActionRecord.ModelVersion` 的公開契約在 `/Users/timlai/Developer/coimnet/learning/online.go:91-108`，明定 hash 是「產生這個答案的參數」。`ENG.md:197-203` 及 `/Users/timlai/Developer/coimnet/docs/individual-state.md:11-15` 也說明個體操作共用鎖、快照等候操作完成後複製完整狀態。
- `OnlineLearner.apply` 在 `/Users/timlai/Developer/coimnet/learning/online.go:393-428` 先於 `:406` 呼叫 `episodePredict`、於 `:410` 計算 `weightedTarget`、再於 `:414` 呼叫 `TrainEpisode`。`episodePredict` 自己在 `:512-532` 取得並釋放 `i.mu`；`TrainEpisode` 又在 `individual.go:833-850` 另取一次。因此外部 `TrainEpisode` 可在 prediction 與 training 中間改變參數。
- `/Users/timlai/Developer/coimnet/learning/online.go:475-505` 和 `/Users/timlai/Developer/coimnet/docs/tickets/23-online-learning-replay-and-adaptive-evaluation.md:24-39` 定義 `weight = clamp(score, 0, 1)`。`weight=0` 時 effective target 必須等於同一個 episode prediction，殘差為零；這不表示整個 optimizer state 不變。`Trainer.stepWithGradient` 在 `/Users/timlai/Developer/coimnet/learning/trainer.go:517-572` 仍會按可訓練 mask 遞增 Adam steps、提交 update counter，`Individual.TrainEpisode` 於 `individual.go:850` 遞增 episode counter。

### 2. 已有 root 執行證據確認舊行為

Root 已回報 v3 普通與 race 診斷均通過，並以完整 state control 排除了把正常 Adam counter 當成 score=0 缺陷。既有 `/Users/timlai/Developer/coimnet/runs/.tmp/online-boundary-audit-20261014/concurrency/v3-notes.md:14-37,69-84` 記錄的 Act 證據為 `V0=3a0ef20e20f448e675a20615778ce840b3914dc396408fd7eb8ac47110a94140`、`V1=da8d93f7c150eb2e2d5a70c3b90eafee1daaa7325ba4f4525bdda65d4b6d8d76`，舊輸出仍等於 `oldOutput`，但 `recordModelVersion=V1`。這是來源契約違反，不只是未承諾的跨物件原子性。上述 runtime 結果是 root 提供的證據，本輪沒有重跑。

### 3. 沒有足夠穩定的公開 callback seam

- `RewardGate` 是公開 callback，但 `/Users/timlai/Developer/coimnet/learning/online.go:22-49` 的 callback 只在 `apply` 完成梯度階段後於 `:434-447` 執行，無法停在 `episodePredict` `:406` 與 `TrainEpisode` `:414` 之間，也完全不涵蓋 `Act` 的 `Advance`／`Snapshot` 間隙。
- `context.Context` 只能提供取消。`cancellableMutex.LockContext` 在 `/Users/timlai/Developer/coimnet/learning/trainer.go:592-612` 的 `Done` case 沒有「receiver 已停在等待隊列」的公開確認。v3 的 `Done` ack 只表示 select 表達式正在求值，不能用來宣稱 goroutine 已排隊；用固定 `Err` 次數或 `runtime.Stack` frame 會把實作細節變成測試契約。
- `Intervene` 也不能當正式 seam。`/Users/timlai/Developer/coimnet/learning/intervene.go:65-69` 明定它跨越多個完整操作，不能與其他 mutating call 並行。
- 化學來源沒有可用的公開阻塞 callback。`modulation.Source` 雖在 `/Users/timlai/Developer/coimnet/modulation/modulation.go:25-47` 公開，但 `/Users/timlai/Developer/coimnet/modulation/config.go:27-55,57-145` 的 `SourceSpec.Build` 只接受五種固定 body，沒有 caller supplied source。Controller 只有宣告資料與 `Release`，見 `/Users/timlai/Developer/coimnet/modulation/controller.go:18-48,117-193`，沒有測試 callback。`learning/chemistry.go:168-232` 只在既定 chemistry row 內呼叫 source。不能為了同步而添加 production hook。

因此，若要求「舊公開呼叫在修正前必然行為 RED」且同時禁止固定 `Err` 次數、stack probe 及 production hook，目前沒有合法且穩定的公開 seam。v3 應保留為缺陷發現證據，不應改名成正式 regression。

## Proposed tests

### 最小可維護的內部 seam

在 `learning` package 內抽出既有 body 的 private held-lock helpers，讓 public wrapper 與 ticket 86 的 combined operation 共用。這不是 callback、field、exported API 或 runtime observer：

1. `advanceRowsLocked`：承接 `/Users/timlai/Developer/coimnet/learning/individual.go:470-614` 的現有 body；`advanceRows` 只負責 `i.mu.LockContext`／unlock。另以 `advanceVersionedLocked` 在同一把 `i.mu` 內完成 advance 與 parameter hash，`advanceVersioned` 只負責一次加鎖，`Act` 呼叫 combined operation。
2. `episodePredictLocked`：承接 `/Users/timlai/Developer/coimnet/learning/online.go:520-532` 的 prediction body，不再取得 `i.mu`。它仍依現有順序取得 `Trainer.mu`，化學路徑沿用 `/Users/timlai/Developer/coimnet/learning/individual_chemical_training.go:64-110`。
3. `trainEpisodeLocked`：承接 `/Users/timlai/Developer/coimnet/learning/individual.go:837-851` 的 overflow、ordinary／chemical training 與 episode counter。`TrainEpisode` 仍是 public wrapper，加鎖後呼叫它。
4. `trainWeightedEpisodeLocked`：在 caller 已持有 `l.mu` 與 `i.mu` 時，依序呼叫 prediction helper、`weightedTarget`、training helper。對外的 `trainWeightedEpisode` 只取得一次 `i.mu`。現有 `Update` 的 lock order 是 `/Users/timlai/Developer/coimnet/learning/online.go:362-365` 的 `l.mu` 再到 Individual，測試必須維持這個順序，避免引入反向鎖死。

這些名稱是 seam 形狀的具體建議，實作者可用等價 private 名稱。測試只在 `package learning` 存取 held-lock helpers，不能將 helper 匯出，也不應增加 production callback。這個選擇的代價是測試與 private helper 名稱耦合，但耦合面小且直接對應修正的鎖範圍。

### Test 1：版本化 Act 與外部 TrainEpisode

建議名稱：`TestOnlineVersionedActSerializesExternalTrainEpisode`。

同步流程：

1. 建立現有 online ordinary fixture，保存 `before := i.Snapshot()` 與 `V0 := parameterVersion(before.Parameters)`。建立一份 serial control，對同一個 `before` 只做外部 `TrainEpisode(ctx, input, []float64{.4})`。
2. 測試按 production lock order 取得 `l.mu`，再取得 `i.mu`。啟動一個 wrapper 先以 channel 回報「已開始呼叫」，再呼叫公開 `i.TrainEpisode`；此 channel 只證明 goroutine 已被啟動，不宣稱它已停在 mutex receiver。持鎖期間若 result channel 已完成則失敗，這只驗證 caller 仍持有鎖，不作 FIFO 或 parked 判斷。
3. 持有 `i.mu` 時同步呼叫 `advanceVersionedLocked`，取得 output 與 version，計算 output hash 並建立等價 `ActionRecord` 欄位。完成後先比較欄位，再釋放 `i.mu`／`l.mu`，最後以 bounded `select` 等待外部 `TrainEpisode` 完成。沒有 sleep、固定 `Err` 次數或 stack 字串。
4. 正確結果：記錄的 Output、OutputHash、ModelVersion 都對應 `V0`；外部步驟完成後的完整 `IndividualSnapshot` 必須等於 serial control 的 `before → TrainEpisode(.4)`。外部成功後的版本應與 `V0` 不同，若 fixture 的 `.4` 沒有改變參數，測試仍以完整 control equality 為準，不硬編參數差異。
5. 另用 restored twin 走一次 public `Act`，比對 `ActionRecord` 的 Output、OutputHash、ModelVersion 與 held helper 所得欄位，確認 wrapper 沒有重新從後續 snapshot 取 hash。`ActionRecord` 一次寫入且不被 feedback/update 改寫的既有契約仍由 `/Users/timlai/Developer/coimnet/learning/online_test.go:330` 附近既有 public test 覆蓋。

舊碼沒有 `advanceVersionedLocked`，所以正式 test 在 ticket 86 前會 compile RED；這是目前不允許 production hook 下唯一可穩定釘住「combined operation 必須存在」的來源級 RED。它不應被誤稱為舊公開競態的行為重現，v3 才是該行為的 diagnostic evidence。

### Test 2：weighted prediction／training 的完整 state 順序

建議名稱：`TestOnlineWeightedEpisodeSerializesPredictionAndTraining`，subtest 至少為 `score_0`、`score_half`、`score_1`。每個 subtest 使用同一個 `before`，另建三組 control：

- `freshThenExternal`：先以 P0 完成 online weighted episode，再以公開 `TrainEpisode(.4)`。
- `externalThenStaleP0`：先以公開 `TrainEpisode(.4)`，再使用 interleave 前保存的 P0 prediction 計算 effective target，重播 stale episode。這是舊碼錯誤序列的 negative control。
- `actual`：持有 `l.mu`→`i.mu`，啟動外部 `.4` wrapper，再呼叫 `trainWeightedEpisodeLocked`，完成後才解鎖並等外部結果。

每個 score 的預期：

| score | weighted step 的直接結果 | 解鎖後 actual 的完整 state |
| --- | --- | --- |
| `0` | `Applied=true`、`GradientNorm=0`、`UpdateNorm=0`、Parameters 與 weighted step 前相同；Adam Steps、Updates、Episodes 仍按來源正常遞增 | 逐欄等於 `freshThenExternal`，並且不等於可區分的 `externalThenStaleP0`；不能把 counter 變化誤報為參數移動 |
| `.5` | `StepResult` 與 fresh control 相同，effective target 是 P0 prediction 與 declared target 的中點 | `Parameters`、AdamState、Updates、Accumulator、Episodes、Neural、Plastic、Chemical 全部等於 `freshThenExternal` |
| `1` | `StepResult` 與 declared target 的 ordinary episode control 相同 | 同上，完整 snapshot 等於 `freshThenExternal`，不是 stale-P0 control |

`reflect.DeepEqual` 或等價逐欄比較必須包含 `IndividualSnapshot` 全部欄位，不能只看 Parameters。普通、`AccumulateSteps=2`、frozen mask、啟用 chemistry 各至少跑一個 subcase。化學 training 的 candidate／state commit 來源在 `/Users/timlai/Developer/coimnet/learning/individual_chemical_training.go:64-110`；mask 與 accumulation 的共同路徑在 `/Users/timlai/Developer/coimnet/learning/trainer.go:392-428`、`:517-572`。若任一 subcase 出錯，應保留該完整 snapshot 差異，不放寬成只比 norm。

取消控制沿用 package-internal 先持有 private mutex、再以 done channel 等候結果的 prior art：`/Users/timlai/Developer/coimnet/learning/trainer_cancel_test.go:48-106`。持鎖時以已取消 context 呼叫外部 `TrainEpisode`，預期 `context.Canceled`、Parameters、optimizer、Updates、Episodes 及其餘 snapshot 完整不變。這個控制只驗證錯誤路徑，不用來證明等待隊列。

### 為什麼舊碼會失敗

目前 source 沒有上述 held-lock helpers，因此正式測試先以缺少 seam compile RED。若 root 要求「同一份 test 在舊碼可編譯且必然行為失敗」，目前公開 API 沒有足夠同步點；只能重新引入 v3 的 fragile context／runtime probe，或新增 production hook，兩者都違反本任務限制。建議採 private helper seam 加 source review，並保留 v3 作為舊缺陷的獨立重現證據。

## Unknown

- 本輪沒有執行任何 Go 或 root overlay 命令，沒有宣稱正式測試已通過。
- 目前尚未實測 private helper 名稱與最終 lock scope；若實作者把 helper 拆分成不同名稱，測試應按同一 held-lock contract 更新。
- chemistry、frozen mask、partial accumulation 在新 combined helper 下的實際完整 snapshot equality 尚待 root 驗證。
- `Source.Release`、`RewardGate`、`Context.Done` 都沒有可攜且不依賴 runtime 實作的「已排隊」回呼；本報告不把任何 Done ack 或 goroutine 啟動 ack 當成 parked proof。
- 既有 v3 normal/race 結果由 root 提供，非本輪獨立重跑；其 hash 與本報告使用的來源 hash 見下表。

## Root exact verification command

新增正式 test 並採用上列名稱後，root 應先執行普通測試，再執行 race：

```text
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 -run '^(TestOnlineVersionedActSerializesExternalTrainEpisode|TestOnlineWeightedEpisodeSerializesPredictionAndTraining)$' ./learning
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -race -count=1 -run '^(TestOnlineVersionedActSerializesExternalTrainEpisode|TestOnlineWeightedEpisodeSerializesPredictionAndTraining)$' ./learning
```

這兩條命令是建議命令，本輪沒有執行。正式採用後仍需依 `/Users/timlai/Developer/coimnet/runs/.tmp/online-boundary-audit-20261014/boundary86/skeleton.md:17-25` 完成完整套件與既有 retry、chemistry、continuation 驗證。

## SHA-256

| 檔案 | SHA-256 |
| --- | --- |
| `boundary86/skeleton.md` | `d456f7079f61e0aee0d79d827677106bd622f40c41db2860506c4783912209e9` |
| `concurrency/v3-notes.md` | `41e9e3e6a724d14587ec13a44d87310abbf2cd72eb1d6dc240cd8025b851b610` |
| `concurrency/public_interleave_v3_test.go.txt` | `718980b1d6bb9514bd9bb250d1a25da17c27d842f0534f6a2757366aa98ba817` |
| `learning/online.go` | `2f742e1c266e018d4964bfb7b92b6eabaa543fb7c4043108ee51dc51ba4dae25` |
| `learning/individual.go` | `e432bfdb794be69953a1d3b7f76ab4de61099f44f5934c407c678213b4dca4de` |
| `learning/individual_chemical_training.go` | `dcf14d76ede60f16e36f48f63fff48124d63c645dbde55355e43b617972f399e` |
| `learning/trainer.go` | `0a2f2cce3a72e00df4198767553f4a5c58785b3d3eaaae2af1b3731c95928943` |
| `learning/trainer_cancel_test.go` | `5bd66c70479f533e3792e2da25fa56f472fbdab4a2372f33f6274630dfdd006f` |
| `learning/online_test.go` | `8b2cedcdbdec4cde142f15e50299c69356752a31b6a7f6e7534b8d3f49237bee` |
| `learning/intervene.go` | `ea68fd6a5eed8bc9c55cda737e24010216820d1dae0990b746a859a2eafbca4d` |
| `learning/chemistry.go` | `4e6164cbafd843c56c357558e9b8a580598b607daeccffbfc74a714052a52032` |
| `modulation/modulation.go` | `435e9f4eee501587161228dc27729010292269ff1feb1c01f5ee9d6613ce2e66` |
| `modulation/config.go` | `07afd7684f18afe02dd751ffd4069d0c44ec2a2eea0a7f6004f5a66427bb2ea0` |
| `modulation/controller.go` | `b46d2f8675071dd4ccee261c6a6be2c412b55a81acd4877834747e9f73e4da01` |
| `docs/individual-state.md` | `5b404f809517a27a92eeeae56dd70e6d643d66e5563d30f655bab598902dabf7` |
| `docs/tickets/23-online-learning-replay-and-adaptive-evaluation.md` | `20870c35d8e8d244a1eea0d6273954bfa56c4911fff4646065093f1856efe41e` |
| `ENG.md` | `7d495442e2d578fe3ccfda6068ff69e4cb8970ce7e0f00d1cfc6a8890a06efe` |

## Next

Root 可依本報告的 private held-lock seam 冻結正式 RED，完成 repair 後執行上列兩條命令；若拒絕 compile-red 而要求舊碼行為 RED，必須另行核准新增同步 seam，否則沒有可靠的公開方案。

Skills：investigate


