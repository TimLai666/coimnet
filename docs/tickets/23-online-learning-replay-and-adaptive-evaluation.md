# 23 — 使用者可以在運行中依新經驗學習、使用受限容量的重播，並做不污染保留資料的適應性評估

Epic：訓練與持續學習（運行中學習、重播、評估）

User Story：使用者可以讓一個持續個體在運行中先作答、再收到事後回饋、再依授權更新，已輸出的答案不被
後到的回饋或教師回應改寫；可以選擇不重播、容量受限的重播或按任務／時間維持比例的重播，每種都記錄
容量、抽樣、seed、隱私政策與移除規則；可以在事先宣告的適應分割上做允許的局部更新，然後在評分分割上
評估，題目之間依宣告政策重設快速權重與化學狀態，測試資料不進入重播也不用來調參。

Blocked by：18 局部可塑性、20 三分離與來源、21 化學狀態（重設政策要能重設它）、17 最佳化器（運行中
梯度更新只動授權參數）

Status：verified_scoped（兩階段皆已驗證，2026-09-17）

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

- [x] 第一階段：`Act → Receive → Update` 順序、偽造時間拒絕、輸出 hash 不變、`Evaluate` 模式拒絕更新、
  三種抽樣與兩種淘汰的手算（小容量、固定 seed）、測試資料拒絕、快照接續抽樣序列相同；
  `go test`、race、vet；`evidence/LRN-06/`、`evidence/LRN-07/`。
- [x] 第二階段：兩種模式、三種重設政策、污染檢查三項、打亂順序逐位相同、CLI `examples run evaluate`
  把模式寫進結果；`evidence/LRN-10/`；文件。

## 第一階段證據（2026-09-16）

環境：macOS arm64、go1.26.5，全部為 fixture 規模，不用外部資料。命令與結果：`gofmt -l .`（無輸出）、
`go vet ./...`（無輸出）、`go test -count=1 ./...`（全部 ok）、
`go test -race -count=1 ./learning/ ./replay/ ./checkpoint/ ./signal/`（全部 ok）。

- `evidence/LRN-06/`：`verification.json`、`test.log`、`red-online.log`（online.go 之前的失敗輸出）、
  `red-accumulator.log`（`OptimizerSnapshot.Accumulator` 之前的失敗輸出）。
- `evidence/LRN-07/`：`verification.json`、`test.log`、`red-replay.log`（replay 套件之前的失敗輸出）。

新增與修改：`learning/online.go`、`learning/online_test.go`、`replay/`（`policy.go`、`store.go`、
`sampling.go`、`replay_test.go`）為新增；`learning/individual.go`（`OptimizerSnapshot.Accumulator`）、
`learning/individual_test.go`、`checkpoint/checkpoint.go`（`State.Replay`）、`checkpoint/individual.go`
（累積器必填走訪與正規化）、`checkpoint/individual_test.go`、`checkpoint/options_compat_test.go` 為修改。

手算序列（容量 4、seed 7、`rand.New(rand.NewPCG(7, 0))`，前六個抽值已在測試中釘住並對生成器本身核對）：

| 項目 | 手算 | 實測 |
| --- | --- | --- |
| fifo 淘汰（加入 step 1..6） | 留下 3, 4, 5, 6，seen 6，draws 0 | 相同 |
| reservoir 接受／拒絕（加入 step 1..10） | e5 % 5 = 2 換槽 2、e6 % 6 = 2 換槽 2、e7 % 7 = 2 換槽 2、e8 % 8 = 5 丟棄、e9 % 9 = 0 換槽 0、e10 % 10 = 1 換槽 1；留下 9, 10, 7, 4，draws 6 | 相同 |
| uniform 抽樣（4 筆取 2） | 洗牌 [2 3 1 0]，回傳 step 3, 4，draws 3 | 相同 |
| task_balanced（a:1,2,4；b:3） | a 洗成 2,1,4，輪流取得 2, 3, 1, 4；每任務 a:3 b:1，draws 2 | 相同 |
| time_balanced（step 10,20,30,40） | ceil(sqrt(4)) = 2 桶，兩桶皆不換位，輪流取得 10, 30, 20, 40；每桶 [2 2]，draws 2 | 相同 |
| 運行中迴圈順序 | Act 前的 Receive 被拒；act-0 在 step 3；`AvailableAt` 10 的回饋在 now = 5 只計 Deferred 1、快照逐位不變；now = 10 消費並做一次梯度更新；再更新一次沒有回饋 | 相同 |

## 第一階段偏離（2026-09-16）

1. **`NewOnlineLearner` 的第三個參數改為 `learning.RewardGate` 介面**，不是契約寫的
   `*modulation.RewardMapper`。`learning` 不能 import `modulation`：`modulation` import `simulate`、
   `simulate` import `connectome`，而 `connectome` 的套件內測試 import `learning`，`go vet` 直接回
   「import cycle not allowed in test」。映射規則沒有改變，測試以 `learning.RewardGateFunc` 把 ticket 20
   的 `RewardMapper` 接進來，閘門仍是 `mapper.Map(score).Applied[0]`。
2. **重播狀態放在 `checkpoint.State.Replay *replay.Snapshot`，不是 `learning.TrainingSnapshot.Replay`。**
   訓練器不擁有重播存放區，這個欄位不會有人寫，`Trainer.Snapshot()` 只能永遠回 nil，等於留一個假欄位。
   `learning/trainer.go` 因此完全沒有改動。舊 checkpoint 沒有 `replay` 鍵，載入後 `Replay` 為 nil，
   意義就是「不重播」的對照組。
3. **型別命名**：套件是 `replay`，宣告是 `Buffer`、活的物件是 `Store`、狀態是 `State`、落盤的一對是
   `Snapshot{Buffer, State}`。契約的 `ReplayState` 只帶 `Items`／`Seen`／`RNGState`，但 `Restore(b, s)`
   同時需要宣告與狀態，所以多一個 `Snapshot` 把兩者綁在一起；`State` 另帶 `schema_version`，與專案其他
   落盤型別一致。RNG 狀態以 `Draws`（已取用的抽值數）表示，`Restore` 重放同樣次數。
4. **`Retention` 是列舉而不是自由字串**，兩個值都有實際行為：`keep_until_evicted`（預設，靠容量淘汰）與
   `drop_after_sample`（抽中即移出）。只記錄不執行的政策字串等於佔位。註記：`drop_after_sample` 與
   `reservoir` 併用時，存放區不再是「看過的全部」的均勻樣本，套件文件與 `Report` 都寫明這一點。
5. **`Split` 是宣告好的四個值**（`train`／`adaptation`／`scoring`／`test`），未知字串一律拒絕，避免打錯的
   `"tes"` 變成第五種可以繞過 test 阻擋的分割。`scoring` 目前可以進重播，第二階段的污染檢查再加上計數拒絕。
6. **`Experience` 多了 `Raw`、`InputHash`、`TargetHash` 三個由存放區自己寫的欄位**，這是
   `StoreRaw == false` 只留 hash 的必要表示；呼叫端自帶這三個欄位會被拒絕。指紋在兩種模式都會算，所以
   兩種存放區描述同一筆經驗的方式一致。
7. **`ActionRecord.OutputHash` 的原像不含形狀**（依提示字面實作：row-major float64 的 little-endian
   IEEE-754 位元組）。同一個個體的輸出寬度固定，所以這不造成碰撞。
8. **個體 checkpoint 的累積器只有「不存在」代表沒有開啟的視窗，明寫 `null` 會被拒絕。** 這個文件格式本來
   就不允許任何 null（`checkUniqueJSONRejectNull`），可選的 `plastic` 也是同樣待遇。在 `learning` 層
   直接解碼 `IndividualSnapshot` 時，`null` 仍然解成 nil。
9. **`OnlinePolicy.UpdateEvery` 為 0 時等於 1**，與 `Options.AccumulateSteps` 的慣例一致。
10. **多了幾條提示沒寫、但屬於同一句話的拒絕**：沒有可塑政策卻給了 reward gate、可塑政策卻沒有先開啟
    局部可塑性、回饋的 experience 與該行動的 observation 不同、回饋的時間單位不是 `model_step`、
    `SetTargets` 指向不存在的行動或寬度不符的目標。
11. **`Sample` 會先把整個順序定下來再取前 n 筆**，因此一次抽樣的抽值次數只由存放區內容與政策決定，與 n
    無關。這正是「中途快照接續後序列相同」能成立的原因。
12. **`Update` 中途失敗不回滾**：已消費的回饋離開佇列、已套用的更新留著，錯誤與當下的 `UpdateReport`
    一起回傳，讓呼叫端看得到做到哪裡。
13. **`checkpoint/state.go` 不存在**，`checkpoint.State` 住在 `checkpoint/checkpoint.go`，改動落在該檔。
14. **尚未處理**：ticket 17 root 裁決 3 要求 ENG 的個體段落寫明「視窗中間存檔會遺失部分累積」這個限制。
    累積器補上後該限制已消失，但 `ENG.md` 本輪由 ticket 19 的執行者持有，這段文字需要由持有者更新；
    `docs/requirements-status.json` 與 `delivery-status.md` 同樣不在本輪範圍內。

## 第二階段證據（2026-09-17）

環境：macOS arm64、go1.26.5，全部為 fixture 規模，不用外部資料。
驗證命令：
- `go run ./cmd/coimnet examples run evaluate --mode fixed --reset neural,plastic,chemical --seed 1 --out evidence/LRN-10/evaluate-fixed.json`
- `go run ./cmd/coimnet examples run evaluate --mode adaptive --reset neural,plastic,chemical --seed 1 --out evidence/LRN-10/evaluate-adaptive.json`
- `go test -count=1 -race -v -run 'TestAdaptiveEvaluation|TestRunAdaptiveEvaluation|TestReplayRejection|TestShuffleInvariance|TestEvaluate' ./experiment/ ./internal/cli/ > evidence/LRN-10/test.log 2>&1`（14 項頂層測試與所有子測試全數通過）

五張小票之實作、測試與證據路徑：

1. **型別與驗證（`AdaptiveEvaluation.Validate`）**：
   - 測試：`TestAdaptiveEvaluationValidate`（包含 7 條規則、17 個案例：合法 fixed/adaptive 宣告通過；非法模式、空 scoring 分割、fixed 帶 adaptation 分割、跨分割重複 ID、空 ID、非法 input 列、NaN/Inf、空 target、未宣告 allowed_feedback、scoring 帶 allowed_feedback、重複或空 feedback_available 等拒絕）。
   - 證據路徑：`evidence/LRN-10/test.log`。
2. **fixed 模式（`RunAdaptiveEvaluation`）**：
   - 測試：`TestRunAdaptiveEvaluationFixedMode`（4 個子測試：basic_properties、deterministic、nil_individual、target_width_mismatch）、`TestRunAdaptiveEvaluationFixedModeUnchanged`。
   - 證據路徑：`evidence/LRN-10/test.log`、`evidence/LRN-10/evaluate-fixed.json`。
3. **adaptive 模式（`runAdaptiveEvaluation`）**：
   - 測試：`TestRunAdaptiveEvaluationAdaptiveMode`（驗證基礎參數凍結、局部可塑狀態改變）、`TestRunAdaptiveEvaluationAdaptiveNeedsPlasticity`（驗證未開啟可塑性時拒絕）。
   - 證據路徑：`evidence/LRN-10/test.log`、`evidence/LRN-10/evaluate-adaptive.json`。
4. **污染檢查（`ContaminationChecks`）**：
   - 測試：`TestReplayRejectionCountRefusesEveryScoringItem`（8 題評分分割資料送入 replay store 作為 test split 均遭拒絕）、`TestShuffleInvarianceHoldsWithFullReset`（全重設下順序打亂輸出逐位相同）、`TestShuffleInvarianceFailsWithoutNeuralReset`（未重設神經狀態時鑑別出 false）、`TestRunAdaptiveEvaluationFillsContaminationChecks`（full_reset 與 partial_reset）。
   - 證據路徑：`evidence/LRN-10/test.log`。
5. **CLI 與 fixture（`examples run evaluate`）**：
   - 測試：`TestEvaluateFixedWritesReport`、`TestEvaluateAdaptiveWritesReport`、`TestEvaluateRejectsUnknownMode`、`TestEvaluateRefusesExistingOut`、`TestEvaluateHelp`。
   - 證據路徑：`evidence/LRN-10/test.log`、`evidence/LRN-10/evaluate-fixed.json`、`evidence/LRN-10/evaluate-adaptive.json`。

Limitations：
- 只在 macOS arm64 fixture 跑過。
- 梯度型適應被政策關閉。
- 沒有真實任務資料。
- 重播拒絕來自 replay 套件對 test 分割的固定規則。
- 教師來源未接（ticket 27 第三階段）。
- 全重設政策（`--reset neural,plastic,chemical`）下兩份報告的八題 MSE 逐位相同：適應分割學到的快速變化在每題前被清掉；可塑狀態確實改變由 `TestRunAdaptiveEvaluationAdaptiveMode` 證明，成績差異要用不重設 plastic 的政策才看得到，本紀錄沒有跑。

## 依據

- 主規格 6.3（三分離、不可回寫、測試模式不呼叫教師）、7.2（運行中梯度學習、適應性評估）、10.1、
  10.4（重播策略、三層記憶）、10.6（測試污染防護）、15.1、15.5、16.2（`evaluate`）。
