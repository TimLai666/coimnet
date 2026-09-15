# 23 — 使用者可以在運行中依新經驗學習、使用受限容量的重播，並做不污染保留資料的適應性評估

Epic：訓練與持續學習（運行中學習、重播、評估）

User Story：使用者可以讓一個持續個體在運行中先作答、再收到事後回饋、再依授權更新，已輸出的答案不被
後到的回饋或教師回應改寫；可以選擇不重播、容量受限的重播或按任務／時間維持比例的重播，每種都記錄
容量、抽樣、seed、隱私政策與移除規則；可以在事先宣告的適應分割上做允許的局部更新，然後在評分分割上
評估，題目之間依宣告政策重設快速權重與化學狀態，測試資料不進入重播也不用來調參。

Blocked by：18 局部可塑性、20 三分離與來源、21 化學狀態（重設政策要能重設它）、17 最佳化器（運行中
梯度更新只動授權參數）

Status：draft（契約已於 2026-09-15 定案；分兩階段派工，待 18／20 完成後第一階段可開始）

對應需求：LRN-06（推論、回饋與更新分開；輸出不被後到教師回應回寫）、LRN-07（容量、取樣、刪除政策、
狀態保留與不重播對照皆可查）、LRN-10（預先宣告可用回饋、適應／評分分割與狀態重設政策）。
主規格 6.3、7.2（運行中梯度學習、適應性評估兩種模式）、10.1、10.4、10.6、15.1（重播資料與索引進
快照）、15.5、16.2（`evaluate`）。

## Root 決策（2026-09-15）

### 第一階段：運行中學習迴圈（LRN-06）與重播（LRN-07）

1. **運行中學習迴圈（`learning` 套件）**：`OnlineLearner{individual *Individual; policy OnlinePolicy;
   log []ActionRecord; pending []signal.Feedback}`，`OnlinePolicy{AllowGradient bool（只在 Options 的
   遮罩範圍內）; AllowPlastic bool; UpdateEvery uint64（每收到幾筆可用回饋做一次梯度更新）;
   ClockUnit signal.TimeUnit}`。三個公開步驟，順序由型別強制：
   `Act(ctx, obs signal.Observation, input [][]float64) (ActionRecord, error)` 產生輸出並把
   `ActionRecord{ActionID string; Step uint64; Output [][]float64（唯讀副本）; OutputHash string;
   ModelVersion string}` 追加到 log；`Receive(ctx, fb signal.Feedback) error` 只接受
   `ActionID` 已存在於 log、`ProducedAt ≥ 該行動的時間` 且 `AvailableAt ≥ ProducedAt` 的回饋，否則拒絕
   （偽造更早可取得時間的回饋被拒）；`Update(ctx, now signal.Timestamp) (UpdateReport, error)` 只用
   `signal.AvailableFeedback(pending, now)` 過濾後的回饋，依政策做梯度（把回饋分數映射為損失權重
   的規則明示：`loss_weight = clamp(score, 0, 1)`，目標由訓練器內部持有的 `Target` 給，不由回饋給）
   或可塑性閘門（`gate = mapper.Map(score).Applied[0]`，用 20 的 `RewardMapper`）。
   **不可回寫**：`ActionRecord.Output` 與 `OutputHash` 在 `Receive`／`Update` 後逐位不變（測試：
   先 `Act`，保存 hash，再送回饋與更新，再比對）。`Update` 之後的 `Act` 才會反映更新。
   測試模式：`OnlinePolicy.Evaluate = true` 時 `Update` 拒絕任何梯度或可塑性更新並回
   `ErrEvaluateMode`，教師相關來源（TCH 票）在此模式不得被呼叫的規則以介面文件與測試釘住。
2. **重播（`replay` 新套件）**：`Buffer{Capacity int; Sampling string ∈ uniform|task_balanced|
   time_balanced; Eviction string ∈ fifo|reservoir; Seed uint64; Privacy PrivacyPolicy{StoreRaw bool;
   Retention string}}`，`Add(exp Experience{TaskID string; Step uint64; Input, Target [][]float64;
   Weight float64; Split string}) error`（`Split == "test"` 一律拒絕，測試證明測試資料進不了重播），
   `Sample(n int) ([]Experience, SampleReport, error)`（`uniform` 用 PCG(seed, draws) 抽；
   `task_balanced` 每個任務等比例；`time_balanced` 依 `Step` 分桶等比例；報告含每任務／每桶的數量）、
   `Evict` 依政策（`fifo` 淘汰最舊；`reservoir` 以 PCG 決定是否替換，機率 `capacity/seen`）。
   `NoReplay` 是同介面的空實作，`Sample` 永遠回空與 `replayed: false`，供對照。狀態
   `ReplayState{Items; Seen uint64; RNGState}` 可序列化，進 `TrainingSnapshot.Replay *ReplayState`
   （`omitempty`，舊快照不變）；中途快照接續後的抽樣序列與連續執行相同（測試）。
   三層記憶（暫時 `plastic`、較慢 `slow`、基礎參數）的穩定化規則在 ticket 22 第一階段；本票只保證
   重播不會把同一 `Experience` 重複計入同一次更新（`Sample` 不放回；`SampleReport.Duplicates == 0`）。
3. 證據：`evidence/LRN-06/`、`evidence/LRN-07/`（fixture）。

### 第二階段：適應性評估（LRN-10）

4. **評估協定（`experiment` 套件 + CLI `evaluate`）**：`AdaptiveEvaluation{Adaptation, Scoring
   []Item（事先分割，`Item{ID; Input; Target; AllowedFeedback []string}`）; FeedbackAvailable
   []string（宣告哪些回饋種類可用，例如 `score`、`correctness`）; Reset ResetPolicy{PlasticAtItemStart,
   ChemicalAtItemStart, NeuralAtItemStart bool}; Mode string ∈ fixed|adaptive}`。執行：`fixed` 模式
   凍結一切只推論；`adaptive` 模式在 `Adaptation` 上依 `OnlinePolicy{AllowGradient: false,
   AllowPlastic: true}` 做允許的局部更新（基礎參數永遠凍結：測試比對 `Parameters` 逐位不變），
   然後在 `Scoring` 上只推論，題目之間依 `Reset` 重設。報告 `EvaluationReport{Mode; Reset;
   FeedbackAvailable; AdaptationItems, ScoringItems int; Scores []ItemScore; ContaminationChecks
   {ParametersUnchanged bool; ScoringFeedbackRejected int; ReplayRejected int}}`，模式寫進結果
   （主規格 16.2）。**污染檢查**：評分分割的回饋一律拒絕（`Receive` 回錯並計數）；評分分割的
   `Experience` 送進重播被拒（計數）；`Reset` 全開時，把評分題目順序打亂再跑一次，逐題輸出逐位相同
   （狀態殘留為零的證明）。
5. 證據：`evidence/LRN-10/`（fixture：小型延遲任務，適應 8 題、評分 8 題）；文件：README 訓練段、
   ENG、`docs/model-and-mechanisms.md` 模式表。

## 契約摘要

```go
package learning
type OnlinePolicy struct { AllowGradient, AllowPlastic, Evaluate bool; UpdateEvery uint64; ClockUnit signal.TimeUnit }
type ActionRecord struct { ActionID string; Step uint64; Output [][]float64; OutputHash, ModelVersion string }
func NewOnlineLearner(ind *Individual, p OnlinePolicy, mapper *modulation.RewardMapper) (*OnlineLearner, error)
func (l *OnlineLearner) Act(ctx context.Context, obs signal.Observation, input [][]float64) (ActionRecord, error)
func (l *OnlineLearner) Receive(ctx context.Context, fb signal.Feedback) error
func (l *OnlineLearner) Update(ctx context.Context, now signal.Timestamp) (UpdateReport, error)

package replay
type Buffer struct { ... }; type NoReplay struct{}
type Experience struct { TaskID string; Step uint64; Input, Target [][]float64; Weight float64; Split string }
func (b *Buffer) Add(e Experience) error
func (b *Buffer) Sample(n int) ([]Experience, SampleReport, error)
func (b *Buffer) State() ReplayState; func Restore(s ReplayState) (*Buffer, error)

package experiment
func RunAdaptiveEvaluation(ctx context.Context, ind *learning.Individual, e AdaptiveEvaluation) (EvaluationReport, error)
```

## 驗收

- [ ] 第一階段：`Act → Receive → Update` 順序、偽造時間拒絕、輸出 hash 不變、`Evaluate` 模式拒絕更新、
  三種抽樣與兩種淘汰的手算（小容量、固定 seed）、測試資料拒絕、快照接續抽樣序列相同；
  `go test`、race、vet；`evidence/LRN-06/`、`evidence/LRN-07/`。
- [ ] 第二階段：兩種模式、三種重設政策、污染檢查三項、打亂順序逐位相同、CLI `examples run evaluate`
  把模式寫進結果；`evidence/LRN-10/`；文件。

## 依據

- 主規格 6.3（三分離、不可回寫、測試模式不呼叫教師）、7.2（運行中梯度學習、適應性評估）、10.1、
  10.4（重播策略、三層記憶）、10.6（測試污染防護）、15.1、15.5、16.2（`evaluate`）。
