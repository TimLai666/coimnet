# 17 — 使用者可以限制可更新參數、符號與範圍，並使用完整的損失與最佳化器流程

Epic：訓練與持續學習

User Story：使用者可以按邊與節點集合指定哪些參數能更新，讓有依據的作用符號固定、只學幅度，
限制數值範圍，並使用損失縮放、梯度累積、裁切、學習率排程與可恢復的最佳化器狀態。

Blocked by：04 連續核心訓練、11 LIF 核心、13 參數 adapter（推導的符號是固定符號的來源）

Status：ready（契約已於 2026-09-15 定案，分兩階段派工；驗收項目驗證後才勾選）

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

- [ ] 第一階段：逐項遮罩下凍結項在含 weight decay 的多步後逐位不變、動量與步數不動；固定符號邊在
  大學習率下跑 1,000 步符號不翻轉且幅度 > 0；鏈鎖梯度以連續核心的有限差分驗證（`EdgeSigns` 混合
  自由與固定）；`EdgeSigns` 全零逐位等於改前；範圍投影計數正確且不動動量；舊快照可讀；
  `SignsFromParameterSet` 對 ticket 13 的 fixture 參數集給出 +1／−1／unknown 計數與政策結果；
  `go test`、race、vet。
- [ ] 第二階段：`LossScale` 縮放前後更新在 1e-12 內相同、溢位拒絕；累積 k 步等於一次大 batch 的
  平均梯度（手算小例）、中途快照恢復逐位相同；三種排程的 LR 曲線手算、恢復後接續；裁切順序測試；
  `evidence/LRN-03/`、`evidence/COR-10/`；ticket、ENG、README。
- [ ] COR-07 證據紀錄與 requirements-status（root）。

## 依據

- 主規格 7.4（凍結與更新遮罩）、9.1–9.2（稀疏試金石）、10.1、10.3（固定符號不跨零）、15.1
  （訓練快照含最佳化器、排程器、累積梯度）、18.2。規格抽取見 root 的工作紀錄。
