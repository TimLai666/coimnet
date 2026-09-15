# 21 — 研究者可以運行有時間衰退的化學濃度、設定選擇性受體，並調節當下敏感度與有效閾值

Epic：化學與荷爾蒙調節（第二層：濃度、受體、效果）

User Story：研究者可以讓 ticket 20 的釋放率驅動每個區域、每個通道的非負濃度，濃度依正時間常數
衰退並可在區域間傳輸；指定哪些細胞帶哪種受體、受體是「實驗指出不反應」「未知」還是「假設反應」，
以穩定的反應曲線算出佔用率；再把佔用率映射成當下敏感度（增益與偏移）或暫時的有效閾值調整。
中性值時逐位等於未啟用；暫時效果永遠不寫回基礎參數；個體快照保存化學狀態並能精確接續。

Blocked by：20 調節來源（釋放率 q）、16 LIF 個體（持續路徑與快照）、11 LIF 核心（`theta_base`）

Status：draft（契約已於 2026-09-15 定案；待 20 完成後派工，分兩階段）

對應需求：MOD-02（非負濃度、穩態、清除、單位、中性狀態）、MOD-03（未知／無反應／假設分離，
飽和與極端數值穩定）、MOD-04（中性值與未啟用相同；基礎參數不被暫時效果永久覆寫）。主規格 5.5
（`ReceptorRecord` 欄位）、8.2（`theta_effective = bounded(theta_base + adaptation + homeostasis +
modulation)`）、11.3、11.4、11.5（當下敏感度、有效閾值）、15.1（化學狀態進快照）。

## Root 決策（2026-09-15）

1. **濃度狀態（`modulation` 套件，MOD-02）**：`Chemistry{Regions, Channels int; DT float64; Tau []float64
   （每通道，> 0）; Units string（宣告字串，預設 `normalized_unit`，框架不換算）; Transport
   *Transport}`；狀態 `ChemistryState{Concentration [][]float64 /*[region][channel] ≥ 0*/; Steps uint64}`。
   每步用主規格 11.3 原式：`lambda_k = exp(-dt/tau_k)`，
   `c'[r,k] = lambda_k*c[r,k] + tau_k*(1-lambda_k)*q[r,k]`。`q` 由 ticket 20 的來源提供，負值或非有限
   在進入前就被拒絕，濃度永遠不會為負。**區域傳輸**：`Transport{Fraction [][]float64}`，`Fraction[r][s]`
   是每步從 r 流到 s 的比例（≥ 0，`r != s`，每列總和 ≤ 1，建構時驗證），在本地更新後套用：
   `c''[s] = c'[s]*(1 - Σ_t Fraction[s][t]) + Σ_r c'[r]*Fraction[r][s]`。這樣非負性由構造保證，質量不因
   傳輸增加，清除只靠 `tau`。傳輸是化學資料，與神經接線分開，不得由邊陣列推導。
   參考測試：固定 `q` 下 `c → tau*q`（步數足夠後相對誤差 < 1e-6，手算幾何級數）；`q = 0` 時指數清除
   到 0；`c = 0, q = 0` 的中性狀態逐步保持 0；隨機非負 `q` 跑 1,000 步全非負；傳輸後總量 ≤ 傳輸前；
   負 `q`、非有限、`tau ≤ 0`、列總和 > 1 都被拒絕。
2. **受體（MOD-03）**：`Receptor{Cells []int（節點索引，遞增）; CellType string（可空，只作標示）; Channel
   int; Signal string（通道名稱）; Status string ∈ unresponsive|unknown|hypothesized; Kd, N float64;
   Evidence string; MeasurementKind string; MappingVersion string}`，欄位對應主規格 5.5 的 `ReceptorRecord`
   （適用細胞／類型、訊號名稱、證據來源、量測種類、未知狀態、工程映射版本），缺 `Evidence`、
   `MeasurementKind` 或 `MappingVersion` 的紀錄不接受。佔用率 `occupancy(c) = c^n/(Kd^n + c^n)`
   以對數域實作：`c = 0 → 0`，否則 `1/(1 + exp(n*(ln Kd − ln c)))`，`Kd > 0`、`n ≥ 1`；`c = 1e300, n = 8`
   不得出現 NaN／Inf，且 → 1；`c = Kd → 0.5`。**狀態語意**：`unresponsive` 的受體佔用率恆為 0 且報告
   標 `unresponsive`（不是量測到 0）；`unknown` 只有在紀錄同時給了 `Kd`／`N` 並在設定中把
   `AllowAssumedCoefficients` 設為 true 時才計算，報告一律標 `assumed: true`，否則佔用率為 0 並計入
   `unknown_skipped`；`hypothesized` 正常計算並標 `hypothesized`。任何報告都不得把 `unknown` 或
   `hypothesized` 的係數描述成量測值（文件與 JSON 欄位名都寫 `engineering_coefficients`）。
   同一細胞多個受體的混合規則明示：`Mix string ∈ sum|max`（對同一種效果，佔用率乘各自比例後相加或取
   最大），預設 `sum`，結果再進效果的有界映射。
3. **效果（MOD-04）**：`Effect{Kind string ∈ sensitivity|threshold; Receptor int（索引）; GammaScale,
   BetaScale float64（sensitivity）; GammaMin, GammaMax, BetaAbsMax float64（有界，預設 0.5／2／1）;
   ThetaScale, ThetaAbsMax float64（threshold）}`。每步對每個受體所在的細胞算出：
   - 當下敏感度（FiLM 形式）：`gamma = clamp(1 + GammaScale*occ, GammaMin, GammaMax)`，
     `beta = clamp(BetaScale*occ, −BetaAbsMax, BetaAbsMax)`，核心的輸入電流變成
     `I_eff = gamma*I + beta`（只作用於指定細胞，其餘 `gamma = 1, beta = 0`）。
   - 有效閾值（只對 LIF）：`m = clamp(ThetaScale*occ, −ThetaAbsMax, ThetaAbsMax)`，
     `theta_eff = max(theta_base + adaptation + homeostasis + m, theta_min)`；`m = 0` 時 `max` 是
     no-op（既有三項都 ≥ `theta_base` > `theta_min`），所以未啟用逐位不變。
   `occ = 0` 時 `gamma = 1, beta = 0, m = 0`：測試證明「宣告效果但濃度為 0」與「未宣告」的核心輸出
   逐位相同。效果只產生每步的暫時陣列，永遠不寫回 `Parameters`（測試：跑 100 步後 `Parameters` 逐位
   不變，包括 `theta_raw`）。
4. **dynamics 的接口**：`dynamics.Modulation{Gain, Offset [][]float64 /*[step][node]*/; Threshold
   [][]float64 /*LIF 專用*/}`，新增 `(*Continuous).AdvanceModulated(ctx, p, s, inputs, mod *Modulation)`
   與 `(*LIF).AdvanceModulated(...)`；既有 `Advance` 改為呼叫 `AdvanceModulated(…, nil)`，nil 與全中性
   （1／0／0）逐位等於原路徑（測試兩者）。`Forward`（episode 訓練路徑）本票不接調節，反向路徑維持
   不變；MOD-07 需要的可微路徑在 ticket 22 決定。長度不符或非有限的調節陣列拒絕整個呼叫。
5. **個體整合與順序**：`learning.Individual.EnableChemistry(c modulation.ChemistryConfig)`，其中
   `ChemistryConfig{Chemistry; Sources []SourceSpec（每通道一個，可序列化：kind + 該來源的宣告資料，
   `neural_activity` 以明示的節點索引保存）; Receptors []Receptor; Effects []Effect; Regions
   RegionAssignment{NodeRegion []int}（每個節點屬於哪個區域）}`。每步固定順序：
   來源釋放 `q`（`SourceContext.Activity` 是上一步的節點輸出；回饋由 `Individual.OfferFeedback` 佇列
   經 `signal.AvailableFeedback` 過濾；資源由 `SetResource` 提供）→ 濃度更新 → 佔用率 → 效果陣列 →
   核心 `AdvanceModulated` 一步 →（18 的可塑性若啟用，於其後）。單一 goroutine、逐步推進；
   `ChemistryReport{Concentration 最後一步; Occupancy 每受體; Assumed, UnknownSkipped, Unresponsive
   計數; ClampedGamma, ClampedBeta, ClampedTheta 計數}` 由 `Advance*` 回傳（與 18 的 `PlasticReport`
   併成一個 `AdvanceReport`，root 在派工時定案欄位名）。
6. **快照（COR-01、STA-03 前置）**：`IndividualSnapshot.Chemical *ChemicalPart{Config ChemistryConfig;
   State ChemistryState}`（`omitempty`；未啟用為 nil），`RestoreIndividual` 驗證形狀與非負；
   `checkpoint/individual.go` 視為可選區塊，出現時每個欄位必填，非法即拒絕；schema 維持
   `coimnet-individual-checkpoint/v1`。中途保存再接續與連續執行的濃度、佔用率與輸出逐位相同（測試）。
7. **證據**：`evidence/MOD-02/`、`evidence/MOD-03/`、`evidence/MOD-04/`（fixture）；文件：
   `docs/model-and-mechanisms.md` 機制表加三列，`docs/individual-state.md` 加化學區塊，ENG、README。

## 契約（第一階段：`modulation` 濃度、受體、效果 + `dynamics.Modulation`）

```go
package modulation
type Chemistry struct { Regions, Channels int; DT float64; Tau []float64; Units string; Transport *Transport }
type Transport struct { Fraction [][]float64 }
type ChemistryState struct { Concentration [][]float64; Steps uint64 }
func NewChemistry(c Chemistry) (*Kinetics, error)
func (k *Kinetics) NewState() ChemistryState
func (k *Kinetics) Step(s ChemistryState, release [][]float64) (ChemistryState, error)   // release[region][channel] ≥ 0
type Receptor struct { Cells []int; CellType, Signal string; Channel int; Status string; Kd, N float64; Evidence, MeasurementKind, MappingVersion string }
func Occupancy(c, kd, n float64) float64
type Effect struct { Kind string; Receptor int; GammaScale, BetaScale, GammaMin, GammaMax, BetaAbsMax, ThetaScale, ThetaAbsMax float64 }
type Receptors struct { Records []Receptor; Mix string; AllowAssumedCoefficients bool }
func (r *Receptors) Occupancies(state ChemistryState, regionOf []int) ([]OccupancyRecord, error)
func ApplyEffects(effects []Effect, occ []OccupancyRecord, nodes int) (gain, offset, threshold []float64, ClampReport, error)

package dynamics
type Modulation struct { Gain, Offset, Threshold [][]float64 }
func (m *Continuous) AdvanceModulated(ctx context.Context, p Parameters, s State, inputs [][]float64, mod *Modulation) (State, [][]float64, error)
func (m *LIF) AdvanceModulated(ctx context.Context, p LIFParameters, s LIFState, inputs [][]float64, mod *Modulation) (LIFState, [][]float64, [][]float64, error)
```

## 契約（第二階段：個體整合與快照）

```go
package learning
type ChemicalPart struct { Config modulation.ChemistryConfig `json:"config"`; State modulation.ChemistryState `json:"state"` }
// IndividualSnapshot.Chemical *ChemicalPart `json:"chemical,omitempty"`
func (i *Individual) EnableChemistry(c modulation.ChemistryConfig) error
func (i *Individual) DisableChemistry()
func (i *Individual) OfferFeedback(f signal.Feedback) error
func (i *Individual) SetResource(name string, value float64) error
// Advance / AdvanceGated 回傳的報告加入 ChemistryReport
```

## 驗收

- [ ] 第一階段：濃度五組參考測試（穩態手算、清除、中性、非負、傳輸不增量）與拒絕案例；佔用率
  極端值（`c = 1e300, n = 8`、`c = 0`、`c = Kd`）無 NaN／Inf；三種狀態分離且報告標示；混合規則
  手算；效果中性逐位等於未啟用（連續與 LIF 各一）；`AdvanceModulated(nil)` 逐位等於 `Advance`；
  `go test`、race、vet；`evidence/MOD-02/`、`evidence/MOD-03/`。
- [ ] 第二階段：個體每步順序手算（2 區域、1 通道、2 受體）；100 步後 `Parameters` 逐位不變；回饋
  早於可取得時間不進來源；中途快照接續逐位相同；`checkpoint` 拒絕非法化學區塊；
  `evidence/MOD-04/`；文件。

## 依據

- 主規格 5.5、8.2、11.3、11.4、11.5、15.1；規格抽取紀錄（root 工作紀錄 `spec-extract-30.md`
  的 MOD-02／03／04 段）。
