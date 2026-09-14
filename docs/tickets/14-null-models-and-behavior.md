# 14 — 研究者可以用空模型與判讀協定歸因接線的貢獻

Epic：原生模擬

User Story：研究者可以對同一份刺激，在原始接線、打亂接線、不同動態模型之間比較指定神經元
集合的活動指標，並以事前寫定的門檻判讀，不宣稱生物行為證明。

Blocked by：12 runner、13 參數集

Status：draft（root 決策已定，待派工；驗收項目驗證後才勾選）

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

## 契約

```go
package simulate
type NullModelSpec struct { Kind string; Seed uint64; SwapFactor float64 }
func DeriveNullModel(ctx, g *connectome.Graph, set *ParameterSet, spec NullModelSpec, limits) (*connectome.Graph, *ParameterSet, NullModelReport, error)
type NamedSet struct { Name string; Selectors []Selector }
type Metric struct { Name, Kind string; Set string; Window [2]int; Baseline string }
type Threshold struct { Metric string; Op string; Value float64 }
type CompareProtocol struct { Run Protocol; Variants []Variant; Sets []NamedSet; Metrics []Metric; Thresholds []Threshold; Seeds []uint64 }
func Compare(ctx, ...) (CompareReport, error)
```

## 驗收

- [ ] fixture：小圖上三種空模型的度數、pair 唯一性、符號與權重計數守恆與 seed 可重現；
  原圖檔前後指紋不變；指標與門檻的手算案例；latency 在無放電時為未定義而非 0。
- [ ] 同圖兩核心與至少一種空模型的比較矩陣可執行，報告差異與分位數。
- [ ] 真實資料：13 的參數集在原圖與三種空模型（各 ≥ 3 個 seed）上執行同一刺激協定，
  記錄命令、環境、指紋、耗時；報告只陳述指標，不做行為宣稱。
- [ ] 文件與 `evidence/NAT-03/`、`NAT-04/`、`NAT-05/`。

## 依據

- [研究方向](../research-directions/connectome-native.md)、主規格 3.2（科學界線）、14.4
  （對照組）、18.3（核心確實學習的證明精神移用到歸因）、20.2（負面結果保存）。
