# 08 — 使用者可以由官方 MaleCNS 原件建立可追溯標準化接線圖

Epic：真實接線資料

User Story：研究者可以在不改寫原始 Feather 的前提下，依明示且可重建的規則建立
`raw_segments` 與 `annotated_neurons` 視圖，並取得可核對的接線統計。

Blocked by：06 官方資料下載、07 Feather 逐批讀取

Status：done（in-process 圖與報告；durable GraphStore 切到 09）

## 交付

這張票只切一條可驗證的資料流程：輸入已由 06 取得、由 07 可讀取且與
`DatasetManifest` 相符的 MaleCNS v1.0 三份原件，輸出一個不可變的標準化圖視圖、
可逆的節點索引與統計報告。原始檔維持只讀，標準化結果是可由 manifest、原件雜湊、
轉換器版本與選取條件重建的衍生物。

本票的最低交付是具體的 Go 建構 API、可串流讀取的不可變圖結果、兩個具名視圖與
報告。固定的磁碟 GraphStore 位元組格式及 `data graph` CLI 若要在本票完成，必須
由 root 在實作前把它加入驗收；較小且可獨立驗收的建議是另開下一張票，沿用本票
產生的 canonical stream 與報告，不因此刪減全圖建構目標。

建構器是可重用的通用框架能力；MaleCNS v1.0 只是本票的第一個明示 adapter 與
驗收來源。任何果蠅特定訓練程式、模型權重或任務 pipeline 都只能作為另行標示的
範例，不能變成 repo 的產品主體或偷渡成圖建構器的隱含行為。

## Root 決策（2026-09-13）

以下裁決固定本票的共用契約，實作與測試依此進行：

1. **選取欄位**：predicate 使用 annotations 的 `status` 欄位，條件為 `status == "Traced"`，canonical form 為 `annotations.status == "Traced"`。`statusLabel` 只保留在 `NodeRecord` 作品質註記，不參與選取。理由：`status` 是六值的粗粒度整理欄位，[來源值查核](../../evidence/malecns-source-20260913/graph-values-audit.json) 顯示 Traced 165,122 列且 `bodyId` 無重複；`statusLabel` 有 19 值與 887 null，語意更細但沒有官方定義可對應「已整理神經元」。此條件在 manifest 中明示且標為 engineering selection，不代表生物完整性。
2. **端點身份**：manifest 必須明示 `identity.weights_endpoints_are_annotation_ids` 與 `identity.neurotransmitter_ids_are_annotation_ids` 及 evidence 文字。MaleCNS v1.0 的依據是官方下載頁把 weights 描述為同一 release 中 segment（body）之間的 connection strengths、annotations 為 per-body 註記，三份檔案同屬 `v1.0`／`minconf-0.5` 命名，且來源值查核中 165,122 個 Traced body 對應 25,563,197 列兩端皆 Traced 的邊。這是有來源出處的工程身份假設，不是量測驗證的生物映射。manifest 未宣告時 `Build` 以 `ErrIdentityMapping` 拒絕，不猜測。
3. **比例分母**：`annotated_neurons` 的傳導物質未知比例分母為選入節點數，分子為沒有 NT 列、`consensus_nt` 為 null／空字串／`unclear` 的節點數，basis 標為 `consensus_nt`。raw NT 未知比例分母為 NT 檔列數。受體維持 `not_derived`，另報 `receptorType` 標籤為 null 的節點數／選入節點數。所有 ratio 保存分子、分母與 basis。
4. **索引順序與 ID**：外部 ID 保存為 namespace 加無損十進位字串；canonical 順序為同一 namespace 內的數值遞增，等於 `(namespace, 20 位零填十進位)` 的字典序。負值列為 `invalid_id`，不轉成浮點或 32 位元。索引寬度依節點數選 32 或 64 位元儲存，超出 `uint32` 時改用 64 位元，不截斷。
5. **容量驗收值**：真實資料驗收使用 `MaxMemoryBytes = 4 GiB`、`MaxTempBytes = 16 GiB`、`MaxRunFiles = 256`、`MaxArrowBytes = 128 MiB`、`MaxFooterBytes = 16 MiB`、`MaxRows = 250,000,000`。fixture 測試以刻意小的限制強制多個外部 run。
6. **Durable GraphStore／CLI**：固定位元組格式的 GraphStore 切到下一張票（09），以本票的 canonical stream 與報告為輸入。本票新增 `data import --manifest FILE` CLI，執行 `Build` 並輸出報告 JSON，用於真實資料 evidence；圖結果只存在程序內。
7. **raw view 串流**：`raw_segments` 不在記憶體複製 151M 列。`StreamRawSegments` 重新逐批讀取 weights 原件，讀取前後比對 manifest 指紋，不一致即錯誤。raw view 的 duplicate pair／row 統計與 annotated view 走同一條 bounded external sort 路徑。

## 已核對的來源事實與限制

來源查核見 [MaleCNS audit](../malecns-source-audit.md)、[Feather 驗證](../../evidence/malecns-source-20260913/cli-v4/verification.json) 與
[主規格第 5 章](../handoff/CoImNet_Implementation_Plan.zh-TW.md)。目前可用的
原件契約如下：

| 檔案角色 | 已核對 schema／現況 | 本票的語意界線 |
| --- | --- | --- |
| `connectome-weights-male-cns-v1.0-minconf-0.5.feather` | `body_pre`、`body_post`、`weight`，皆 nullable `int64`；目前逐批讀取 151,856,684 列 | 官方說明為 segment-to-segment connection strengths。沒有 ROI、突觸位置或 total／partition 標記，不能把 `weight` 改稱突觸數，也不能自行聚合。 |
| `body-annotations-male-cns-v1.0-minconf-0.5.feather` | 36 個 nullable 欄位；`bodyId` 為 `int64`，`statusLabel` 是 dictionary；目前逐批讀取 211,577 列 | `bodyId` 與 weights 端點的身份關係必須由 manifest 欄位映射明示且驗證，不能只因欄位名稱相似就拼接。 |
| `body-neurotransmitters-male-cns-v1.0.feather` | 10 個 nullable 欄位；`body` 為 `int64`；目前逐批讀取 1,835,518 列 | `predicted_nt`／`consensus_nt` 是來源預測欄位，不能當量測出的作用或受體。 |

來源掃描另回報 Glia 11,864。完整欄位、未知值與選取統計已由
[graph-v1 實測](../../evidence/malecns-source-20260913/graph-v1/verification.json)
補上，摘要見 [來源稽核](../malecns-source-audit.md#標準化圖建構實測)。

## 公開契約草案

實作應提供具體型別，不以空介面或任意 callback 取代資料契約。名稱可依既有套件
慣例調整，但語意必須保留：

```go
type BuildRequest struct {
	Manifest  DatasetManifest
	Selection SelectionPredicate
	Limits    ResourceLimits
	TempDir   string
}

func Build(ctx context.Context, req BuildRequest) (*GraphBuildResult, error)

type GraphBuildResult struct {
	Graph  *Graph
	Report GraphReport
}

func (g *Graph) Node(index uint64) (NodeRecord, error)
func (g *Graph) StreamAnnotatedNodes(ctx context.Context, fn func(NodeRecord) error) error
func (g *Graph) StreamRawSegments(ctx context.Context, fn func(RawSegment) error) error
func (g *Graph) StreamAnnotatedEdges(ctx context.Context, fn func(EdgeRecord) error) error
```

`Build` 驗證後才建立結果。`Node` 回傳值的欄位不可由呼叫端改動；串流 callback 收到
的 record 僅在該次呼叫有效，呼叫端要保存時自行複製。建構器、圖結果及匯出器不得
共享可變切片。
圖的節點和邊順序固定，重跑相同輸入會有相同索引、canonical 順序與報告雜湊。

`DatasetManifest` 至少要帶 schema 版本、資料集／來源版本、檔案 role／路徑／校驗、
授權來源、取得時間、官方欄位映射、選取條件、座標單位與轉換歷史。建構器也要
保存轉換器版本、每個 source fingerprint、predicate canonical form 與 hash。

`SelectionPredicate` 必須是可序列化、可比較的明示條件，不接受不可重建的任意
函式。候選條件是 `annotations.status == "Traced"`，但 `status` 和
`statusLabel` 的正式採用欄位仍待 root 審核；在決策前不能把它當預設，也不能把
它標成官方生物完整性。未提供、重複或不支援的 predicate 直接返回錯誤。

標準化 record 的必要欄位為：

- `NodeRecord`：dataset namespace、無損 `ExternalID`、連續 `Index`、有明確未知狀態的
  type／region／status 欄位、來源位置與品質註記。`statusLabel` dictionary 的 null、
  dictionary value 與其他 list／nullable 欄位不可用空字串或零代替。
- `RawSegment`：原始 namespace、來源及目標 external ID、nullable 的原始 `weight`、
  source file role、record batch、batch 內 row 與絕對 row。原始 signed `int64` 端點值
  在 canonical ID 無效時仍保留，該列不可因無法建立 index 而遺失。每一列都保留，沒有
  聚合。
- `EdgeRecord`：來源及目標連續索引、同一份原始值與來源位置、`aggregation_evidence`
  和可選的 transmitter／作用符號證據。weights 沒有這些證據時狀態是 unknown，
  不從 `receptorType`、預測傳導物質或欄位名稱推導。

外部 ID 以 `namespace + lossless decimal string` 作為穩定鍵，index 順序固定為該 tuple
的字典序，不依首次出現順序。來源的 signed `int64`
先拒絕負值，再以無損十進位形式保存；轉成內部 `uint64` 或 slice index 前要檢查
範圍，不能轉成 32 位元或浮點數。索引表必須可由 index 找回 namespace／external ID，
且節點數、row 數、byte 計算和累加結果都要做容量溢位檢查。被保留的 float 欄位若為
NaN／Inf，沒有來源定義的 null 語意時直接返回值驗證錯誤。

## 使用流程與狀態

```text
DatasetManifest + 三份只讀 Feather
        │ 角色／版本／校驗／schema／欄位映射驗證
        ▼
逐批 Scan ──> typed raw rows（null、dictionary、list、source position 保留）
        │
        ├── endpoint ID catalog：排序、去除相鄰重複、穩定指派連續 index
        ├── explicit predicate：建立 annotated node set，記錄選取 hash
        └── edge runs：依 (source index, target index, source position) 排序
                         bounded k-way merge，數重複但不合併
        ▼
不可變 Graph{raw_segments, annotated_neurons} + GraphReport
        │
        └── 只有 final input fingerprint、輸出校驗與報告完成後才可發布結果
```

狀態依序為 `unvalidated → scanning → catalogued → normalized → merged → complete`。
任一錯誤或取消進入 `failed`，不得把部分結果標成 complete。若採用可恢復的暫存
run，checkpoint 必須含 source fingerprint、manifest hash、converter version、
predicate hash、sort key 與 phase；只有全部相符才可接續。未知或不相符的暫存檔
不可刪除或重設。

`raw_segments` 輸出 weights 的每一個原始 row，包含 null endpoint／null weight，
並以原始來源欄位命名。`annotated_neurons` 只輸出通過已核准 predicate 的節點及
兩端都能依 manifest 驗證到該 namespace 的邊；其餘原始 rows 仍留在 raw view，並
以單一、可計數的 exclusion reason 排除。端點 identity 映射沒有來源證據時，建構器
可以完成 raw view，但必須拒絕 annotated view 並指出缺少的映射，不能猜測。

## 報告契約

報告同時保存來源檔案角色／URL／大小／ETag／校驗性質、manifest 與 converter hash、
predicate canonical form／hash、視圖名稱及以下計數。每個 ratio 都要保存 numerator、
denominator 和 source field basis，避免不同人用不同分母解讀：

- 原始 segment row 數、選入 neuron 數、raw endpoint 數及 unique endpoint 數。
- 依 `null_endpoint`、`invalid_id`、`missing_annotation`、`predicate_false`、
  `identity_mapping_unverified` 等原因的排除數。annotation 列的 `predicate_field_null`
  是 `predicate_false` 的子集，不另計為一種排除原因。
- 無對應 annotation 的 endpoint occurrence 與 unique ID 數、神經元 pair 數。
- 原始 `weight` 的有效值數、null 數與精確十進位加總；此欄命名為 source raw value，
  不宣稱是突觸／contact count。加總若超過選定表示範圍要錯誤，不截斷。
- `predicted_neurotransmitter_unknown_ratio` 僅能以明示的 prediction 欄位與分母
  計算；量測傳導物質作用仍標示 `not_available`。
- `chemical_receptor_unknown_ratio` 在沒有經來源證據驗證的化學受體欄位時標示
  `not_derived`；可另報 `receptorType` annotation label 的 null／unknown 比例，
  但不能把兩者混在一起。
- 選定 view 的 isolated node 數、self-loop 數、duplicate pair 數與 duplicate row
  數。duplicate 定義為相同 `(source index, target index)` 的相鄰 run，保留全部 rows，
  不自行去重或加總。

weights 的 duplicate 語意目前是 unknown。raw row view 可以原樣保留並報告重複；任何
聲稱每個 pair 唯一或已聚合的 normalized view，在沒有來源證據時必須以
`ErrAggregationEvidence` 拒絕，不能把計數報告誤當成合法去重。

`male-full` 若在日後使用，只代表 manifest 所宣告的完整選取視圖，不代表涵蓋每個
身體組織、全部化學傳遞或生物完整性。

## 151M rows 的排序與容量方案

不能以全量 `map[pair]struct{}` 偵測 duplicates，也不能使用 O(N²) 比較。建議的最小
正確方案如下：

1. 第一次掃描將 endpoint ID 寫入受 `MaxMemoryBytes` 限制的 compact chunks，逐 chunk
   排序並 merge 成穩定、可逆的 ID catalog；以排序後的 catalog 查找 index，避免把
   所有 edge pair 放進 map。若 catalog 或 byte 計算超出上限，返回明確容量錯誤。
2. 第二次掃描將固定寬度的 edge key、原始 nullable value 與 source position 寫入
   bounded sorted runs，排序鍵為 `(source index, target index, source file role,
   batch, row)`，再做 bounded k-way merge。相鄰 pair 直接計數 duplicate，保留每個
   record 的原始位置和值。
3. `MaxTempBytes`、輸出容量與檔案數也要檢查乘法溢位。暫存空間不足時停止並回傳
   錯誤，不降採樣、不只建立小圖、不偷偷改用全量 pair map。小型 fixture 可以落在
   同一排序／merge 路徑，不能另有會改變順序的 shortcut。

這個方案的工作記憶體是由 limits 約束，完整 151,856,684 rows 仍以外部 runs 處理；
它不替 root 決定最終 GraphStore 的 byte layout。若 root 要求輸出同一輪落盤，writer
必須直接消費 merge stream，並以 manifest、node map、edge sort key 與 report 一起
原子發布。

## Error／rescue map

| 操作 | 失敗／錯誤 | 建構器行為與使用者可見結果 | owner／測試 |
| --- | --- | --- | --- |
| Manifest preflight | schema、role、版本、校驗、field mapping 或 namespace 缺漏／重複 | `ErrInvalidManifest`，不建立 graph；指出檔案 role／欄位 | T08／fixture unit |
| Source preflight | 路徑非 regular、來源已改變、雜湊／長度／Feather magic 不符 | `ErrSourceChanged` 或 schema error；既有成果不覆寫 | T08／06、07 integration |
| Batch scan | footer、record、型別、dictionary、list 或 callback 錯誤 | 傳回原始 error 加 file role／batch／row；不得 complete | T08／corrupt fixture、callback error |
| ID canonicalization | null／負 signed ID、範圍或 namespace 映射不合法 | raw row 保留可報告的 unknown；無法建立 key 時按 reason 排除或拒絕，不能截斷 | T08／boundary unit |
| Predicate validation | 未提供、重複、不支援或欄位語意未決 | `ErrInvalidSelection`；不套用隱含 `Traced` | root＋T08／unit |
| ID catalog／sort | memory、temporary bytes、run count 或 byte multiplication 溢位 | `ErrCapacity`；保留未知暫存，沒有小圖 fallback | T08／forced multi-run test |
| Identity join | weights endpoint 到 annotation 的映射未被 manifest／來源證實 | raw view 可完成；annotated view 拒絕並指出 mapping gap | root／integration |
| Duplicate handling | 同 pair 多列，或 aggregation evidence unknown | 計數 pair／row、保留全部列；要求聚合時返回明確錯誤 | T08／duplicate fixture |
| NT／receptor fields | nullable、無匹配、只有 prediction 或 `receptorType` label | 保留 unknown／not-derived 與分母；不產生作用符號 | T08／unknown fixture |
| Merge／output | run 損壞、寫入／校驗／最後 input fingerprint 不符 | 不發布新結果；若已發布但 durability 未確認，報告 `published=true` 與未確認狀態 | T08＋後續 Store ticket／fault test |
| Cancellation | batch、sort、merge 或 callback 間收到 `context.Canceled` | 停止且不標完整；checkpoint 只供相同輸入／predicate 的明確 resume | T08／cancel-resume integration |

不使用 `recover`／`catch all` 吞掉未知錯誤。Feather reader 已將其 parser 防護轉成
錯誤；graph builder 只包裝有上下文的具體錯誤並保留原 error。

## Shadow paths 與邊界

| 路徑 | 預期行為 | 負責驗收 |
| --- | --- | --- |
| Happy path | 三份檔案、manifest、identity mapping 與 predicate 都驗證；兩視圖、可逆 index、穩定排序和完整報告產生 | T08 |
| Null／unknown | null endpoint／weight、dictionary null、list null、無 annotation、無 NT prediction 均保留 unknown；只依規則計數或排除 | T08 |
| Empty／zero | 零 rows 可產生 `raw_segments` 的空報告；零 selected nodes 可產生明示空 annotated view；零 weight 不被當缺值；零／負 limits 拒絕 | T08 |
| Upstream error | manifest／hash／schema／讀取／來源中途改變或 callback 錯誤停止流程，保留原件與未知暫存，不發布半成品 | T08＋06／07 |
| Repeated run | 相同輸入、predicate、converter 與 limits 產生相同 mapping／sort／report hash；不同 batch 分段不改 index | T08 |
| Concurrent output | 同一 destination 的第二次發布失敗或使用明確 lock；不得覆寫已存在的完成成果 | 後續 Store ticket；本票先保留決策 |
| Boundary size | 大於 `2^53` 的 fixture ID、接近 `int64` 邊界、pair duplicate、self-loop、isolated node、累加／row bytes 溢位都返回可判讀結果 | T08 |

## 測試 seam 與責任

最高測試 seam 是公開 `Build(context.Context, BuildRequest)` 加實際 `feather.Scan`。
測試用小型 Feather fixture、實際暫存目錄和刻意小的 limits 強制走多 run；不針對
私有排序函式寫鏡像測試。來源錯誤使用 corrupt／改 hash／取消的 fixture，不靠不可
重現的程序 kill。

| 測試層級 | 必須驗證 | owner |
| --- | --- | --- |
| Unit | manifest／schema mapping、signed ID 邊界與 round-trip、nullable／dictionary／list、canonical predicate、unknown／exclusion 分母、self／isolated／duplicate 定義、禁止 aggregation | T08 |
| Property／fuzz | 不同 batch/run 分段的 deterministic index／sort；排序 merge 與小型獨立 reference 的計數一致；任何輸入錯誤不改 source | T08 |
| Integration | 三份官方-like fixture 經 06／07 路徑產生兩視圖、manifest／source fingerprint、cancel-resume、forced external runs、callback error 和 output immutability | T08 |
| Real-data | 三份 v4 原件完整掃描，保存命令、環境、輸入指紋、row／unknown／selection 報告；151M rows 走 bounded path，不宣稱生物完整性 | root＋資料統計 owner |
| Cross-check | root 審核 `status`／`statusLabel` predicate、weights 到 `bodyId` 的 identity mapping、Glia 11,864 evidence 與所有未知比例的分母 | root |

## 驗收

- [x] [Owner：T08] 已驗證的 `DatasetManifest` 與三份官方原件可進入同一條 Build 流程，
  原始檔前後 fingerprint 一致，錯誤與取消不發布半成品。
- [x] [Owner：T08] `raw_segments` 逐列保留所有 weights row、nullable value、原始欄位
  名稱、batch／row／file role；沒有自行改稱 synapse/contact 或做 aggregation。
- [x] [Owner：root＋T08] `annotated_neurons` 使用 manifest 中明示且已核准的 predicate、
  保存 canonical form／hash；候選 `status == "Traced"` 只標工程選取，不能標官方完整性。
- [x] [Owner：T08] namespace／lossless external ID 與連續 index 可雙向往返，含大於
  浮點精確範圍的 fixture；負值、index／byte／累加溢位明確失敗。
- [x] [Owner：T08] 報告含原始 rows、選入 nodes、排除原因、未知 endpoint、unique pair、
  raw value sum、prediction unknown、receptor `not-derived`、isolated、self-loop、
  duplicate pair／row，並為 ratio 保存分子、分母與 field basis。
- [x] [Owner：T08] duplicate 以 compact sorted edge slices／bounded external sort 的
  相鄰 run 計數，沒有 O(N²) 或全量 pair map；所有 151,856,684 rows 仍可在容量足夠時
  走完整路徑，容量不足只返回錯誤。
- [x] [Owner：T08] 建構與匯出不共享可變切片；相同輸入與 predicate 的 view、index、
  canonical order 和 report hash 可重建。
- [x] [Owner：root] evidence 保存三份原件的命令、環境、fingerprint、實際未知／選取
  統計與限制；不得用目前 row counts 代替尚未完成的 field-level evidence。
- [x] [Owner：root] 決定本票是否同時包含 durable GraphStore／CLI；若否，下一張票明定
  byte format、原子發布、讀回校驗，且以本票的 canonical stream／report 作為輸入。

## 驗證證據

- SDK：`connectome`、`internal/extsort` 的單元、property 與整合測試（公開 `Build` 加實際 `feather.Scan`、小型 fixture、強制多 run、取消、callback 錯誤、指紋改變、容量、聚合證據、不共享切片），日誌見 [sdk-test.log](../../evidence/connectome-20260913/sdk-test.log)、[sdk-race.log](../../evidence/connectome-20260913/sdk-race.log)、[sdk-vet.log](../../evidence/connectome-20260913/sdk-vet.log)，來源指紋見 [source.sha256](../../evidence/connectome-20260913/source.sha256)。Red 狀態：實作前 `go test ./connectome/` 因公開型別不存在而編譯失敗；extsort 由 subagent 先寫失敗測試再實作。
- CLI：`data import` 以相對路徑 manifest 建立報告，失敗案例與 help 皆有測試（`internal/cli/import_test.go`）。
- 真實資料：三份官方原件一次建構，410 秒、最高 RSS 3.40 GB、7 個 run、4.1 GB 暫存、結束後暫存為空；所有與獨立稽核共有的計數相等，raw 零重複 pair、annotated 25,563,197 邊、535 孤立、101 自環。以 `scripts/graph-evidence.sh` 重跑；命令、環境、指紋、報告與交叉核對見 [graph-v1](../../evidence/malecns-source-20260913/graph-v1/verification.json)；需求層級的紀錄見 [DAT-03](../../evidence/DAT-03/verification.json)、[DAT-04](../../evidence/DAT-04/verification.json)、[DAT-05](../../evidence/DAT-05/verification.json)、[DAT-08](../../evidence/DAT-08/verification.json)。
- 順手修正：`feather.validateFixedWidth` 對零長度子陣列的 nil buffer 誤判為錯誤（空 list 欄位批次會被拒絕），已加回歸測試 `TestScanAcceptsListColumnWithNoChildValues`。
- 對抗性審查（Claude subagent，只讀）後修正：manifest JSON 巢狀深度上限 64 且路徑只在錯誤時組字串（原本無上限且二次方配置）；未映射的 consensus／receptorType 欄位報 `not_mapped` 與未定義比例，不再算成 100% 未知；Arrow 字串值改為複製，避免保留整個批次緩衝；報告同時給 `reserved_peak_bytes`（帳面保留峰值，`sort_buffer_bytes` 為 0 時依設計等於上限）與 `buffered_peak_bytes`（實際持有高水位）；合併出的索引與列號做範圍檢查；`predicate_field_null` 明示為 `predicate_false` 的子集。

## 未納入本票

durable GraphStore 位元組格式、原子發布與讀回校驗（09）；把 `Graph` 接上 `dynamics`／`learning` 建立真實子圖訓練（DAT-07）；FlyWire adapter（DAT-06）；Ubuntu／Windows 的真實資料重跑。

## 分工與未決決策

- 06 擁有下載、來源回條、暫存與原子發布；07 擁有 Feather parser、逐批生命週期與
  schema／容量錯誤。本票只消費其已驗證契約，不複製 downloader 或 reader。
- 本票擁有 MaleCNS raw／annotated view、ID catalog、選取報告、bounded sort、未知／
  duplicate／isolated／self 統計與 fixture／real-data acceptance。
- root 已於「Root 決策」裁決 predicate 欄位、identity evidence、ratio 分母、容量
  驗收值與 durable GraphStore／CLI 的切分。
- FlyWire adapter、syn-points／ROI、細胞化學受體與作用符號、完整媒體重取樣、訓練／
  GPU 和生物完整性聲明不屬本票。未知不會被補成零、正負或空字串。

## 依據

- [主規格 5.1–5.5](../handoff/CoImNet_Implementation_Plan.zh-TW.md#chapter-05)：
  來源、只讀原件、具名視圖、無損 ID、聚合限制、manifest 與標準欄位。
- [主規格 7.1](../handoff/CoImNet_Implementation_Plan.zh-TW.md#chapter-07)：解剖資料
  與基礎參數／個體狀態／訓練器狀態分離；本票只產生解剖資料衍生物。
- [共用工程設計](../../ENG.md)：固定順序、O(N+E) 稀疏原則、錯誤不可留半次更新、
  原始資料與衍生物分離。
- [07 Feather ticket](07-feather-reader.md)：已驗證的逐批 reader、null／dictionary／
  list 生命週期及官方 row counts。
- [MaleCNS source audit](../malecns-source-audit.md)：官方檔案用途、schema、fingerprint、
  nullable signed IDs，以及 weights 沒有 ROI／duplicate aggregation evidence 的限制。
