# 線上邊界唯讀查驗

## Problem

目前 `OnlineLearner` 的內部多步驟操作只由 `OnlineLearner.mu` 串行；外部仍可直接呼叫同一個 `Individual` 的公開操作。需確認兩種邊界是否會把答案、參數版本或 score=0 的損失權重接到不同模型上。

## Confirmed

1. **`episodePredict → weightedTarget → TrainEpisode` 存在可插入點。** `learning/online.go:397-405` 先用 `episodePredict` 得到 `P0`，`learning/online.go:457-467` 對 `score=0` 計算 `w=0`、`E=P0`，之後才呼叫 `TrainEpisode`。`episodePredict` 在 `learning/online.go:498-515` 返回時已釋放 `Individual.mu` 與 `Trainer.mu`。外部公開 `Individual.TrainEpisode` 可在 `learning/individual.go:833-851` 取得同一把個體鎖後完成一次獨立訓練。

   使用既有 continuous fixture 的具體值：`input=[[0.7],[0],[0.2]]`、外部訓練 `target=[0.4]`、線上回饋 `score=0`。事件順序為：

   ```text
   P0 = F(theta0, input)
   E  = P0 + 0 * ([0.4] - P0) = P0
   external Individual.TrainEpisode(input, [0.4]) -> theta1
   online Individual.TrainEpisode(input, E) -> F(theta1, input)
   ```

   若 `F(theta1,input) != P0`，最後一次訓練的 residual 不是零，因此 score=0 的線上步驟可有 `GradientNorm > 0`、`UpdateNorm > 0`。這是鎖定順序可行的語意 TOCTOU，並非未保護的記憶體競態。現有註解明定 `w=0` 應留下零 residual；ticket 81 的驗收也要求 fresh optimizer、`WeightDecay=0` 時 score=0 參數逐位不變（`ENG.md:245-251`、`docs/individual-state.md:45`、`docs/tickets/81-chemical-online-loss-weight.md:12-18`）。

2. **`Act → Advance → Snapshot` 也存在參數版本錯接。** `learning/online.go:254-265` 先鎖線上學習器，呼叫 `Individual.Advance`，再另一次呼叫 `Individual.Snapshot` 算 `ModelVersion`。`Advance` 在 `learning/individual.go:466-469` 只保護該次操作，並於 `learning/individual.go:605-614` 提交持續神經狀態；它不改基礎參數。`Snapshot` 在 `learning/individual.go:394-402` 重新取個體鎖。外部可在兩次個體操作之間呼叫公開 `TrainEpisode`（`learning/individual.go:829-851`）：

   ```text
   V0 = SHA256(parameters theta0)
   Act -> Advance(input) -> output O0, generated with theta0
   external TrainEpisode(input, [0.4]) -> theta1, V1 != V0
   Snapshot -> parameterVersion(theta1) = V1
   ActionRecord{Output: O0, ModelVersion: V1}
   ```

   `ActionRecord` 的公開註解明定 `ModelVersion` 是「產生這個答案的參數」指紋（`learning/online.go:91-102`）。因此若上述插入成立，完整發生位置必須同列 `online.go:261`、`individual.go:466-469`、`online.go:265`、`individual.go:394-402`、`individual.go:829-851`，不能只報最後的 hash 不同。

3. **既有鎖契約只保證單次操作邊界。** `Individual` 的文件寫「Methods serialize at complete operation boundaries」（`learning/individual.go:129-133`），工程契約則寫所有操作共用同一把鎖（`ENG.md:197-200`）。未找到 `OnlineLearner.Update` 內部 `episodePredict`、加權與 `TrainEpisode` 的跨操作交易保證，也未找到 `Act` 將 `Advance` 和 `Snapshot` 綁成一個不可插入區段的保證。故目前已確認「可插入且會讀到新狀態」的來源條件，尚未僅憑文件把外部直接操作在這些區段的行為定性為已承諾的併行 API。

## Needs investigation

- **是否屬公開契約違反需 Root 以重現結果裁決。** 若允許呼叫端在同一 `Individual` 被 `OnlineLearner` 綁定期間直接呼叫 `Advance`／`TrainEpisode`，上面兩條都是缺陷：第一條破壞既有 `score=0` 權重契約，第二條破壞 `ActionRecord.ModelVersion` 契約。若既有契約只允許每個 `Individual` 一次一個完整高階流程，則目前證據只證明缺少跨操作 atomicity 說明，不能擴張成資料競態或要求立即加交易層。
- **普通核心的外部 `Advance` 本身不會造成 score=0 的參數錯接。** `Advance` 保持參數不變（`docs/individual-state.md:11-12`），所以需要外部 `TrainEpisode`，或開啟化學後讓 `Advance` 改變下一個 episode 的 boundary。化學分支會在 `learning/individual_chemical_training.go:64-110` 依當下 boundary 建立訓練 episode，應另以化學 fixture 實測；本輪沒有執行 Go 或新增正式測試。
- **`ActionRecord` 的版本錯接沒有現成公開序列測試。** `learning/online_test.go:505-536` 只驗證更新後的下一個 Act 反映新參數，`learning/online_test.go:538-558` 只驗證無插入時的 hash；兩者都沒有在 `Advance` 與 `Snapshot` 之間插入外部訓練。

## Reproduction draft and SHA

- 草稿：[reproduction.go.txt](/Users/timlai/Developer/coimnet/runs/.tmp/online-boundary-audit-20261014/concurrency/reproduction.go.txt)
- SHA-256：`ea5db63d51786a08bde17c00eedc42992d82e69583aba67b219083f1183574d5`
- 草稿只描述暫時 overlay seam，不是正式測試：
  - `Act` 在 `online.go:263` 後、`online.go:265` 前呼叫同步 hook，插入外部 `TrainEpisode(input,[0.4])`，比對 `V0`、`V1` 及 `ActionRecord.Output`。
  - `apply` 在 `online.go:403` 後、`online.go:405` 前呼叫同步 hook，插入外部 `TrainEpisode(input,[0.4])`，以同一 snapshot 的 control 個體比對 `theta1` 與含 score=0 線上步驟的 `theta2`。
  - score=0 的 no-hook control 應保持參數逐位相同，避免把既有 loss-weight 行為誤判成插入缺陷。

## Suggested validation

Root 可在指定環境下執行既有回歸與暫時 overlay：

```text
go test -count=1 -run 'TestLossWeightScalesTheTargetResidual|TestOnlineChemicalWeightUsesTheExplicitEpisodePrediction|TestActReflectsTheUpdate|TestActionRecordNamesTheModelThatProducedIt' ./learning
go test -count=1 -race -run 'TestOnlineScoreZeroInterleaveTrainEpisode|TestActModelVersionInterleaveTrainEpisode' ./learning
```

第二行的兩個名稱只存在草稿，必須由 Root 在暫時測試工作區 materialize；本輪沒有執行。

## Unverified

- 尚未取得實際 `V0`、`V1`、`P0`、`GradientNorm`、`UpdateNorm` 或兩份參數快照的逐位差異。
- 尚未驗證化學開啟時「外部 `Advance` 改 boundary → score=0 仍產生梯度」的具體數值。
- 尚未判定專案是否要支援外部直接操作與 `OnlineLearner` 共享同一 `Individual` 的跨高階操作併行；這是契約裁決，不應由本次唯讀查驗自行補定。
- 未執行 Go、build、cache、正式測試、race、vet 或任何外部傳送。

## Problem / Done / Changed / Actions / Verified / Notes / Next

- **Problem：** 查驗線上更新邊界與 `ActionRecord.ModelVersion` 是否能因外部公開操作而讀到不同參數／狀態。
- **Done：** 完成指定來源、互斥鎖、現有契約與相鄰回歸測試唯讀查驗；確認兩個可插入區段，保留 score=0 與版本錯接的同步重現草稿。
- **Changed：** 只新增 `runs/.tmp/online-boundary-audit-20261014/concurrency/reproduction.go.txt` 與本報告。未修改 production、正式測試、docs 或 evidence。
- **Actions：** 僅計算指定來源與草稿 SHA-256；未 commit、stash、reset、build 或發送 repo。
- **Verified：** 來源基準 commit `5f26c932981319627e3b76a3e8d47a5ae741c05e`；`learning/online.go` SHA `e4b67dbdb037e8ca4aebbb5ae6a4f6a8ecb5c0ff9ebbc7ab768e491470893e7d`；`learning/individual.go` SHA `e432bfdb794be69953a1d3b7f76ab4de61099f44f5934c407c678213b4dca4de`；`learning/trainer.go` SHA `0a2f2cce3a72e00df4198767553f4a5c58785b3d3eaaae2af1b3731c95928943`。完整清單與契約行號見本報告前段。
- **Notes：** 目前是來源級可行性與重現設計，不是已完成的 runtime 數值驗收。原子性邊界與是否允許共享 `Individual` 仍需 Root 決定。
- **Next：** Root materialize 草稿、記錄 `V0/V1/P0/GradientNorm/UpdateNorm` 與參數逐位差異，再決定是否建立正式失敗測試或先補公開契約說明。

Skills：無
