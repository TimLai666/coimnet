# 保存、載入與替換輸入輸出映射

`signal.Projection` 保存外部通道與神經元之間的加權關係。輸入投影把通道值送到指定細胞，輸出投影從指定細胞的當下活動計算輸出。`signal.Mapping` 原有的外部 ID 與索引格式保持相容，適合只需要身份對照的用途。

可執行的人工兩節點範例位於 [ExampleBindProjections](../learning/projections_example_test.go)：

```sh
go test ./learning -run ExampleBindProjections -count=1 -v
```

範例建立輸入與輸出投影，將輸入投影轉成 JSON 再讀回，接上模型並訓練一步，最後使用新的輸出係數建立另一個訓練器。

## 選取與重建

呼叫 `NewProjection` 時，提供 `NeuronCandidate` 陣列，順序必須與模型核心的節點索引相同。每筆保存 `NeuronID`，也可提供已知的 `CellType` 與 `Region`。框架不會替空欄位推測生物註記。

| 選取方式 | 設定 | 係數對應順序 |
| --- | --- | --- |
| 明確 ID 集合 | `SelectExplicit` 與 `Selection.Neurons` | 呼叫者明示的 ID 順序 |
| 細胞類型 | `SelectCellType` 與 `Selection.Value` | 符合該值的細胞，依 namespace／external ID 字串排序 |
| 區域 | `SelectRegion` 與 `Selection.Value` | 符合該值的細胞，依 namespace／external ID 字串排序 |

任一方式都能提供明確 `Weights`，或使用 `ProjectionRandom{Seed, Scale}` 產生固定隨機係數。後者使用 `splitmix64-sign/v1`，每個係數等於正或負 `Scale`。Seed 可以是零，Scale 必須為有限正值。格式同時保存演算法、seed、scale 及實際係數，讀回時逐項核對重建結果。 細胞類型／區域投影的神經元順序須符合上述排序，載入時即檢查。

保存請呼叫 `json.Marshal(projection)`，讀取請使用 `signal.DecodeProjection(reader)`。JSON 會保存來源、人工標記、方向、通道形狀、選取條件、解析後的 ID 順序、神經元指紋、輸入／輸出維度與係數。神經元指紋是依序列化 ID 列表計算的 SHA-256，保護係數的身份對應，不包含整張圖或全部係數的校驗。

`Spec()` 返回建立投影的設定。將它傳給 `NewProjection` 會依當下候選資料建立一份新投影。要恢復已保存的同一份映射，使用 `DecodeProjection` 與 `Bind`，才能核對原先解析的細胞集合。

`Bind(candidates)` 重新找出實際圖索引。圖順序改變時，會把同一係數接回同一 ID。細胞消失、ID 重複或 metadata 選到不同集合時返回錯誤，使用者須明確建立新的投影。圖的連線與核心參數也必須依該圖的實際索引排列，單獨更動 candidate 陣列不能代替重新排列整個模型。

## 形狀與模型接合

`ChannelShape` 記錄原始通道形狀。將值攤平成一列數字以前，呼叫 `ValidateChannelShape` 核對完整形狀，`[2, 3]` 與 `[6]` 不會被視為相同。模型的 `InputSize`／`OutputSize` 則使用攤平後的數量。

輸入係數按列排列為 `[通道數, 選定輸入細胞數]`，輸出係數為 `[選定讀出細胞數, 通道數]`。`learning.BindProjections(config, parameters, candidates, input, output)` 同時檢查圖、方向、通道數與核心參數，成功後返回各自持有資料的 Config／Parameters。

`Config.InputNodes` 指定實際接收外部輸入的核心索引。編碼器只保存這些細胞的欄位，前向時將編碼結果填入完整核心輸入，反向時取回對應梯度。未指定 InputNodes 時維持原本全節點編碼器。輸出始終來自核心活動。

## 替換與接續訓練

替換時先取舊訓練器的 `Snapshot()`，將其中的 Config／Parameters 與新投影交給 `BindProjections`，再用結果呼叫 `NewTrainer`。這會保留核心參數，以提供的兩份投影替換整個 Encoder／Readout，並建立新的最佳化器。舊訓練器保持原狀。

投影物件保存的是建構時的係數，不會跟著訓練器更新。若要保留已學到的某一側係數，從投影的 `Spec()` 取出設定，再以快照中的 Encoder 或 Readout 更新 Weights、清除 Random 並記錄新的 Source，建立一份新的投影後綁定。原來的細胞集合與係數順序須一致。

固定隨機輸入投影若要在訓練時保持不變，將 `Options.Trainable.Encoder` 設為 false。固定輸出投影同理使用 `Trainable.Readout=false`。`DefaultOptions` 預設會訓練全部參數群組。

若只是中斷後繼續同一個訓練器，使用既有 `checkpoint.Save`／`Load` 與 `RestoreTrainer`，保留 InputNodes、全部參數與最佳化器。這個流程已以新程序續訓比對連續訓練。舊快照未包含 InputNodes 時保持全節點行為，新投影設定需要本版框架才能讀取。

## 來源與容量

人工感官映射須設 `Artificial=true`，並填寫可追溯的 Source。宣告 `Artificial=false` 時必須提供 Evidence。框架驗證欄位完整性，生物證據的正確性由建立映射的人查核。

每份投影最多 1,048,576 個係數，候選圖與選入集合最多 1,048,576 個神經元。JSON 最多 16 MiB、巢狀深度最多 64 層，拒絕未知或重複欄位、數值 null、非有限係數、非法 UTF-8、未配對的 Unicode surrogate 與形狀不符。合法的 Unicode 替換字元及成對的跳脫編碼可保存讀回。係數數量在隨機配置前檢查。這些限制不包含 Go 解碼暫存與程序 RAM，完整模型成本另見[記憶體估算](resources.md#訓練記憶體如何估算)。
