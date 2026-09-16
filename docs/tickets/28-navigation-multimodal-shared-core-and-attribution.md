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

- [ ] 第一階段：環境不變量測試（連通、碰撞、視野、時間上限與終止分離）、四任務各 3 seed、對照三組、
  新地圖、觀察不含評估器狀態（型別 + 汙染）；`go test`、race、vet；`evidence/TSK-08/`。
- [ ] 第二階段：缺失 ≠ 零、ID 不進模型、同步／非同步、未見組合分割與評估；`evidence/TSK-10/`。
- [ ] 第三階段：指紋一致、個體不共用、更新份額報告、三種排程；`evidence/TSK-09/`。
- [ ] 第四階段：七組對照各 3 seed、重連檢查、排查清單、不自動加大外圍、README 界線；`evidence/TSK-12/`。

## 依據

- 主規格 2.1（五類任務與共用核心）、3.2（固定編碼器不等於歸因完成）、13.1（共通完成條件）、13.6
  （二維環境、四類任務、專家標籤、評估不洩漏、座標與單位、對照組）、14.1（同一核心、指紋、個體不共用）、
  14.2（配對資料、ID 不進模型、缺失 ≠ 零）、14.3（共同訓練排程與權重、不讓最大任務主導、共同概念的
  判斷）、14.4（必備對照組、重連檢查、依賴性 ≠ 優越性）、14.5（多 seed、全部保留）、18.3（核心確實
  學習的證明與排查順序）。
