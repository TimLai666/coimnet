# CoImNet 文件索引（單一入口）

此檔是專案文件的單一入口，列出手動維護的文件、自動產生的狀態檔與既有目錄，方便接手者從任何一個需求或疑慮找到對應來源。副本與主文不一致時以主文為準；主文是 [docs/handoff/CoImNet_Implementation_Plan.zh-TW.md](handoff/CoImNet_Implementation_Plan.zh-TW.md)。

## 專案概覽

- [README](../README.md)：框架定位、能力狀態表、科學界線、SDK 與 CLI 用法。
- [工程設計 ENG](../ENG.md)：跨功能的實作決策與共用契約，更動架構或公開契約前先讀。
- [專案工作規則](../AGENTS.md)：開發、驗證與 sub-agent 使用順序。
- [開發進度](../delivery-status.md)：目前階段、阻礙、下一個可驗證成果與決策紀錄。

## 交接文件（docs/handoff/）

| 檔案 | 內容 |
| --- | --- |
| [CoImNet_Implementation_Plan.zh-TW.md](handoff/CoImNet_Implementation_Plan.zh-TW.md) | 主規格：需求、工作包與驗收（另有 [PDF](handoff/CoImNet_Implementation_Plan.zh-TW.pdf)） |
| [requirements.json](handoff/requirements.json) | 85 項必要需求的定義副本 |
| [sources.json](handoff/sources.json) | 研究來源與資料清單 |
| [README.zh-TW.md](handoff/README.zh-TW.md) | 交接包說明 |
| [AGENT_START_HERE.zh-TW.md](handoff/AGENT_START_HERE.zh-TW.md) | 接手起始指南 |
| [SHA256SUMS.txt](handoff/SHA256SUMS.txt)・[VALIDATION.json](handoff/VALIDATION.json) | 交接原件校驗 |

需求狀態不在交接檔改，一律更新 `docs/requirements-status.json`；`docs/handoff/` 內的檔案是原始交接副本，與主文不一致時以主文為準。

## API

`go doc ./...` 可列出全部套件的公開 API。開放的套件路徑：

- `checkpoint`：模型包、個體快照、訓練快照與恢復
- `config`：組態展開
- `connectome`：標準化接線圖與圖儲存
- `download`：官方資料下載與續傳
- `dynamics`：連續、LIF 與混合核心
- `feather`：Feather V2 逐批讀取
- `learning`：核心、外圍模型、訓練器與持續個體
- [`learning/rl`](../learning/rl/README.md)：PPO 更新入口、適用狀態與限制
- [`tasks/asr`](../tasks/asr/README.md)：音訊轉文字、因果串流與恢復
- `modulation`：調節來源、化學、受體與效果
- `params`：動態參數推導與參數集
- `plasticity`：局部學習規則
- `replay`：重播存放策略
- `resources`：記憶體與資源預估
- `signal`：具名訊號、時間對齊、多通道與投影
- `simulate`：原生模擬 runner、空模型與比較矩陣
- `teacher`：教師紀錄與存取
- `experiment`：內建模擬範例（`delayed`、`lif-threshold`、調節與適應性評估）

`internal/` 底下的套件（`cli`、`extsort`、`fileio`、`jsonkey`、`sparse`、`strictjson`）供同一儲存庫內部使用，不保證對外相容。

## 例子

- [experiment/gridnav/README.md](../experiment/gridnav/README.md)：人工走廊的模仿與 PPO 範例、取樣與評估契約。
- [examples/lifthreshold/README.md](../examples/lifthreshold/README.md)：三顆 LIF 神經元的閾值可訓練性檢查。
- [examples/multichannel/README.md](../examples/multichannel/README.md)：多通道 adapter 完整流程。
- [examples/realsubgraph/README.md](../examples/realsubgraph/README.md)：ALIN 真實子圖選取與短訓練。

用 `coimnet examples list` 列出所有內建例子，`coimnet examples run <name>` 執行。命令本身的權限與用法用 `coimnet examples run <name> --help` 查。

## 檔案格式

repo 內 Go 原始碼宣告的 schema 版本字串（`grep -rhoE '"coimnet-[a-z-]+/v[0-9]+"' --include='*.go' . | sort -u`），每個一列並標所在套件：

| Schema 版本 | 套件 |
| --- | --- |
| `coimnet-adaptive-evaluation/v1` | `experiment` |
| `coimnet-config/v1` | `config` |
| `coimnet-connectome-builder/v1` | `connectome` |
| `coimnet-connectome-builder/v999` | `connectome`（測試用版本） |
| `coimnet-continuous-state/v1` | `dynamics` |
| `coimnet-data-sources/v1` | `internal/cli` |
| `coimnet-dataset-manifest/v1` | `connectome` |
| `coimnet-dataset-manifest/v9` | `connectome`（相容讀取） |
| `coimnet-delayed-report/v2` | `experiment` |
| `coimnet-derivation-report/v1` | `params` |
| `coimnet-derivation-rules-hash/v1` | `params` |
| `coimnet-derivation-rules/v1` | `params` |
| `coimnet-doctor/v1` | `internal/cli` |
| `coimnet-download-part/v1` | `download` |
| `coimnet-download-receipt/v1` | `download` |
| `coimnet-dry-run/v1` | `internal/cli` |
| `coimnet-episode-checkpoint/v1` | `checkpoint` |
| `coimnet-episode-training/v1` | `checkpoint` |
| `coimnet-feather-report/v1` | `feather` |
| `coimnet-graph-import/v1` | `internal/cli` |
| `coimnet-graph-report/v1` | `connectome` |
| `coimnet-graph-store/v1` | `connectome` |
| `coimnet-graph-store/v9` | `connectome`（相容讀取） |
| `coimnet-graph-validate/v1` | `internal/cli` |
| `coimnet-individual-checkpoint/v1` | `checkpoint` |
| `coimnet-individual-checkpoint/v9` | `checkpoint`（相容讀取） |
| `coimnet-individual/v1` | `learning` |
| `coimnet-lif-state/v0` | `dynamics`（歷史聯集前的舊檔相容讀取） |
| `coimnet-lif-state/v1` | `dynamics` |
| `coimnet-lif-threshold-example/v1` | `experiment` |
| `coimnet-mixed-state/v1` | `dynamics` |
| `coimnet-model-package/v1` | `checkpoint` |
| `coimnet-model-package/v2` | `checkpoint` |
| `coimnet-parameter-derive/v1` | `internal/cli` |
| `coimnet-parameter-set/v1` | `params` |
| `coimnet-parameter-validate/v1` | `internal/cli` |
| `coimnet-prediction/v1` | `internal/cli` |
| `coimnet-ppo-experiment/v1` | `experiment` |
| `coimnet-real-subgraph-example/v1` | `examples/realsubgraph` |
| `coimnet-replay/v0` | `checkpoint`（歷史版本） |
| `coimnet-replay/v1` | `replay` |
| `coimnet-sign-rule/v1` | `params` |
| `coimnet-simulate-compare-report/v1` | `simulate` |
| `coimnet-simulate-compare/v1` | `simulate` |
| `coimnet-simulate-compare/v2` | `simulate` |
| `coimnet-simulate-parameters/v1` | `simulate` |
| `coimnet-simulate-protocol/v1` | `simulate` |
| `coimnet-simulate-protocol/v2` | `simulate` |
| `coimnet-simulate-run/v1` | `simulate` |
| `coimnet-simulate-state/v1` | `simulate` |
| `coimnet-simulate-state/v2` | `simulate` |
| `coimnet-teacher-records/v1` | `teacher` |
| `coimnet-train-result/v1` | `internal/cli` |

格式變更的相容規則見 [ENG](../ENG.md) 與對應 [tickets](tickets/)。欄位層級的格式說明見 [config-schema.md](config-schema.md)、[個體狀態](individual-state.md)、[訊號映射](signal-projections.md)、[訊號取樣](signal-resampling.md)。

## 來源與授權

- 資料來源稽核：[docs/malecns-source-audit.md](malecns-source-audit.md)（官方發布與欄位、取得與校驗）。
- 發布內容盤點：[docs/malecns-release-catalog.md](malecns-release-catalog.md)。
- 依賴與資料來源授權盤點：[docs/licenses.md](licenses.md)。
- 研究來源清單：[docs/handoff/sources.json](handoff/sources.json)。
- 儲存庫授權：[LICENSE](../LICENSE)。接線資料、研究程式與外部模型依各自授權使用；發布授權未確認前不公開發布（見 [release-checklist.md](release-checklist.md)）。
- 模型定位與可選生物機制：[docs/model-and-mechanisms.md](model-and-mechanisms.md)。

## 風險與限制

- 目前阻礙：[delivery-status.md](../delivery-status.md#目前阻礙)（含資料、裝置與硬體受阻項）。
- 效能與硬體：[docs/resources.md](resources.md)（區分實測、算術估算與尚未驗證的完整圖訓練）。
- 研究方向（Connectome 原生模擬與可學習模式兩條路徑）：[docs/research-directions/connectome-native.md](research-directions/connectome-native.md)。

## 需求證據

- 需求狀態：[docs/requirements-status.json](requirements-status.json)（85 項必要需求加 NAT 增補；`passed` 必須附實際證據路徑）。
- 需求補遺定義：[docs/requirements-addendum.json](requirements-addendum.json)。
- 需求定義（原始交接副本）：[docs/handoff/requirements.json](handoff/requirements.json)。
- 證據目錄：[evidence/](../evidence/)（每個需求一個子目錄，含 `verification.json`、環境、指紋、報告與日誌）。

## 票與決策

- 工作票：[docs/tickets/](tickets/)（01–30，每個 ticket 含 root 決策、契約、驗收與依據）。
- 決策紀錄：[delivery-status.md](../delivery-status.md#決策紀錄)。
- 驗證與提交程序的執行規範：[AGENTS.md](../AGENTS.md) 的「實作與驗證」與「資料與操作範圍」兩段。
