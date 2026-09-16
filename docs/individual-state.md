# 持續個體、重設與保存

`learning.Individual` 讓同一份基礎模型建立多個隔離個體。每次 `Advance` 接續上一段神經狀態，回傳每一步讀出。參數、最佳化器及神經狀態由個體各自擁有。

連續核心與 LIF 放電核心都支援持續個體，核心使用 float64，Insyra 編碼器與讀出使用 float32。兩種核心各有自己的 profile：連續是 `continuous-f64-insyra-f32-persistent-inference-episode-learning/v1`，LIF 是 `lif-f64-insyra-f32-persistent-inference-episode-learning/v1`。設定與保存格式固定運算精度及生命週期，未知版本會回傳錯誤。

## 推進與學習

以 `NewIndividual(config, parameters, options, initialVoltage)` 建立個體，`config` 選 `Dynamics` 或 `LIF` 其中一種核心。`config`、`parameters` 與初始電位會複製，建立另一個個體時可再次使用相同資料。也可以用 `checkpoint.NewIndividualFromPackage(pkg, options, initialVoltage)` 由模型包建立，結果與用模型包裡的設定、參數直接呼叫 `NewIndividual` 相同。

- `Advance(ctx, observations)` 接續持續神經狀態，回傳所有步的讀出，參數與最佳化器保持不變。空序列、錯誤形狀、非有限值及取消都會返回錯誤，整次操作不更新任何步數。
- `TrainEpisode(ctx, observations, target)` 沿用既有 `Trainer.Step`，每次從零電位開始獨立訓練序列。它只更新此個體的參數與最佳化器，保留持續推論的神經狀態。下一次 `Advance` 使用更新後的參數及保留下來的歷史。
- `Snapshot()` 等候目前操作完成，再複製完整狀態。快照與輸出陣列可由呼叫者修改，不影響個體。呼叫期間不要同時修改傳入的陣列。

`TrainEpisode` 的梯度不跨越 `Advance` 呼叫。LIF 個體多一組可訓練的基礎閾值參數 `ThetaRaw`，由 `Trainable.Theta` 決定是否訓練；慢速穩定與短期適應是狀態不是參數，反向視為常數。此確定性模式沒有內部隨機產生器、教師或重播資料；快速可塑性與化學調節是要各自明示開啟的可選機制，未開啟時前向路徑與從未宣告逐位相同。資料提供者的游標與隨機來源由呼叫者另外管理，完整訓練工作恢復依 STA-03 後續實作。

## 四種資料與明確重設

| 資料 | 保存位置 | 重設行為 |
| --- | --- | --- |
| 解剖與運算設定 | 原始 `connectome.Graph`／GraphStore，快照的 `Config` 保存動態及輸入／讀出索引 | 設定不可變，更換接線須建立新個體 |
| 基礎參數 | `snapshot.Parameters` | `ResetParameters(ctx, parameters)` 只換參數，保留神經狀態與最佳化器 |
| 個體神經狀態 | `snapshot.Neural`（聯集：`core` 指名核心，`continuous` 或 `lif` 只出現宣告的那一半） | `ResetNeural(ctx, initialVoltage)` 清除步數與舊歷史，建立新的初始狀態 |
| 訓練器狀態 | `snapshot.Optimizer` 中的 Options、AdamState、Updates | `ResetOptimizer(ctx, options)` 更換設定並清除動量與更新步數，保留參數與神經狀態 |

各部分可用 Go JSON 分開保存。若要接續同一個體，使用下述完整個體快照，由框架一併檢查各部分。重設參數後是否清除舊動量，由呼叫者明確選擇 `ResetOptimizer`。

`ConfigHash` 是完整運算設定的 SHA-256，涵蓋輸入／讀出選取。它不是生物來源證據，原始 GraphStore 的來源回條與外部神經元 ID 映射需另外保留。

## 神經狀態的兩種形狀

`snapshot.Neural` 是一個聯集：`core` 指名這份狀態屬於哪個核心，兩半只會出現宣告的那一半，缺一半或同時出現兩半都會被拒絕。

| 核心 | `core` | 保存內容 |
| --- | --- | --- |
| 連續 | `continuous` | 版本、設定指紋、步數、電位、延遲所需的輸出史 |
| LIF | `lif` | 版本、設定指紋、步數、電位、延遲所需的突觸跡歷史、適應值、不應期計數，慢速穩定開啟時另有活動估計與閾值偏移 |

慢速穩定關閉或未宣告時，活動估計與閾值偏移兩個陣列根本不存在，載入時也要求它們不存在。

聯集出現以前寫出的連續個體檔案，`neural` 直接是連續狀態本身，`LoadIndividual` 會把那種形狀讀成 `core: continuous`，重新保存時寫出聯集。

## 化學調節狀態

`EnableChemistry(config)` 為個體開啟化學調節層，設定會依這個個體的節點數完整檢查；再呼叫一次會換掉宣告並把濃度歸零，已宣告的資源與仍在排隊的回饋則保留。`DisableChemistry()` 關掉整層並丟掉濃度、資源與回饋佇列，之後的前向路徑與從未開啟過逐位相同。

開啟後，每次 `Advance` 或 `AdvanceGated` 的每一列都走同一個固定順序：

1. 每個來源依這一列的模型步數釋放速率。來源看到的活動是「上一步的節點輸出」，全新個體的第一列沒有上一步，所以是不存在而不是 0；回饋先經 `signal.AvailableFeedback` 以同一個步數過濾。
2. 濃度前進一步。
3. 依新的濃度算出每個受體在每顆細胞上的佔用率。
4. 佔用率變成這一列的增益、偏移與閾值陣列。
5. 核心用這組陣列走一步。
6. 局部可塑性若啟用，接在核心那一步之後。

`ChemistryReport()` 回報最近一次推進的結果：步數、最後一步的濃度與佔用率、`unresponsive`／`unknown_skipped`／`assumed` 的計數（描述最後一步的佔用率紀錄）、三個夾限計數（整次呼叫累加）與每通道的釋放總量。推進失敗時什麼都不提交，上一份報告原樣保留；未啟用時回傳零值。

`SetResource(name, value)` 宣告任務目前持有多少某項資源，`OfferFeedback(f)` 把一筆回饋排進佇列。兩者都需要先開啟化學層，回饋必須使用 `model_step` 時間單位。佇列是宣告而不是消耗品：回饋抵達後會一直可用，也會進快照。

快照多一個可選區塊 `chemical`：

| 欄位 | 內容 |
| --- | --- |
| `config` | 完整宣告：濃度動力學與可選傳輸、每通道一個來源、受體集合、效果、節點對區域的對應 |
| `state` | 每區域每通道的非負濃度與步數 |
| `resources` | 已宣告的資源名稱與數量 |
| `pending_feedback` | 仍在佇列中的回饋 |

未開啟時整個鍵不存在，這就是「這一層從未啟用」的表示；`checkpoint` 視它為可選區塊，出現時四個欄位都必填，形狀或值不合法一律拒絕。**來源讀的「上一步活動」不另外保存**：它就是持續神經狀態的延遲歷史裡最新的那一列，所以中途保存再接續，與不中斷執行的濃度、佔用率與輸出逐位相同。

暫時效果不進 `Parameters`。增益、偏移與閾值是單次核心呼叫的引數，跑一百步之後基礎參數（含 `theta_raw`）逐位不變。`TrainEpisode` 的前向與反向目前不接調節，所以獨立 episode 訓練看不到化學層。

## 保存與讀回

`checkpoint.SaveIndividual(ctx, path, individual.Snapshot())` 保存新格式 `coimnet-individual-checkpoint/v1`。`LoadIndividual` 讀回後，以 `learning.RestoreIndividual` 建立隔離個體，再呼叫 `Advance` 即可接續。

保存使用同目錄的 0600 暫存檔，寫入並同步後以排他 hardlink 發布，再同步目錄。目的檔已存在時拒絕覆寫。檔案系統須支援 hardlink 與目錄同步；發布之後才發生錯誤時，錯誤訊息會說明檔案已發布或耐久性未確認。

載入檢查 SHA-256、版本、profile、完整設定指紋、參數／最佳化器形狀及神經歷史。既有 `Save`／`Load` 與 CLI `train delayed`／`resume` 繼續使用獨立 episode 格式。新個體格式另拒絕缺失的必填欄位與 `null`。

## 三種保存物

| 保存物 | schema | 有什麼 | 沒有什麼 |
| --- | --- | --- | --- |
| 模型包 | `coimnet-model-package/v1` | 拓撲指紋、完整設定、基礎參數、宣告的時間單位、證據路徑清單、可以建立哪些 schema | 持續神經狀態、最佳化器動量、更新次數、資料游標 |
| 個體快照 | `coimnet-individual-checkpoint/v1` | 設定、參數、持續神經狀態、最佳化器選項／動量／更新次數，以及可選的 `plastic` 與 `chemical` 區塊 | 資料 seed 與樣本游標 |
| 訓練快照 | `coimnet-episode-checkpoint/v1` | 訓練器完整狀態、資料 seed、下一筆樣本位置 | 持續神經狀態 |

六種交叉載入都會被拒絕，錯誤訊息會指名讀到的是哪一種。模型包的訊息另外說明它只有設定與參數，不是個體快照，要用 `NewIndividualFromPackage` 建立新個體而不是拿它恢復。

模型包的拓撲指紋是節點數、邊數，以及「sources 全部再 targets 全部、各以 little-endian uint32 編碼」後的 SHA-256，與 `simulate` 報告裡的 topology hash 同一種編碼。它認的是保存下來的邊陣列，同一組邊換個順序就是不同的指紋。載入時會重算並比對，不符就拒絕。單位字串（例如 `model_step`）只是宣告，框架不做任何換算，也不代表毫秒或任何量測到的生物時間尺度。證據路徑只檢查是相對路徑且不會跳出紀錄集，框架不會去開啟它們。

可執行的人工 SDK 範例：

```sh
go test ./checkpoint -run ExampleSaveIndividual -count=1 -v
```

## 延遲與容量

連續狀態保留 `max(0, steps-maxDelay)` 到目前步數的輸出，零時刻以前使用初始輸出。歷史長度為 `min(steps, maxDelay)+1`，剛建立個體時不會因很大的延遲值立刻配置整段歷史。LIF 狀態用同一條規則保留突觸跡歷史。

`dynamics.MaxStateValues` 為 1,048,576。電位元素數、保留歷史總元素數、每次輸出元素數各受此上限限制，`Individual.Advance` 的輸入／編碼／核心／選取／讀出矩陣也各受同一上限限制。超限會回報錯誤，不會縮小圖或丟棄仍需要的歷史。JSON 檔上限 64 MiB，巢狀深度上限 64。

持續神經狀態的 float64 數值儲存量為 `8 × N × (min(steps, maxDelay)+2)` bytes，另有每列與物件配置、參數、最佳化器及每次運算的暫存。LIF 個體每顆神經元再多適應值、不應期計數，以及慢速穩定開啟時的活動估計與閾值偏移。這是算術估算，不能當成程序記憶體上限。完整 BPTT trace 的資源仍依[資源說明](resources.md)另計。

載入時，最新歷史輸出與電位經活化函數的計算值最多容許相鄰四個 float64 值的差異（4 ULP），不改寫保存的歷史。Go 1.26.5 的 Mac arm64／Ubuntu amd64 實測曾出現 tanh 1 ULP、softplus 2 ULP 差異；此容許範圍只用於狀態驗證，不保證跨處理器後續運算逐位元相等。
