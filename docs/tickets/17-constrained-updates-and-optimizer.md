# 17 — 使用者可以限制可更新參數、符號與範圍，並使用完整的損失與最佳化器流程

Epic：訓練與持續學習

User Story：使用者可以按邊與節點集合指定哪些參數能更新，讓有依據的作用符號固定、只學幅度，
限制數值範圍，並使用損失縮放、梯度累積、裁切、學習率排程與可恢復的最佳化器狀態。

Blocked by：04 連續核心訓練、11 LIF 核心、13 參數 adapter（推導的符號是固定符號的來源）

Status：verified_scoped（兩階段皆已驗證，2026-09-16；COR-10、LRN-03 標 passed；只有 fixture、只在 macOS，見「Root 裁決」的後續項目）

對應需求：COR-10（遮罩涵蓋動量及權重衰減；快速變化不翻轉受限作用符號）、LRN-03（損失縮放、
梯度累積、裁切、排程與最佳化器恢復皆測試）、COR-07（證據紀錄：`internal/sparse` 的三節點試金石
已用主規格 9.2 的原數字，全圖從未建 N×N，見 NAT-01 記憶體帳）。主規格 7.4、9.1、9.2、10.1、
10.3、15.1、18.2。

## Root 決策（2026-09-15）

1. **細粒度遮罩**：`Options.Trainable` 的群組旗標維持；新增 `Options.Masks *UpdateMasks{Edges []bool,
   Nodes []bool}`（nil 表示全開）。有效遮罩 = 群組旗標 AND 逐項遮罩（邊遮罩作用在 weights，節點遮罩
   作用在 bias／log_tau／theta_raw）。被遮罩的參數：梯度不用、Adam 動量與步數不動、權重衰減不作用
   （既有實作已是「整個跳過」，要加測試釘住：凍結項在多步含 weight decay 後逐位不變）。遮罩進
   `TrainingSnapshot`（隨 `Options` 序列化，長度與拓撲一起驗證）。
2. **固定符號、只學幅度**：`Config.EdgeSigns []int8`（+1／−1／0 = 自由；nil = 全自由；長度 = 邊數）。
   符號固定的邊以對數幅度參數化：raw 參數 `rho_e`，有效權重 `w_e = s_e * exp(rho_e)`；自由邊
   `w_e = raw_e`。`Parameters.Core.Weights` 從此存 **raw**（自由邊即權重本身，固定邊為 log 幅度），
   `learning.EffectiveWeights(config, params) []float64` 給出核心實際使用的權重，`Predict`／
   `LossGradient`／`Step` 一律經它；梯度用鏈鎖 `d loss/d rho_e = d loss/d w_e * w_e`。
   零幅度不可表示（exp>0），符號永不翻轉；`MinLogMagnitude`（預設 −20）以投影防止下溢並計數。
   `EdgeSigns` 全為 0 時所有結果與改前逐位相同（有測試）。既有快照（無 `edge_signs`）視為全自由。
   推導參數集的接法：`learning.SignsFromParameterSet(set *params.Set, policy)` 把 +1／−1 帶入，
   unknown 依 `free|excitatory|inhibitory` 政策明示處理並回報計數（與 `simulate` 的政策同名）。
3. **數值範圍**：`Options.Ranges *ParameterRanges{WeightMagnitudeMax, BiasAbsMax, LogTauMin, LogTauMax,
   ThetaRawAbsMax float64}`（0 = 不限制），更新後以投影套用並在 `StepResult.Projected` 回報各群組
   被投影的數量；投影不改變最佳化器動量（文件寫明這是宣告的投影規則，主規格 8.2）。
4. **損失縮放、累積、排程**：`Options.LossScale float64`（預設 1；梯度在裁切前除回，非有限即拒絕該
   步，不留半次狀態）、`Options.AccumulateSteps int`（預設 1；`Step` 累積 k 次梯度平均後才更新，
   累積器與計數進 `TrainingSnapshot.Accumulator`，中途快照再恢復與連續執行逐位相同）、
   `Options.Schedule *Schedule{Kind constant|step|cosine, WarmupUpdates, DecayUpdates, FinalFactor,
   StepEvery, StepFactor}`（學習率是 `updates` 的純函數，恢復後自然接續；每步回報實際 LR）。
   裁切維持既有 `ClipNorm`，加測試證明裁切在累積平均之後、Adam 之前。
5. **COR-07 證據**：不改程式，寫 `evidence/COR-07/verification.json` 指向
   `internal/sparse/operator_test.go` 的試金石（輸入 `[2,4,8]` → `[0,-1,3]`，權重梯度 `[4,16,12]`，
   輸入梯度 `[1,2.25,-0.5]`）、`go test -run` 的日誌，以及 NAT-01 全圖執行的邊陣列記憶體帳
   （25,563,197 條邊、無 N×N），並更新 `docs/requirements-status.json`（root 做）。

## 契約（第一階段：遮罩、符號、範圍）

```go
type UpdateMasks struct { Edges []bool `json:"edges,omitempty"`; Nodes []bool `json:"nodes,omitempty"` }
type ParameterRanges struct { WeightMagnitudeMax, BiasAbsMax, LogTauMin, LogTauMax, ThetaRawAbsMax float64 }
// Config.EdgeSigns []int8 `json:"edge_signs,omitempty"`; Config.MinLogMagnitude float64 `json:"min_log_magnitude,omitempty"`
func EffectiveWeights(c Config, p Parameters) ([]float64, error)
func SignsFromParameterSet(set *params.Set, unknownPolicy string) ([]int8, SignSummary, error)
// StepResult 加 Projected map[string]int、LearningRate float64
```

## 契約（第二階段：損失縮放、累積、排程）

```go
type Schedule struct { Kind string; WarmupUpdates, DecayUpdates uint64; FinalFactor float64; StepEvery uint64; StepFactor float64 }
// Options.LossScale float64; Options.AccumulateSteps int; Options.Schedule *Schedule
// TrainingSnapshot.Accumulator *GradientAccumulator{Sum []float64; Count int}
func LearningRateAt(o Options, updates uint64) float64
```

## 驗收

- [x] 第一階段：逐項遮罩下凍結項在含 weight decay 的多步後逐位不變、動量與步數不動；固定符號邊在
  大學習率下跑 1,000 步符號不翻轉且幅度 > 0；鏈鎖梯度以連續核心的有限差分驗證（`EdgeSigns` 混合
  自由與固定）；`EdgeSigns` 全零逐位等於改前；範圍投影計數正確且不動動量；舊快照可讀；
  `SignsFromParameterSet` 對 ticket 13 的 fixture 參數集給出 +1／−1／unknown 計數與政策結果；
  `go test`、race、vet 全數通過。
- [x] 第二階段：`LossScale` 縮放前後更新在 1e-12 內相同、溢位拒絕；累積 k 步等於一次大 batch 的
  平均梯度（手算小例）、中途快照恢復逐位相同；三種排程的 LR 曲線手算、恢復後接續；裁切順序測試；
  `evidence/LRN-03/`、`evidence/COR-10/`；ticket、ENG、README。
- [x] COR-07 證據紀錄與 requirements-status（root；`evidence/COR-07/verification.json` 由 opencode 依 root
  查證的事實寫成，root 逐項核對後標 passed）。

## 第一階段證據

`evidence/COR-10/`（2026-09-15，macOS，`uptime` 記於 `verification-full.log`：up 9 days, load 2.04）。
先寫失敗測試再實作：`learning/constrained.go` 先只放型別與空實作，四份 `red-*.log` 是行為紅燈而不是
編譯錯誤，實作後同樣的 `-run` 過濾器產生對應的 `green-*.log`。

- 遮罩：`red-masks.log` → `green-masks.log`。`TestPerItemMasksFreezeOneEdgeAndOneNodeThroughWeightDecay`
  在 `weight_decay=0.1` 下跑 50 步，被遮罩的 `weights[0]`、`bias[0]`、`log_tau[0]` 值逐位不變，
  `First`／`Second`／`Steps` 全為 0，未遮罩的三項都動且 `Steps=50`。群組旗標與逐項遮罩的 AND 由
  `TestGroupFlagAndPerItemMaskBothHaveToAllowAnUpdate` 三格釘住；長度驗證涵蓋 `NewTrainer` 與
  `RestoreTrainer`；`theta_raw` 的節點遮罩在 LIF 核心上另有一測。
- 固定符號：`red-signs.log` → `green-signs.log`。
  `TestFixedSignEdgesNeverFlipUnderALargeLearningRate` 在 `learning_rate=1.0` 下跑 1,000 步，
  每一步都檢查有效權重的符號：edge 0（`+1`）最小幅度 0.07602767734034184，
  edge 2（`−1`）最小幅度 0.47519607783859258，兩者都 > 0 且從未翻轉；最終 raw 值
  `[-1.9782315349015838 -0.9886289490396335 0.4029712819746961 0.95797632963738]`。
  `MinLogMagnitude` 投影在 `TestMinLogMagnitudeProjectionKeepsTheMagnitudeRepresentable`
  的 30 步中觸發 17 次，raw 值從未低於地板。`EdgeSigns` 全零與「不宣告」在 20 步後的
  `Parameters`／`Optimizer`／`Updates` JSON 位元組相同；沒有 `edge_signs` 的舊快照載入為全自由，
  且 `Predict` 結果逐位相同。
- 鏈鎖梯度（連續核心，中央差分 `eps=0.01`，混合自由與固定邊）：

  | edge | sign | analytic | central difference | relative error |
  | --- | --- | --- | --- | --- |
  | 0 | +1 | -0.0169804527869481 | -0.01698069649127 | 1.44e-05 |
  | 1 | 0 | -0.018380476235569 | -0.0183803676063465 | 5.91e-06 |
  | 2 | -1 | -0.00118676156045005 | -0.00118681987066049 | 4.91e-05 |
  | 3 | 0 | -0.128878357933245 | -0.128878545411534 | 1.45e-06 |

  最差相對誤差 4.91e-05，低於票面的 1e-4。同一測試另外比對
  `d loss/d rho == d loss/d w * w`（1e-12 相對誤差內），並確認該 fixture 上兩者確實不同，
  所以「少乘一次 w」的實作不可能通過。
- 範圍：`red-ranges.log` → `green-ranges.log`。一步之後 `Projected` 為
  `map[weights:2 bias:2 log_tau:2]`，三組都夾到宣告的界線；與沒有 `Ranges` 的對照訓練器相比
  `AdamState` 逐位相同、`GradientNorm` 相同，證明投影不動動量。被遮罩的群組不投影，
  `Projected` 也不會出現該鍵。固定符號邊的 `WeightMagnitudeMax` 在 log 空間夾成
  `log(max)`，有效權重正好等於上限。非有限、負的幅度上限、反轉的 log_tau 區間，
  以及低於對數幅度地板的權重上限都被 `NewTrainer` 拒絕。
- 推導參數集：`red-signs-from-set.log` → `green-signs-from-set.log`。
  `learning/derived_fixture_test.go` 把 ticket 13 的四節點 fixture 重建一份（真的跑
  `params.Derive`，不是抄答案），六條邊的符號為 `[1 -1 0 -1 0 0]`。三種政策的結果與計數：
  `free` → `[1 -1 0 -1 0 0]`、`excitatory` → `[1 -1 1 -1 1 1]`、`inhibitory` → `[1 -1 -1 -1 -1 -1]`，
  三者的 `SignSummary` 都是 `PositiveEdges:1 NegativeEdges:2 UnknownEdges:3` 加上該政策名。
- 快照相容：`red-checkpoint-compat.log` → `green-checkpoint-compat.log`
  （`checkpoint/options_compat_test.go`，新檔案，沒有動 `checkpoint/individual_test.go`）。
  未使用時 `masks`、`ranges`、`edge_signs`、`min_log_magnitude` 都不寫出；手工剝掉這四個欄位的
  payload 仍可載入且載入為無限制；四個欄位可完整往返；欄位內的 JSON null 被必填檢查擋下，
  但整個可選指標為 null 仍合法；長度不符的遮罩在 `Load` 就被拒絕。
- 全套驗證：`verification-full.log`（`gofmt -l .` 無輸出、`go vet ./...`、`go test -count=1 ./...`）
  與 `verification-race.log`（`go test -race -count=1 ./learning/ ./checkpoint/ ./experiment/ ./examples/...`）。
  `learning`、`checkpoint`、`experiment`、`examples/lifthreshold`、`examples/realsubgraph` 全綠。

偏離與待決策：

1. **`SignsFromParameterSet` 的簽章與票面不同。** 票面寫
   `SignsFromParameterSet(set *params.Set, unknownPolicy string)`，但 `params` 依賴 `connectome`，
   而 `connectome/individual_test.go` 是 `package connectome` 的內部測試且 import `learning`
   （commit `d357b69`，早於本票）。`learning` 一旦 import `params` 就在 connectome 的測試 binary
   形成 import cycle，`go test ./connectome/` 直接編譯失敗。因為 `connectome/` 不在本輪可改範圍，
   改為 `SignsFromParameterSet(set DerivedSigns, unknownPolicy string)`，`DerivedSigns` 只帶
   `Source`／`EdgeSigns`／`Edges` 三個欄位，呼叫端一行轉換：
   `learning.DerivedSigns{Source: set.Source, EdgeSigns: set.EdgeSign, Edges: set.Edges()}`。
   `learning.DerivedSetSource` 重複了 `params.SetSource` 的字串，外部測試套件
   （可以同時 import 兩者）有一測釘住兩者相等。若 root 願意把
   `connectome/individual_test.go` 改成 `package connectome_test`，就能把簽章換回票面原樣。
2. **`Individual.Advance` 也走 `EffectiveWeights`。** 票面只點名 `Predict`、`spikeEvents`、
   `LossGradient` 與 `Step`，但持續個體的推進若不換算，固定符號邊會把對數幅度當權重用。
   已一併接上，並有測試比對「固定符號 + raw」與「自由符號 + 有效權重」兩條路徑逐位相同。
3. **`examples/multichannel/example_test.go:111` 已在本輪修好。** `StepResult` 依票面新增
   `Projected map[string]int` 後不再是可比較型別，該行原本的 `if a != b` 無法編譯；
   經 root 同意後改成 `if !reflect.DeepEqual(a, b) {`（該檔本來就 import `reflect`），
   語意不變。修正後 `gofmt -l .` 無輸出，`go vet ./...`、`go test -count=1 ./...` 與
   `go test -race -count=1 ./learning/ ./checkpoint/ ./experiment/ ./examples/...` 全數通過
   （`evidence/COR-10/verification-full.log` 第三段與 `verification-race.log`）。
4. **`Projected` 的鍵。** 票面只說「各群組被投影的數量」。實作用
   `weights`／`bias`／`log_tau`／`theta_raw` 四個群組鍵，另加 `min_log_magnitude`
   給固定符號邊的下溢地板，因為那是 root 決策 2 的規則而不是 root 決策 3 的範圍。
   沒有任何投影時 `Projected` 為 nil（`omitempty`）。
5. **投影只作用在可更新的參數。** 若對被遮罩的參數也投影，「凍結項逐位不變」就不成立，
   兩條 root 決策會互相矛盾。已在 `ParameterRanges` 的註解與測試中寫明。
6. **`StepResult` 多了 `learning_rate` 欄位，`internal/cli` 的 `last_step` JSON 因此多一個鍵。**
   `internal/cli/train.go` 直接序列化 `learning.StepResult`。這是純新增欄位，沒有既有測試比對
   該 JSON 的位元組，但 `evidence/` 裡舊的 `train-*.json` 不再與新輸出逐位相同。

## 第二階段證據

`evidence/LRN-03/`（macOS arm64、go1.26.5。紅燈 2026-09-15，`uptime`：up 9 days,
load averages 2.18 2.10 2.05；綠燈與四項驗證於 2026-09-16 在最終工作樹重跑，`uptime`：up 11 days,
load averages 4.52 4.42 3.15。工作樹同時有 ticket 18／20 另外兩個 agent 的進行中改動）。
一樣先寫失敗測試再實作：`learning/schedule.go` 與 `learning/accumulate.go` 先只放型別，
`LearningRateAt` 直接回傳基礎學習率，`Step` 完全不看新欄位，五份 `red-*.log` 是行為紅燈
而不是編譯錯誤；實作後同樣的 `-run` 過濾器產生對應的 `green-*.log`。

- 損失縮放：`red-loss-scale.log` → `green-loss-scale.log`。
  `TestLossScaleLeavesTheUpdateUnchanged` 用同一組三個 batch 跑兩輪共六步，
  `loss_scale` 1024、1000、65536、1e6、1e-3 的參數與兩組動量與未縮放的對照
  **逐位相同**（實測最大差 0，票面的 1e-12 是宣告上限而不是量到的偏差）。
  `TestNonFiniteScaledGradientRejectsTheWholeStep` 用 target 1e9 的 episode 取得
  梯度範數 1.152681898513553e+09：`loss_scale` 1 正常更新，`loss_scale` 1e300 讓乘積
  離開 float64 範圍，整步被拒絕，`Snapshot()` 前後 `reflect.DeepEqual` 相同；
  同一個拒絕發生在累積視窗中間時，`Accumulator` 的 `Sum` 與 `Count` 也逐位不變。
  負值、NaN、Inf 的 `loss_scale` 在 `NewTrainer` 就被拒絕。
- 梯度累積：`red-accumulate.log` → `green-accumulate.log`。
  `TestAccumulatedWindowAppliesOneAdamWStepOnTheMeanGradient` 用兩節點連續 fixture 的
  三個 batch（梯度在視窗內不動，因此三個梯度都取自同一組參數）：前兩步 `Applied` 為
  false、`Updates` 與動量全部不動，`Accumulator.Count` 為 1、2 且 `Sum` 等於
  `g1`、`g1+g2`（1e-12 內）；第三步 `Applied` 為 true、`Updates` 為 1、`Accumulator` 歸 nil。
  平均梯度為 `[-0.015501780644746763 0.00085021960331920128 -0.05405473625733398
  -0.45369432804432802 0.0032237214411788783 -0.010068422137445212 0.001930061262100935
  -0.1126475667891403 0.03708363945285479]`，手算的一次 AdamW（step 1 兩個偏差修正相消，
  更新量是 `lr * d / (|d| + eps)`，`lr=0.1`、`eps=0.1`）給出
  `[0.31342124819047262 -0.20084305181155127 0.13508800675043228 -0.018060506480751623
  0.096876956772948858 0.20914742116033291 0.4981064847423784 0.15297383294342642
  0.77294816536760513]`，與實際參數在 1e-12 內相同；測試把這組數字直接寫成常數。
  `TestAccumulationSnapshotInTheMiddleOfAWindowResumesBitIdentically` 在 Count 1／3 時
  取快照、`RestoreTrainer` 再跑完，最終 `TrainingSnapshot` 與未中斷的一次跑完
  `reflect.DeepEqual` 相同，快照的累積器也證明不與訓練器共用記憶體。
- 排程：`red-schedule.log` → `green-schedule.log`。手算的 LR 表（基礎 0.2，容差 1e-12）：

  | 排程 | updates → 學習率 |
  | --- | --- |
  | 無排程 | 0、7、1048576 → 0.2 |
  | `constant`，warmup 4 | 0 → 0；1 → 0.05；2 → 0.1；3 → 0.15；4 → 0.2；9 → 0.2 |
  | `step`，每 5 步 ×0.5 | 0 → 0.2；4 → 0.2；5 → 0.1；9 → 0.1；10 → 0.05；20 → 0.0125 |
  | `step`，warmup 3，每 5 步 ×0.5 | 0 → 0；1 → 0.0666…7；2 → 0.1333…3；3 → 0.2；5 → 0.1；12 → 0.05 |
  | `cosine`，warmup 4，decay 8，final 0.1 | 0 → 0；2 → 0.1；4 → 0.2；6 → 0.17363961030678929；8 → 0.11；12 → 0.02；100 → 0.02 |

  `TestStepReportsTheScheduledLearningRateAndUsesIt` 在 `step`（每 2 步 ×0.5）下連跑六步，
  `StepResult.LearningRate` 依序為 0.2、0.2、0.1、0.1、0.05、0.05，第一步的參數等於用
  0.2 手算的 AdamW 第一步。`TestScheduleResumesOnTheSameCurve` 在 cosine 曲線中途
  `RestoreTrainer`，六步的 LR 序列與最終快照都與未中斷的一次跑完相同。
  未知 kind、空 kind、`step` 缺 `step_every` 或 `step_factor`、負的 `step_factor`、
  `cosine` 缺 `decay_updates`、`final_factor` 超出 [0,1] 或非有限，以及「該 kind 不讀的欄位
  被寫了值」共 13 種情況都被 `NewTrainer` 拒絕，未知 kind 也被 `RestoreTrainer` 拒絕。
- 裁切順序：`red-clip-order.log` → `clip-order.log`。三個 batch 的梯度範數為
  0.49798667625178134、0.18481102604035227、1.1049572432483197，平均梯度範數
  0.47242104858621886。`clip_norm` 1 時第三個 batch 單獨會被裁、平均不會：實際更新等於
  「未裁切平均的一次 AdamW」（1e-12 內），而「先逐 batch 裁再平均」的結果與它差
  0.0020587961634574437，所以順序錯了不可能通過。`clip_norm` 0.2 時平均本身被裁成
  0.423351162270449 倍，更新等於「裁切後平均的一次 AdamW」
  `[0.3061585311248784 -0.20035865052532303 0.11862253035545056 -0.034238130590445115
  0.098653608872326312 0.20408821877930369 0.49918952863175292 0.13229036954243426
  0.7864308703323335]`，證明裁切在 AdamW 之前。
- 快照相容：`red-checkpoint-compat.log` → `green-checkpoint-compat.log`
  （`checkpoint/options_compat_test.go` 新增三個測試，沒有動
  `checkpoint/individual_test.go` 與 `checkpoint/package*.go`）。未使用時
  `loss_scale`、`accumulate_steps`、`schedule`、`accumulator` 都不寫出，載入後四者皆為零值；
  四個欄位（含 Count 1 的部分累積器）可完整往返；`sum` 長度不符、`count` 等於或超過視窗、
  `count` 為負、沒有宣告視窗卻帶累積器、缺 `sum`、缺 `count`、`sum` 內有 null、`count` 為
  null、未知或 null 的 `schedule.kind`、負的 `loss_scale` 共 12 種 payload 都在 `Load` 被拒絕，
  整個 `accumulator` 為 JSON null 仍合法。
- 全套驗證：`test.log`、`race.log`、`vet.log`、`gofmt.log`。

偏離與待決策：

1. **「平均範數超過 `ClipNorm` 但每個 batch 都低於」在數學上不可能。** 平均的範數不會大於
   被平均者的最大範數，所以票面兩種構造只有「單個 batch 超過、平均不超過」這一種存在，
   證據用的就是它，另外再加一組「平均本身被裁」釘住裁切在 AdamW 之前。
2. **`Individual.ResetOptimizer` 不會清掉累積器。** 累積器放在 `Trainer` 上，而
   `ResetOptimizer` 只換掉 `options` 與 `optimizer`（`learning/individual.go:303` 一帶），
   因此在累積視窗中間重設最佳化器會留下上一個視窗的部分梯度。`learning/individual.go`
   屬於 ticket 18 的範圍，本輪沒有改：`Step` 先做防禦，遇到與目前視窗不符的累積器就重新開一個
   視窗，但正確的修法是在 `ResetOptimizer` 裡加一行清空。請 root 指派。
3. **`checkpoint.State` 仍要求 `next_sample == updates`。** 累積 k 步時一次更新消耗 k 個樣本，
   所以「每次更新一個樣本」的既有檢查（`checkpoint/checkpoint.go:81`）會擋下真的用
   `accumulate_steps > 1` 跑出來的 checkpoint。本輪沒有改這個已驗證的契約，建議改成
   `next_sample == updates * accumulate_steps + accumulator.count`，但那會在中途改動
   `accumulate_steps` 時誤擋，需要 root 決定。`IndividualSnapshot` 同樣沒有累積器欄位，
   持續個體在視窗中間存檔會遺失部分累積，這兩點都屬於 ticket 18／root 的範圍。
4. **`StepResult` 又多了兩個欄位。** `applied` 與 `accumulated` 是純新增，
   `internal/cli` 的 `last_step` JSON 因此再多兩個鍵；沒有既有測試比對該 JSON 的位元組。
   累積視窗未滿的那一步 `applied` 為 false、`updates` 不變、`update_norm`、`learning_rate`
   與 `projected` 都是無更新的值，`gradient_norm` 一律是「這一步自己的梯度範數」
   （裁切前、平均前），視窗為 1 時與改前完全相同。
5. **排程拒絕該 kind 不讀的欄位。** 票面只點名未知 kind、`step` 的 `step_every` 為 0、
   `final_factor` 超出 [0,1] 與負值。實作另外拒絕 `cosine` 的 `decay_updates` 為 0、
   `step` 的 `step_factor` 不為正，以及例如「`constant` 卻寫了 `step_factor`」，
   理由與 `simulate` 不允許「第二個被忽略的旋鈕」相同。
6. **暖身第一步的學習率是 0。** 「linear warmup from 0 to base over WarmupUpdates」按字面
   實作，`LearningRateAt(o, 0)` 為 0，所以宣告暖身時第一次更新不動參數，但動量與步數照常
   累積。若要第一步就有非零學習率，暖身要改成 `(updates+1)/WarmupUpdates`，這是契約問題。

## Root 裁決（2026-09-16，第二階段偏離）

1. 裁切構造：接受偏離 1，證據用「單個 batch 超過、平均不超過」與「平均本身被裁」兩組。
2. `Individual.ResetOptimizer` 清空累積器：已指派給 ticket 18 第一階段的執行者（同一輪加一行與一個斷言）。
3. `checkpoint.State` 的 `next_sample` 規則：改為 `next_sample == updates * accumulate_steps + accumulator.count`，
   前提是同一次訓練的 `accumulate_steps` 固定不變（`resume` 在選項與快照不一致時拒絕，除非明示
   `--allow-option-change`，見 ticket 24 第二階段）。在 CLI 尚未暴露 `accumulate_steps` 之前，CLI 不會寫出
   這種 checkpoint，因此這條改動與 CLI 的旗標一起落在 ticket 24 第二階段。`IndividualSnapshot` 的累積器欄位
   （`Optimizer.Accumulator`）已於 ticket 23 第一階段補上（2026-09-16，`OptimizerSnapshot.Accumulator`，
   checkpoint 以可選部件走訪），持續個體在視窗中間存檔不再遺失累積，ENG 不需要另寫限制。
4. `StepResult` 新欄位、排程的嚴格驗證：接受。
5. 暖身第一步學習率為 0：維持字面實作（`updates / WarmupUpdates`），文件已寫明；動量與步數照常累積是
   宣告行為，不改。

## 依據

- 主規格 7.4（凍結與更新遮罩）、9.1–9.2（稀疏試金石）、10.1、10.3（固定符號不跨零）、15.1
  （訓練快照含最佳化器、排程器、累積梯度）、18.2。規格抽取見 root 的工作紀錄。
