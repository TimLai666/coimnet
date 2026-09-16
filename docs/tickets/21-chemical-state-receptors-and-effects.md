# 21 — 研究者可以運行有時間衰退的化學濃度、設定選擇性受體，並調節當下敏感度與有效閾值

Epic：化學與荷爾蒙調節（第二層：濃度、受體、效果）

User Story：研究者可以讓 ticket 20 的釋放率驅動每個區域、每個通道的非負濃度，濃度依正時間常數
衰退並可在區域間傳輸；指定哪些細胞帶哪種受體、受體是「實驗指出不反應」「未知」還是「假設反應」，
以穩定的反應曲線算出佔用率；再把佔用率映射成當下敏感度（增益與偏移）或暫時的有效閾值調整。
中性值時逐位等於未啟用；暫時效果永遠不寫回基礎參數；個體快照保存化學狀態並能精確接續。

Blocked by：20 調節來源（釋放率 q）、16 LIF 個體（持續路徑與快照）、11 LIF 核心（`theta_base`）

Status：兩階段皆完成（2026-09-16）。第一階段 `modulation` 濃度／受體／效果與 `dynamics.Modulation`，
證據見下方「第一階段證據」；第二階段個體整合、每步順序、`ChemistryReport` 與快照，證據見「第二階段證據」

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
- [x] 第二階段：個體每步順序手算（2 區域、1 通道、2 受體，連續與 LIF 兩個核心各一張表）；100 步後
  `Parameters` 逐位不變（含 `theta_raw`）；回饋早於可取得時間不進來源；中途快照接續逐位相同；
  `checkpoint` 拒絕非法化學區塊；`evidence/MOD-04/`；文件。

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

## 第二階段證據（2026-09-16）

證據目錄：`evidence/MOD-04/`，有 `verification.json`、`test.log` 與三份紅燈日誌。
環境：go1.26.5 darwin/arm64、macOS 26.6.2 arm64，共用機器，`uptime` 在這批命令前是
`19:46 up 11 days, 1:24, load averages: 3.86 3.88 3.94`，之後是 `19:47 …, 6.70 4.58 4.19`。
驗證命令全部通過：`gofmt -l .`（無輸出）、`go vet ./...`、`go test -count=1 ./...`、
`go test -race -count=1 ./modulation/ ./learning/ ./checkpoint/ ./dynamics/`、
`go list -deps ./modulation | grep TimLai666`、`bash scripts/check-target-flow.sh`。

### 先寫失敗測試

| 紅燈日誌 | 當時的失敗內容 |
| --- | --- |
| `evidence/MOD-04/red-config.log` | `undefined: ChemistryConfig`／`SourceSpec`／`RegionAssignment`，`modulation/config.go` 還不存在 |
| `evidence/MOD-04/red-individual.log` | `EnableChemistry`／`DisableChemistry`／`OfferFeedback`／`ChemistryReport` 在 `*learning.Individual` 上都還不存在 |
| `evidence/MOD-04/red-snapshot.log` | `checkpoint` 還沒有 `requireIndividualChemical`，三個手改過的化學區塊（少 `steps`、少 `resources`、少 `pending_feedback`）被接受 |

`learning` 這一側的快照（`IndividualSnapshot.Chemical`、`Snapshot`／`RestoreIndividual`）與個體整合寫在同一次
改動裡，測試緊接在後而不是在前；`checkpoint` 那一半是先寫測試的。這一點也寫進 `verification.json` 的 limitations。

### 每步順序（固定，不可重排）

來源釋放（活動＝上一步的節點輸出，回饋先過 `signal.AvailableFeedback`，資源由 `SetResource` 提供）
→ 濃度一步 → 佔用率 → 效果陣列 → `AdvanceModulated` 走一步 → 可塑性（若啟用）。
步數是個體自己的模型步數（持續神經狀態的 `Steps`），釋放與回饋過濾用的是同一個數。

### 連續核心手算表（2 節點 2 區域 1 通道，`dt = 1`、`tau = 2`、`lambda = exp(-0.5) = 0.6065306597126334`）

宣告：`external_timeline` 在 step 1 釋放 1；受體 0 `hypothesized`（節點 0，`Kd = 0.5`、`n = 1`），
受體 1 `unresponsive`（節點 1）；效果 `sensitivity`、`GammaScale = 1`；節點 0 每列輸入 1。
膜更新 `v' = exp(-1)*v + (1-exp(-1))*gamma`。

| 列 | 濃度 期望／實際 | 佔用率 期望／實際 | gamma 期望／實際 | 節點 0 電位 期望／實際 |
| --- | --- | --- | --- | --- |
| 0 | 0／0 | 0／0 | 1／1 | 0.6321205588285577／0.6321205588285577 |
| 1 | 0.7869386805747332／相同 | 0.611481100422978／0.6114811004229779 | 1.6114811004229779／相同 | 1.2511944916758613／1.2511944916758615 |
| 2 | 0.4773024370823822／相同 | 0.48838764641507565／相同 | 1.4883876464150756／相同 | 1.401129161199922／1.4011291611999221 |
| 3 | 0.289498562046025／相同 | 0.3666866235902634／相同 | 1.3666866235902635／相同 | 1.379357325078631／相同 |

差異最多 1 ULP，全部在 1e-12 內。節點 1 每列電位是逐位精確的 0；不反應的受體每列都回報宣告的 0，
狀態標 `unresponsive`，計數為 `{assumed 0, unknown_skipped 0, unresponsive 1}`。
一次四列的呼叫與四次一列的呼叫結果相同，釋放總量回報 `[1]`。

### LIF 核心手算表（同一份化學，效果改為 `threshold`、`ThetaScale = 0.5`、`ThetaAbsMax = 1`）

`theta_min 0.1`、`theta_max 2`、`theta_raw 0`，所以 `theta_base = 1.05`；節點 0 每列輸入 2。

| 列 | 閾值位移 期望／實際 | 有效閾值 | 候選膜電位 | 事件 | 電位 期望／實際 |
| --- | --- | --- | --- | --- | --- |
| 0 | 0／0 | 1.05 | 1.2642411176571153 | 放電 | -0.5／-0.5 |
| 1 | 0.305740550211489／0.30574055021148894 | 1.355740550211489 | 1.0803013970713942 | 抑制 | 1.0803013970713942／相同 |
| 2 | 0.24419382320753782／相同 | 1.2941938232075378 | 1.6616617919084682 | 放電 | -0.5／-0.5 |
| 3 | 0.1833433117951317／相同 | 1.2333433117951318 | 1.0803013970713942 | 抑制 | 1.0803013970713942／相同 |

同一份 fixture 不宣告效果時四列全部放電（每個候選都越過 1.05，每列都落到重設值 -0.5），測試裡一併驗證，
所以被抑制的事件確實是效果造成的。第 2 列會放電，正是因為第 1 列被抑制後膜電位留在 1.08。

### 其餘驗收項目

| 檢查 | 結果 |
| --- | --- |
| 未啟用逐位相同 | 開啟再關閉後，兩個核心的輸出與電位與從未啟用逐位相同；未啟用時 `ChemistryReport()` 回零值 |
| 濃度為 0 逐位相同 | 宣告好的化學但來源不釋放時，佔用率精確為 0、無夾限，兩個核心輸出與電位逐位等於未宣告 |
| `Parameters` 不被寫回 | LIF 核心跑 100 列、每 3 步一個脈衝、結束時佔用率仍大於 0，weights／bias／log_tau／theta_raw／encoder／readout 位元樣式全部不變 |
| 回饋不提早進來源 | `AvailableAt = 3` 的回饋排隊後，step 0–2 與 step 3 都推進成功；同一個來源拿到未過濾的佇列在 step 0 直接拒絕，所以成功不是沉默略過 |
| 活動來源 | 神經來源在全新個體的第一列拒絕（沒有上一步可平均），暖機一步後釋放的正是該步節點 0 的輸出 `tanh(1-exp(-1)) = 0.5595106570525964`，經一步動力學得到 0.44030057822847224 |
| 資源 | 未宣告資源時來源拒絕且該次推進不提交任何步；`SetResource("energy", 0.75)` 後依宣告規則 `2*max(0.75-0.25,0) = 1`，濃度同為 0.7869386805747332 |
| 中途快照接續 | 3 列 → JSON 快照 → 還原 → 3 列，與不中斷的 6 列在輸出、濃度、佔用率、釋放總量與電位上逐位相同，使用的是讀上一步活動的神經來源 |
| `RestoreIndividual` 拒絕 | 11 種壞掉的化學區塊（區域數不符、負值、非有限、區域圖節點數不符、缺來源、受體缺 mapping_version、效果指向不存在的受體、資源非有限、資源無名稱、回饋時間單位錯、回饋本身不合法） |
| `checkpoint` 可選區塊 | 未啟用時不寫 `chemical` 鍵；缺鍵載入為未啟用且其餘部分不變；啟用時存讀逐位相同且文件中沒有任何 `null`；16 種手改區塊被拒絕 |
| 與可塑性併用 | 兩者同時啟用時，靜默的化學讓輸出、`PlasticReport` 與快速狀態逐位等於只開可塑性；同一宣告帶脈衝時輸出不同，所以這個 fixture 不是空測 |
| import 相依 | `go list -deps ./modulation` 只列 `internal/jsonkey`、`internal/strictjson`、`signal`，沒有 `simulate` 與 `connectome`；`scripts/check-target-flow.sh` 仍然通過 |

### 與票面的偏離與補充決策（第二階段）

| 項目 | 票面 | 實作 | 理由 |
| --- | --- | --- | --- |
| `NeuralActivity` 的集合 | `simulate.ResolvedSet` | `Nodes []int`（遞增、非空）＋ `SetName string`（只作標示） | root 2026-09-16 決策。`modulation` import `simulate`、`simulate` → `connectome`，而 `connectome` 的套件內測試 import `learning`，所以 `learning` 一旦 import `modulation` 就在測試 binary 形成 import cycle。呼叫端改為傳 `set.Nodes()` 與 `set.Name`，解析仍然只能來自 `ResolveSets`。ticket 20 的手算數字不變 |
| `experiment/target_taint_test.go` | 不在本票可改範圍 | 改了一行，改用新的 `NeuralActivity` 欄位 | 上一列的型別改動會讓這個測試檔無法編譯。只動建構那一行，語意與斷言不變 |
| 報告的回傳方式 | 與 18 的 `PlasticReport` 併成一個 `AdvanceReport` | `Advance`／`AdvanceGated` 簽名不變，另加 `ChemistryReport()` | 改簽名會直接打斷 ticket 23 的 `OnlineLearner`（`Act` 用 `Advance`、`Update` 用 `AdvanceGated`）。報告是「最近一次推進」的結果，推進失敗時不覆寫 |
| `SourceSpec` 與通道的關係 | 「每通道一個來源」 | `Sources[k].Channel` 必須等於 `k`，來源本身宣告的通道數必須剛好 `Channel+1`，而且執行時只讀 `rates[Channel]`，其他位置非零就拒絕 | 四種來源本來就把 `Channel` 當成釋放向量的最後一格；這樣宣告過寬或錯位的來源會被拒絕，不會被安靜丟掉 |
| 來源的區域範圍 | 未指明 | 本階段來源對所有區域全域：一個通道的釋放率加到每個區域 | 票面沒有決定每個來源屬於哪個區域。全域是唯一不需要新宣告欄位的讀法，已寫進文件與 `verification.json` 的 limitations，逐區域來源留給後續決策 |
| `ChemicalPart` 的內容 | `{Config, State}` | 另加 `Resources map[string]float64` 與 `PendingFeedback []signal.FeedbackSpec` | 少了這兩項，中途快照接續時資源來源會拒絕、回饋佇列會消失，「接續逐位相同」就不成立 |
| 上一步活動 | 未指明怎麼保存 | 不另外保存，從持續神經狀態的延遲歷史最新一列讀回 | 那一列就是上一步的節點輸出（連續是活化輸出，LIF 是突觸跡）。多存一份會是同一個數字的第二個真相來源 |
| 報告的計數語意 | 只寫「計數」 | `Assumed`／`UnknownSkipped`／`Unresponsive` 描述最後一步的佔用率紀錄（與 `Occupancy` 一致），三個夾限計數則整次呼叫累加（與 `PlasticReport` 一致） | 兩組計數描述的東西不同：一組數的是紀錄，一組數的是每列發生的事件 |
| `SetResource`／`OfferFeedback` | 未指明前提 | 必須先啟用化學層，否則拒絕 | 個體其他部分都不讀這兩項，未啟用時快照也沒有地方放，拒絕比存了又在下次保存時消失誠實 |
| 連續核心上的 threshold 效果 | 「只對 LIF」 | 不在 `EnableChemistry` 拒絕，等位移第一次非零時由連續核心拒絕 | 宣告了 threshold 效果但濃度為 0 時，結果與未宣告逐位相同；提早拒絕會打破「中性等於未啟用」 |
| 回饋佇列 | 未指明 | 抵達後不移出佇列 | 四種來源都不讀分數，留著不改變任何數字；`pending_feedback` 是宣告不是消耗品。代價是每步都排入回饋的呼叫端佇列會一直長大，已寫進 limitations |
| 新增 API | 未提 | `ChemistryConfig.Validate/ValidateState/Clone`、`SourceSpec.Build`、`NeuralActivity.Validate`、`Source*` 常數、`coreModel.advanceModulated` | 設定要能在啟用與還原時被完整檢查並被個體擁有；`advanceModulated` 讓 `advance` 變成它的 nil 呼叫，兩者逐位相同 |

尚未做到（另票）：`Forward`／反向的可微調節路徑（MOD-07，ticket 22）、逐區域來源、真實資料與全腦規模、
跨平台執行。

## 依據

- 主規格 5.5、8.2、11.3、11.4、11.5、15.1；規格抽取紀錄（root 工作紀錄 `spec-extract-30.md`
  的 MOD-02／03／04 段）。
