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

- `backend/webgpu`：目前已實測的稀疏 GPU 運算原語；完整訓練仍見 OPS-05
- `checkpoint`：模型包、個體快照、訓練快照與恢復
- `config`：組態展開
- `connectome`：標準化接線圖與圖儲存
- `download`：官方資料下載與續傳
- `dynamics`：連續、LIF 與混合核心
- `feather`：Feather V2 逐批讀取
- `learning`：核心、外圍模型、訓練器與持續個體
- [`learning/rl`](../learning/rl/README.md)：PPO 更新入口、適用狀態與限制
- [`tasks/asr`](../tasks/asr/README.md)：音訊轉文字、因果串流與恢復
- `tasks/media/realdata`：授權影音來源的嚴格匯入（範例用）
- `tasks/nav2d/trajectory`：授權果蠅軌跡的逐批匯入、試次隔離、因果樣本、逐列刺激紀錄與獨立目標計分資料（範例用）
- `tasks/ocr`：單行 OCR 與頁面區塊處理
- `tasks/textgen`：文字語料匯入與生成
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

- [experiment/gridnav/README.md](../experiment/gridnav/README.md)：人工走廊的模仿與 PPO 範例、取樣與評估契約，以及起始提示的逐回合對照。
- [examples/lifthreshold/README.md](../examples/lifthreshold/README.md)：三顆 LIF 神經元的閾值可訓練性檢查。
- [examples/multichannel/README.md](../examples/multichannel/README.md)：多通道 adapter 完整流程。
- [examples/realsubgraph/README.md](../examples/realsubgraph/README.md)：ALIN 真實子圖選取與短訓練。
- [examples/realasr/README.md](../examples/realasr/README.md)：Mini LibriSpeech 真實語音小樣本驗證。
- [examples/realtext/README.md](../examples/realtext/README.md)：Gutenberg 英文文字小樣本驗證。
- [examples/realocr/README.md](../examples/realocr/README.md)：KMNIST 單字元小樣本驗證。
- [examples/realmedia/README.md](../examples/realmedia/README.md)：Blender 開源影片的影像、音訊與影片小樣本流程驗證。
- [examples/realnav/README.md](../examples/realnav/README.md)：果蠅軌跡的單步位移預測與自主行走工程評估範例。
- [examples/realnavmemory/README.md](../examples/realnavmemory/README.md)：完整刺激歷史、遮罩模仿學習、控制組與凍結模型刺激清除檢查的工程範例。

用 `coimnet examples list` 列出所有內建例子，`coimnet examples run <name>` 執行。命令本身的權限與用法用 `coimnet examples run <name> --help` 查。

## 檔案格式

本索引列出 `coimnet-名稱/v數字` 形式的版本識別，來源包括 Go 原始碼中的直接字串、`scripts/` 的 shell 與 `evidence/` 的 Python 引用，以及四份治理 JSON 的 `schema_version`。每個版本列出用途與來源行號，由 `go test` 核對一致性。下載資料、模型、歷史 JSON 成果及規格中尚未實作的提案不在盤點範圍。

### 正式格式

| 版本 | 套件/用途範圍 | 用途說明 | 來源 |
| --- | --- | --- | --- |
| `coimnet-ablation/v1` | `experiment` | 消融報告格式，也記錄消融證據使用的 MappingVersion | [experiment/ablation.go:16](../experiment/ablation.go#L16) |
| `coimnet-adaptive-evaluation/v1` | `experiment` | 適應性評估報告格式 | [experiment/adaptive_run.go:276](../experiment/adaptive_run.go#L276) |
| `coimnet-asr-dataset/v1` | `tasks/asr` | 授權語音資料集 manifest 格式 | [tasks/asr/dataset.go:21](../tasks/asr/dataset.go#L21) |
| `coimnet-asr-fixture/v1` | `internal/cli` | ASR fixture 範例報告格式 | [internal/cli/asr.go:16](../internal/cli/asr.go#L16) |
| `coimnet-asr-stream/v1` | `tasks/asr` | ASR 串流檔案狀態格式 | [tasks/asr/stream_file.go:16](../tasks/asr/stream_file.go#L16) |
| `coimnet-attribution/v1` | `experiment` | 核心與外圍歸因報告格式 | [experiment/attribution.go:531](../experiment/attribution.go#L531) |
| `coimnet-benchmark/v2` | `internal/cli` | benchmark 報告格式 | [internal/cli/benchmark.go:34](../internal/cli/benchmark.go#L34) |
| `coimnet-bio-inspired/v1` | `experiment` | 生物啟發機制設定與報告格式 | [experiment/bioinspired.go:19](../experiment/bioinspired.go#L19) |
| `coimnet-byte-vocab/v1` | `tasks/textgen/tokenizer` | 位元組詞表的保存與解碼格式 | [tasks/textgen/tokenizer/tokenizer.go:25](../tasks/textgen/tokenizer/tokenizer.go#L25) |
| `coimnet-checkpoint-bundle/v2` | `checkpoint` | checkpoint bundle 目錄格式 | [checkpoint/bundle.go:23](../checkpoint/bundle.go#L23) |
| `coimnet-config/v1` | `config` | CLI 與 SDK 組態文件格式 | [config/config.go:23](../config/config.go#L23) |
| `coimnet-continual-matrix/v1` | `experiment` | 持續學習矩陣報告格式 | [experiment/continual.go:9](../experiment/continual.go#L9) |
| `coimnet-continuous-state/v1` | `dynamics` | 連續核心個體狀態格式 | [dynamics/state.go:14](../dynamics/state.go#L14) |
| `coimnet-data-sources/v1` | `internal/cli` | 資料來源清單報告格式 | [internal/cli/data.go:16](../internal/cli/data.go#L16) |
| `coimnet-dataset-manifest/v1` | `connectome` | 接線資料集 manifest 格式 | [connectome/manifest.go:25](../connectome/manifest.go#L25) |
| `coimnet-delayed-report/v2` | `experiment` | 延遲任務報告格式 | [experiment/delayed.go:116](../experiment/delayed.go#L116) |
| `coimnet-derivation-report/v1` | `params` | 參數推導報告格式 | [params/report.go:10](../params/report.go#L10) |
| `coimnet-derivation-rules/v1` | `params` | 參數推導規則文件格式 | [params/rules.go:33](../params/rules.go#L33) |
| `coimnet-distill/v1` | `distill` | 蒸餾資料格式 | [distill/distill.go:15](../distill/distill.go#L15) |
| `coimnet-doctor/v1` | `internal/cli` | doctor 能力檢查報告格式 | [internal/cli/doctor.go:19](../internal/cli/doctor.go#L19) |
| `coimnet-download-part/v1` | `download` | 下載分段暫存狀態格式 | [download/download.go:33](../download/download.go#L33) |
| `coimnet-download-receipt/v1` | `download` | 下載回條格式 | [download/download.go:31](../download/download.go#L31) |
| `coimnet-dry-run/v1` | `internal/cli` | CLI dry-run 檢查報告格式 | [internal/cli/config_run.go:224](../internal/cli/config_run.go#L224) |
| `coimnet-episode-checkpoint/v1` | `checkpoint` | 獨立 episode 訓練快照格式 | [checkpoint/checkpoint.go:28](../checkpoint/checkpoint.go#L28) |
| `coimnet-episode-training/v1` | `checkpoint` | 訓練快照使用的訓練狀態版本 | [checkpoint/package.go:30](../checkpoint/package.go#L30) |
| `coimnet-feather-report/v1` | `feather` | Feather 逐批讀取報告格式 | [feather/reader.go:21](../feather/reader.go#L21) |
| `coimnet-full-graph-resume/v1` | `experiment/fullgraph` | 全圖續跑摘要格式 | [experiment/fullgraph/resume.go:13](../experiment/fullgraph/resume.go#L13) |
| `coimnet-full-graph-short-training/v1` | `experiment/fullgraph` | 全圖短訓練報告格式 | [experiment/fullgraph/run.go:23](../experiment/fullgraph/run.go#L23) |
| `coimnet-graph-import/v1` | `internal/cli` | 圖匯入報告格式 | [internal/cli/validate.go:15](../internal/cli/validate.go#L15) |
| `coimnet-graph-report/v1` | `connectome` | 標準化接線圖報告格式 | [connectome/manifest.go:27](../connectome/manifest.go#L27) |
| `coimnet-graph-store/v1` | `connectome` | 圖儲存格式 | [connectome/store.go:27](../connectome/store.go#L27) |
| `coimnet-graph-validate/v1` | `internal/cli` | 圖驗證報告格式 | [internal/cli/validate.go:16](../internal/cli/validate.go#L16) |
| `coimnet-handoff-requirements/v1` | `docs/handoff` | 原始交接需求 JSON 的治理格式 | [docs/handoff/requirements.json:2](../docs/handoff/requirements.json#L2) |
| `coimnet-handoff-sources/v1` | `docs/handoff` | 原始交接來源 JSON 的治理格式 | [docs/handoff/sources.json:2](../docs/handoff/sources.json#L2) |
| `coimnet-imitation/v1` | `experiment` | 模仿學習報告格式 | [experiment/imitation.go:15](../experiment/imitation.go#L15) |
| `coimnet-individual-checkpoint/v1` | `checkpoint` | 連續個體快照格式 | [checkpoint/individual.go:25](../checkpoint/individual.go#L25) |
| `coimnet-individual/v1` | `learning` | 持續個體與訓練狀態版本 | [learning/individual.go:20](../learning/individual.go#L20) |
| `coimnet-lif-state/v1` | `dynamics` | LIF 個體狀態格式 | [dynamics/lif_state.go:14](../dynamics/lif_state.go#L14) |
| `coimnet-lif-threshold-example/v1` | `experiment` | LIF 閾值範例報告格式 | [experiment/lif_threshold.go:16](../experiment/lif_threshold.go#L16) |
| `coimnet-media/v1` | `tasks/media` | 媒體範例報告格式 | [tasks/media/run.go:15](../tasks/media/run.go#L15) |
| `coimnet-mixed-state/v1` | `dynamics` | 混合核心個體狀態格式 | [dynamics/mixed_state.go:17](../dynamics/mixed_state.go#L17) |
| `coimnet-model-package/v1` | `checkpoint` | 模型包格式 | [checkpoint/package.go:26](../checkpoint/package.go#L26) |
| `coimnet-multimodal-eval/v1` | `experiment/multimodaleval` | 多模態評估報告格式 | [experiment/multimodaleval/run.go:14](../experiment/multimodaleval/run.go#L14) |
| `coimnet-multimodal-sample/v1` | `multimodal` | 多模態樣本格式 | [multimodal/sample.go:16](../multimodal/sample.go#L16) |
| `coimnet-multitask/v1` | `experiment` | 多任務實驗報告格式 | [experiment/multitask.go:13](../experiment/multitask.go#L13) |
| `coimnet-nav2d-suite/v1` | `experiment` | 二維導航四項任務的比較報告格式 | [experiment/nav2d_suite.go:16](../experiment/nav2d_suite.go#L16) |
| `coimnet-nav2d-trajectory/v1` | `tasks/nav2d/trajectory` | 果蠅軌跡資料格式 | [tasks/nav2d/trajectory/trajectory.go:31](../tasks/nav2d/trajectory/trajectory.go#L31) |
| `coimnet-nav2d/v1` | `experiment` | 二維導航環境報告格式 | [experiment/nav2d_suite.go:14](../experiment/nav2d_suite.go#L14) |
| `coimnet-ocr-fixture/v1` | `tasks/ocr` | OCR fixture 報告格式 | [tasks/ocr/fixture.go:15](../tasks/ocr/fixture.go#L15) |
| `coimnet-parameter-derive/v1` | `internal/cli` | CLI 參數推導報告格式 | [internal/cli/derive.go:19](../internal/cli/derive.go#L19) |
| `coimnet-parameter-set/v1` | `params` | 導出參數集格式 | [params/set.go:28](../params/set.go#L28) |
| `coimnet-parameter-validate/v1` | `internal/cli` | CLI 參數驗證報告格式 | [internal/cli/derive.go:20](../internal/cli/derive.go#L20) |
| `coimnet-ppo-experiment/v1` | `experiment` | PPO 實驗設定與報告格式 | [experiment/ppo.go:15](../experiment/ppo.go#L15) |
| `coimnet-prediction/v1` | `internal/cli` | CLI 預測輸出格式 | [internal/cli/train.go:153](../internal/cli/train.go#L153) |
| `coimnet-real-asr-example/v1` | `examples/realasr` | 真實語音範例報告格式 | [examples/realasr/main.go:23](../examples/realasr/main.go#L23) |
| `coimnet-real-subgraph-example/v1` | `examples/realsubgraph` | 真實子圖範例報告格式 | [examples/realsubgraph/main.go:25](../examples/realsubgraph/main.go#L25) |
| `coimnet-real-text-example/v1` | `examples/realtext` | 真實文字範例報告格式 | [examples/realtext/main.go:28](../examples/realtext/main.go#L28) |
| `coimnet-realmedia-example/v1` | `examples/realmedia` | 真實影音範例報告格式 | [examples/realmedia/main.go:32](../examples/realmedia/main.go#L32) |
| `coimnet-realmedia/v1` | `tasks/media/realdata` | 授權影音 manifest 格式 | [tasks/media/realdata/manifest.go:24](../tasks/media/realdata/manifest.go#L24) |
| `coimnet-realnav-example/v1` | `examples/realnav` | 真實導航範例報告格式，也作 realnavmemory 的 base model 版本 | [examples/realnav/main.go:26](../examples/realnav/main.go#L26) |
| `coimnet-realnav-memory-bundle/v1` | `examples/realnavmemory` | 導航記憶 bundle 格式 | [examples/realnavmemory/storage.go:23](../examples/realnavmemory/storage.go#L23) |
| `coimnet-realnav-memory-counterfactual/v1` | `examples/realnavmemory` | 導航記憶反事實報告格式 | [examples/realnavmemory/counterfactual.go:15](../examples/realnavmemory/counterfactual.go#L15) |
| `coimnet-realnav-memory-inference/v1` | `examples/realnavmemory` | 導航記憶推論報告格式 | [examples/realnavmemory/main.go:22](../examples/realnavmemory/main.go#L22) |
| `coimnet-realnav-memory-plan/v1` | `examples/realnavmemory` | 導航記憶規劃資料格式 | [examples/realnavmemory/storage.go:22](../examples/realnavmemory/storage.go#L22) |
| `coimnet-realnav-memory-rollout/v1` | `examples/realnavmemory` | 導航記憶 rollout 報告格式 | [examples/realnavmemory/rollout.go:16](../examples/realnavmemory/rollout.go#L16) |
| `coimnet-realnav-rollout/v1` | `examples/realnav` | 真實導航 rollout 報告格式 | [examples/realnav/rollout.go:24](../examples/realnav/rollout.go#L24) |
| `coimnet-realocr-data/v1` | `examples/realocr` | 真實 OCR 資料報告格式 | [examples/realocr/main.go:34](../examples/realocr/main.go#L34) |
| `coimnet-realocr/v1` | `examples/realocr` | 真實 OCR 報告格式 | [examples/realocr/main.go:35](../examples/realocr/main.go#L35) |
| `coimnet-replay/v1` | `replay` | 重播狀態格式 | [replay/policy.go:28](../replay/policy.go#L28) |
| `coimnet-report/v1` | `internal/cli` | 需求匯出總報告格式 | [internal/cli/report.go:24](../internal/cli/report.go#L24) |
| `coimnet-requirements-addendum/v1` | `docs` | 需求補遺治理 JSON 格式 | [docs/requirements-addendum.json:2](../docs/requirements-addendum.json#L2) |
| `coimnet-requirements-status/v1` | `docs` | 需求狀態治理 JSON 格式 | [docs/requirements-status.json:2](../docs/requirements-status.json#L2) |
| `coimnet-roles/v1` | `tasks/media` | 媒體生成角色報告格式 | [tasks/media/roles.go:12](../tasks/media/roles.go#L12) |
| `coimnet-sign-rule/v1` | `params` | 參數推導規則中的 sign rule block 格式 | [params/rules.go:35](../params/rules.go#L35) |
| `coimnet-simulate-compare-report/v1` | `simulate` | 空模型比較報告格式 | [simulate/compare.go:23](../simulate/compare.go#L23) |
| `coimnet-simulate-compare/v1` | `simulate` | 空模型比較協定格式 | [simulate/compare.go:21](../simulate/compare.go#L21) |
| `coimnet-simulate-protocol/v1` | `simulate` | 原生模擬協定格式 | [simulate/protocol.go:34](../simulate/protocol.go#L34) |
| `coimnet-simulate-run/v1` | `simulate` | 原生模擬執行報告格式 | [simulate/protocol.go:36](../simulate/protocol.go#L36) |
| `coimnet-simulate-state/v1` | `simulate` | 原生模擬狀態格式 | [simulate/protocol.go:38](../simulate/protocol.go#L38) |
| `coimnet-student-evaluation/v1` | `experiment/studenteval` | 學生獨立評估報告格式 | [experiment/studenteval/studenteval.go:23](../experiment/studenteval/studenteval.go#L23) |
| `coimnet-teacher-records/v1` | `teacher` | 離線教師紀錄格式 | [teacher/records.go:13](../teacher/records.go#L13) |
| `coimnet-textgen-corpus/v1` | `tasks/textgen` | 文字語料 manifest 格式 | [tasks/textgen/corpus.go:24](../tasks/textgen/corpus.go#L24) |
| `coimnet-textgen/v1` | `tasks/textgen` | 文字生成報告格式 | [tasks/textgen/run.go:15](../tasks/textgen/run.go#L15) |
| `coimnet-train-result/v1` | `internal/cli` | CLI 訓練結果格式 | [internal/cli/train.go:107](../internal/cli/train.go#L107) |
| `coimnet-vector-state/v1` | `dynamics` | 向量核心個體狀態格式 | [dynamics/vector_core.go:13](../dynamics/vector_core.go#L13) |
| `coimnet-video-timeline/v1` | `tasks/media` | 影片時間線格式 | [tasks/media/video_output.go:49](../tasks/media/video_output.go#L49) |
| `coimnet-video/v1` | `tasks/media` | 影片範例報告格式 | [tasks/media/video.go:370](../tasks/media/video.go#L370) |

### 規則與雜湊識別

| 版本 | 套件/用途範圍 | 用途說明 | 來源 |
| --- | --- | --- | --- |
| `coimnet-connectome-builder/v1` | `connectome` | 接線建構器的正規化規則版本 | [connectome/manifest.go:30](../connectome/manifest.go#L30) |
| `coimnet-derivation-rules-hash/v1` | `params` | 參數推導規則雜湊識別 | [params/rules.go:39](../params/rules.go#L39) |
| `coimnet-distill-label/v1` | `distill` | 標籤蒸餾的類別詞彙識別 | [distill/train.go:132](../distill/train.go#L132) |
| `coimnet-logmel/v1` | `tasks/asr/audio` | Log-Mel 前端特徵版本 | [tasks/asr/audio/frontend.go:11](../tasks/asr/audio/frontend.go#L11) |
| `coimnet-nav2d-causal-next-displacement/v1` | `tasks/nav2d/trajectory` | 導航因果下一步位移取樣規則 | [tasks/nav2d/trajectory/trajectory.go:33](../tasks/nav2d/trajectory/trajectory.go#L33) |
| `coimnet-policy-version/v2` | `learning/rl` | PPO 策略版本雜湊識別 | [learning/rl/update.go:35](../learning/rl/update.go#L35) |
| `coimnet-realnav-memory-causal-features/v1` | `examples/realnavmemory` | 導航記憶因果特徵規則版本 | [examples/realnavmemory/storage.go:28](../examples/realnavmemory/storage.go#L28) |
| `coimnet-realnav-preprocessing/v1` | `examples/realnav` | 真實導航前處理規則版本 | [examples/realnav/data.go:22](../examples/realnav/data.go#L22) |
| `coimnet-simulate-parameters/v1` | `simulate` | 模擬參數雜湊領域識別 | [simulate/parameters.go:18](../simulate/parameters.go#L18) |
| `coimnet-textgen-fixture/v1` | `tasks/textgen` | 文字 fixture 來源識別 | [tasks/textgen/corpus.go:298](../tasks/textgen/corpus.go#L298) |

### 驗證工具格式

| 版本 | 套件/用途範圍 | 用途說明 | 來源 |
| --- | --- | --- | --- |
| `coimnet-clean-env-run/v1` | `scripts` | 乾淨環境驗證流程的執行報告格式 | [scripts/clean-env-verify.sh:423](../scripts/clean-env-verify.sh#L423) |
| `coimnet-counterfactual-summary/v1` | `evidence/TSK-11/stimulus-counterfactual-20261003` | 刺激反事實分析工具的報告格式 | [evidence/TSK-11/stimulus-counterfactual-20261003/analyze.py:56](../evidence/TSK-11/stimulus-counterfactual-20261003/analyze.py#L56) |
| `coimnet-multitask-suite/v1` | `experiment` | 多任務測試報告格式 | [experiment/multitask_evidence_test.go:55](../experiment/multitask_evidence_test.go#L55) |
| `coimnet-platform-matrix/v1` | `scripts` | 跨平台編譯與測試矩陣工具的報告格式 | [scripts/platform-matrix.sh:198](../scripts/platform-matrix.sh#L198) |
| `coimnet-ppo-gradient-diagnostic/v1` | `evidence/LRN-09/gradient-diagnostic-20261004` | 完整梯度診斷工具的報告格式 | [experiment/ppo_gradient_diagnostic_test.go:304](../experiment/ppo_gradient_diagnostic_test.go#L304) |
| `coimnet-ppo-horizon-comparison/v1` | `evidence/LRN-09/horizon-comparison-20261005` | PPO 期限比較工具的報告格式 | [experiment/ppo_horizon_evidence_test.go:134](../experiment/ppo_horizon_evidence_test.go#L134) |
| `coimnet-ppo-horizon-reproduction/v1` | `evidence/LRN-09/horizon-comparison-20261005` | PPO 期限比較的跨程序重現報告格式 | [evidence/LRN-09/horizon-comparison-20261005/verify_report.py:178](../evidence/LRN-09/horizon-comparison-20261005/verify_report.py#L178) |
| `coimnet-ppo-horizon-summary/v1` | `evidence/LRN-09/horizon-comparison-20261005` | PPO 期限比較的核對摘要格式 | [evidence/LRN-09/horizon-comparison-20261005/verify_report.py:169](../evidence/LRN-09/horizon-comparison-20261005/verify_report.py#L169) |
| `coimnet-ppo-training-feedback-summary/v1` | `evidence/LRN-09/training-feedback-20261004` | PPO 訓練回饋核對摘要格式 | [evidence/LRN-09/training-feedback-20261004/verify_report.py:198](../evidence/LRN-09/training-feedback-20261004/verify_report.py#L198) |
| `coimnet-ppo-training-feedback/v1` | `evidence/LRN-09/training-feedback-20261004` | PPO 訓練回饋工具的報告格式 | [experiment/ppo_training_feedback_evidence_test.go:143](../experiment/ppo_training_feedback_evidence_test.go#L143) |
| `coimnet-recompute-rss/v1` | `scripts` | 重算模式 RSS 證據工具的報告格式 | [scripts/recompute-evidence.sh:158](../scripts/recompute-evidence.sh#L158) |
| `coimnet-short-goal-cue-audit/v1` | `evidence/LRN-09/short-goal-cue-audit-20261003` | 短期目標提示對照工具的報告格式 | [experiment/ppo_short_goal_evidence_test.go:109](../experiment/ppo_short_goal_evidence_test.go#L109) |
| `coimnet-short-goal-reproduction/v1` | `evidence/LRN-09/short-goal-cue-audit-20261003` | 短期目標提示對照的重現報告格式 | [evidence/LRN-09/short-goal-cue-audit-20261003/verify_report.py:150](../evidence/LRN-09/short-goal-cue-audit-20261003/verify_report.py#L150) |
| `coimnet-synthetic-goal-cue-audit/v1` | `experiment` | 合成目標提示對照工具的報告格式 | [experiment/ppo_goal_memory_evidence_test.go:304](../experiment/ppo_goal_memory_evidence_test.go#L304) |

### 測試資料與反例

| 版本 | 套件/用途範圍 | 用途說明 | 來源 |
| --- | --- | --- | --- |
| `coimnet-connectome-builder/v999` | `connectome` | 接線建構器未知版本拒絕反例 | [connectome/store_receipt_test.go:82](../connectome/store_receipt_test.go#L82) |
| `coimnet-dataset-manifest/v9` | `connectome` | 接線資料集未知版本拒絕反例 | [connectome/build_test.go:278](../connectome/build_test.go#L278) |
| `coimnet-graph-store/v9` | `connectome` | 圖儲存未知版本拒絕反例 | [connectome/store_test.go:246](../connectome/store_test.go#L246) |
| `coimnet-individual-checkpoint/v9` | `checkpoint` | 個體快照未知版本拒絕反例 | [checkpoint/package_test.go:281](../checkpoint/package_test.go#L281) |
| `coimnet-lif-state/v0` | `dynamics` | LIF 狀態不支援版本的拒絕反例 | [dynamics/lif_state_test.go:358](../dynamics/lif_state_test.go#L358) |
| `coimnet-model-package/v2` | `checkpoint` | 模型包未來版本拒絕反例 | [checkpoint/package_test.go:254](../checkpoint/package_test.go#L254) |
| `coimnet-real-task-evidence/v1` | `privacy` | 隱私掃描測試的人造資料識別，沒有正式讀寫契約 | [privacy_test.go:196](../privacy_test.go#L196) |
| `coimnet-realnav-memory-bundle/v999` | `examples/realnavmemory` | 導航記憶 bundle 未知版本拒絕反例 | [examples/realnavmemory/storage_test.go:49](../examples/realnavmemory/storage_test.go#L49) |
| `coimnet-replay/v0` | `checkpoint` | 重播狀態未知版本拒絕反例 | [checkpoint/options_compat_test.go:493](../checkpoint/options_compat_test.go#L493) |
| `coimnet-simulate-compare/v2` | `simulate` | 空模型比較未知版本拒絕反例 | [simulate/compare_test.go:345](../simulate/compare_test.go#L345) |
| `coimnet-simulate-protocol/v2` | `simulate` | 原生模擬協定未知版本拒絕反例 | [simulate/simulate_test.go:567](../simulate/simulate_test.go#L567) |
| `coimnet-simulate-state/v2` | `simulate` | 原生模擬狀態未知版本拒絕反例 | [simulate/simulate_test.go:729](../simulate/simulate_test.go#L729) |
| `coimnet-unknown/v9` | `checkpoint` | checkpoint 遷移的未知 schema 拒絕反例 | [checkpoint/migrate_test.go:187](../checkpoint/migrate_test.go#L187) |

格式變更的相容規則見 [ENG](../ENG.md) 與對應 [tickets](tickets/)。欄位層級的格式說明見 [config-schema.md](config-schema.md)、[個體狀態](individual-state.md)、[訊號映射](signal-projections.md)、[訊號取樣](signal-resampling.md)。

## 來源與授權

- 資料來源稽核：[docs/malecns-source-audit.md](malecns-source-audit.md)（官方發布與欄位、取得與校驗）。
- 發布內容盤點：[docs/malecns-release-catalog.md](malecns-release-catalog.md)。
- 依賴與資料來源授權盤點：[docs/licenses.md](licenses.md)。
- 研究來源清單：[docs/handoff/sources.json](handoff/sources.json)。
- 真實任務範例資料與授權：[docs/real-data-sources.md](real-data-sources.md)。
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

- 工作票：[docs/tickets/](tickets/)（01–49，每個 ticket 含 root 決策、契約、驗收與依據）。
- 決策紀錄：[delivery-status.md](../delivery-status.md#決策紀錄)。
- 驗證與提交程序的執行規範：[AGENTS.md](../AGENTS.md) 的「實作與驗證」與「資料與操作範圍」兩段。
