# MaleCNS v1.0 官方發布檔案盤點

盤點日期 2026-09-14（UTC）。方法：官方專案頁與下載頁（S01、S02）、GCS bucket JSON 列表
（`https://storage.googleapis.com/storage/v1/b/flyem-male-cns/o?prefix=v1.0/`）、HEAD 取得
`Content-Length`、大檔只以 Range 讀 Arrow footer 取 schema，總下載約 16.1 MB。整份資料集
授權為下載頁所述 CC-BY（4.0）。本頁只記事實，各檔案要不要納入框架見各 ticket。

## 已確認：下載頁列出且大小、格式已核對

flat-connectome 前綴：`https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/`

| 檔案 | bytes | 格式 | 官方描述（節錄） | 帶的生物量 | 目前狀態 |
| --- | ---: | --- | --- | --- | --- |
| `body-annotations-male-cns-v1.0-minconf-0.5.feather` | 14,483,314 | Feather，36 欄 | curated neuron annotations, excluding neurotransmitter properties | 細胞類型階層（superclass／class／subclass／type／supertype／instance）、soma 側與位置、hemilineage、跨資料集對應 | 已納入（ticket 06–08） |
| `body-neurotransmitters-male-cns-v1.0.feather` | 43,282,834 | Feather，10 欄 | aggregate neurotransmitter predictions for each neuron | 每個 body 與每個細胞類型的傳導物質預測、信心度、`consensus_nt` | 已納入 |
| `connectome-weights-male-cns-v1.0-minconf-0.5.feather` | 1,051,241,946 | Feather，3 欄 | segment-to-segment connection strengths; the full connection graph | `body_pre`、`body_post`、`weight` | 已納入 |
| `body-stats-male-cns-v1.0-minconf-0.5.feather` | 778,062,826 | Feather，11 欄（Range 讀 schema） | summary statistics (synapse counts) of all segments | 每個 body 的 `pre`、`post`、`downstream`、`synweight`、`rank`；沒有 ROI 拆分 | 未納入 |
| `syn-partners-male-cns-v1.0-minconf-0.5.feather` | 6,777,179,098 | Feather，11 欄 | synaptic partner pairs with body IDs and primary neuropil | 逐突觸的 pre／post 座標、body、`conf_pre`／`conf_post`、`primary_post` ROI | 未納入 |
| `tbar-neurotransmitters-male-cns-v1.0.feather` | 2,651,680,218 | Feather，17 欄 | neurotransmitter prediction probabilities for each pre-synapse | 逐突觸前位置的七種傳導物質機率（acetylcholine、dopamine、gaba、glutamate、histamine、octopamine、serotonin）、座標、`conf`、`body`、ROI | 未納入 |
| `syn-points-male-cns-v1.0-minconf-0.5.feather` | 13,061,489,098 | Feather，41 欄 | pre- and post-synapse locations, body ID, encompassing ROIs | x／y／z（8 nm voxel）、`kind`、`conf`、`body`、`compartment`、四層 ROI、視葉 column／layer | 未納入（稽核時已知） |

體積、形態與資料庫（下載頁有列，目錄樹未加總大小）：

| 路徑 | 格式 | 官方描述（節錄） | 帶的量 |
| --- | --- | --- | --- |
| `gs://flyem_cns_z0720_07m_dvidcoords_n5`、`…/em/em-clahe-jpeg` | N5、precomputed | aligned EM volume | 原始 EM 影像 |
| `…/v1.0/segmentation`（含 meshes、multi-res／single-res meshes） | precomputed | proofread neuron segmentation | 分割標籤、網格 |
| `…/v1.0/segmentation/skeletons-malecns/skeletons-{swc,precomputed}/`、`-mirrored`、`skeletons-unisex-template` | SWC／precomputed | neuron skeletons in SWC format | 骨架（可量路徑長度） |
| `…/v1.0/malecns-v1.0-nuclei-seg-16nm` | precomputed uint64 | nuclear segmentation, not filtered | 細胞核 |
| `…/rois/fullbrain-roi-v4`、`…/rois/malecns-vnc-neuropil-roi-v0` | precomputed uint64 | neuropil compartment segmentation | ROI 遮罩（腦 ≤96、VNC ≤27） |
| `…/v1.0/database/neo4j/` | neo4j 4.4.16 dump | complete neo4j database backing neuprint | 完整 neuprint 圖 |
| `…/v1.0/database/neuprint-inputs/`（18 檔） | CSV／Feather | input CSV files used to construct the neo4j database | 見下 |

`neuprint-inputs/Neuprint_Meta.csv`（1,247,784 bytes，已整份讀取）：50 個 neuron property、
五層 `roiHierarchy`（CNS → CentralBrain／Optic(L)／Optic(R)／CV／VNC → …）、`roiInfo` 內
5,619 個 ROI 的 pre／post 突觸數；全集 `totalPreCount=45,656,140`、
`totalPostCount=311,833,243`（與我們 weights 加總 311,833,243 相同）；門檻
`postHighAccuracyThreshold=0.5`、`preHPThreshold=0.0`、`postHPThreshold=0.7`。同目錄大檔：
`Neuprint_SynapseSet.csv` 26.5 GB、`Neuprint_SynapseSet_to_Synapses.csv` 24.0 GB、
`Neuprint_Neuron_Connections.csv` 16.9 GB（另有 3.5 GB Feather）、
`Neuprint_Synapse_Connections.csv` 11.7 GB、`Neuprint_Neurons.feather` 4.6 GB、
`roi_elements.feather` 3.7 GB、`body_elements.feather` 704 MB。

## 已確認存在但下載頁未描述（用途為推論）

| 路徑 | 大小／數量 | 依據 |
| --- | --- | --- |
| `…/flat-connectome/connectome-weights-…-{traced,significant}-only.feather` | 508,025,642／502,169,298 | HEAD 200；名稱暗示預先過濾，過濾定義未公布 |
| `…/flat-connectome/syn-partners-…-{traced,significant}-only.feather` | 2,965,367,002／2,965,702,122 | 列表 |
| `…/v1.0/nblasts/`（14 檔＋README） | 最大 21.3 GB | README：full NBLAST matrix；形態相似度，非生理 |
| `…/v1.0/synapse-ground-truth/` | 3 CSV（12.6／62.9／712.8 KB）＋2 目錄 | 人工標註突觸 ground truth |
| `…/v1.0/malecns-v1.0-soma-points/`、`…-optic-lobe-column-pins/` | precomputed annotation | soma 點雲、視葉 column pins |
| `…/v1.0/supervoxels/`、`…/male-cns-meshes-transformed-to-fafb-flywire/`、`…/v1.0/database/dvid-exports/` | 目錄 | 列表 |
| `…/v1.0/male-cns-v1.0.json`、`male-cns-v1.0-clio.json` | 60,253／58,058 | Neuroglancer 場景 |
| 根目錄 `malecns-semantic-masks/`、`malecns-tbar-point-cloud-512nm-smoothed/`、`micro-ct/`、`auxiliary-data/retinotopy-tbars/`、`dsx-clones/`、`fruitless-clones/`、`banc2mcns_meshes/`、`flywire2mcns_meshes/`、`hemibrain2mcns_meshes/`、`manc2mcns_meshes/`、`versions-special/`、`v0.9/`、`v0.11/`、`v0.13/` | 目錄 | `README_RELEASE_BUCKET.md` 描述，但該檔仍寫 v0.9 |

## 受體：發布內容沒有受體表現資料

整個 v1.0 發布沒有每個細胞或每個細胞類型的神經傳導物質受體表現。唯一名稱像受體的欄位是
annotations 的 `receptorType`：211,577 列中 210,825 為 null，非 null 只有三個值
`putative_ppk23`（269）、`putative_ppk25`（257）、`putative_IR52b`（226），合計 752 列
（0.36%）。這三個是標記特定費洛蒙／味覺感覺神經元族群的感覺受體基因，不是突觸後傳導物質
受體；`Neuprint_Meta.csv` 也只把它列為一般字串 property，沒有控制詞彙。metadata 另有
`subclass` 值 `strand receptor`（機械感覺器官名稱）。下載頁與發布頁完全沒有 receptor、
transcriptom、single-cell、delay 等字。

受體表現要來自另一類資料集：成蟲腦與 VNC 的單細胞／單核 RNA-seq 圖譜、以 knock-in／GAL4
建立的傳導物質與受體基因 chemoconnectome 資源（專案 S16 已引用 Deng 等 2019）、原位表現庫。
接進來都需要另外的細胞類型名稱對應，發布內沒有提供；本次沒有核對這些外部資料的 URL。

## 發布內可當生物參數的量（與可對應的框架機制）

| 量 | 來源檔 | 可對應的框架機制 |
| --- | --- | --- |
| 每條邊的傳導物質機率分布 | `tbar-neurotransmitters`（逐突觸前）＋`syn-partners`（突觸→pre／post body） | 邊的正負號與信心度（比目前「每個神經元一個預測」細） |
| 每個 body 的突觸總數、下游數、`synweight`、`rank` | `body-stats` | 權重正規化、選取門檻 |
| 每條邊落在哪些 ROI、各 ROI 的突觸數 | `syn-partners`（`primary_post`）、`syn-points`、`Neuprint_Meta.csv` `roiInfo` | 區域分割的加總語意、評估用分區 |
| 突觸座標 | `syn-points`、`syn-partners` | 距離代理；真正的傳導延遲需由骨架量路徑長度 |
| 細胞類型階層與 ROI 階層 | annotations、`Neuprint_Meta.csv` | 按類型共享參數、感覺／運動神經元指定 |
| 沒有的：傳導延遲純量、軸突長度純量、受體表現 | — | 延遲需自骨架推導；受體需外部資料 |

## 未決事項

- `-traced-only`／`-significant-only` 的過濾定義未公布，不可推測。
- 根目錄 `README_RELEASE_BUCKET.md` 仍以 v0.9 為現行版本，連結指向 v0.9；以下載頁為準。
- `body-stats` 無 ROI 欄；每個神經元的 ROI 突觸數只在 `Neuprint_Neurons.feather`（4.6 GB）
  或 neo4j dump 內，本次未讀。
- `body-annotations` 與 `body-stats` 的狀態欄位名稱不同（`statusLabel`／`status` 對
  `status_fine`），關係未核對。
- 傳導物質預測的模型、類別集與校準在 manuscript methods，本次未找到原文。
- `receptorType` 三個 `putative_*` 值的判定依據發布內沒有說明。

## 實際取用的 URL

GCS JSON 列表（根、`v1.0/`、`connectome-data/`、`database/{,neuprint-inputs,neo4j}/`、
`segmentation/`、`synapse-ground-truth/`、`nblasts/`、soma-points、column-pins、
`supervoxels/`、meshes-transformed、`auxiliary-data/`）皆 200；
`README_RELEASE_BUCKET.md`（10,954 B）、`nblasts/README`（2,824 B）、`Neuprint_Meta.csv`
（1,247,784 B）、`male-cns-v1.0.json`（60,253 B）、`body-annotations`（整檔 14,483,314 B，
SHA-256 與既有稽核相同）皆 200；六個大檔 HEAD 200；四個大檔 Range 讀 footer 206；
`male-cns.janelia.org/`、`/download/`、`/release/`、`/sitemap.xml` 皆 200。
