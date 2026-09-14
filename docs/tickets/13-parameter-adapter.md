# 13 — 研究者可以由官方發布資料推導動態參數並取得假設報告

Epic：原生模擬

User Story：研究者可以把 MaleCNS 發布中的突觸統計、逐突觸傳導物質機率、突觸配對與 ROI
階層接進框架，依明示規則產生每條邊的正負號與強度、每個神經元的標籤，輸出可校驗的參數集
與推導報告，未知保持未知。

Blocked by：08 標準化接線圖、09 GraphStore、12 runner（消費者）

Status：verified_scoped（兩階段皆已驗證：`params` 套件與檔案格式、`data derive`／`data validate --params`、`simulate` 的 `derived_release/v1` 來源與 `simulate run --params`、全圖推導與對照實跑；NAT-02 標 passed。範圍限制：全圖只在 macOS arm64 跑過，unknown 政策只有 `exclude` 跑過全圖，`min_matched_fraction` 門檻在真實資料上沒有觸發，節點純量仍是統一工程值）

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

- [x] fixture：小型 tbar／syn-partners／body-stats／Meta 檔案，手算每條邊的機率平均、sign、
  信心度、正規化強度與 unknown 計數；未配對突觸、`post_total = 0`、規則缺欄位、座標不
  相符、null 機率皆有明確處理與測試。
- [x] bounded external sort 路徑走多 run；取消與容量錯誤不留暫存；參數集往返、竄改拒絕、
  與 GraphStore hash 不符拒絕。
- [x] 真實資料：四份原件下載回條與逐批讀取驗證；全部選入邊推導完成，報告配對率、sign
  分布、unknown 比例與耗時／記憶體；12 的 runner 以此參數集執行至少一次並記錄與
  `engineering_uniform_positive` 的差異。（25,563,197 條邊、配對比例 1.000、sign
  +13,636,628／−9,172,513／unknown 2,754,056、207.31 s、3.87 GB RSS；對照見下節與
  `evidence/NAT-02/verification.json`。）
- [x] 文件：來源稽核、機制文件（正負號為規則推導，非量測）、README、delivery-status；
  `evidence/NAT-02/`。（來源稽核於 2026-09-14 新增原件章節；機制文件與 README 由 Opus 更新；
  delivery-status 與 requirements-status 的 NAT-02 由 root 補齊。）

## 依據

- [研究方向](../research-directions/connectome-native.md)、[發布盤點](../malecns-release-catalog.md)、
  主規格 5.1–5.5、7.4、11.7、S16（chemoconnectome，僅作背景）。

## 第一階段證據（2026-09-15）

### 交付

新套件 `params`：`rules.go`（`DecodeRules`、驗證、規則 hash）、`derive.go`（八步流程、
`Limits`、記憶體與暫存帳）、`set.go`（`Set`、`Save`、`Load`、`LoadWithReceipt`、
`CheckGraph`、區段＋footer＋SHA-256 格式）、`report.go`（`Report` 與分位數）、
`meta.go`（`Neuprint_Meta.csv` 有界讀取）、`columns.go`（Arrow 欄位存取）。
CLI：`internal/cli/derive.go`（`data derive`、`data validate --params` 的實作），
`internal/cli/validate.go` 加 `--params`，`internal/cli/data.go` 加四筆來源與 `derive`
分派，`internal/cli/run.go` 加總覽行。

### 先寫失敗測試

`evidence/NAT-02/red-params.log`（`go test -count=1 ./params/`：`undefined: Set`、
`undefined: Rules`、`undefined: Limits` 等，build failed）與
`evidence/NAT-02/red-cli.log`（`go test -count=1 ./internal/cli/`：
`source count = 3, want 7`、`data derive: unknown data command`）。

### 手算 fixture

四個 body（10、20、30、40，節點索引 0..3）、12 列 tbar、14 列 syn-partners、6 列
body-stats、6 條邊。機率一律取 16 分之一的整數倍，float32 與 float64 都精確，期望值是
寫下來的，不是用同一段程式重算的。基準規則為 `gain = 2`、`min_probability = 0.5`、
`min_matched_fraction = 0.25`、`normalizer = post_total`。

| 邊 | pair | raw | 配對突觸 | 勝出機率平均 | 傳導物質 | sign | 配對比例 | post_total 權重 | pre_total 權重 | none 權重 |
| --- | --- | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: |
| 0 | (0,1) | 4 | 2 | (0.75+0.5)/2 = 0.625 | acetylcholine | +1 | 2/4 = 0.5 | 2·4/4 = 2 | 2·4/5 = 1.6 | 8 |
| 1 | (0,2) | 2 | 1 | 0.6875 | gaba | −1 | 1/2 = 0.5 | 2·2/6 | 2·2/5 = 0.8 | 4 |
| 2 | (0,3) | 4 | 1 | 0.625 | dopamine | 0 | 1/4 = 0.25（含邊界） | 0（post_total 為 0） | 2·4/5 = 1.6 | 8 |
| 3 | (1,2) | 3 | 1 | 0.5625 | glutamate | −1 | 1/3 | 2·3/6 = 1 | 2·3/3 = 2 | 6 |
| 4 | (2,3) | 1 | 0 | 無（唯一突觸配到 null 機率 T-bar） | 無 | 0 | 未定義 | 0（post_total 為 0） | 2·1/2 = 1 | 2 |
| 5 | (3,0) | 8 | 0 | 無（唯一突觸座標 z 差一格） | 無 | 0 | 未定義 | 2·8/7 | 0（pre_total 為 0） | 16 |

body-stats 的 pre／post 為 10:(5,7)、20:(3,4)、30:(2,6)，body 40 整列缺席（`bodies_missing_from_body_stats = 1`，
pre／post 視為 0）。body 20 有重複列且第一列勝出，另有一列 null body 與一列未選入的 body 99。
sign 分布為 +1 一條、−1 兩條、unknown 三條，`unknown_ratio = 3/6`；unknown 原因為
「規則對應 unknown」一條、「沒有配對」兩條。傳導物質邊數為 acetylcholine、dopamine、gaba、
glutamate 各一條，histamine／octopamine／serotonin 各零條，未配對兩條。
`normalizer_zero_edges` 在 post_total 為 2、pre_total 為 1、none 為 0。權重分位數用
nearest-rank：排序後 0、0、2·2/6、1、2、2·8/7，p0 = 0、p25 = 0、p50 = 2·2/6、p75 = 2、
p100 = 2·8/7。

計數驗證涵蓋：tbar 12 列（1 列 null 座標、1 列 null body 跳過，10 列進排序，9 個相異鍵，
1 個 ambiguous 鍵含 2 列，1 個 null 機率鍵，7 個可用鍵，1 個鍵沒有對應突觸）；
syn-partners 14 列（1 列 null body、1 列 null 座標、2 列 body 未選入，10 列進排序，
6 個配對、2 個未配對、1 個落在 ambiguous 鍵、1 個落在 null 機率鍵）；pair 5 個，
其中 (1,3) 不在圖上（`pairs_not_in_graph = 1`）；`edges_without_match = 2`。
`primary_post` 眾數為 ["", "AL", "MB", "CX"]，節點 0 只有 null ROI。
門檻另以兩組規則驗證：`min_probability = 0.65` 得 +0／−1／unknown 5，
`min_matched_fraction = 0.4` 得 +1／−1／unknown 4。

測試入口：`params/derive_test.go` 的 `TestDeriveMatchesTheHandCalculatedFixture`、
`TestDeriveNormalizersAndZeroCounts`、`TestDeriveThresholdsTurnEdgesUnknown`、
`TestDeriveKeepsConfidenceForUnknownSigns`、`TestDeriveIsDeterministic`、
`TestDeriveRejectsChangedSources`；規則檔缺欄位、glutamate basis 缺「假設」、
值域與路徑限制在 `params/rules_test.go`。

### 有界排序與檔案格式

`TestDeriveUsesMultipleSortRuns` 用 2,000 對突觸與 200 KiB 記憶體限制，確認四個排序中至少
兩個走多 run 且都寫出暫存位元組，結束後暫存目錄為空。
`TestDeriveCancellationAndCapacityLeaveNoTemporaryFiles` 確認取消（`context.Canceled`）
與暫存超限（同時包住 `params.ErrCapacity` 與 `extsort.ErrCapacity`）都不留檔案。
`params/set_test.go` 覆蓋往返一致、兩次落盤位元組相同、拒絕覆寫、八種竄改（頭尾 magic、
區段位元組、中段位元組、footer 長度、footer hash、截斷、刪一個位元組）、三種上限與
`CheckGraph` 對不同圖與被改過的 hash 的拒絕。

### CLI

`internal/cli/derive_test.go` 以 `importFixture` 建的 store 走完
`data derive` → `data validate --params`（含 `--store` 與不含 `--store` 兩種），確認輸出
schema、檔案大小與 SHA-256 一致、第二次 derive 不覆寫，以及 15 種失敗參數與說明文字。
`data sources` 的既有三筆維持原值，新增四筆的 URL、角色、大小、ETag、CRC32C 取自
`evidence/malecns-source-20260914/*-download.json`，`audit_date` 為 2026-09-14，
`checked_at` 取回條的 `acquired_at`。

### 驗證命令

```
gofmt -l .                                          # 無輸出
go vet ./...                                        # 無輸出
go test -count=1 ./...                              # 全部 ok，含 params 與 internal/cli
go test -race -count=1 ./params/ ./internal/cli/    # 兩個套件皆 ok
```

環境：Go 1.26.5、darwin/arm64。

### 尚未完成

真實資料推導（25,563,197 條邊）與 `simulate` 整合屬第二階段，本階段未執行，
因此「真實資料」與「文件」兩項驗收保持未勾選：`delivery-status.md` 與
`docs/model-and-mechanisms.md` 不在本次派工的可修改範圍。
`post_total = 0` 以「body 40 不在 body-stats」的路徑驗證（`normalizer <= 0` 同一條分支），
body-stats 內明寫 0 的列未另外建 fixture。

### root 審查（2026-09-15）

逐檔讀過 `params` 六個檔、CLI 與四筆來源；手算表由 root 獨立重算一次（九個 tbar 鍵、六個配對、
五個 pair、六條邊的 sign／三種 normalizer 權重／分位數／ROI 眾數）與程式一致；來源四筆的
ETag、CRC32C、大小對回 `evidence/malecns-source-20260914/*-download.json`。root 重跑
gofmt／vet／`go test ./...`／race 全數通過（params 與 cli 共 53 個測試）。接受第一階段的九項
契約偏離。待改善（不阻擋）：`alignEdges` 只信任 connectome 的邊序，沒有在執行期檢查 (source,
target) 單調遞增，建議加一個便宜的防護；重複 pair 緩衝的記憶體保留只增不減。（兩項已於
ticket 15 補上：`orderedEdges` 防護與只計成長差額的保留。）

## 第二階段證據（2026-09-15）

### 交付

`simulate` 新增第二個參數來源 `derived_release/v1`：`protocol.go` 的
`DerivedParameters{unknown_sign, weight_scale}`（兩個欄位都必填、沒有預設值）與
`validateParameterSource`，`parameters.go` 的 `FromDerived`、`DerivedSummary` 與放寬後的
`validate`，`report.go` 的五個新報告欄位，`runner.go` 依來源改寫 `assumptions`。
CLI：`internal/cli/simulate.go` 加 `--params` 與 `simulateParameters`（derived 必填、uniform 拒絕，
以 `params.LoadWithReceipt` 讀檔後 `CheckGraph` 再 `FromDerived`）。
腳本：`scripts/derive-evidence.sh`（新增）與 `scripts/simulate-evidence.sh` 的第三個參數。
文件：README 的 `simulate run`／`data derive` 兩節、ENG 的 `simulate`／`params` 兩條、
`docs/model-and-mechanisms.md` 的「由發布資料推導的邊參數」一列。

### 先寫失敗測試

`evidence/NAT-02/red-stage2-simulate.log`（`undefined: ParameterSourceDerived`、
`undefined: FromDerived`、`unknown field Derived in struct literal of type Protocol`，build failed）與
`evidence/NAT-02/red-stage2-cli.log`（`undefined: simulate.DerivedParameters`、
`p.Derived undefined`，build failed）。實作後兩個套件全綠。

### 手算 fixture

`simulate/derived_fixture_test.go` 重建第一階段的四份推導來源與四節點六邊圖（Go 不能跨套件
import 測試檔，因此複製寫入器與列表，期望值另外由票面表格寫出），`simulate/derived_test.go`
以 `params.Derive` 真的推導一次再套 `FromDerived`。基準規則 `gain = 2`、`normalizer = post_total`
的六條邊為 sign `+1 / −1 / 0 / −1 / 0 / 0`、強度 `2 / 2·2/6 / 0 / 1 / 0 / 2·8/7`，取
`weight_scale = 4`（2 的冪，乘法逐位精確）後三種政策的期望權重為：

| 政策 | 邊 0 | 邊 1 | 邊 2 | 邊 3 | 邊 4 | 邊 5 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| exclude | 8 | −4·(4/6) | 0 | −4 | 0 | 0 |
| excitatory | 8 | −4·(4/6) | 0 | −4 | 0 | 4·(16/7) |
| inhibitory | 8 | −4·(4/6) | 0 | −4 | 0 | −4·(16/7) |

`DerivedSummary` 為 +1 一條、−1 兩條、unknown 三條。另有：兩種核心 × 三種政策實跑、
報告五個新欄位與 `assumptions` 內容、兩次執行 JSON 位元組相同、中途保存狀態再接續與一次跑完
相同、參數集對不上的圖被拒、hash 隨參數集檔 SHA-256／政策／scale 改變而 uniform 的 hash 不變、
以及八種不合法 protocol。CLI 另測 `data derive` → `simulate run --params` 全流程、
uniform 報告不帶 derived 欄位、`--state-out`／`--state-in` 接續，以及缺 `--params`、
uniform 配 `--params`、uniform protocol 帶 `derived` 區塊、缺政策、derived 配 `uniform.gain`
等八種失敗。

### 真實資料

命令與完整判讀見 `evidence/NAT-02/verification.json`。

| 項目 | 實測 |
| --- | --- |
| 推導規模 | 165,122 節點、25,563,197 條邊；四份來源 SHA-256 與下載回條相同 |
| 座標配對 | 45,656,140 個 T-bar 鍵，0 個重複鍵、0 個 null 機率鍵；配對比例 124,025,046／124,025,046 = 1.000，未配對 0 |
| pair | 25,563,197 個，`pairs_not_in_graph` 0，`edges_without_match` 0 |
| sign 分布 | +13,636,628／−9,172,513／unknown 2,754,056（10.77%） |
| unknown 原因 | 沒配對 0、低於 `min_probability` 1,038,293、低於 `min_matched_fraction` 0、規則對應 unknown 1,715,763 |
| 每種傳導物質邊數 | acetylcholine 13,947,812、gaba 4,705,861、glutamate 4,633,150、dopamine 1,233,608、serotonin 581,130、octopamine 314,307、histamine 147,329 |
| 強度分位數 | p0 8.02e-06、p25 0.000899、p50 0.002311、p75 0.005682、p100 1；`normalizer_zero_edges` 0 |
| 外部排序 | tbar 3 runs／1.87 GB、syn-partners 5 runs／2.48 GB、pair 7 runs／4.46 GB、ROI 2 runs／0.78 GB；中介檔 1.87 GB 與 1.74 GB；暫存目錄跑完為空 |
| 推導資源 | 牆鐘 207.31 s、max RSS 3,870,539,776 B、帳面峰值 3,557,271,500 B（上限 6 GiB 記憶體、40 GiB 暫存皆未觸及） |
| 參數集 | 463,451,966 B、SHA-256 `c4db0f3f…0c58c77`；`data validate --params --store` 在新程序通過 |

`weight_scale` 取 21.6：非零推導強度的中位數是 0.002310803004043905，
`0.05 / 0.002310803004043905 = 21.6375`，取 21.6 後中位權重大小為 0.0499，與 NAT-01 用
`gain 0.025 × 中位原始 weight 2 = 0.05` 同一量級，兩次執行因此可比。

與 `engineering_uniform_positive`（`evidence/NAT-01/fullgraph-uniform-v1/report.json`）在同一
store、同一 protocol 形狀與同一刺激下的差異：

| 觀察 | uniform | derived（exclude、21.6） |
| --- | --- | --- |
| 沉默比例 | 0.0206514（3,410 顆） | 0.6439844（106,336 顆） |
| 放電率分位數 | 0、0.47333、0.47333、0.47667、0.48333 | 0、0、0、0.02333、0.48333 |
| 最大全群放電比例 | 0.5466564（第 130 步） | 0.0793474（第 167 步） |
| 第 31 步後平均全群放電比例 | 0.4878298 | 0.0668042（最小 0.0269437、末步 0.0722799） |
| 穩定旗標 | `["max_population_rate_exceeded"]` | `[]`（同一組門檻） |
| `alin_injected_spikes` 峰值 | 1（第 10 步） | 1（第 10 步） |
| `alin_injected_trace` 峰值 | 3.0332448（第 186 步） | 1.7441003（第 25 步） |
| `descending_neuron_spikes` 峰值 | 0.5936073（第 15 步） | 0.1255708（第 68 步） |
| `vnc_motor_spikes` 峰值 | 0.5169492（第 20 步） | 0.1200565（第 114 步） |
| 牆鐘／max RSS | 93.08 s／4,378,574,848 B | 82.57 s／5,976,768,512 B |

同一份推導以兩個不同 build、兩個行程各跑一次，除了耗時、峰值記憶體與因此改變的參數集檔
hash 外，每個計數、比例、直方圖與分位數都相同；兩次全圖模擬的探針時序與 monitors 也完全
相同，只有 `parameter_hash` 因為納入參數集檔 SHA-256 而不同。保留的證據是第二次（其
`binary.sha256` 對應現在的原始碼）。

### 與票面契約的差異（皆為刻意）

1. `FromDerived` 的簽章多一個參數：票面寫 `FromDerived(set, protocol)`，但同一句要求 hash 納入
   參數集檔的 SHA-256，而 `params.Set` 不帶該指紋，因此改為
   `FromDerived(set *params.Set, setSHA256 string, protocol Protocol)`，由呼叫端傳入。
2. derived 來源的 uniform `gain` 規則定為「不寫或為零」，非零即拒絕（有測試）。理由是邊強度
   來自參數集與 `weight_scale`，留一個會被忽略的第二個強度旋鈕是陷阱。兩種來源都仍需要
   uniform 區塊，因為 bias／log_tau／theta_raw 沒有別的來源。
3. `RunReport.unknown_sign_edges` 是 `*uint64`：其餘四個 derived 欄位用 `omitempty` 即可，但
   unknown 邊數為 0 是有意義的結果，指標讓 derived 一定印出、uniform 一定不印。
4. hash 只納入票面點名的四項出處（參數集檔 SHA-256、規則 hash、政策、scale）加四組陣列；
   `DerivedSummary` 的三個計數不入 hash，因為它們由陣列與政策決定。編碼寫在 `FromDerived`
   的註解，uniform 的既有編碼未變（NAT-01 的 `parameter_hash` 仍可重現，有測試）。
5. 乘積為零時一律存正零，避免 `-0` 進到 hash 與報告。
6. `min_matched_fraction 0.5` 在真實資料上沒有擋掉任何一條邊（所有配對比例都是 1.0），因此
   這個門檻只在 fixture 上驗證過。
7. 既有測試 `TestProtocolValidationRejectsUnsupportedConfigurations` 原本要求錯誤訊息寫出
   「ticket 13」，改為要求列出兩個實際可用的來源。
8. `docs/model-and-mechanisms.md` 除了票面要求的新增一列，另修正兩句已被本次結果推翻的敘述
   （runner 那列的「目前只有 `engineering_uniform_positive`」與「還沒有將其轉成作用符號」）。
9. 第一次全圖推導用的是第二階段改動前的 build。為了讓證據裡的 `binary.sha256` 對應交付的
   原始碼，推導與模擬都以最終 build 重跑一次，只保留第二次；第一次的數值一致性記在上面。

### 尚未完成

空模型對照（NAT-03）與具名神經元判讀（NAT-04）依 ticket 14 處理。`excitatory`／`inhibitory`
兩種政策只在 fixture 上跑過，全圖只跑 `exclude`；全圖也只在 macOS arm64 上跑過。

### root 審查（2026-09-15，第二階段）

逐檔讀過 `simulate` 四個檔的 diff、CLI、兩支證據腳本、規則檔與 `verification.json`；報告裡的
三個總和（sign、四個 unknown 原因、七種傳導物質）都等於 25,563,197，unknown 原因的拆解與各傳導物質
低於門檻的邊數互相吻合，規則檔兩份 SHA-256 相同，`validate.json` 確認參數集與 store 的 hash 相符。
root 重跑 gofmt／vet／`go test ./...`／race 全數通過（simulate、params、cli 共 81 個測試）。接受九項
契約偏離。delivery-status 與 requirements-status 由 root 補齊後勾選「文件」。
