# 13 — 研究者可以由官方發布資料推導動態參數並取得假設報告

Epic：原生模擬

User Story：研究者可以把 MaleCNS 發布中的突觸統計、逐突觸傳導物質機率、突觸配對與 ROI
階層接進框架，依明示規則產生每條邊的正負號與強度、每個神經元的標籤，輸出可校驗的參數集
與推導報告，未知保持未知。

Blocked by：08 標準化接線圖、09 GraphStore、12 runner（消費者）

Status：draft（root 決策已定，待派工；驗收項目驗證後才勾選）

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

## 契約

```go
package connectome  // 或 params 子套件；命名依既有慣例

type SignRule struct { ... }
type DerivationRequest struct { Graph *Graph (或 store 路徑); Sources ParameterSources; Sign SignRule; Strength StrengthRule; Limits ResourceLimits; TempDir string }
func DeriveParameters(ctx, req DerivationRequest) (*ParameterSet, DerivationReport, error)
func SaveParameterSet(ctx, path string, set *ParameterSet) (receipt, error)
func LoadParameterSet(ctx, path string, limits StoreLimits) (*ParameterSet, error)
```

CLI：`data sources`（新增四筆）、`data download`（既有）、`data derive --store FILE
--manifest FILE --rules FILE --out FILE`（輸出推導報告）、`data validate --params FILE`。

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
