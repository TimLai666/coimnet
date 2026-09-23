# 28 — 研究者可以在二維環境訓練導航與位置記憶、建立配對／缺失／非同步多模態資料、讓多任務共用同一核心，並比較核心、外圍與接線本身的貢獻

Epic：任務與多模態；同一核心的能力歸因

User Story：研究者可以在可重現的二維環境裡用有限視野與隱藏狀態訓練導航與目標記憶，評估時不洩漏最短
路徑或看不見的目標座標；可以建立同一實體的多模態配對資料，缺失不是零值、實體 ID 不進模型，並評估未見
組合；可以讓多個模態與任務共用同一顆核心而不是暗建多個核心，每次結果都有拓撲與參數指紋；可以用凍結、
移除後重訓、重新接線、一般網路與預算匹配對照說出核心、外圍與接線各自的貢獻。

Blocked by：26 第二階段（`LossGradientFrom`、gridnav 的 PPO 骨架）、23（評估協定）、22 第三階段
（容量匹配對照）、14（保留度數的重連）

Status：draft（契約已於 2026-09-15 定案；分四階段派工）

對應需求：TSK-08（有限觀察、隱藏狀態、碰撞、轉向、目標記憶與新地圖測試完整）、TSK-10（實體 ID 不直接
成答案；缺失與零值分離；配對與未見組合可評估）、TSK-09（模型與狀態指紋可查，不為各任務暗建獨立核心）、
TSK-12（凍結、移除後重訓、重新接線、一般網路與預算匹配對照可執行）。主規格 2.1、3.2、13.1、13.6、
14.1–14.5、18.3。

## Root 決策（2026-09-15）

### 第一階段：二維導航環境（TSK-08）

1. **環境（新套件 `experiment/nav2d`）**：格狀地圖（預設 9×9，牆以 seed 生成，保證起點到目標連通）、
   位置 `(x, y)`（整數格、單位 `cell`）、方向 `heading ∈ {0, 90, 180, 270}` 度、有限視野（前方 3 格的
   錐形，牆與目標只有在視野內才出現在觀察）、行動 `forward|turn_left|turn_right|stay`、碰撞（撞牆不動並
   計數）、獎勵（到達 `+1`、每步 `−0.01`、碰撞 `−0.05`）、終止（到達）與時間上限（預設 60 步）分開
   回報。`Observation` 型別只含：視野內的格子種類、`heading`、是否碰撞、語言目標 token（任務 4）；
   **沒有**絕對座標、目標座標、最短路徑欄位（型別層面保證；另有汙染測試：把評估器的 `Info{ShortestPath,
   GoalXY}` 填成 NaN 標記，模型輸出仍有限）。座標系、單位與相對／絕對規則寫進套件文件。
2. **四個任務**：`remember_goal`（目標在第 0 步可見一次後隱藏）、`avoid_obstacles`（目標常見，牆隨機）、
   `adapt_after_change`（同一 episode 中途換地圖，觀察含「規則改變」位元）、`language_goal`（4 個 token
   對應 4 個角落）。專家為 BFS 最短路徑（只供模仿標籤，評估時不可取得）。訓練沿用 26 的模仿與 PPO。
3. **評估與對照**：報告成功率、平均步數、碰撞數、回報、**新地圖**（未見 seed）表現；對照組：隨機策略、
   無記憶控制器（前饋，只看當步觀察）、重新接線對照（14 的 `degree_preserving_rewire` 版核心，同參數量
   同預算）。3 seed，全部保留。
4. 證據：`evidence/TSK-08/`；CLI `examples run nav2d`。

### 第二階段：多模態配對資料（TSK-10）

5. **資料契約（新套件 `multimodal`）**：`Sample{EntityID, EventID string; Modalities map[string]Modality;
   Timestamps map[string]signal.Timestamp}`，`Modality{Present bool; Values []float64; Shape []int}`；
   `Present = false` 時 `Values` 必須為 nil（不是全零；載入時檢查），adapter 產生的模型輸入為
   「值通道 + presence 通道」（沿用 03 的缺值規則），測試證明「缺少音訊」與「全零音訊」進模型後
   的輸入不同。`EntityID`／`EventID` 預設**不**進模型：adapter 沒有編碼它們的路徑；只有評估器讀
   （用來配對與分組）。同步配對（同一 `EventID` 同一時間戳）與非同步配對（同一 `EntityID` 不同事件）
   都支援。
6. **Fixture 生成器**：`multimodal/synthetic`：可控形狀（影像：8×8 的幾何圖形）、文字（token 序列描述
   形狀與顏色）、聲音事件（短 WAV 區塊，頻率對應形狀）、位置（2-D）；未見組合分割器：訓練集排除特定
   （形狀, 顏色）組合，評估集只含它們（`SplitUnseenCombinations`）。
7. 評估：配對檢索（給一個模態找同實體的另一模態）與未見組合泛化，報告分開；「共同概念形成」只由這兩
   個測試判斷，不由視覺化斷言（文件明寫）。證據 `evidence/TSK-10/`。

### 第三階段：同一核心的多任務（TSK-09）

8. **共用核心**：`experiment.MultiTask{Core checkpoint.ModelPackage（唯一一份）; Tasks []TaskBinding{Name;
   Adapter{Encoder, Readout 版本}; Generator; LossScale, MixWeight, SamplingRatio float64; UpdateEvery
   uint64}; Schedule string ∈ interleaved|same_experience|missing_modality; Budget uint64}`；每個測試
   個體是獨立 `learning.Individual`（測試：兩個個體的 `NeuralState` 指標不同且互不影響）；每次結果記錄
   `brain_topology_hash`（16 的指紋）、`base_parameter_hash`（參數 SHA-256）、`state_lineage`（個體
   ID + 父快照 hash 鏈）、任務識別、adapter 版本；測試：所有任務的結果檔 `brain_topology_hash` 與
   `base_parameter_hash` 完全一致（同一顆腦），且沒有第二個核心被建立（`Network` 建構計數）。
9. **不讓最大任務無聲主導**：每個任務的更新份額（`updates_per_task / total`）與宣告的 `SamplingRatio`
   比對，偏離超過宣告容差就在報告標旗；損失尺度、混合權重、取樣比例與更新頻率全部寫進報告。
10. 證據：`evidence/TSK-09/`（fixture：導航 + 延遲關聯兩個任務共用核心，模態缺失排程）。

### 第四階段：能力歸因對照（TSK-12）

11. **對照矩陣（`experiment.RunAttribution`）**：至少七組，同資料、同 seed（≥ 3）、同更新預算、參數量
    記錄：`normal`（核心與外圍都學）、`frozen_core`（只學外圍）、`core_only`（固定低容量外圍，只學核心；
    主規格 18.3 的核心確實學習證明）、`generic_matched`（參數量相近的一般遞迴網路：隨機拓撲、同邊數）、
    `rewired`（14 的保留度數重連，檢查邊方向、重複、自我連線、作用符號與連通性並寫進報告）、
    `ablated_retrained`（移除核心後重新訓練的外圍）、`capacity_matched_modulator`（22 第三階段）。
    報告每組任務指標、參數量、預算、活動變化；事前選定的比較方法與不確定性區間（26 的
    `PreRegistered`）；文字結論只有一句固定範本：「凍結／移除造成退步證明的是依賴性，不是原始接線的
    優越性」（主規格 14.4）。
12. **未達標的排查順序**：`core_only` 沒學會時，報告自動列出五項檢查結果（輸入可達性：輸入節點到讀出
    節點是否連通；梯度範數；訊號尺度；狀態重設；答案洩漏測試），**不**自動加大外圍解碼器（文件與測試：
    `RunAttribution` 沒有任何會改外圍容量的路徑）。
13. 證據：`evidence/TSK-12/`（fixture：導航任務，七組 × 3 seed）；CLI `examples run attribution`；
    README 科學界線段落加「固定編碼器不等於能力歸因已完成」（主規格 3.2）。

### 第四階段補充決策（2026-09-23，主 agent）

14. **fixture 核心**：七組對照都建在第一階段的導航策略上（輸入節點 → 隱藏遞迴區塊 → 讀出節點，另有輸入到讀出的直連；讀出矩陣同時讀隱藏節點與讀出節點），每觀察呈現 3 列、只在最後一列計損失，訓練一律用 `trainNav2D`（示範暖身後一半回合是 DAgger，模仿損失依標籤反頻率加權），偏置在每一組都不訓練。這個核心是隨機稀疏遞迴區塊，不是果蠅接線；對照矩陣證明的是「七組都能以同資料、同 seed、同排程執行並回報」，真實接線的能力歸因是另外的研究結果，不在本票宣稱。
15. **七組的精確定義**（參數數量全部寫進報告）：
    - `normal`：核心權重與外圍（encoder、readout）都學。
    - `frozen_core`：核心權重固定在初始化值，只學 encoder 與 readout。
    - `core_only`：外圍固定為低容量：encoder 單位矩陣、readout 只讀讀出節點且為單位矩陣（第 j 個讀出節點就是第 j 個動作的 logit，隱藏列固定為 0），只學核心權重。
    - `generic_matched`：同樣的節點分組與同樣的邊數，邊的起點從輸入與隱藏節點、終點從隱藏與讀出節點均勻抽取（不重複），訓練方式同 `normal`；參數數量與 `normal` 相同。
    - `rewired`：隱藏遞迴區塊做保留出入度的雙邊交換（第一階段的 `nav2dRewire`），其餘同 `normal`，附重連檢查報告（方向、重複、自我連線、連通性；本 fixture 的邊沒有作用符號，報告寫明）。
    - `ablated_retrained`：移除核心（沒有輸入與隱藏節點、沒有任何核心邊），觀察經 encoder 直接進讀出節點，從頭訓練 encoder 與 readout，排程與回合數同其他組。
    - `capacity_matched_modulator`：`normal` 加一個不帶調節作用的附加容量：讀出額外讀取前 k 個輸入節點，k = ceil(P / 動作數)，P 是 MOD-10 預設可訓練控制器的參數量（視窗 3、讀回饋、4 個隱藏單元，P = 21），附加列初始化為 0；報告同時寫 P 與附加的實際參數量（差距小於動作數）。它檢驗「多出一個調節器大小的容量」本身帶來多少差異。
16. **比較方法（事前選定）**：以 `normal` 為基準，對每組在未見地圖上的成功率與專家一致率，取同 seed 的配對差，做 1,000 次配對 bootstrap、95% 區間（`pairedBootstrap`，亂數種子取第一個 seed）；三個 seed 的區間很寬，報告照實寫，不挑最好的一次。結論只有一句固定文字：「A drop after freezing or removing the core shows dependence on it, not that the original wiring is superior.」
17. **`core_only` 沒學會時的排查**：訓練後已見地圖一致率沒有高於訓練前時，報告自動附五項檢查：輸入可達性（輸入節點經核心邊到每個讀出節點的 BFS）、核心權重在訓練前後的實際位移量（有沒有任何梯度到達核心）、訊號尺度（探針序列上核心輸入與讀出節點輸出的平均絕對值，標出飽和或消失）、狀態重設（同一輸入兩次推論逐位相同）、答案洩漏（策略只收到觀察前綴，型別檢查）。`RunAttribution` 沒有任何會加大外圍的設定或程式路徑。

## 契約摘要

```go
package nav2d
type Env struct { ... }; func New(c Config, seed uint64) (*Env, error)
func (e *Env) Reset() Observation; func (e *Env) Step(a Action) (Observation, float64, bool, Info, error)

package multimodal
type Sample struct { EntityID, EventID string; Modalities map[string]Modality; Timestamps map[string]signal.Timestamp }
func SplitUnseenCombinations(samples []Sample, holdOut []Combination) (train, eval []Sample, error)

package experiment
func RunMultiTask(ctx context.Context, m MultiTask) (MultiTaskReport, error)
func RunAttribution(ctx context.Context, a AttributionConfig) (AttributionReport, error)
```

## 驗收

- [x] 第一階段：環境不變量測試（連通、碰撞、視野、時間上限與終止分離）、四任務各 3 seed、對照三組、
  新地圖、觀察不含評估器狀態（型別 + 汙染）；`go test`、race、vet；`evidence/TSK-08/`。
- [x] 第二階段：缺失 ≠ 零、ID 不進模型、同步／非同步、未見組合分割與評估；`evidence/TSK-10/`。
- [x] 第三階段：指紋一致、個體不共用、更新份額報告、三種排程；`evidence/TSK-09/`。
- [x] 第四階段：七組對照各 3 seed、重連檢查、排查清單、不自動加大外圍、README 界線；`evidence/TSK-12/`。

## 第一階段證據（2026-09-23）

`DefaultNav2DConfig`（seed 1、2、3，每個 seed 60 個訓練回合、各 20 個評估回合，隱藏 16、遞迴入度 4、學習率 0.05）在四個任務 ×
四種策略上跑完 48 個 run，沒有失敗。環境的 12 個測試（連通、手畫地圖的視野、撞牆原地不動並扣 0.05、到達與時間上限分開回報、
觀察型別沒有座標與最短路徑欄位且評估器欄位填 NaN 時輸出仍有限、四個任務各一）和策略與套件的 18 個測試在 race 下全部通過，
vet 沒有輸出。未見地圖上三個 seed 的平均：三種學習策略訓練後的專家一致率 0.305–0.401，隨機策略 0.243–0.254；遞迴策略每回合
碰撞從訓練前的 26–32 次降到 0.9–6.1 次；成功率仍低（0.00–0.25）。3 個 seed × 20 回合下，成功率差 0.05 只是每個 seed 差一回合，
本票也沒有事前選定策略之間的比較方法，所以不宣稱哪個策略比較好。重連對照的 12 份報告都保持出入度與方向、沒有重複邊與
自我連線、讀出可達。同一份報告在另一個 commit、不同負載下重跑，位元組相同。完整資料見
[TSK-08 證據](../../evidence/TSK-08/verification.json)。

已知限制：前饋對照每個觀察更新一次，更新次數是遞迴策略的 25–28 倍，參數 2664 對 2728，沒有對齊最佳化步數；報告的
`config.env` 記的是呼叫端的零值，不是補完後的預設。第 4 點的 CLI 是 `coimnet examples run nav2d`（預設取 `DefaultNav2DConfig`）。

## 第二階段證據（2026-09-23）

資料契約的測試在 race 下全部通過：`EntityID`／`EventID` 沒有進模型的路徑；缺少的模態必須是 nil 值，模型輸入帶存在通道，
「缺少音訊」和「全零音訊」進模型後不同；同一事件的同步配對與同一實體的非同步配對都支援；未見組合分割互斥且完整，
留出的組合從不進訓練。`DefaultConfig`（180 筆、音訊缺失率 0.2，留出 (shape 2, colour 1)，模型 seed 1、2、3，6 個 epoch）
三個 seed 都完成：已見組合的影像→文字檢索從訓練前 0.105、0.148、0.148 升到 1.000、1.000、0.716，文字→影像與音訊→影像形狀
都到 1.000。留出組合在整個評估集裡的影像↔文字檢索三個 seed 都是 0（訓練前後一樣），音訊→影像形狀是 1；影像頭說對形狀、
說錯顏色，文字頭剛好相反。所以配對與未見組合都評估了，模型學會已見的配對，但沒有泛化到留出的組合，本票不宣稱形成跨模態
的共同概念。同一 commit 重跑位元組相同。完整資料見 [TSK-10 證據](../../evidence/TSK-10/verification.json)。

已知限制：只有一個留出組合、18 筆未見樣本。CLI 是 `coimnet examples run multimodal`（預設取 `multimodaleval.DefaultConfig`）。

## 第三階段證據（2026-09-24）

走廊模仿與延遲關聯兩個任務共用一顆核心：輸入通道 0–3 給走廊觀察、通道 4 給延遲脈衝，輸出 0–2 是走廊動作、輸出 3 是延遲預測；每個任務的 adapter 是自己的 encoder 列與 readout 欄。`DefaultMultiTaskConfig`（seed 2、預算 400、容差 0.1）在 interleaved、same_experience、missing_modality 三種排程下都只建一個 trainer，每份結果的 `brain_topology_hash`（ticket 16 的模型包指紋）與 `base_parameter_hash` 都相同；三種排程的拓撲指紋一致，參數指紋因為訓練方式不同而不同。每個任務在自己的 `learning.Individual` 上評估（由同一份模型包建立、用 `Advance` 逐步行動），分數與 trainer 的 `PredictAll` 逐位相同；`state_lineage` 是「模型包、剛建立的個體、評估後的個體」三個雜湊，兩個任務前兩個相同、第三個不同，推進其中一個個體不會動到另一個。更新份額：interleaved 與 same_experience 都是宣告的 0.5／0.5；missing_modality 延遲任務拿到 400 個索引中的 300 個（0.429 對 0.5，在 0.1 容差內不標旗）；損失尺度、混合權重、取樣比例與更新頻率都寫進報告。兩個任務在三種排程下都有進步（走廊一致率 0.789 → 1.000，延遲 −MSE 從 −0.086 升到 −0.000004 至 −0.0136）；走廊 fixture 只有兩種回合形狀，所以這只說明共用核心能同時訓練兩個頭，不代表任務之間互相幫忙。race 對照 18 個通過，重跑位元組相同。完整資料見 [TSK-09 證據](../../evidence/TSK-09/verification.json)。

已知限制：兩個任務共用一個 AdamW 狀態，閒置任務的 adapter 也會被動量推動；每種排程只跑一個 seed；本階段沒有 CLI，證據走環境變數控制的測試。

## 第四階段證據（2026-09-23）

`DefaultAttributionConfig`（avoid_obstacles，seed 1、2、3，60 回合，同一份地圖與排程）七組 × 3 seed 共 21 個 run 都完成。
參數量照實寫進報告：normal、generic_matched、rewired 2728（可更新 2608），frozen_core 2728（1680），core_only 2728（928），
capacity_matched_modulator 2752（2632，附加 24 對 MOD-10 控制器 21），ablated_retrained 184（176）。重連組三份檢查報告
保持出入度與方向、沒有重複邊與自我連線、讀出可達。core_only 在 seed 1、2 訓練後已見地圖一致率沒有上升，報告自動附上五項
檢查（輸入可達、核心權重位移 10.65 與 10.76、訊號尺度沒有飽和或消失、狀態重設逐位相同、只收觀察前綴），seed 3 有學到所以
沒有附；`RunAttribution` 沒有加大外圍的路徑。事前選定的配對 bootstrap（1,000 次、95%，以 normal 為基準）：除了 core_only
在 seed 2 的一致率差 −0.238，各組各 seed 的差都 ≥ 0，例如 frozen_core 一致率 +0.294 [+0.035, +0.452]、ablated_retrained
+0.354 [+0.144, +0.479]。

這些正差主要來自 normal 本身沒學好：normal 同時學核心權重與 encoder，活動量從 0.25 升到 0.93，未見地圖成功率三個 seed 都是
0、一致率 0.091，是七組最低；只差在 encoder 不學的 TSK-08 遞迴策略，在同任務、同 seed、同排程下一致率是 0.367。所以這次
沒有出現「凍結或移除後退步」，不能據此說依賴核心，更不能說原接線比較好；報告只寫主規格 14.4 那一句固定結論。race 對照
15 個通過；第一次跑時碰到 go test 預設的 10 分鐘上限（負載 18），紀錄保留，加 `-timeout 0` 重跑通過。重跑報告位元組相同。
完整資料見 [TSK-12 證據](../../evidence/TSK-12/verification.json)。

已知限制：normal 在事前選定的學習率下沒學會，要改善得先在其他 seed 上重新事前選定學習率再重跑（見 AGENTS.md
Follow-ups）。第 13 點的 CLI 是 `coimnet examples run attribution`（預設取 `DefaultAttributionConfig`）。

## 依據

- 主規格 2.1（五類任務與共用核心）、3.2（固定編碼器不等於歸因完成）、13.1（共通完成條件）、13.6
  （二維環境、四類任務、專家標籤、評估不洩漏、座標與單位、對照組）、14.1（同一核心、指紋、個體不共用）、
  14.2（配對資料、ID 不進模型、缺失 ≠ 零）、14.3（共同訓練排程與權重、不讓最大任務主導、共同概念的
  判斷）、14.4（必備對照組、重連檢查、依賴性 ≠ 優越性）、14.5（多 seed、全部保留）、18.3（核心確實
  學習的證明與排查順序）。
