# MaleCNS v1.0 資料來源與 Feather 匯入查核

查核日期為 2026-09-13（UTC）。先以官方頁面、HTTP 標頭與有限範圍確認格式，
再下載三份完整原件並比對官方 CRC32C。取得紀錄見
[annotations](../evidence/malecns-source-20260913/annotations-download.json)、
[neurotransmitters](../evidence/malecns-source-20260913/neurotransmitters-download.json) 與
[weights](../evidence/malecns-source-20260913/weights-download.json)。正式讀取器使用 Go。

## 官方來源與檔案標頭

主來源是 [MaleCNS 官方專案頁](https://male-cns.janelia.org/)（S01）與
[官方下載頁](https://male-cns.janelia.org/download/)（S02）。下載頁列出的
flat-connectome 根目錄為
`gs://flyem-male-cns/v1.0/connectome-data/flat-connectome/`；以下使用其
HTTPS 對應網址。三個檔案的 HEAD 都是 HTTP 200、`Content-Type:
application/octet-stream`、`Accept-Ranges: bytes`。

| 檔案 | 大小（bytes） | Last-Modified | ETag | `x-goog-hash` CRC32c |
|---|---:|---|---|---|
| [connectome-weights-male-cns-v1.0-minconf-0.5.feather](https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/connectome-weights-male-cns-v1.0-minconf-0.5.feather) | 1,051,241,946 | Wed, 03 Jun 2026 13:54:47 GMT | `f30e9dcca25cfd021bf1e7b3d975599e` | `dKRPVQ==` |
| [body-annotations-male-cns-v1.0-minconf-0.5.feather](https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/body-annotations-male-cns-v1.0-minconf-0.5.feather) | 14,483,314 | Wed, 03 Jun 2026 13:54:38 GMT | `50a7718770c57220f160ba4f431ab89e` | `vjz9cg==` |
| [body-neurotransmitters-male-cns-v1.0.feather](https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/body-neurotransmitters-male-cns-v1.0.feather) | 43,282,834 | Mon, 08 Jun 2026 05:01:39 GMT | `3d842b12fe5c49eefade528d7dd24a1f` | `jcpNFg==` |

HEAD 回應的 `Date` 分別為 2026-09-13 08:24:01Z、08:24:04Z、08:24:07Z。
官方下載頁將 annotations 說明為整理過的 classes/types/sides 等神經元註記，
將 neurotransmitters 說明為每個神經元的 aggregate neurotransmitter predictions，
將 weights 說明為沒有突觸的 segment 排除後之 segment-to-segment connection
strengths 與 full connection graph。頁面明示 Male CNS dataset 使用
[CC-BY 4.0](https://creativecommons.org/licenses/by/4.0/)；匯入器應保留此來源
與授權 attribution。

## 有限 Range 的 Arrow IPC schema probe

隔離 probe 位於 `/private/tmp/coimnet-malecns-probe-20260913-01`，只使用
`github.com/apache/arrow/go/v17 v17.0.0` 的 `arrow/ipc.NewFileReader`、
`FileReader.Schema`、`NumRecords` 與 `NumDictionaries`。自訂 `io.ReaderAt` 以
HTTP Range 讀取檔案開頭 6 bytes、結尾 10 bytes、footer 及必要的 dictionary
metadata/body，並驗證每個 `Content-Range`。probe 輸出在
[`evidence/malecns-source-20260913/feather-schema.json`](../evidence/malecns-source-20260913/feather-schema.json)。

總取回量是 67,302 bytes，低於 5 MiB 上限：weights 56,634 bytes、annotations
7,522 bytes、neurotransmitters 3,146 bytes。三個檔案的前後 magic 都是
`ARROW1`，Arrow IPC metadata version 都是 `V5`，表示實際內容可由 Arrow IPC
file reader 解析；官方下載頁同時將它們標為 Apache Arrow Feather。

| 檔案 | footer bytes | record batches | dictionaries | schema |
|---|---:|---:|---:|---|
| weights | 56,608 | 2,318 | 0 | available |
| annotations | 6,856 | 4 | 1 | available |
| neurotransmitters | 3,120 | 29 | 0 | available |

### `connectome-weights...feather`

| 欄位 | Arrow type | nullable |
|---|---|---|
| `body_pre` | `int64` | true |
| `body_post` | `int64` | true |
| `weight` | `int64` | true |

### `body-annotations...feather`

所有 36 個 top-level 欄位都為 nullable。`statusLabel` 是唯一 dictionary
欄位，index 是 `int8`、value 是 `utf8`、`ordered=true`。

| 欄位 | Arrow type |
|---|---|
| `assignedOlHex1` | `float64` |
| `assignedOlHex2` | `float64` |
| `bodyId` | `int64` |
| `flywireType` | `utf8` |
| `group` | `float64` |
| `instance` | `utf8` |
| `somaSide` | `utf8` |
| `statusLabel` | `dictionary<values=utf8, indices=int8, ordered=true>` |
| `superclass` | `utf8` |
| `type` | `utf8` |
| `vfbId` | `utf8` |
| `hemibrainType` | `utf8` |
| `itoleeHl` | `utf8` |
| `supertype` | `utf8` |
| `birthtime` | `utf8` |
| `mancBodyid` | `float64` |
| `mancGroup` | `float64` |
| `mancType` | `utf8` |
| `subclass` | `utf8` |
| `synonyms` | `utf8` |
| `class` | `utf8` |
| `rootSide` | `utf8` |
| `somaNeuromere` | `utf8` |
| `trumanHl` | `utf8` |
| `dimorphism` | `utf8` |
| `matchingNotes` | `utf8` |
| `entryNerve` | `utf8` |
| `mancSerial` | `float64` |
| `mcnsSerial` | `float64` |
| `serialMotif` | `utf8` |
| `fruDsx` | `utf8` |
| `exitNerve` | `utf8` |
| `receptorType` | `utf8` |
| `somaLocation` | `list<item: int64, nullable>` |
| `tosomaLocation` | `list<item: int64, nullable>` |
| `status` | `utf8` |

### `body-neurotransmitters...feather`

所有 10 個 top-level 欄位都為 nullable。

| 欄位 | Arrow type |
|---|---|
| `body` | `int64` |
| `cell_type` | `utf8` |
| `total_nt_predictions` | `int32` |
| `predicted_nt_confidence` | `float64` |
| `predicted_nt` | `utf8` |
| `ground_truth` | `utf8` |
| `celltype_total_nt_predictions` | `int32` |
| `celltype_predicted_nt` | `utf8` |
| `celltype_predicted_nt_confidence` | `float64` |
| `consensus_nt` | `utf8` |

上述有限 Range probe 沒有讀取 record batch 的資料值。`body_pre`、`body_post`、`bodyId`、
`body` 實際 schema 是 signed `int64`，不是已驗證的 `uint64`；adapter 必須先
檢查非負與範圍，再轉成內部 `uint64` 或保留無損十進位字串，不能無聲截成 32 位元。
所有 nullable 欄位與 dictionary decode 都必須在標準化前保留其未知狀態。

## annotations 實際值查核

另以 `/private/tmp/coimnet-malecns-values-20260913-01` 的獨立 Go probe，使用
`github.com/apache/arrow/go/v17 v17.0.0` `ipc.NewFileReader` 讀取本機完整
annotations 檔案的全部 4 個 record batches。檔案大小為 14,483,314 bytes，
SHA-256 為 `2177e246113e4cfbf1e7772ec37c6da1955ff22e8063d0b1f833101f99a9a3b2`，
probe 執行時間為 2026-09-13T08:56:26Z，完整輸出見
[`annotation-values.json`](../evidence/malecns-source-20260913/annotation-values.json)。

批次列數為 `[65536, 65536, 65536, 14969]`，合計 211,577 rows。`bodyId` 沒有
null、負值或重複，非 null unique 數為 211,577，最大值為 1,571,825,087。
`statusLabel` 為 887 null／210,690 non-null，`status` 為 5,472／206,105，
`class` 為 185,064／26,513，`type` 為 47,071／164,506，`type` 的 non-null
unique 總數為 11,751；可列舉的欄位完整 value counts 保存在 evidence JSON。

這些值只描述來源資料，不足以定義 `annotated_neurons` 篩選規則；本查核沒有依
status、class、type 或任何其他欄位推導選取條件，也沒有將 signed `int64` ID 轉成
其他型別。

## 邊、ROI 與重複語意

weights 的實際 schema 只有 `body_pre`、`body_post`、`weight`，沒有 ROI、突觸
位置、區域分段或 total-vs-partition 標記。官方下載頁只證實它是排除無突觸
segments 後的 segment-to-segment strengths/full graph，沒有證實相同端點的多筆
紀錄是否可相加，也沒有證實 ROI 聚合規則。因此目前應記錄：

- `roi_semantics`: unknown（weights 檔沒有 ROI 欄位）。
- `duplicate_edge_semantics`: unknown（不可依端點自行去重或加總）。
- `aggregation`: not permitted until source evidence identifies regional partitions
  and distinguishes them from an already aggregated total.

下載頁對另一個 `syn-points` 檔案描述 pre/post rows 與 encompassing ROIs，這不足以
推導 weights 的重複或 ROI 語意；本次沒有下載或解析該 12.7 GB 檔案。

## 標準化圖建構實測

2026-09-13 以 [manifests/malecns-v1.0.json](../manifests/malecns-v1.0.json) 執行
`coimnet data import`，完整報告與命令見
[graph-v1](../evidence/malecns-source-20260913/graph-v1/verification.json)。
建構前後三份原件的 SHA-256 不變，與上述獨立稽核程式共有的每個計數皆相等。
新觀察如下，數字都是來源列的統計，不是生物語意：

- weights 151,856,684 列中沒有 null 或負的端點與 `weight`，也沒有 0 值；
  依 `(body_pre, body_post)` 排序後**沒有任何重複 pair**，所以本版本不需要
  聚合。`duplicate_edge_semantics` 仍記為 unknown，因為零重複不能說明重複
  出現時該如何解讀。
- 端點 unique 88,384,522 個，其中 88,192,826 個沒有 annotation 列，對應
  133,806,112 個端點出現次數。
- `annotations.status == "Traced"` 選入 165,122 個 body（211,577 列中 46,455 列
  predicate 為 false，含 5,472 列 `status` 為 null）。兩端都選入的邊 25,563,197
  條，`weight` 加總 124,025,046；自環 101 條，孤立選入節點 535 個；因單端無
  annotation 排除 125,828,298 列，因單端未選入排除 465,189 列。
- 選入節點中 3,602 個沒有可用的 `consensus_nt` 預測（null／unclear／無列），
  比例 2.18%；`receptorType` 為 null 的有 164,370 個（99.5%），受體證據本身
  維持 `not_derived`。

## Insyra v0.3.2 與 upstream 查核

指定模組 `/Users/timlai/go/pkg/mod/github.com/!hazelnut!paradise/insyra@v0.3.2`
的 `go.mod:139` 直接依賴 Arrow Go v17.0.0，但 `parquet/api.go:54-128` 的
`Inspect(path string)` 與 `parquet/api.go:168-243` 的
`Read(ctx context.Context, path string, opt ReadOptions)` 都只建立 Parquet
reader。codebase-memory project `coimnet-insyra-v032` 對 `*.go` 搜尋 `feather`
得到 `total_grep_matches=0`；搜尋 Arrow v17 只找到 `arrow` 與 `parquet` 匯入，沒有
Insyra Feather wrapper 或 `arrow/ipc` reader。

GitHub `main` 在 2026-09-13 由 `git ls-remote` 仍是
`1f1cdb949c51a10be104965cf4cf30f8f0aaa648`，與 v0.3.2 相同。該 commit 的
recursive tree 有 `parquet/` 與 `py/internal/ipc/`，沒有 `feather` 或
`arrow/ipc` 封裝。GitHub issue search `repo:HazelnutParadise/insyra feather`
與 `repo:HazelnutParadise/insyra Arrow IPC` 都回傳 0 筆；目前沒有發現重複的
Feather issue。現有 [#375](https://github.com/HazelnutParadise/insyra/issues/375)
是 custom tape/external VJP，[#376](https://github.com/HazelnutParadise/insyra/issues/376)
是 optimizer state 與 tape graph reset，兩者都不是 Feather 讀取 API；鄰近的
Parquet API issues 也不提供 Feather API。

## 2026-09-14 新增原件（參數 adapter 用）

四份檔案已於 2026-09-14 以 `coimnet data download` 下載並完成逐批讀取驗證。

### 下載回條

| 檔案 | bytes | ETag | 上游 CRC32C | SHA-256 | hash_status |
|---|---:|---|---|---|---|
| `body-stats-male-cns-v1.0-minconf-0.5.feather` | 778,062,826 | `404c3349c28580148e16815eb99f382a` | `MGOCPQ==` | `ca5dc83a26382ae70c8d8f42fc09ce2dbc1af7c03f3a001a1936b5e142540647` | `upstream_verified` |
| `tbar-neurotransmitters-male-cns-v1.0.feather` | 2,651,680,218 | `51b02c11690662aedef28f86d394ff0d` | `RrR5/g==` | `bade84c9eab431dd537ff644aaf3d203d639a819c739ecedb338e7d109064f4d` | `upstream_verified` |
| `syn-partners-male-cns-v1.0-minconf-0.5.feather` | 6,777,179,098 | `58efcf712f8c4d4de5f2ad51e97def76` | `jTlNIA==` | `959d8ef4173b35382a3e6acfaf5167c795b6d10b877572d146af04e1b487bc07` | `upstream_verified` |
| `Neuprint_Meta.csv` | 1,247,784 | `ee9e55000a7e81813c959a88cbc47886` | `qW0Qsg==` | `227ff1e873881a1ab124b10aa51ada94012773ddb9c4ad986e5cc83df8d360f7` | `upstream_verified` |

### 逐批掃描結果

| 檔案 | 列數 | 批數 | 欄數 | complete |
|---|---:|---:|---:|---|
| `body-stats-male-cns-v1.0-minconf-0.5.feather` | 88,384,522 | 1,349 | 11 | `true` |
| `tbar-neurotransmitters-male-cns-v1.0.feather` | 45,656,140 | 697 | 17 | `true` |
| `syn-partners-male-cns-v1.0-minconf-0.5.feather` | 311,833,243 | 4,759 | 11 | `true` |

### 欄位與型別

`body-stats`：`body int64`、`pre int32`、`post int32`、
`status_fine dictionary<values=utf8, indices=int8, ordered=true>`、`superclass utf8`、
`class utf8`、`type utf8`、`instance utf8`、`downstream int64`、`synweight int64`、
`rank int64`。

`tbar-neurotransmitters`：`point_id uint64`、`x int32`、`y int32`、`z int32`、
`conf float32`、`sv int64`、`body int64`、
`major dictionary<values=utf8, indices=int8, ordered=true>`、
`primary dictionary<values=utf8, indices=int16, ordered=true>`、
`nt_acetylcholine_prob float32`、`nt_dopamine_prob float32`、
`nt_gaba_prob float32`、`nt_glutamate_prob float32`、
`nt_histamine_prob float32`、`nt_octopamine_prob float32`、
`nt_serotonin_prob float32`、
`split dictionary<values=utf8, indices=int8, ordered=false>`。

`syn-partners`：`x_pre int32`、`y_pre int32`、`z_pre int32`、`body_pre int64`、
`conf_pre float32`、`x_post int32`、`y_post int32`、`z_post int32`、
`body_post int64`、`conf_post float32`、
`primary_post dictionary<values=utf8, indices=int16, ordered=true>`。

`Neuprint_Meta.csv`：本票只讀 `roiHierarchy`、`roiInfo` 與門檻欄位
（`postHighAccuracyThreshold`、`preHPThreshold`、`postHPThreshold`）。

### 交叉核對

- `body-stats` 88,384,522 列 = `connectome-weights` 的 unique endpoint 數（票 13 核對）。
- `tbar-neurotransmitters` 45,656,140 列 = `Neuprint_Meta.csv` 的 `totalPreCount`（票 13 核對）。
- `syn-partners` 311,833,243 列 = `Neuprint_Meta.csv` 的 `totalPostCount` = `connectome-weights` 的 `weight` 加總 311,833,243（票 13 核對）。

### 下載命令

```
coimnet data download --url <url> --out <path> --max-bytes <size> [--timeout <d>]
```

`--max-bytes` 設為官方檔案大小；syn-partners 因 6.78 GB 另設 `--timeout 90m`。

回條位於 `evidence/malecns-source-20260914/*-download.json`，逐批掃描報告位於 `*-inspect.json`。

## 建議與未決事項

Arrow Go v17 公開 `ipc.NewFileReader` 已能讀取這些檔案的 footer/schema，
CoImNet 的 adapter 可直接使用指定版本。Insyra 缺少封裝的需求已在主 agent
查核後提交 [#377](https://github.com/HazelnutParadise/insyra/issues/377)，
並確認為 OPEN。提報涵蓋有容量限制的批次讀取、schema、nullable 欄位、
dictionary、無損 ID 與錯誤傳遞。這項缺口不阻擋 CoImNet 繼續實作。

來源：

- [S01 MaleCNS 官方專案頁](https://male-cns.janelia.org/)
- [S02 MaleCNS 官方下載頁](https://male-cns.janelia.org/download/)
- [Apache Arrow Feather 文件](https://arrow.apache.org/docs/python/feather.html)
- [Apache Arrow Go v17 IPC 文件](https://pkg.go.dev/github.com/apache/arrow/go/v17/arrow/ipc)
- [Insyra v0.3.2 go.mod](https://github.com/HazelnutParadise/insyra/blob/1f1cdb949c51a10be104965cf4cf30f8f0aaa648/go.mod)
- [Insyra v0.3.2 parquet API](https://github.com/HazelnutParadise/insyra/blob/1f1cdb949c51a10be104965cf4cf30f8f0aaa648/parquet/api.go)
