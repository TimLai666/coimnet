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

- [ ] fixture：小圖上三種空模型的度數、pair 唯一性、符號與權重計數守恆與 seed 可重現；
  原圖與參數集不變（hash 相同）；指標與門檻的手算案例；latency 在無放電時為未定義而非 0。
- [ ] 同圖兩核心與至少一種空模型的比較矩陣可執行，報告差異與分位數。
- [ ] 真實資料：13 的參數集在原圖與三種空模型（各 ≥ 3 個 seed）上執行同一刺激協定，
  記錄命令、環境、指紋、耗時；報告只陳述指標，不做行為宣稱。
- [ ] 文件與 `evidence/NAT-03/`、`NAT-04/`、`NAT-05/`。

## 依據

- [研究方向](../research-directions/connectome-native.md)、主規格 3.2（科學界線）、14.4
  （對照組）、18.3（核心確實學習的證明精神移用到歸因）、20.2（負面結果保存）。
