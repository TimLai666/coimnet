# 12 — 研究者可以不經訓練直接執行接線圖（LIF 持續狀態與原生模擬 runner）

Epic：原生模擬

User Story：研究者可以把 GraphStore 讀回的接線接上任一動態模型，用固定注入把刺激送進
指定神經元、以探針讀指定神經元的活動，不經 encoder、readout 或訓練器，並在真實全圖上
量到容量與時間。

Blocked by：09 GraphStore、11 LIF 核心

Status：draft（root 決策已定，待派工；驗收項目驗證後才勾選）

對應需求：NAT-01（原生執行）、COR-03 的持續狀態部分。參數推導屬 13，空模型與判讀屬 14。

## Root 決策（2026-09-14）

1. **LIF 持續狀態**：`dynamics.LIFState{SchemaVersion, ConfigHash, Steps, Voltage, History
   [][]float64（突觸跡 x 的延遲史，語意同連續核心 History）, Adaptation []float64,
   Refractory []int}`，`LIFStateVersion = "coimnet-lif-state/v1"`；`(*LIF).NewState(initial)`、
   `ValidateState`、`Advance(ctx, p, s, inputs) (LIFState, [][]float64 outputs, [][]float64
   spikes, error)`。History 保留 `max(0, steps-maxDelay)` 至 `steps` 的 x，零時刻以前為零。
   `Forward` 整段與 `Advance` 分段推進必須逐位相等（含放電、不應期、適應）。容量沿用
   `MaxStateValues`。
2. **runner 不碰 Insyra**：新套件 `simulate`。注入為固定線性映射
   `Injection{Channel int, Node int, Gain float64}` 的列表（一個通道可注入多個節點），
   每步 `input[node] += gain * stimulus[t][channel]`；沒有可訓練矩陣。探針
   `Probe{Name, Nodes []int, Reduce}`，`Reduce ∈ {mean_output, sum_output, spike_count,
   spike_fraction}`（連續核心只允許前兩種）。節點可直接給索引，或以
   `Selector{Field, Equals}` 由 `connectome.Graph` 的節點註記解析（欄位限 class／type／
   superclass／subclass／instance／soma_side），解析結果與來源欄位寫進報告。
3. **核心無關**：runner 以私有介面包連續核心與 LIF；`Protocol` 指定 `core: continuous|lif`
   與各自設定；參數集由呼叫端提供（13 之前用明示的工程假設：`weight = gain *
   raw_weight / max(1, post_total)`… 這條規則屬 13，本票的真實全圖實測只允許
   「uniform_positive」：所有邊 `weight = gain * raw_weight`、全部興奮、統一 tau／theta，
   報告必須標 `parameter_source: "engineering_uniform_positive"`，不得標為生物參數）。
4. **監控（主規格 8.5）**：每次執行報告沉默節點比例、每步全群放電比例、非有限值（立即
   失敗）、放電率分布分位數、探針時序；提供 `max_population_rate` 與 `min_active_fraction`
   的可選門檻，超出時報告 `stability_flags`，不自動改參數。
5. **輸出與可重現**：`RunReport{schema_version "coimnet-simulate-run/v1", core config hash,
   graph store hashes（node index／edge order）, parameter set hash, protocol hash, steps,
   probes 時序, monitors, resource（wall time、peak RSS 由 CLI 記錄）}`；相同輸入兩次執行
   位元相同；狀態可在任意步保存（`LIFState`／`State` JSON）並接續得到相同輸出。
6. **CLI**：`coimnet simulate run --store FILE --protocol FILE [--params FILE] [--steps N]
   [--state-out FILE] [--state-in FILE]`，protocol 為嚴格 JSON（注入、探針、刺激序列或
   刺激產生器規格、核心設定、門檻）；輸出 RunReport JSON；門檻旗標不改變退出碼，錯誤才
   非零。`--params` 缺省時只允許 `engineering_uniform_positive` 且必須在 protocol 明示。
7. **容量**：從 GraphStore 建 `dynamics.LIFConfig`／`Config` 時邊陣列 `[]int` 複製一次，
   25,563,197 條邊約 400 MB 加權重 200 MB；`NewLIF`／`NewContinuous` 再複製一次。全圖實測
   限制 8 GiB 帳面記憶體，超出拒絕。LIF 個體（`learning.Individual`）本票不做，維持明確
   錯誤，待可塑性票。

## 契約（第一階段：dynamics）

```go
const LIFStateVersion = "coimnet-lif-state/v1"
type LIFState struct { SchemaVersion string; ConfigHash string; Steps uint64; Voltage []float64; History [][]float64; Adaptation []float64; Refractory []int }
func (m *LIF) NewState(initial []float64) (LIFState, error)
func (m *LIF) ValidateState(s LIFState) error
func (m *LIF) Advance(ctx context.Context, p LIFParameters, s LIFState, inputs [][]float64) (LIFState, [][]float64, [][]float64, error)
```

## 契約（第二階段：simulate 與 CLI）

```go
type Selector struct { Field, Equals string }
type Injection struct { Channel int; Node int; Gain float64 }            // 或 NodeSelector *Selector
type Probe struct { Name string; Nodes []int; Selector *Selector; Reduce string }
type Protocol struct { SchemaVersion string; Core string; Continuous *dynamics.Config; LIF *dynamics.LIFConfig; Injections []Injection; Probes []Probe; Stimulus StimulusSpec; Thresholds Thresholds; ParameterSource string }
func Build(ctx, g *connectome.Graph, params ParameterSet, protocol Protocol) (*Runner, error)
func (r *Runner) Run(ctx, stimulus [][]float64) (RunReport, error)
func (r *Runner) State() any / SaveState / LoadState
```

`ParameterSet` 的檔案格式在 13 定案；本票先用記憶體型別 `simulate.ParameterSet{Weights,
Bias, LogTau, ThetaRaw, Source string, Hash string}`，由 `engineering_uniform_positive`
產生器或 13 的檔案讀取器提供。

## 驗收

- [ ] LIF `Advance` 與 `Forward` 逐位一致（含延遲、不應期、適應），狀態 JSON 往返與
  fingerprint 檢查，非法狀態拒絕；`go test`、race、vet。
- [ ] runner fixture：三顆神經元固定注入與探針，手算探針時序；兩種核心同一 protocol；
  選擇器由註記解析並寫入報告；監控與門檻旗標；取消與非有限值不留半次狀態；兩次執行
  位元相同；中途保存狀態再接續結果相同。
- [ ] CLI：help、protocol 錯誤、缺 store、缺 params 但未宣告工程假設、輸出失敗皆有測試。
- [ ] 真實全圖：`data/malecns-v1.0/graph-v1.coimgraph` 以 `engineering_uniform_positive`
  跑 ≥ 200 步，記錄命令、環境、指紋、牆鐘時間、RSS、放電率分布與穩定旗標；報告明示這是
  工程假設，不是生物參數。
- [ ] 文件：README、ENG.md、機制文件、delivery-status 更新；`evidence/NAT-01/`。

## 探索紀錄（2026-09-14，唯讀盤點）

- `learning.Individual` 整條路徑綁定連續核心：`IndividualProfile` 字串含 `continuous`、
  `IndividualSnapshot.Neural` 是具體的 `dynamics.State`、`NewIndividual`／`RestoreIndividual`／
  `Advance`／`ResetNeural` 都經 `core.continuous()`；`checkpoint/individual.go` 的必填檢查要求
  `config.dynamics` 與 `neural{schema_version,config_hash,steps,voltage,history}` 的連續形狀。
  因此 runner 不得建立在 `Individual` 之上；LIF 個體另開票時要新增 profile／schema 版本
  （`coimnet-individual/v1`、`coimnet-individual-checkpoint/v1` 的 LIF 變體與
  `coimnet-lif-state/v1`），並讓 `coreModel` 提供 LIF 的狀態操作。
- `learning.NewNetwork`／`NewTrainer` 要求 `InputSize`、`OutputSize`、`ReadoutNodes`、encoder
  與 readout 矩陣都存在，`Advance` 一律經 Insyra float32 matmul；這正是原生 runner 要繞開的。
- `signal.Projection`／`BindProjections` 只產生 encoder／readout 矩陣並以完整前向驗證，
  runner 的固定注入不沿用它，但選擇器解析可共用 `connectome.Graph` 的節點註記。

## 依據

- [研究方向](../research-directions/connectome-native.md)、主規格 7.2（固定模型評估）、
  8.5（活動穩定監控）、15.4（個體隔離）、16.2（`run` 與 `--dry-run` 精神）。
