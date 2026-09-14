# 14 — 研究者可以用空模型與判讀協定歸因接線的貢獻

Epic：原生模擬

User Story：研究者可以對同一份刺激，在原始接線、打亂接線、不同動態模型之間比較指定神經元
集合的活動指標，並以事前寫定的門檻判讀，不宣稱生物行為證明。

Blocked by：12 runner、13 參數集

Status：verified_scoped（兩階段皆已驗證：fixture 的空模型、集合、指標、門檻與比較矩陣，以及
全腦 LIF 與連續核心各十格的真實資料對照，見「第二階段證據」。範圍限制：只在 macOS arm64 跑過，
每種空模型三個 seed，`unknown_sign` 只有 `exclude`、`weight_scale` 只有 21.6、`swap_factor` 只有 1，
兩個核心沒有字面共用的指標種類，`population_sync` 未實作。`docs/requirements-status.json` 的
NAT-03／NAT-04／NAT-05 由 root 依本票證據更新）

對應需求：NAT-03（空模型）、NAT-04（判讀協定）、NAT-05（同圖多模型）。

## Root 決策（2026-09-14）

1. **空模型（原圖不變，產生新衍生物）**：
   - `degree_preserving_rewire`：固定每個節點的出度與入度，以固定 seed 的邊交換
     （double-edge swap）進行 `k × E` 次嘗試，拒絕自環與重複 pair（與原圖一致），報告
     實際成功交換數；權重與正負號跟著邊走。
   - `sign_shuffle`：邊集合不變，`+1`／`-1`／`unknown` 標籤在邊間隨機置換（保留各類數量）。
   - `weight_shuffle`：邊集合與符號不變，強度在邊間隨機置換。
   每種輸出新的 GraphStore／參數集，footer 記錄 `null_model{kind, seed, attempts, applied}`
   與來源 hash；讀回時可辨識為衍生物，不可與原圖混淆。
2. **具名神經元集合**：以 `Selector{Field, Equals}` 或多條件交集自註記選取，保存來源欄位、
   值、解析出的節點數與 ID 指紋；集合名稱由使用者給（例如 `descending_candidates`），
   框架不預設任何集合等於某種行為。
3. **指標與門檻**：`Metric ∈ {spike_fraction, mean_rate, latency_to_first_spike,
   activity_ratio_vs_baseline, population_sync}`，每個指標定義寫在協定檔並進 hash；門檻
   （例如「集合 A 的 spike_fraction 在刺激後 20 步內 ≥ 0.1」）事前寫定，報告 pass／fail
   與數值，不因結果調門檻。判讀結果只能表述為「在此規則下集合 A 的活動指標達門檻」。
4. **比較矩陣**：`simulate compare --protocol FILE` 對 `{original, null models…} ×
   {continuous, lif} × {parameter sets…}` 執行相同刺激，輸出每格指標、與原圖的差、seed、
   耗時；矩陣中每格都是獨立可重跑的 RunReport。
5. **統計**：多 seed 的空模型給指標分布（分位數），原圖指標相對分布的位置以分位數報告，
   不做未事前宣告的檢定；報告寫明 seed 數。

## 契約（2026-09-15 root 定案，分兩階段派工）

全部放在 `simulate`（第四、五層的判讀與對照），不改 `connectome` 與 `params`。與 root 決策 1 的
差異：空模型不另存 GraphStore／參數集檔，改為記憶體內衍生物，靠 `seed`＋決定性演算法＋衍生
拓撲／陣列的 SHA-256 重現；理由是 GraphStore 屬第一層不可變結構，落盤衍生圖會把假的「來源
指紋」寫進第一層格式。報告仍記錄 `null_model{kind, seed, attempts, applied}` 與衍生 hash，
讀者可由原 store＋seed 重算並比對 hash。`population_sync` 本票不做（定義尚無共識），指標留四種。

```go
package simulate

// 空模型（原圖與參數集不變，產生新的 sources/targets 或參數陣列）
type NullModelSpec struct { Kind string; Seed uint64; SwapFactor float64 }  // kind ∈ degree_preserving_rewire | sign_shuffle | weight_shuffle；SwapFactor 只對 rewire 有效，嘗試次數 = ceil(SwapFactor × E)
type NullModelReport struct { Kind, Seed, Attempts, Applied, Rejected{SelfLoop, Duplicate} ; TopologyHash string; ParameterHash string }
// rewire：double-edge swap，兩條邊 (a→b),(c→d) 交換目標成 (a→d),(c→b)，拒絕自環與既有 pair；
//   每條邊保留自己的 weight／sign（「參數跟著邊走」）；pair 集合用開放定址 uint64 雜湊表，記憶體計入 Limits。
// sign_shuffle：只允許 derived 來源，對 EdgeSign 做 seed 決定的 Fisher–Yates 置換，再套 FromDerived 的政策。
// weight_shuffle：對強度陣列（uniform：gain×raw；derived：推導強度）做置換，sign 不動。
// PRNG：math/rand/v2 的 PCG(seed, 0)，演算法與種子寫進報告，跨平台相同。
func DeriveNullModel(ctx, g *connectome.Graph, set *params.Set /*可為 nil*/, protocol Protocol, spec NullModelSpec, limits Limits) (Variant, NullModelReport, error)
type Variant struct { Name string; Sources, Targets []int; Params ParameterSet; Null *NullModelReport }  // 原圖也是一個 Variant（Null == nil）
func BuildVariant(ctx, g *connectome.Graph, v Variant, protocol Protocol, limits Limits) (*Runner, error)   // 與 Build 相同，但拓撲來自 v；RunReport 加 topology_hash 與 null_model

// 具名集合、指標、門檻
type NamedSet struct { Name string; Selectors []Selector }                    // 交集；解析結果含 count 與排序後節點索引的 SHA-256
type Metric struct { Name, Kind, Set string; Window [2]int; Baseline [2]int }   // kind ∈ spike_fraction | mean_rate | latency_to_first_spike | activity_ratio_vs_baseline
// spike_fraction：Window 內至少放電一次的節點數 / 集合節點數
// mean_rate：Window 內集合的平均每步放電比例
// latency_to_first_spike：集合內任一節點在 Window 內第一次放電的步數減 Window[0]；沒有放電為未定義（defined=false），不是 0
// activity_ratio_vs_baseline：Window 的 mean_rate / Baseline 的 mean_rate；分母為 0 為未定義
// 連續核心：只允許 mean_rate 改以 mean_output 定義的 `mean_output`，其餘指標拒絕
type MetricResult struct { Name, Kind, Set string; Value float64; Defined bool; Numerator, Denominator float64; Basis string }
type Threshold struct { Metric, Op string; Value float64 }                     // op ∈ >= | > | <= | <；未定義指標一律 fail 並註明
type ThresholdResult struct { Metric, Op string; Value, Observed float64; Passed bool; Reason string }

type CompareProtocol struct { SchemaVersion "coimnet-simulate-compare/v1"; Run Protocol; Sets []NamedSet; Metrics []Metric; Thresholds []Threshold; NullModels []NullModelSpec /*kind 與 swap_factor*/; Seeds []uint64 }
type CompareCell struct { Variant string; Kind string; Seed uint64; Null *NullModelReport; Run RunReport; Metrics []MetricResult; Thresholds []ThresholdResult; WallSeconds float64 }
type CompareReport struct { SchemaVersion "coimnet-simulate-compare-report/v1"; ProtocolHash; GraphHashes; ParameterSource; ParameterSetSHA256; Cells []CompareCell; Summary []MetricSummary }
type MetricSummary struct { Metric, Kind string; Original MetricResult; PerKind map[string]{Seeds int; Defined int; Quantiles [5]float64; OriginalPercentile float64 /*原圖值在該分布的百分位，依 (小於的數+0.5×相等的數)/n*/ } }
func Compare(ctx, g, set, cp CompareProtocol, limits Limits) (CompareReport, error)   // 原圖一格 + 每種 kind × 每個 seed 一格；同一格內容可用 simulate run 重跑（報告寫出重跑方式）
```

指標由 runner 的「集合追蹤」提供：`Build`／`BuildVariant` 接受 `TrackSets []resolved set`，`Run` 逐步
記錄每個集合節點的放電計數與第一次放電步數（只保留 Window 需要的範圍），不改變探針語意。

CLI：`simulate compare --store FILE --protocol FILE [--params FILE] [--out-dir DIR]`，印出
`CompareReport` JSON；`--out-dir` 另存每格的 RunReport（不覆寫）。門檻結果不改退出碼。

第一階段（fixture）：空模型三種（度數、pair 唯一性、符號與權重計數守恆、seed 可重現、拒絕計數）、
集合解析、四種指標手算、門檻含未定義、`Compare` 在小圖上跑原圖＋三種 kind × 2 seeds、決定性、
CLI 與失敗案例。第二階段（真實資料）：以 `params-derive-v1.coimparams` 與 NAT-02 的 protocol，
LIF 核心跑原圖＋三種空模型 × 3 seeds（NAT-03），集合 `descending_neuron`／`vnc_motor`／ALIN，
指標與門檻事前寫在 protocol（NAT-04）；同一 compare protocol 換連續核心再跑一次（NAT-05）；
證據 `evidence/NAT-03/`、`NAT-04/`、`NAT-05/`，報告只陳述指標與百分位，不做行為宣稱。

## 驗收

- [x] fixture：小圖上三種空模型的度數、pair 唯一性、符號與權重計數守恆與 seed 可重現；
  原圖與參數集不變（hash 相同）；指標與門檻的手算案例；latency 在無放電時為未定義而非 0。
- [x] 同圖兩核心與至少一種空模型的比較矩陣可執行，報告差異與分位數。
- [x] 真實資料：13 的參數集在原圖與三種空模型（各 ≥ 3 個 seed）上執行同一刺激協定，
  記錄命令、環境、指紋、耗時；報告只陳述指標，不做行為宣稱。
- [x] 文件與 `evidence/NAT-03/`、`NAT-04/`、`NAT-05/`。

## 第一階段證據（2026-09-15，fixture）

### 交付

`simulate` 新增三個檔案。`nullmodel.go`：`NullModelSpec`、`NullModelReport`、`Variant`、
`OriginalVariant`、`DeriveNullModel`、`topologyHash` 與 rewire 用的開放定址 pair 集合。
`metrics.go`：`NamedSet`／`ResolvedSet`／`ResolveSets`、四種指標加 `mean_output` 的定義、
`Threshold` 與 `EvaluateMetrics`／`EvaluateThresholds`、最近秩分位數。`compare.go`：
`CompareProtocol`（`coimnet-simulate-compare/v1`）、`Compare` 與
`CompareReport`（`coimnet-simulate-compare-report/v1`）。既有檔案：`runner.go` 把 `Build`
改寫成「串流原圖拓撲後呼叫 `BuildVariant`」並加上 `TrackSets`／`Measurements` 與逐步集合追蹤，
`report.go` 的 `RunReport` 加 `topology_hash`（一律輸出）與 `null_model`（omitempty）。
CLI：`internal/cli/simulate.go` 加 `simulate compare` 與共用的 `loadParameterSet`，
`internal/cli/run.go` 加總覽一行。文件：README 在 derive 一節之後新增一節，ENG 新增一條。

### 先寫失敗測試

`evidence/NAT-03/red-simulate.log`（`undefined: CompareProtocol`、`undefined: NamedSet`、
`undefined: Metric`，build failed）與 `evidence/NAT-03/red-cli.log`
（`undefined: simulate.CompareProtocol`、`undefined: simulate.MetricSpikeFraction`，build failed）。
實作後兩個套件全綠。

### 指標與門檻的手算案例

用 ticket 12 的三節點 fixture（0→1→2，原始 weight 2 與 3，gain 2，theta_base 1，v_reset −0.5，
一次兩單位脈衝打進 node 0）。逐步推導見 `TestLIFProbeSeriesMatchTheHandCalculatedFixture`，
放電是 node 0 在步 0、node 1 在步 1、node 2 在步 2 與步 3。註記讓集合解析成
`alpn` = class ALPN = {1,2}、`left_alpn` = class ALPN ∩ soma_side L = {2}、`alin` = class ALIN = {0}。

| 指標 | 集合 | 窗（baseline） | 分子／分母 | 值 |
| --- | --- | --- | --- | --- |
| spike_fraction | alpn | [0,4) | 2／2 | 1 |
| mean_rate | alpn | [0,4) | 3／8 | 0.375 |
| latency_to_first_spike | alpn | [0,4) | 1 減 0 | 1 |
| latency_to_first_spike | alpn | [2,4) | 2 減 2 | 0 |
| spike_fraction | left_alpn | [2,4) | 1／1 | 1 |
| latency_to_first_spike | left_alpn | [0,2) | 沒有放電 | 未定義 |
| activity_ratio_vs_baseline | left_alpn | [2,4)（[0,4)） | 1／0.5 | 2 |
| activity_ratio_vs_baseline | left_alpn | [2,4)（[0,2)） | 1／0 | 未定義 |
| mean_rate | alin | [1,4) | 0／3 | 0 |

四個運算子各測一次：`alpn_fraction >= 1` pass、`alpn_rate > 0.375` fail、`alin_rate <= 0` pass、
`alpn_rate < 0.4` pass。未定義的 `left_latency_quiet >= 0` fail 並記 `reason: "undefined"`，
`observed` 為 0，`defined=false` 時 latency 的分子分母也歸零，不會被讀成「延遲 0 步」。
測試在 `simulate/metrics_test.go`。

### 空模型的手算案例

- 三節點 fixture 只有 0→1 與 1→2，唯一可能的交換會做出自環 1→1，所以 `swap_factor 3` 的
  六次嘗試全部記在 `rejected.self_loop`，`applied` 為 0，拓撲一個索引都沒動。
- 四節點六邊的推導 fixture（0→1、0→2、0→3、1→2、2→3、3→0）只有 {1→2, 3→0} 一組能換成
  1→0 與 3→2，其餘配對不是重複 pair 就是自環。`swap_factor 2` 的 12 次嘗試在 seed 1 記
  5 self_loop 與 7 duplicate、seed 2 記 6 與 6，`applied + self_loop + duplicate == attempts`。
- 四十節點三百邊的產生圖（每個節點出度 7，前二十個節點各多一條到 i+8）在 `swap_factor 2` 下
  有成功交換。測試逐一比對出入度向量、pair 唯一性與無自環，同一 seed 兩次結果完全相同，
  換一個 seed 拓撲 hash 就不同，權重多重集與參數 hash 都沒變（權重跟著邊走）。
- `sign_shuffle` 後 +1／−1／unknown 仍是 1／2／3，參數 hash 改變。`weight_shuffle` 後強度多重集
  不變、每條邊的正負號不動。兩者跑完後，重新串流原圖算出的拓撲 hash 與參數集陣列指紋都和
  跑之前相同，`sign_shuffle` 對 uniform 來源直接拒絕。
- `pairSet` 的加入、刪除與查詢另外對照一份 map 跑二十萬步隨機操作，pin 住 backward shift 刪除。
  測試在 `simulate/nullmodel_test.go`。

### 比較矩陣

`simulate/compare_test.go` 在四節點推導 fixture 上跑 exclude 政策、`weight_scale` 4、六步、
三種 kind × seed 1 與 2，共七格。原圖的 `alin_fraction` 是 1／2、`alin_rate` 是 1／12、
`alpn_rate` 是 2／12（node 0 在步 0 放電一次，node 1 在步 1 與步 2 各一次，node 2 與 node 3 全程沉默）。
兩個 `sign_shuffle` seed 的 `alpn_rate` 都變成 0，兩個 `weight_shuffle` seed 都是 1／12，
兩個 rewire seed 維持 2／12。n = 2 時最近秩的 `rank = ceil(q×2)` 讓 p0、p25、p50 取小的值、
p75 與 p100 取大的值，所以三種 kind 的五個分位數各自都等於該 kind 的那一個值；原圖百分位
`(小於的個數 + 0.5 × 相等的個數) ÷ 2` 為 sign_shuffle 1、weight_shuffle 1、rewire 0.5。
兩次 `Compare` 除了每格的 `wall_seconds` 之外 JSON 位元組相同。同一張圖、同一份參數集、
同一份刺激另跑一次連續核心（指標改用 `mean_output`，空模型用 `weight_shuffle` × 2 seeds），
七格與三格兩份報告的圖 hash、每格拓撲 hash 與參數 hash 逐一相同。

CLI 端另建一個帶 class 註記的四節點 store，測 `--out-dir` 每格一個 `cell-N-變體.json`、
uniform 與 derived 兩條路徑、遮掉 `wall_seconds` 後兩次輸出相同，以及缺旗標、壞 protocol、
未知 kind、重複 seed、門檻指向不存在的指標、`--out-dir` 已存在、uniform 配 `sign_shuffle`、
derived 缺 `--params`、uniform 多給 `--params` 等十五種失敗與說明文字。

### 驗證

`gofmt -l .` 無輸出，`go vet ./...` 無輸出，`go test -count=1 ./...` 全數 ok，
`go test -race -count=1 ./simulate/ ./internal/cli/` 兩個套件 ok。全部在 fixture 與產生圖上執行，
沒有碰 `data/malecns-v1.0/`。

### 已知限制與待決

- `degree_preserving_rewire` 要求原圖的 pair 唯一，否則直接報錯，因為 uint64 集合表示不了重數。
  MaleCNS v1.0 的 import 報告 `duplicate_pairs` 為 0，所以第二階段目前不受影響；若之後換成會
  產生重複 pair 的選取條件，這裡要先改成可計數的表。
- 契約的 `CompareCell` 與 `MetricSummary` 沒有「與原圖的差」欄位（root 決策 4 的原句有），
  目前只能從 `summary.original` 與分位數自行相減。要不要補一個明示欄位請 root 決定。
- `DeriveNullModel` 與 `OriginalVariant` 比契約多收參數集檔的 SHA-256，理由與 `FromDerived`
  相同：hash 要指名檔案而 `params.Set` 不帶它。`MetricSummary.PerKind` 用切片而非 map，
  因為 Go 的 map JSON 會改成鍵排序，丟掉宣告順序。`mean_output` 在 LIF 核心也開放（讀突觸跡），
  契約只限制連續核心不能用另外四種。

### root 審查（2026-09-15，第一階段）

逐檔讀過 `nullmodel.go`、`metrics.go`、`compare.go`、runner 與 CLI 的 diff；指標手算表由 root 逐格
重算一致；重連演算法只動 targets，出入度與每條邊自帶的參數都保持；pair 集合的反向搬移刪除有
map 對照測試。root 重跑 gofmt／vet／`go test ./...`／race 全數通過。接受五項契約偏離。裁決：
「與原圖的差」補為每格每個指標的 `delta_from_original{value, defined}`（兩邊都有定義才有值），
第二階段一併做；驗收第二項待第二階段真實資料的兩核心矩陣跑過再勾。

## 第二階段證據（2026-09-15，真實全腦）

### 交付

`simulate/compare.go` 加 `MetricDelta` 與 `CompareCell.Deltas`（JSON `deltas_from_original`），
依指標順序記每一格與原圖的差，兩邊都有定義才有值，原圖那一格在指標有定義時是 0。
其餘報告欄位不動。新增 `scripts/compare-evidence.sh`（比照 `simulate-evidence.sh`：建置、
`doctor`、二進位／store／protocol／參數集的 SHA-256、前後 `uptime`、`/usr/bin/time -l`、
起訖時間、`command.txt`，每格另存 run report）。兩份事前寫定的 compare protocol：
`evidence/NAT-03/compare-fullgraph-derived-lif.json` 與
`evidence/NAT-05/compare-fullgraph-derived-continuous.json`。三份驗證紀錄
`evidence/NAT-03/verification.json`、`NAT-04/verification.json`、`NAT-05/verification.json`。
文件：README 比較一節、ENG 空模型一條、[機制說明](../model-and-mechanisms.md)新增一列。

先寫失敗測試：`evidence/NAT-03/red-stage2.log`（`CompareCell has no field or method Deltas`，
build failed）。實作後 `simulate` 與 `internal/cli` 全綠，CLI 測試不必改。

### 執行

同一份 store（SHA-256 `a6c0ddff…`）、同一份 `params-derive-v1.coimparams`（`c4db0f3f…`）、
同一份刺激（一通道 300 步，步 10–29 打 2 個單位進 24 個 ALIN），兩個核心各跑一次十格矩陣
（原圖 + 三種空模型 × seed 1、2、3），每格 165,122 個神經元、25,563,197 條邊、300 步。
集合由註記解析：`alin`（class ALIN）24 個、`descending_neuron`（superclass）1,314 個、
`vnc_motor`（superclass）708 個，節點 hash 兩個矩陣相同。LIF 的第 0 格與 NAT-02 的單次全腦
執行完全相同（`protocol_hash 55fb6132…`，probe 與 monitor 序列逐位元相同）。

兩個矩陣是兩個獨立行程，十格的變體名稱、seed、`topology_hash`、`parameter_hash` 與整個
`null_model` 區塊逐格相同，所以「原 store 加 seed 可重算出同一份衍生物」有了實測。

### 空模型計數

| kind | seed | attempts | applied | 拒絕 self_loop | 拒絕 duplicate |
| --- | --- | --- | --- | --- | --- |
| `degree_preserving_rewire`（swap_factor 1） | 1 | 25,563,197 | 25,235,685 | 724 | 326,788 |
| `degree_preserving_rewire` | 2 | 25,563,197 | 25,234,331 | 715 | 328,151 |
| `degree_preserving_rewire` | 3 | 25,563,197 | 25,233,927 | 768 | 328,502 |
| `sign_shuffle` | 1／2／3 | 25,563,196 | 25,563,196 | 0 | 0 |
| `weight_shuffle` | 1／2／3 | 25,563,196 | 25,563,196 | 0 | 0 |

三個 rewire seed 的 `applied + self_loop + duplicate` 都正好等於嘗試次數，參數 hash 維持原圖的
`2a93e2b0…`（權重與符號跟著邊走），拓撲 hash 三個各不相同。兩種 shuffle 的抽樣次數是
Fisher–Yates 的 n−1，拓撲 hash 維持原圖的 `5d205bf8…`。

### 指標與百分位

值四捨五入到四位有效數字，完整值見各 `compare.json`。三個 seed 有定義時，最近秩讓
p0 = p25、p75 = p100，所以 p0／p50／p100 就是完整的分位數集合。百分位是
`(小於的個數 + 0.5 × 相等的個數) ÷ 有定義的個數`，三個 seed 只可能是 0、1/6、1/3、1/2、2/3、5/6、1。
**這是位置，不是檢定**，本票沒有宣告任何檢定，也沒有做。

LIF（`evidence/NAT-03/compare-lif-v1/compare.json`）：

| 指標 | 原圖 | rewire p0／p50／p100（百分位） | sign_shuffle（百分位） | weight_shuffle（百分位） |
| --- | --- | --- | --- | --- |
| `alin_spike_fraction` | 1 | 1／1／1（0.5） | 1／1／1（0.5） | 1／1／1（0.5） |
| `alin_mean_rate` | 0.2465 | 0.1511／0.1903／0.2156（1） | 0.4586／0.4784／0.4892（0） | 0.3699／0.4106／0.4488（0） |
| `alin_latency` | 0 | 0／0／0（0.5） | 0／0／0（0.5） | 0／0／0（0.5） |
| `alin_activity_ratio` | 0.7302 | 0.3681／0.4268／0.4747（1） | 0.9656／0.9992／1.053（0） | 0.8578／0.9254／0.9531（0） |
| `descending_neuron_spike_fraction` | 0.5639 | 0.8097／0.8105／0.8204（0） | 0.9399／0.9482／0.9536（0） | 0.6469／0.6537／0.6606（0） |
| `descending_neuron_mean_rate` | 0.0879 | 0.2097／0.21／0.2125（0） | 0.4436／0.4466／0.4479（0） | 0.2426／0.2474／0.2521（0） |
| `descending_neuron_latency` | 13 | 3／3／5（1） | 2／2／2（1） | 2／2／3（1） |
| `descending_neuron_activity_ratio` | 56.34 | 28.5／29.99／36.26（1） | 5.144／5.217／7.069（1） | 2.409／2.482／2.655（1） |
| `vnc_motor_spike_fraction` | 0.5989 | 0.7514／0.774／0.7853（0） | 0.9195／0.9223／0.9266（0） | 0.589／0.637／0.6427（0.3333） |
| `vnc_motor_mean_rate` | 0.07604 | 0.1746／0.1913／0.1928（0） | 0.4221／0.4303／0.4309（0） | 0.1695／0.1779／0.1896（0） |
| `vnc_motor_latency` | 21 | 5／5／5（1） | 5／6／7（1） | 6／6／6（1） |
| `vnc_motor_activity_ratio` | 未定義 | 29.45／33.3／44.16（未定義） | 7.792／10.95／16.02（未定義） | 2.014／2.16／2.232（未定義） |

連續核心（`evidence/NAT-05/compare-continuous-v1/compare.json`）：

| 指標 | 原圖 | rewire p0／p50／p100（百分位） | sign_shuffle（百分位） | weight_shuffle（百分位） |
| --- | --- | --- | --- | --- |
| `alin_mean_output_from_onset` | 0.08336 | -0.003683／0.02009／0.05195（1） | 0.7082／0.7254／0.88（0） | -0.03694／0.232／0.2879（0.3333） |
| `alin_mean_output_after` | 0.04322 | -0.03862／-0.0183／0.02792（1） | 0.691／0.707／0.8734（0） | -0.06623／0.2214／0.2733（0.3333） |
| `descending_neuron_mean_output_from_onset` | 0.07222 | -0.007528／-0.0006945／0.001589（1） | 0.6472／0.6718／0.6839（0） | -0.03931／0.02034／0.06988（1） |
| `descending_neuron_mean_output_after` | 0.07501 | -0.008943／-0.001327／0.002248（1） | 0.6674／0.6953／0.7059（0） | -0.04413／0.02264／0.06944（1） |
| `vnc_motor_mean_output_from_onset` | -0.008258 | -0.006127／-0.004247／0.005597（0） | 0.5539／0.5767／0.6227（0） | -0.0003445／0.001195／0.01239（0） |
| `vnc_motor_mean_output_after` | -0.003608 | -0.005168／-0.003227／0.006304（0.3333） | 0.5726／0.5946／0.6437（0） | -0.00185／0.001345／0.01225（0） |

`vnc_motor_activity_ratio` 在原圖未定義：該集合在 baseline 窗 [10,30) 一次都沒放電，分母為 0，
報告寫未定義而不是 0，也因此不給百分位；九個空模型格反而都有值。每一格的
`deltas_from_original` 也依同一條規則：`vnc_motor_activity_ratio` 十格全部未定義，
`descending_neuron_mean_rate` 的差是 rewire +0.1246／+0.1218／+0.1221、
sign_shuffle +0.3587／+0.3557／+0.3600、weight_shuffle +0.1547／+0.1642／+0.1595。

**同一份接線，換一個核心，百分位方向可以相反。** 可比的是同集合同窗的速率類讀出
（LIF 的 `mean_rate` 對連續核心的 `mean_output`）：對 `sign_shuffle`，三個集合在兩個核心都是
百分位 0；對 `degree_preserving_rewire` 與 `weight_shuffle`，`descending_neuron` 在 LIF 是 0
（比每個 seed 都低）、在連續核心是 1（比每個 seed 都高）。同一個 LIF 矩陣內部，換一個指標
定義也一樣：`descending_neuron` 與 `vnc_motor` 的 `spike_fraction` 與 `mean_rate` 在三種空模型
下幾乎都是百分位 0（只有 `vnc_motor_spike_fraction` 對 `weight_shuffle` 是 1/3），但同樣兩個
集合的 `latency` 三種都是 1，`descending_neuron_activity_ratio` 三種也都是 1。歸因結論必須連同
核心、參數來源、空模型種類與指標定義一起陳述。

### 門檻

五條 LIF 門檻與六條連續核心門檻都在跑之前寫進 protocol，protocol 的 SHA-256 也在跑之前記下
（`de8319fc…`、`5b64b248…`），事後沒有調整。門檻結果不改退出碼，兩個矩陣都是 exit 0。

| LIF 門檻（事前寫定） | 原圖觀察值 | 原圖 | 九個空模型格通過 |
| --- | --- | --- | --- |
| `alin_spike_fraction >= 0.5` | 1 | 通過 | 9／9 |
| `descending_neuron_spike_fraction >= 0.1` | 0.5639 | 通過 | 9／9 |
| `vnc_motor_spike_fraction >= 0.1` | 0.5989 | 通過 | 9／9 |
| `descending_neuron_latency < 100` | 13 | 通過 | 9／9 |
| `vnc_motor_latency < 150` | 21 | 通過 | 9／9 |

| 連續核心門檻（事前寫定） | 原圖觀察值 | 原圖 | 九個空模型格通過 |
| --- | --- | --- | --- |
| `alin_mean_output_from_onset >= 0.01` | 0.08336 | 通過 | 7／9 |
| `alin_mean_output_after >= 0.01` | 0.04322 | 通過 | 6／9 |
| `descending_neuron_mean_output_from_onset >= 0.001` | 0.07222 | 通過 | 6／9 |
| `descending_neuron_mean_output_after >= 0.001` | 0.07501 | 通過 | 6／9 |
| `vnc_motor_mean_output_from_onset >= 0.001` | -0.008258 | 未通過 | 6／9 |
| `vnc_motor_mean_output_after >= 0.001` | -0.003608 | 未通過 | 6／9 |

LIF 的五條門檻在十格全部通過，所以它們分不出真實接線與打亂的接線；通過只能表述為
「在此規則下該集合的指標達到宣告值」，不能當成關於接線或行為的證據。連續核心的六條有真的
失敗：原圖的兩條 `vnc_motor` 門檻未通過，因為該集合的平均輸出是負的（tanh 可以為負，
放電比例不行），而九個空模型格的通過數也各不相同。

### 資源

| 矩陣 | 牆鐘 | 十格合計 | 最大 RSS | 峰值記憶體 | 起訖（UTC） |
| --- | --- | --- | --- | --- | --- |
| LIF | 890.94 s（user 820.68、sys 65.05） | 852.24 s | 5,909,626,880 B | 9,543,671,152 B | 21:48:19 → 22:03:10 |
| 連續核心 | 929.97 s（user 815.61、sys 98.44） | 890.17 s | 5,659,115,520 B | 10,513,997,816 B | 22:03:25 → 22:18:55 |

每格 80.4–91.7 s。兩個矩陣先後執行，沒有同時跑。預設 8 GiB 的帳面上限都沒碰到，沒有降過
seed、步數、指標或邊。機器是 macOS arm64、8 核、17,179,869,184 B 記憶體，一分鐘負載
LIF 前 3.03 後 2.65、連續核心前 2.82 後 2.48。

### 與契約及派工的偏離

1. 差的欄位取 root 裁決的語意，但放成 `CompareCell.Deltas`（JSON `deltas_from_original`）的
   切片，每一筆自帶 `metric` 名稱，讀者不必靠索引對齊兩份清單。裁決原句寫的是每個指標一個
   `delta_from_original{value, defined}`，語意相同。
2. `NAT-05` 的空模型宣告順序沿用 NAT-03 的（rewire、sign_shuffle、weight_shuffle），派工單
   列舉時的順序是 rewire、weight_shuffle、sign_shuffle。這樣兩個矩陣的第 i 格才是同一個變體，
   本票的兩核心逐格比對才成立。
3. 連續核心的 protocol 只宣告 `min_active_fraction`，刻意不寫 `max_population_rate`：該核心
   不產生事件，那個界限永遠不會觸發，寫了就是一個被默默忽略的旋鈕。
4. LIF compare protocol 的 `run` 區塊是 `evidence/NAT-02/protocol-fullgraph-derived.json`
   逐位元貼進去的（只差檔尾換行），所以那一段沿用原檔自己的兩格縮排，在巢狀物件裡看起來少
   縮一層。這是為了「與 NAT-02 同一份 protocol」可以逐位元檢查；`protocol_hash` 也確認相同。
5. LIF protocol 沒有宣告 `mean_output`（派工單指定四種事件指標），連續核心只能用
   `mean_output`，所以兩個矩陣沒有任何一種指標種類是字面共用的。共用的是圖、參數集、刺激、
   三個集合與 [10,300)、[30,300) 兩個窗，跨核心比較的是百分位的方向而不是同一個量。
6. `population_sync` 仍未實作，與第一階段相同。

### 已知限制

- 每種空模型只有三個 seed。分位數是某個 seed 真的產生過的值，百分位只是位置。
- 只有一份 protocol、一種 `unknown_sign` 政策、一個 `weight_scale`、一個 `swap_factor`、
  一台 macOS arm64 機器，沒有 Ubuntu 或 Windows 的真實資料執行。
- 不做行為宣稱。`alin`、`descending_neuron`、`vnc_motor` 是註記值，不是已證實的行為單元。
- `degree_preserving_rewire` 保住每個節點的出入度，但不保住自環數：它拒絕造出自環，卻可以
  換掉原本就有的自環。這份 store 的匯入報告記 101 個自環與 0 個重複 pair。
- 兩個矩陣各跑一次。逐位元重現由 fixture 上的套件與 CLI 測試直接驗證；全腦這邊的重現證據是
  第 0 格對上 NAT-02 的單次執行，以及兩個行程之間逐格相同的衍生物 hash 與計數。
- 牆鐘與最大 RSS 是共用機器上的單次取樣；帳面上限只涵蓋本套件配置的陣列，實測 RSS 還包含
  載入的 store、參數集陣列、Go 配置器餘裕與執行期額外開銷。

### root 審查（2026-09-15，第二階段）

逐檔讀過 delta 欄位的 diff 與測試、證據腳本、兩份事前寫定的 protocol、三份驗證紀錄與文件；
以兩份 `compare.json` 獨立核對：各 10 格、三個 rewire seed 的 `applied + self_loop + duplicate`
等於嘗試次數、每格 `deltas_from_original` 依規則重算一致、LIF 第 0 格的 protocol hash／monitors／
probes 與 NAT-02 相同、兩核心逐格的 `topology_hash`／`parameter_hash`／`null_model` 相同、
百分位與門檻通過數與本節表格一致。root 重跑 gofmt／vet／`go test ./...`／race 全數通過。
接受六項偏離。NAT-03／NAT-04／NAT-05 由 root 標 passed。

## 依據

- [研究方向](../research-directions/connectome-native.md)、主規格 3.2（科學界線）、14.4
  （對照組）、18.3（核心確實學習的證明精神移用到歸因）、20.2（負面結果保存）。
