# 13 — 研究者可以由官方發布資料推導動態參數並取得假設報告

Epic：原生模擬

User Story：研究者可以把 MaleCNS 發布中的突觸統計、逐突觸傳導物質機率、突觸配對與 ROI
階層接進框架，依明示規則產生每條邊的正負號與強度、每個神經元的標籤，輸出可校驗的參數集
與推導報告，未知保持未知。

Blocked by：08 標準化接線圖、09 GraphStore、12 runner（消費者）

Status：ready（契約已於 2026-09-15 定案，分兩階段派工；驗收項目驗證後才勾選）

對應需求：NAT-02；主規格 5.1（只有所選機制需要時才取得細部資料）、5.3、5.5
（`EdgeRecord` 的傳導物質與作用符號證據）、7.4（有依據固定符號、未知明示假設或可訓練）。

## 納入的發布檔案（見 [盤點](../malecns-release-catalog.md)）

| 檔案 | 用途 | 大小 |
| --- | --- | --- |
| `body-stats-male-cns-v1.0-minconf-0.5.feather` | 每個 body 的 `pre`／`post`／`downstream`／`synweight`／`rank`，用於權重正規化與選取門檻 | 778,062,826 |
| `tbar-neurotransmitters-male-cns-v1.0.feather` | 逐突觸前位置七種傳導物質機率 | 2,651,680,218 |
| `syn-partners-male-cns-v1.0-minconf-0.5.feather` | 逐突觸的 pre／post body、信心度、`primary_post` ROI，用來把機率對到邊並做 ROI 分割 | 6,777,179,098 |
| `database/neuprint-inputs/Neuprint_Meta.csv` | ROI 階層與各 ROI 突觸數，用於區域標籤與感覺／運動集合 | 1,247,784 |

`syn-points`（13 GB）與骨架本票不納入；`-traced-only`／`-significant-only` 變體不使用。

## 已核對的來源事實（2026-09-14 下載後）

- 四份檔案已用 `data download` 取得並有回條（`evidence/malecns-source-20260914/*-download.json`，
  提供者 CRC32C 驗證）。
- `body-stats`：88,384,522 列、1,349 批，欄位 `body int64`、`pre int32`、`post int32`、
  `status_fine dictionary<int8,utf8,ordered>`、`superclass`／`class`／`type`／`instance utf8`、
  `downstream int64`、`synweight int64`、`rank int64`。列數與 weights 的 unique endpoint 數
  88,384,522 相同，是身份對應的交叉核對點；`status_fine` 與 annotations 的 `statusLabel`
  關係待核。
- `tbar-neurotransmitters`：45,656,140 列、697 批（等於 `Neuprint_Meta.csv` 的
  `totalPreCount`，交叉核對點）。17 欄皆 nullable：`point_id uint64`、`x`／`y`／`z int32`、
  `conf float32`、`sv int64`、`body int64`、`major dictionary<int8,utf8,ordered>`、
  `primary dictionary<int16,utf8,ordered>`、七個 `nt_*_prob float32`（acetylcholine、dopamine、
  gaba、glutamate、histamine、octopamine、serotonin）、`split dictionary<int8,utf8>`。
  `feather` 讀取器已擴充 float32 等 fixed-width 型別後完整讀取。
- `Neuprint_Meta.csv`：欄位含 `voxelSize float[]`、`primaryRois string[]`、`superLevelRois`、
  `totalPreCount`、`totalPostCount`、`postHighAccuracyThreshold`／`preHPThreshold`／
  `postHPThreshold`；本票只讀 `roiHierarchy`、`roiInfo` 與門檻欄位。
- `syn-partners`：311,833,243 列、4,759 批（等於 `Neuprint_Meta.csv` 的 `totalPostCount`，也等於
  weights 的 weight 總和 311,833,243，交叉核對點）。11 欄皆 nullable：`x_pre`／`y_pre`／`z_pre int32`、
  `body_pre int64`、`conf_pre float32`、`x_post`／`y_post`／`z_post int32`、`body_post int64`、
  `conf_post float32`、`primary_post dictionary<int16,utf8,ordered>`。`data inspect` 逐批讀完 13.35 秒、
  peak RSS 135 MB（負載平均 6.0，另有其他工作同時執行）。tbar 的 `(x,y,z)` 對應本檔的
  `(x_pre,y_pre,z_pre)`；配對率須在實作時以真實資料抽樣核對並寫進報告。

## Root 決策（2026-09-14）

1. **取得與驗證**：四份檔案加入 `data sources`（URL、大小、下載時記錄 ETag／CRC32C），以
   `data download` 取得並保存回條，Feather 以現有 `feather.Scan` 逐批讀取，CSV 以有界讀取。
   新增 manifest 區段 `parameter_sources` 記錄角色、路徑、指紋與欄位映射。
2. **逐邊傳導物質機率**：`tbar-neurotransmitters` 的 `(x,y,z)` 與 `syn-partners` 的突觸前
   座標為同一 8 nm voxel 座標系（盤點確認欄位，實作時須以 fixture 與真實資料抽樣核對配對
   率並寫進報告）。流程：兩檔各以 packed `(x,y,z)` 為鍵走 `internal/extsort`，合併得到
   每個突觸的 `(pre body, post body, probs[7], conf)`；再以 `(pre, post)` 為鍵排序聚合，
   得到每條邊的機率平均、突觸數與配對到的比例。未配對突觸與未配對邊分別計數。
3. **正負號規則（明示、可版本化）**：`SignRule{schema_version, mapping{acetylcholine:+1,
   gaba:-1, glutamate:-1, dopamine:unknown, octopamine:unknown, serotonin:unknown,
   histamine:-1}, min_probability, min_matched_fraction, unknown_policy}`。glutamate 在果蠅
   多為抑制（GluCl）屬工程假設，規則檔必須寫來源與「假設」字樣；調節性傳導物質預設
   unknown。邊的 sign 為 `+1`／`-1`／`unknown`，附 `sign_confidence`（主要傳導物質平均
   機率）與 `sign_basis`（規則版本）。`unknown_policy ∈ {keep_unknown, engineering_default}`，
   後者需明示預設符號，報告分開計數；可訓練符號模式留給可學習票。
4. **強度規則**：`weight = gain * raw_weight / normalizer`，`normalizer ∈ {none,
   post_total(body-stats.post of target), pre_total}`，`gain` 設定；記錄公式、分位數與被
   `post_total = 0` 擋下的邊。原始 `raw_weight` 永遠保留在 GraphStore，參數集只存推導值。
5. **神經元標籤**：由 annotations 與 `Neuprint_Meta.csv` 的 `roiHierarchy` 給每個節點
   `roi_level_1..n`（若可由 `syn-partners` 的 `primary_post` 聚合出主要 ROI）與細胞類型
   階層，供 12／14 的選擇器使用；本票不推導 tau／theta 的細胞類型差異（沒有來源），統一值
   由 protocol 指定並標為假設。
6. **參數集檔案**：`coimnet-parameter-set/v1`，格式沿用 GraphStore 的區段＋footer＋
   SHA-256：`edge_weight`、`edge_sign`（int8：+1／-1／0=unknown）、`edge_sign_confidence`、
   `node_labels`、`derivation_report`（JSON）。footer 含來源指紋（四份檔案＋GraphStore
   hashes）、規則版本與 hash。讀回逐段校驗，與 GraphStore 的 node index／edge order hash
   不符即拒絕。
7. **報告**：來源與指紋、規則、配對率（突觸與邊）、每種傳導物質的邊數、sign 分布、unknown
   比例（分子分母）、強度分位數、`post_total = 0` 邊數、耗時與峰值記憶體。所有比例附
   basis。真實資料實測：全部 25,563,197 條選入邊，走 bounded external sort，暫存上限明示。

## 契約（2026-09-15 root 定案，分兩階段派工）

新套件 `params`（第三層「動態參數來源」，與 `simulate` 平行；依賴 `connectome`、`feather`、
`internal/extsort`、`internal/strictjson`，不依賴 `learning`，不用 Insyra）。與 root 決策 1 的差異：
不改 manifest，改用獨立的規則檔 `coimnet-derivation-rules/v1` 記錄來源與規則，理由是 manifest 與
GraphStore 屬第一層不可變結構，推導輸入屬第三層，分開後既有 manifest hash 不變。

```go
package params

const RulesSchemaVersion = "coimnet-derivation-rules/v1"
type SourceRef struct { Path string; SHA256 string }             // 相對規則檔目錄；使用前比對 SHA-256
type Sources struct { BodyStats, Tbar, SynPartners, NeuprintMeta SourceRef }
type SignRule struct {
    SchemaVersion string                 // "coimnet-sign-rule/v1"
    Mapping map[string]string            // acetylcholine:"+1" gaba:"-1" glutamate:"-1" histamine:"-1" dopamine/octopamine/serotonin:"unknown"
    Basis map[string]string              // 每個鍵的依據字串；glutamate 必須含「假設」
    MinProbability float64               // 主要傳導物質平均機率門檻
    MinMatchedFraction float64           // matched_synapses / raw_weight 門檻
}
type StrengthRule struct { Gain float64; Normalizer string }     // none | post_total | pre_total
type Rules struct { SchemaVersion string; Sources Sources; Sign SignRule; Strength StrengthRule }
func DecodeRules(r io.Reader) (Rules, error)                      // strictjson；Mapping 七鍵齊全、值限 +1/-1/unknown

type Limits struct { MaxMemoryBytes, MaxTempBytes int64; MaxRunFiles int; MaxArrowBytes int64; MaxRows int64; TempDir string }
func Derive(ctx, g *connectome.Graph, rules Rules, rulesDir string, limits Limits) (*Set, Report, error)

const SetSchemaVersion = "coimnet-parameter-set/v1"
type Set struct {                       // 全部依 GraphStore 的 canonical edge／node 順序
    Source string                       // "derived_release/v1"
    RulesHash string
    GraphHashes simulate.GraphHashes 同形（node_index, edge_order）
    EdgeWeight []float64                // gain * raw_weight / normalizer；normalizer 為 0 者為 0 並計數
    EdgeSign []int8                     // +1 / -1 / 0 = unknown
    EdgeSignConfidence []float32        // 主要傳導物質平均機率；unknown 亦保留數值
    EdgeTransmitter []uint8             // 0..6 依固定表 acetylcholine,dopamine,gaba,glutamate,histamine,octopamine,serotonin；255 = 無配對
    EdgeMatchedSynapses []uint32
    NodePrimaryROI []string 或 dictionary（syn-partners primary_post 對 body_post 的眾數；無則空）
    NodePreTotal, NodePostTotal []int64 // body-stats pre／post
    Report Report
}
func Save(ctx, path string, set *Set) (SaveReceipt, error)        // 區段＋footer＋SHA-256，不覆寫，格式同 GraphStore 精神
func Load(ctx, path string, limits LoadLimits) (*Set, error)      // 逐段校驗；schema／hash 不符即拒絕
func (s *Set) CheckGraph(g *connectome.Graph) error               // node_index／edge_order hash 相符
```

推導流程（全部有界、可取消、暫存自清）：

1. 讀規則檔，逐一比對四份來源 SHA-256（有界讀取）。
2. 以 `g.NeuronIDs()` 建立 165,122 個 body 的集合；串流 `body-stats` 一次取選入 body 的
   `pre`／`post`（未出現於 body-stats 的選入 body 計數，`post_total` 視為 0）。
3. 串流 `tbar-neurotransmitters`：記錄 = 鍵 `(x,y,z)`（int32 各以 XOR 符號位轉 big-endian uint32，
   使位元組序等於數值序）12 B ＋ 7 個機率 float32 28 B ＋ 一個位元的「有 null 機率」旗標；任一機率
   null 者只計數不參與平均。走 `extsort` 依鍵排序。同鍵重複的 tbar 列計為 `ambiguous_tbar_keys`，
   整鍵不用。
4. 串流 `syn-partners`：只保留 `body_pre` 與 `body_post` 都在集合內且都非 null 的列（其餘計數），
   記錄 = 鍵 `(x_pre,y_pre,z_pre)` 12 B ＋ pre 索引 uint32 ＋ post 索引 uint32；走 `extsort`
   依鍵排序。
5. 兩個排序串流做 merge-join：syn-partners 列配到 tbar 鍵者輸出 `(pre,post,probs)`；未配到者計數
   `unmatched_synapses`。輸出再以 `(pre,post)` 為鍵走 `extsort`，聚合成每條 pair 的
   `matched_synapses` 與機率平均。
6. 與 `g.StreamAnnotatedEdges` 對齊：先確認 canonical edge 順序是否為 `(source,target)` 索引遞增
   （讀 `connectome` 程式碼並寫進報告）；是則 merge-join，否則建有界索引。圖中沒配到任何突觸的邊
   計 `edges_without_match`；聚合結果中不在圖上的 pair 計 `pairs_not_in_graph`（理論上為 0，
   非 0 要報）。
7. 每條邊：`transmitter = argmax(mean probs)`、`sign_confidence = 該平均機率`；
   `sign = Mapping[transmitter]` 若 `sign_confidence >= MinProbability` 且
   `matched_synapses / raw_weight >= MinMatchedFraction`，否則 unknown。
   `weight = Gain * raw_weight / normalizer`，normalizer 依規則取目標 `post_total` 或來源 `pre_total`，
   為 0 時 weight 為 0 並計數。原始 `raw_weight` 只在 GraphStore。
8. 報告：來源指紋、規則 hash、每一步的計數與比例（分子、分母、basis）、每種傳導物質的邊數、sign
   分布、unknown 比例、強度分位數、`normalizer_zero_edges`、extsort 統計（runs、暫存位元組、
   峰值記憶體）、耗時。

CLI：`data sources` 新增四筆；`data derive --store FILE --rules FILE --out FILE [--temp-dir DIR]
[--max-memory-bytes N] [--max-temp-bytes N] [--max-run-files N] [--max-arrow-bytes N]
[--max-rows N]` 印出推導報告；`data validate --params FILE [--store FILE]` 讀回校驗並印報告。

第二階段（runner 整合，改 `simulate` 與 CLI）：protocol 新增 `parameter_source:
"derived_release/v1"` 與 `derived{unknown_sign: "exclude"|"excitatory"|"inhibitory"}`（必填，
exclude 為權重 0），`simulate run --params FILE` 讀參數集、`CheckGraph`、以
`sign * weight`（unknown 依宣告處理並計數）配合 protocol 的 uniform 節點純量產生
`simulate.ParameterSet`，`Hash` 納入參數集檔 hash；報告 `assumptions` 寫明規則版本與 unknown 處理。
真實資料：`data derive` 跑完整 25,563,197 條邊，`simulate run` 以推導參數集與
`engineering_uniform_positive` 各跑同一 protocol，記錄差異，證據進 `evidence/NAT-02/`。

## 驗收

- [ ] fixture：小型 tbar／syn-partners／body-stats／Meta 檔案，手算每條邊的機率平均、sign、
  信心度、正規化強度與 unknown 計數；未配對突觸、`post_total = 0`、規則缺欄位、座標不
  相符、null 機率皆有明確處理與測試。
- [ ] bounded external sort 路徑走多 run；取消與容量錯誤不留暫存；參數集往返、竄改拒絕、
  與 GraphStore hash 不符拒絕。
- [ ] 真實資料：四份原件下載回條與逐批讀取驗證；全部選入邊推導完成，報告配對率、sign
  分布、unknown 比例與耗時／記憶體；12 的 runner 以此參數集執行至少一次並記錄與
  `engineering_uniform_positive` 的差異。
- [ ] 文件：來源稽核、機制文件（正負號為規則推導，非量測）、README、delivery-status；
  `evidence/NAT-02/`。

## 依據

- [研究方向](../research-directions/connectome-native.md)、[發布盤點](../malecns-release-catalog.md)、
  主規格 5.1–5.5、7.4、11.7、S16（chemoconnectome，僅作背景）。
