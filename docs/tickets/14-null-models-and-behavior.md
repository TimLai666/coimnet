# 14 — 研究者可以用空模型與判讀協定歸因接線的貢獻

Epic：原生模擬

User Story：研究者可以對同一份刺激，在原始接線、打亂接線、不同動態模型之間比較指定神經元
集合的活動指標，並以事前寫定的門檻判讀，不宣稱生物行為證明。

Blocked by：12 runner、13 參數集

Status：ready（契約已於 2026-09-15 定案，分兩階段派工；驗收項目驗證後才勾選）

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
- [ ] 同圖兩核心與至少一種空模型的比較矩陣可執行，報告差異與分位數。
- [ ] 真實資料：13 的參數集在原圖與三種空模型（各 ≥ 3 個 seed）上執行同一刺激協定，
  記錄命令、環境、指紋、耗時；報告只陳述指標，不做行為宣稱。
- [ ] 文件與 `evidence/NAT-03/`、`NAT-04/`、`NAT-05/`。

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

## 依據

- [研究方向](../research-directions/connectome-native.md)、主規格 3.2（科學界線）、14.4
  （對照組）、18.3（核心確實學習的證明精神移用到歸因）、20.2（負面結果保存）。
