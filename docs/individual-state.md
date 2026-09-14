# 持續個體、重設與保存

`learning.Individual` 讓同一份基礎模型建立多個隔離個體。每次 `Advance` 接續上一段電位與延遲輸出史，回傳每一步讀出。參數、最佳化器及神經狀態由個體各自擁有。

這個版本支援純量連續 CPU 模型，核心使用 float64，Insyra 編碼器與讀出使用 float32。設定與保存格式固定運算精度及生命週期，未知版本會回傳錯誤。

## 推進與學習

以 `NewIndividual(config, parameters, options, initialVoltage)` 建立個體。`config`、`parameters` 與初始電位會複製，建立另一個個體時可再次使用相同資料。

- `Advance(ctx, observations)` 接續持續神經狀態，回傳所有步的讀出，參數與最佳化器保持不變。空序列、錯誤形狀、非有限值及取消都會返回錯誤，整次操作不更新任何步數。
- `TrainEpisode(ctx, observations, target)` 沿用既有 `Trainer.Step`，每次從零電位開始獨立訓練序列。它只更新此個體的參數與最佳化器，保留持續推論的神經狀態。下一次 `Advance` 使用更新後的參數及保留下來的歷史。
- `Snapshot()` 等候目前操作完成，再複製完整狀態。快照與輸出陣列可由呼叫者修改，不影響個體。呼叫期間不要同時修改傳入的陣列。

`TrainEpisode` 的梯度不跨越 `Advance` 呼叫。此確定性模式沒有內部隨機產生器、快速可塑性、化學、教師或重播資料。資料提供者的游標與隨機來源由呼叫者另外管理，完整訓練工作恢復依 STA-03 後續實作。

## 四種資料與明確重設

| 資料 | 保存位置 | 重設行為 |
| --- | --- | --- |
| 解剖與運算設定 | 原始 `connectome.Graph`／GraphStore，快照的 `Config` 保存動態及輸入／讀出索引 | 設定不可變，更換接線須建立新個體 |
| 基礎參數 | `snapshot.Parameters` | `ResetParameters(ctx, parameters)` 只換參數，保留神經狀態與最佳化器 |
| 個體神經狀態 | `snapshot.Neural` | `ResetNeural(ctx, initialVoltage)` 清除步數與舊歷史，建立新的初始狀態 |
| 訓練器狀態 | `snapshot.Optimizer` 中的 Options、AdamState、Updates | `ResetOptimizer(ctx, options)` 更換設定並清除動量與更新步數，保留參數與神經狀態 |

各部分可用 Go JSON 分開保存。若要接續同一個體，使用下述完整個體快照，由框架一併檢查各部分。重設參數後是否清除舊動量，由呼叫者明確選擇 `ResetOptimizer`。

`ConfigHash` 是完整運算設定的 SHA-256，涵蓋輸入／讀出選取。它不是生物來源證據，原始 GraphStore 的來源回條與外部神經元 ID 映射需另外保留。

## 保存與讀回

`checkpoint.SaveIndividual(ctx, path, individual.Snapshot())` 保存新格式 `coimnet-individual-checkpoint/v1`。`LoadIndividual` 讀回後，以 `learning.RestoreIndividual` 建立隔離個體，再呼叫 `Advance` 即可接續。

保存使用同目錄的 0600 暫存檔，寫入並同步後以排他 hardlink 發布，再同步目錄。目的檔已存在時拒絕覆寫。檔案系統須支援 hardlink 與目錄同步；發布之後才發生錯誤時，錯誤訊息會說明檔案已發布或耐久性未確認。

載入檢查 SHA-256、版本、profile、完整設定指紋、參數／最佳化器形狀及神經歷史。既有 `Save`／`Load` 與 CLI `train delayed`／`resume` 繼續使用獨立 episode 格式。兩種格式使用不同的 schema。新個體格式另拒絕缺失的必填欄位與 `null`。

可執行的人工 SDK 範例：

```sh
go test ./checkpoint -run ExampleSaveIndividual -count=1 -v
```

## 延遲與容量

連續狀態保留 `max(0, steps-maxDelay)` 到目前步數的輸出，零時刻以前使用初始輸出。歷史長度為 `min(steps, maxDelay)+1`，剛建立個體時不會因很大的延遲值立刻配置整段歷史。

`dynamics.MaxStateValues` 為 1,048,576。電位元素數、保留歷史總元素數、每次輸出元素數各受此上限限制，`Individual.Advance` 的輸入／編碼／核心／選取／讀出矩陣也各受同一上限限制。超限會回報錯誤，不會縮小圖或丟棄仍需要的歷史。JSON 檔上限 64 MiB，巢狀深度上限 64。

持續神經狀態的 float64 數值儲存量為 `8 × N × (min(steps, maxDelay)+2)` bytes，另有每列與物件配置、參數、最佳化器及每次運算的暫存。這是算術估算，不能當成程序記憶體上限。完整 BPTT trace 的資源仍依[資源說明](resources.md)另計。

載入時，最新歷史輸出與電位經活化函數的計算值最多容許相鄰四個 float64 值的差異（4 ULP），不改寫保存的歷史。Go 1.26.5 的 Mac arm64／Ubuntu amd64 實測曾出現 tanh 1 ULP、softplus 2 ULP 差異；此容許範圍只用於狀態驗證，不保證跨處理器後續運算逐位元相等。
