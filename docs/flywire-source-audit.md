# FlyWire 公開釋出資料欄位稽核文件（DAT-06 adapter）

本文件記錄 FlyWire 公開釋出（public release，Codex 下載）的三張表在官方釋出的欄位名與型別，
以及本專案 FlyWire adapter（`connectome/flywire`）對應寫入的 Feather 欄位。

> 欄位名依 Codex 公開下載頁的說明整理，尚未對照實際下載檔驗證。FlyWire 公開資料需要帳號與
> 條款同意才能下載，本票的真實資料狀態為 `blocked_data`，目前只以 fixture 驗證 adapter 流程。
> 取得日期：未取得（blocked_data，需要使用者提供下載檔與帳號條款同意）。
> 條款與下載頁：<https://codex.flywire.ai/app/graph/flywire_public_release>。

整理日期：2026-09-19（UTC）。本頁只記對 adapter 有意義的欄位，不保證是官方檔案的全部欄位。

## connections 表（權重／連線）

官方釋出：每個（pre, post, neuropil）組合一列，同一個 pair 在不同 neuropil 的連線分開列。

| 官方欄位 | 官方型別 | adapter 對應 |
| --- | --- | --- |
| `pre_root_id` | 整數（root id） | `weights.feather.pre_root_id`（int64，`FieldMapping.Weights.Source`） |
| `post_root_id` | 整數（root id） | `weights.feather.post_root_id`（int64，`FieldMapping.Weights.Target`） |
| `neuropil` | 字串 | `weights.feather.neuropil`（string） |
| `syn_count` | 整數（突觸數） | `weights.feather.syn_count`（int64，`FieldMapping.Weights.Value`） |
| `nt_type` | 字串（傳導物質預測） | 未使用（本表不寫傳導物質） |

同一個（pre, post）pair 若分散在多個 neuropil，各自保持一列，`DuplicateSemantics`
宣告為 `additive_partitions`。

## classification 表（節點註記）

官方釋出：每個 root_id 一列的解剖階層註記。

| 官方欄位 | 官方型別 | adapter 對應 |
| --- | --- | --- |
| `root_id` | 整數（root id） | `annotations.feather.root_id`（int64，`FieldMapping.Annotations.ID`） |
| `flow` | 字串 | `annotations.feather.flow`（string，未映射到 manifest，僅寫檔） |
| `super_class` | 字串 | `annotations.feather.super_class`（`FieldMapping.Annotations.Superclass`） |
| `class` | 字串 | `annotations.feather.class`（`FieldMapping.Annotations.Class`） |
| `sub_class` | 字串 | `annotations.feather.sub_class`（`FieldMapping.Annotations.Subclass`） |
| `cell_type` | 字串 | `annotations.feather.cell_type`（`FieldMapping.Annotations.Type`） |
| `side` | 字串（soma 側） | `annotations.feather.side`（`FieldMapping.Annotations.SomaSide`） |
| `hemibrain_type` | 字串 | 未使用 |
| `hemilineage` | 字串 | 未使用 |
| `nerve` | 字串 | 未使用 |

`annotations.feather.included` 是 adapter 加的常數欄，每列固定「released」，代表
「釋出表本身就是選取集」（見 manifests 的 `TransformHistory` 與 `Selection`）。

## neurons 表（傳導物質預測）

官方釋出：每個 root_id 一列的傳導物質預測與信心分數；本 adapter 忽略 `group` 與其他欄位。

| 官方欄位 | 官方型別 | adapter 對應 |
| --- | --- | --- |
| `root_id` | 整數（root id） | `nt.feather.root_id`（int64，`FieldMapping.Neurotransmitters.ID`） |
| `nt_type` | 字串 | `nt.feather.nt_type`（string，`FieldMapping.Neurotransmitters.Predicted`） |
| `nt_type_score` | 浮點數 | `nt.feather.nt_type_score`（float64，`FieldMapping.Neurotransmitters.Confidence`） |
| `group` | 字串 | 未使用（「其他欄位忽略」） |

## 一般處理規則

- root id 一律以 uint64 解析、寫入 int64 Feather 欄位；超過 `math.MaxInt64` 回報
  「root id %s overflows int64」，不做靜默截斷。
- 空字串來源寫成 null（可空欄位）；`nt_type_score` 空字串寫成 null。
- 遺漏 adapter 需要使用的欄位時，錯誤同時指明欄位名與表名。
- 三個 Feather 檔以 250,000 列為一批寫出（沿用 cancel fixture 的批次寫法），讓
  `connectome.Build` 以有限記憶體掃描。

## 限制

1. 上述型別與欄位清單以 Codex 公開下載頁的描述整理，尚未對照實際下載檔逐欄驗證；`blocked_data`
   解除後須用真實檔案重新稽核並逐批讀取驗證。
2. FlyWire 的 `side`／`nerve` 與釋出版本（version 783 等）等語意，以官方說明為準，本 adapter
   只做欄位對應，不新增生物機制假設。
3. 受體表現不在 FlyWire 公開釋出內，本 adapter 不處理。