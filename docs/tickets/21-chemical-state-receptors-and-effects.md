# 21 — 研究者可以運行有時間衰退的化學濃度、設定選擇性受體，並調節當下敏感度與有效閾值

Epic：化學與荷爾蒙調節（第二層：濃度、受體、效果）

User Story：研究者可以讓 ticket 20 的釋放率驅動每個區域、每個通道的非負濃度，濃度依正時間常數
衰退並可在區域間傳輸；指定哪些細胞帶哪種受體、受體是「實驗指出不反應」「未知」還是「假設反應」，
以穩定的反應曲線算出佔用率；再把佔用率映射成當下敏感度（增益與偏移）或暫時的有效閾值調整。
中性值時逐位等於未啟用；暫時效果永遠不寫回基礎參數；個體快照保存化學狀態並能精確接續。

Blocked by：20 調節來源（釋放率 q）、16 LIF 個體（持續路徑與快照）、11 LIF 核心（`theta_base`）

Status：第一階段完成（2026-09-16，`modulation` 濃度／受體／效果與 `dynamics.Modulation` 已實作並驗證，
證據見下方「第一階段證據」）；第二階段（個體整合、報告、快照）待派工

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

- [x] 第一階段：濃度五組參考測試（穩態手算、清除、中性、非負、傳輸不增量）與拒絕案例；佔用率
  極端值（`c = 1e300, n = 8`、`c = 0`、`c = Kd`）無 NaN／Inf；三種狀態分離且報告標示；混合規則
  手算；效果中性逐位等於未啟用（連續與 LIF 各一）；`AdvanceModulated(nil)` 逐位等於 `Advance`；
  `go test`、race、vet；`evidence/MOD-02/`、`evidence/MOD-03/`。
- [ ] 第二階段：個體每步順序手算（2 區域、1 通道、2 受體）；100 步後 `Parameters` 逐位不變；回饋
  早於可取得時間不進來源；中途快照接續逐位相同；`checkpoint` 拒絕非法化學區塊；
  `evidence/MOD-04/`；文件。

## 第一階段證據（2026-09-16）

證據目錄：`evidence/MOD-02/`、`evidence/MOD-03/`，各有 `verification.json` 與 `test.log`。
環境：go1.26.5 darwin/arm64，共用機器，`uptime` 為 `19:13 up 11 days, 52 mins, load averages: 2.55 3.26 3.35`。
驗證命令全部通過：`gofmt -l .`（無輸出）、`go vet ./...`、`go test -count=1 ./...`、
`go test -race -count=1 ./modulation/ ./dynamics/ ./learning/ ./simulate/`。

### 先寫失敗測試

| 紅燈日誌 | 當時的失敗內容 |
| --- | --- |
| `evidence/MOD-02/red-kinetics.log` | `undefined: Chemistry`／`NewChemistry`／`Transport`，`modulation/chemistry.go` 還不存在 |
| `evidence/MOD-03/red-receptors.log` | `undefined: Receptor`／`Occupancy`／`Receptors`，`modulation/receptors.go` 還不存在 |
| `evidence/MOD-03/red-effects.log` | `undefined: Effect`／`ApplyEffects`／`ClampReport`，`modulation/effects.go` 還不存在 |
| `evidence/MOD-03/red-dynamics.log` | `undefined: Modulation`、兩個核心都 `AdvanceModulated undefined`，`dynamics/modulation.go` 還不存在 |

中性逐位相等的測試另做突變檢查，證明它抓得到問題，當場還原並與備份逐位元比對：
`evidence/MOD-03/red-neutral-zero-sign-mutation.log`——把 `modulatedDrive` 的逐節點中性守衛拿掉後，
中性調節與 `nil` 在 `outputs[0][3]` 差在零的正負號（`0x0` 對 `0x8000000000000000`），因為 `(-0)*1 + 0` 是正零。

### 濃度手算表（`dt = 1`、`tau = 4`，`lambda = exp(-0.25) = 0.7788007830714049`）

| 檢查 | 設定 | 期望 | 實際 |
| --- | --- | --- | --- |
| 穩態（60 步） | `q = 2`，`c(0) = 0` | 相對誤差 ≤ `lambda^60 = 3.0590232050182594e-07` | `c = 7.999997552781436`，相對誤差 `3.059023204743383e-07` |
| 穩態（200 步） | 同上 | 相對誤差 < 1e-6 | `c = 7.999999999999998`，相對誤差 `2.220446049250313e-16` |
| 清除一步 | `c = 8`、`q = 0` | `8*lambda = 6.230406264571239` | 逐位相同 |
| 清除兩步 | 同上 | `8*lambda^2 = 4.852245277701067` | 逐位相同 |
| 中性 | `c = 0`、`q = 0`、100 步 | 位元樣式 `0x0`（正零） | 兩區域兩通道全部 `0x0` |
| 非負 | 3 區域 2 通道、隨機 `q ∈ [0,3)`、1,000 步 | 無負值、無非有限 | 成立 |
| 傳輸 | `Fraction = [[0,0.5],[0.5,0]]`、`c' = [8*lambda, 0]` | 兩區各 `4*lambda = 3.1152031322856195`，總量仍是 `6.230406264571239` | 逐位相同 |
| 清除加速（`q = 0`） | `b = 0.75`，`rate = 1/4 + 0.75 = 1` | `8*exp(-1) = 2.9430355293715387`（低於未加速的 6.2304…） | 相對誤差 < 1e-15 |
| 清除加速（`q = 1`） | 同上 | `8*exp(-1) + (1-exp(-1))*1 = 3.5751560882000963` | 相對誤差 < 1e-15 |
| `b = 0` | 同上 | 逐位等於未加速那一支 | 相同 |

拒絕案例：無區域、無通道、`dt = 0`、`dt` 非有限、`tau` 個數不符、`tau ≤ 0`、`tau` 非有限、
`1-exp(-dt/tau)` 捨入為 0（永遠不會變的通道）、傳輸形狀錯、傳輸列長度錯、對角線非 0、負比例、
非有限比例、列總和 > 1（總和剛好 1 接受）；`Step` 另拒絕負／非有限／形狀錯／nil 的釋放、
負／非有限／形狀錯的清除加成、形狀錯／負／非有限的狀態濃度，以及步數溢位。

### 佔用率手算表與極端值

| `c` | `Kd` | `n` | 期望 | 實際 |
| --- | --- | --- | --- | --- |
| 2 | 1 | 1 | `2/3 = 0.6666666666666666` | 相同 |
| 4 | 2 | 2 | `16/(4+16) = 0.8` | 相同 |
| 2.5 | 2.5 | 3 | 0.5 | 相同 |
| 1e300 | 1e300 | 8 | 0.5 | 相同 |
| 0 | 1 | 1 | 0 | 相同 |
| 1e300 | 1 | 8 | 1（無 NaN／Inf） | 相同 |
| 1e-300 | 1 | 8 | 0（無 NaN／Inf） | 相同 |
| `MaxFloat64` | 1 | 8 | 1 | 相同 |
| 5e-324 | 1 | 8 | 0 | 相同 |
| 1e300 | 1e-300 | 8 | 1 | 相同 |

定義域外（`c < 0`、`Kd ≤ 0`、`n < 1`、任一非有限）一律回 NaN。

### 三種狀態分離（3 節點 2 區域 2 通道，通道 0 在區域 0 為 2、區域 1 為 4）

| 受體 | 狀態 | 係數 | `AllowAssumedCoefficients = false` | `= true` |
| --- | --- | --- | --- | --- |
| 0（節點 0、1） | hypothesized | Kd 1、n 1 | 0.6666666666666666，`assumed=false` | 同左 |
| 1（節點 2） | unknown | Kd 2、n 2 | 0，`skipped=true` | 0.8，`assumed=true` |
| 2（節點 0，通道 1） | unresponsive | 有也不看 | 0，非 assumed 非 skipped | 同左（`c = 1e300` 時仍為 0） |
| 3（節點 1） | unknown | 無 | 0，`skipped=true` | 0，`skipped=true` |

計數：`{assumed 0, unknown_skipped 2, unresponsive 1}` 與 `{assumed 1, unknown_skipped 1, unresponsive 1}`。
JSON 欄位名為 `engineering_kd`／`engineering_n`。

### 效果手算表（細胞 0 同時有 0.75 與 0.5 兩個佔用率，細胞 2 只有 0.25；全為二進位精確值）

| mix | 細胞 0 `occ` | `gamma = 1 + 0.5*occ` | `beta = 0.25*occ` | 細胞 2 |
| --- | --- | --- | --- | --- |
| sum（含空字串） | 1.25 | 1.625 | 0.3125 | 1.125／0.0625 |
| max | 0.75 | 1.375 | 0.1875 | 1.125／0.0625 |

閾值效果（只掛受體 0，`ThetaScale = 0.5`）：細胞 0 得 0.375、細胞 2 得 0.125，兩種 mix 相同。

| 夾限檢查 | 宣告 | 結果 | `ClampReport` |
| --- | --- | --- | --- |
| gamma 上界 | `GammaScale 4` | 2（預設上界） | `{Gamma:1}` |
| gamma 下界 | `GammaScale -4` | 0.5（預設下界） | `{Gamma:1}` |
| beta 上／下界 | `BetaScale ±4` | ±1 | `{Beta:1}` |
| 自訂界線優先 | `GammaMin 0.25, GammaMax 1.5, BetaAbsMax 0.25` | 1.5／0.25 | `{Gamma:1, Beta:1}` |
| theta 上／下界 | `ThetaScale ±4, ThetaAbsMax 0.5` | ±0.5 | `{Theta:1}` |
| 界內 | `GammaScale 0.5, BetaScale 0.25` | 1.375／0.1875 | `{}` |

`occ` 全為 0 時回傳的是逐位精確的 `gain = 1`、`offset = +0`、`threshold = +0`，
與「完全不宣告效果」逐位相同（三種 mix 都測）。

### `dynamics` 兩項手算檢查（`dt = 1`、`tau = 1`，`alpha = 1-exp(-1) = 0.6321205588285577`）

| 檢查 | 設定 | 期望 | 實際 |
| --- | --- | --- | --- |
| 連續核心增益 | 初始電位 0、輸入 `[1, 1]`、`gain = [2, 1]`、`offset = [0, 0.5]` | 節點 0 電位 `alpha*2 = 1.2642411176571153`（恰為未調節的兩倍）、節點 1 `alpha*1.5 = 0.9481808382428365` | 逐位相同；輸出 `tanh` 為 0.8522291308053213 與 0.7389583695421298 |
| 增益不吃 bias | 單節點、bias 0.25、輸入 0、`gain = 2` | 與未調節逐位相同 | 相同 |
| 增益吃突觸（含延遲） | 一條 `w = 0.5` 的邊、前史輸出 `tanh(1)`、`gain = 2` | `alpha*0.5*tanh(1) = 0.2407096617316609` → `alpha*tanh(1) = 0.4814193234633218` | 恰為兩倍 |
| LIF 閾值抑制 | `theta_base = 0.1 + 1.9*0.5 = 1.05`、輸入 2 → 候選 `alpha*2 = 1.2642411176571153` | 未調節放電；`+0.5`（1.55）抑制；`+0.2`（1.25）仍放電 | 相同 |
| LIF 增益 | 輸入 1（候選 0.632，未調節不放電）、`gain = 2` | 放電 | 相同 |
| `theta_min` 下限 | `threshold = -1e9`，有效閾值應為 0.1 | 候選 `alpha*0.2 = 0.12642411176571153` 放電、`alpha*0.1 = 0.06321205588285576` 不放電 | 相同 |

`AdvanceModulated(…, nil)` 與 `Advance` 逐位相同，全中性（1／0／0）與 `nil` 也逐位相同：連續核心用含延遲的
四節點 fixture（其中一個節點的輸入、bias 與初始電位都是負零），LIF 用隨機 fixture 的四種組合
（適應開關 × 不應期 0／2，慢速穩定開啟），比對輸出、放電、電位、適應、活動估計、閾值偏移與不應期計數。
形狀不符、非有限、連續核心上非零的 `Threshold` 都拒絕整個呼叫，且不動傳入的狀態；連續核心接受全零的
`Threshold` 陣列並視為中性。另有一條端到端測試：宣告好的化學、受體與效果在濃度為 0 時，兩個核心的輸出
與未宣告逐位相同，且 `Parameters`／`LIFParameters` 事後未被改寫。

### 與票面的偏離與補充決策

| 項目 | 票面 | 實作 | 理由 |
| --- | --- | --- | --- |
| `Kinetics.Step` 參數 | `Step(s, release)` | `Step(s, release, clearanceBoost)` | ticket 20 的 `RewardRecord.ClearanceBoost` 需要落點；`b` 進清除率 `rate = 1/tau + b`，穩態同形為 `q/rate`，`b = 0` 時逐位走票面原式 |
| `Occupancies` 回傳 | `([]OccupancyRecord, error)` | `([]OccupancyRecord, OccupancySummary, error)` | root 決策要求報告 `Assumed`／`UnknownSkipped`／`Unresponsive` 計數，與紀錄一起回傳比讓呼叫端重數可靠 |
| `ApplyEffects` 簽名 | `(effects, occ, nodes)` | `(effects, occ, mix, nodes)` | 混合規則屬於受體設定，效果層需要知道它；`Receptors.Mix` 由呼叫端傳入 |
| 混合語意 | 「佔用率乘各自比例後相加或取最大」 | 先依 mix 累加原始佔用率，再套一組係數 | 每個公式只指名一組係數；同種效果打到同一顆細胞而係數不同時直接拒絕，不自訂規則。係數相同時兩種讀法結果一致 |
| `Occupancy` 定義域外 | 未定義 | 回 NaN | 函式沒有錯誤回傳；NaN 是明顯的「未定義」而不是看起來像量測的 0，`Validate` 是真正的守門 |
| 額外拒絕 | 未提 | `1-exp(-dt/tau)` 捨入為 0 的通道；宣告了該 kind 不讀的欄位（如 sensitivity 帶 `ThetaScale`） | 兩者都是「永遠不會生效的旋鈕」，沿用 `RewardMapper` 對 `Window` 的既有作法 |
| 增益作用範圍 | `I_eff = gamma*I + beta` | `I` 為「外部輸入 + 突觸（含延遲）」，bias 在增益之外 | bias 是神經元的可訓練參數，不是輸入電流 |
| `theta_min` 夾限 | `theta_eff = max(…, theta_min)` | 只在該節點 `threshold != 0` 時套用 | 其餘情況本來就是 no-op（`theta_base ≥ theta_min`，適應與慢速穩定只會抬高），跳過才能保證未啟用逐位不變 |
| 連續核心的 `Threshold` | 「LIF 專用」 | 全零陣列視為中性並接受，只有非零才拒絕 | 讓效果層可以無條件把三個陣列交給任一核心，中性測試也才測得到 |
| `alpha` 的算法 | `tau*(1-lambda)*q` | 直接寫 `1 - lambda`，不用 `expm1` | 這是規格指名的式子，也讓 `tau*q` 是遞迴的精確不動點；代價是 `dt/tau` 低於 float64 解析度時 `alpha` 為 0，已改為建構時拒絕 |
| 係數的 JSON 名稱 | 內文寫 `engineering_coefficients` | 欄位名為 `engineering_kd`／`engineering_n` | 同樣不會被誤讀成量測值，而且是逐欄位的名稱 |
| 新增 API | 未提 | `Kinetics.Config()`、`UnitNormalized`、`Status*`／`Mix*`／`Effect*`／`Default*` 常數 | 設定要能被第二階段序列化與比對；字串常數避免各處手寫字面值 |

尚未做到（第二階段或另票）：個體整合與每步順序、`ChemistryReport`、快照、`Forward`／反向的可微調節路徑
（MOD-07，ticket 22）、跑 100 步後 `Parameters` 逐位不變的長跑檢查、真實資料與全腦規模。

## 依據

- 主規格 5.5、8.2、11.3、11.4、11.5、15.1；規格抽取紀錄（root 工作紀錄 `spec-extract-30.md`
  的 MOD-02／03／04 段）。
