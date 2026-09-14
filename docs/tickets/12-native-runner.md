# 12 — 研究者可以不經訓練直接執行接線圖（LIF 持續狀態與原生模擬 runner）

Epic：原生模擬

User Story：研究者可以把 GraphStore 讀回的接線接上任一動態模型，用固定注入把刺激送進
指定神經元、以探針讀指定神經元的活動，不經 encoder、readout 或訓練器，並在真實全圖上
量到容量與時間。

Blocked by：09 GraphStore、11 LIF 核心

Status：verified_scoped（第一階段 dynamics 與第二階段 simulate／CLI 皆已驗證，全圖實測見「第二階段證據」。範圍限制：只在 macOS arm64 實測；參數來源只有 `engineering_uniform_positive`；CLI 無 `--params`／`--steps`；LIF 個體仍未支援）

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

- [x] LIF `Advance` 與 `Forward` 逐位一致（含延遲、不應期、適應），狀態 JSON 往返與
  fingerprint 檢查，非法狀態拒絕；`go test`、race、vet。（Opus 先寫失敗測試再實作；
  30 組隨機設定 × 4 種切分逐位相等；六種突變皆被測試擋下；`Advance` 拒絕測試用平滑模式；
  突觸跡上限採相對容差 `1/(1-kappa)` 加 `1e-9*(1+1/(1-kappa))`。）
- [x] runner fixture：三顆神經元固定注入與探針，手算探針時序；兩種核心同一 protocol；
  選擇器由註記解析並寫入報告；監控與門檻旗標；取消與非有限值不留半次狀態；兩次執行
  位元相同；中途保存狀態再接續結果相同。（先寫失敗測試，紅燈紀錄在
  `evidence/NAT-01/red-simulate.log`；另加分塊邊界不改變數值的測試，因為全腦每次
  `Advance` 只能推進 6 步。）
- [x] CLI：help、protocol 錯誤、缺 store、缺 params 但未宣告工程假設、輸出失敗皆有測試。
  （紅燈紀錄在 `evidence/NAT-01/red-cli.log`。）
- [x] 真實全圖：`data/malecns-v1.0/graph-v1.coimgraph` 以 `engineering_uniform_positive`
  跑 ≥ 200 步，記錄命令、環境、指紋、牆鐘時間、RSS、放電率分布與穩定旗標；報告明示這是
  工程假設，不是生物參數。（300 步，93.08 s，4.38 GB RSS，見下節。）
- [x] 文件：README、ENG.md、機制文件、delivery-status、requirements-status 更新；
  `evidence/NAT-01/`。（Opus 更新 README／ENG；root 審查後補機制文件、delivery-status 與
  NAT-01 狀態。）

## 第二階段證據（2026-09-15）

命令：`scripts/simulate-evidence.sh evidence/NAT-01/fullgraph-uniform-v1
evidence/NAT-01/protocol-fullgraph-uniform.json`，實際執行
`coimnet simulate run --store data/malecns-v1.0/graph-v1.coimgraph --protocol
evidence/NAT-01/protocol-fullgraph-uniform.json --state-out STATE`，外層為
`/usr/bin/time -l`。完整判讀與限制見 `evidence/NAT-01/verification.json`。

| 項目 | 實測 |
| --- | --- |
| 規模 | 165,122 節點、25,563,197 條邊、300 步 |
| 牆鐘時間 | 93.08 s（user 85.09 s，sys 6.35 s） |
| 最大 RSS | 4,378,574,848 B（peak memory footprint 5,192,913,168 B）；8 GiB 帳面上限未觸及 |
| 非有限值 | 無（`monitors.non_finite` 為 false） |
| 沉默比例 | 0.0206514（165,122 顆中 3,410 顆整段未放電） |
| 放電率分位數 | p0 0、p25 0.4733333、p50 0.4733333、p75 0.4766667、p100 0.4833333 |
| 全群放電比例 | 第 10 步起上升，第 130 步達最大 0.5466564；第 31 步之後在 0.4292 與 0.5466 之間兩步一循環（平均 0.4878） |
| 穩定旗標 | `["max_population_rate_exceeded"]`（門檻 0.2）；`min_active_fraction` 0.01 通過。旗標不改變退出碼 |
| 狀態快照 | 9,402,221 B，SHA-256 `c7d3104a…e9f1e417`（雜湊後刪除，屬衍生資料） |

注入與探針（皆由註記解析，解析結果寫進報告）：`class == "ALIN"` 24 顆（索引
350..132605，gain 1，1 通道脈衝 amplitude 2、第 10..29 步）；探針
`alin_injected` 24 顆、`descending_neuron`（`superclass == "descending_neuron"`）
1,314 顆、`vnc_motor`（`superclass == "vnc_motor"`）708 顆。

參數為明示的工程假設，不是生物參數：`gain` 0.025 使每邊權重的中位數約為 0.05（選入
視圖的原始 weight 中位數為 2，p25 1、p75 4、p90 10、最小 1、最大 2,591，以
nearest-rank 計於 25,563,197 條邊），全部邊為興奮性，全部神經元共用 bias 0、
log_tau 0、theta_raw 0（theta_base 1.05），延遲一律為零。刺激結束後活動並未衰退，
而是進入自我維持的兩步交替放電；這是這組全興奮均一參數的性質，不是果蠅生理的結論。
歸因需要 ticket 14 的空模型，參數推導屬 ticket 13。

與票面草稿的差異（皆為刻意）：CLI 只有
`--store --protocol [--state-in] [--state-out] [--max-memory-bytes] [--max-store-bytes]
[--max-footer-bytes]`，沒有 `--params` 與 `--steps`；參數來源與步數都在 protocol 明示，
避免同一件事有兩個來源。`Runner.State()` 回傳具型別的 `StateSnapshot`（`{schema_version,
core, lif|continuous}`）而非 `any`，接續用 `RestoreState`，都經核心的 `ValidateState` 與
設定指紋檢查。`Selector` 多一個 `AllowEmpty`：零命中預設是錯誤，要接受空集合必須明寫。
`UniformPositive` 多收一個 `context`，因為全圖要串流 2,556 萬筆邊，必須可取消。
`RunReport` 多 `steps_before`／`steps_after` 兩個欄位，讓接續執行可稽核。連續核心沒有事件，
`population_rate_per_step` 為空、`rate_quantiles` 為零，`silent_fraction` 改以「輸出整段
未離開執行前的值」計算，並在 `assumptions` 寫明，不憑空造一個放電率。

此 protocol 在開發過程中以兩個不同版本的 CLI、兩個獨立行程各跑過一次，報告中每一個
數值都相同（四個 hash、沉默比例、分位數、整條全群放電時序、狀態快照的 SHA-256），
只有牆鐘時間與 RSS 不同（前一次 106.16 s、4,545,953,792 B）。證據目錄保留後一次，
其 `binary.sha256` 對應現在的原始碼。

執行環境負載：開始前 1 分鐘平均 2.44、結束後 3.96（`load-average.txt`），遠低於需要
特別標註的 ~10；同時有一個與本票無關的大型下載在寫入 `data/malecns-v1.0/`。

軟體驗證：`go test -count=1 ./...`、
`go test -race -count=1 ./simulate/ ./internal/strictjson/ ./internal/cli/ ./connectome/`、
`go vet ./...`、`gofmt -l .` 全數通過，紀錄在 `evidence/NAT-01/`。

root 審查（2026-09-15）：逐檔讀過 `simulate`、`internal/strictjson`、CLI 與 connectome 的
改動，獨立重跑 gofmt／vet／`go test ./...`／race 全數通過；修正 ENG 對 strictjson 上限的
描述（上限由呼叫端給定，非 16 MiB）。待改善：`configHash` 對全圖 `LIFConfig`（含 2,556 萬筆
sources／targets）先 JSON 序列化再雜湊，一次性暫存數百 MB，可接受但宜改為串流雜湊。
之後由 root 補齊 `docs/model-and-mechanisms.md`、`delivery-status.md` 與
`docs/requirements-status.json`（NAT-01 passed）。未完成：空模型對照（NAT-03）與具名
神經元判讀協定（NAT-04）依 ticket 14 處理。全圖只在 macOS arm64 跑過，位元相同的
重跑保證只在套件與 CLI 測試中直接驗證。

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
