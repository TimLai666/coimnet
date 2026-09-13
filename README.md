# CoImNet

**Connectome-Imprinted Network** 是以真實果蠅神經接線建立可訓練模型的框架，使用 Go 與 Insyra。目標是讓使用者定義輸入訊號、訓練神經動態與閾值，並保存及恢復模型的學習狀態。

儲存庫提供通用 SDK、CLI 與驗證範例。使用者自己的任務程式、資料與訓練模型由使用者專案管理。本文的 `delayed`、`experiment` 套件與小型快照證據都是人工教學／測試範例，不是預訓練果蠅模型。

目前的連續核心屬於稀疏的連續時間循環神經網路。接線圖決定哪些神經元相連，神經動態與學習方法則由框架提供。詳見[模型定位與可選機制](docs/model-and-mechanisms.md)，以及[記憶體與時間量測](docs/resources.md)。

目前已實作 CPU 連續動態、完整與截斷時間梯度、Insyra 輸入與讀出、AdamW 訓練，以及人工延遲訊號範例。官方 MaleCNS 資料可下載、校驗、逐批讀取 Feather，並依明示的 manifest 建成 `raw_segments` 與 `annotated_neurons` 兩個具名視圖及統計報告。把接線圖接上訓練核心、固定格式的圖儲存、脈衝模型、化學調節、五類任務與 GPU 核心尚未完成，完整需求以[開發進度](delivery-status.md)追蹤。

## 建置與範例

```sh
go build -o bin/coimnet ./cmd/coimnet
./bin/coimnet --help
./bin/coimnet doctor
./bin/coimnet examples run delayed
```

`delayed` 是三個人工神經元的五步延遲訊號任務，使用三組固定 seed。每組都執行訓練、凍結核心及打亂答案對照。輸入映射與讀出保持固定，只有核心連線權重可更新。JSON 輸出包含每組結果、設定及指紋，門檻未通過會傳回非零退出碼。

保存與接續單組人工範例訓練：

```sh
run_dir=$(mktemp -d)
./bin/coimnet train delayed --steps 600 --checkpoint "$run_dir/first.json"
./bin/coimnet resume --checkpoint "$run_dir/first.json" --steps 200 --out "$run_dir/second.json"
printf '[[0.7],[0],[0],[0],[0]]\n' > "$run_dir/observations.json"
./bin/coimnet predict --checkpoint "$run_dir/second.json" --input "$run_dir/observations.json"
```

快照保存這個獨立序列模式的完整參數、最佳化器、資料 seed 與下一筆樣本位置，沒有持續個體或化學狀態。檔案上限為 64 MiB，以校驗碼、版本與形狀檢查後恢復。目的路徑已存在時拒絕覆寫，檔案系統須支援 hardlink 與目錄同步。

取得官方原始資料：

```sh
./bin/coimnet data sources
data_dir=$(mktemp -d)
./bin/coimnet data download \
  --url https://storage.googleapis.com/flyem-male-cns/v1.0/connectome-data/flat-connectome/body-annotations-male-cns-v1.0-minconf-0.5.feather \
  --out "$data_dir/annotations.feather" --max-bytes 20000000 \
  > "$data_dir/receipt.json"
```

取消或網路中斷後，以相同命令及目的路徑重試，可接續經版本驗證的暫存檔。既有完整檔案、無法辨識的暫存檔及來源改版都會返回錯誤。強制終止程序後若留下鎖或不一致暫存資料，會拒絕自動續傳並保留檔案。下載器會比對伺服器提供的 CRC32C，也接受使用者提供的官方校驗值。回條中的 SHA-256 一律在本機計算，`hash_status` 另記錄是否已比對上游校驗值。

逐批檢查 Feather 原件：

```sh
./bin/coimnet data inspect --input "$data_dir/annotations.feather" --max-bytes 20000000
```

命令讀取每個批次並報告欄位、資料列與 Arrow 配置量。檔案、footer、Arrow 配置量與資料列均有上限，可由 `--help` 查閱。失敗報告的 `complete` 為 `false`，退出碼非零。Arrow 配置量不包含整個程序的記憶體。讀取原件不代表已完成神經元篩選或建立訓練圖。

由三份原件建立標準化接線圖並輸出報告：

```sh
./bin/coimnet data import --manifest manifests/malecns-v1.0.json > graph-report.json
```

[manifests/malecns-v1.0.json](manifests/malecns-v1.0.json) 明示官方欄位名稱、來源指紋、端點身份假設與選取條件（`annotations.status == "Traced"`，標為工程選取）。相對路徑以 manifest 所在目錄解析，原件保持只讀，讀取前後都比對 SHA-256。報告列出原始列數、選入節點、每一種排除原因、未對應註記的端點、unique／duplicate pair、自環、孤立節點、原始 `weight` 加總、傳導物質預測未知比例與受體 `not_derived` 狀態，每個比例都附分子、分母與依據欄位。記憶體、暫存空間、run 數與列數超限時直接失敗，不會偷偷縮圖。`weight` 是來源原始值，不是突觸數；重複 pair 預設保留每一列，只有 manifest 宣告可加總分割時才允許 `--edge-view aggregated_pairs`。這個命令只輸出報告，圖本身留在程序內，固定格式的圖儲存另行追蹤。

## Go SDK

- `dynamics.NewContinuous` 建立同步稀疏連續模型。`Forward` 支援延遲，`Backward` 提供完整或固定視窗梯度。
- `learning.NewNetwork` 將 Insyra 編碼器及讀出接到核心，`LossGradient` 回傳整條路徑的梯度。
- `learning.NewTrainer` 使用可保存的 AdamW 狀態。凍結參數群組時，權重、動量、步數與衰減一起凍結。
- `signal` 提供具版本訊號、時鐘驗證與事件排序、觀察／答案／回饋分離及無損外部 ID 映射。跨頻率重取樣尚未接入。
- `checkpoint.NewState`、`Save`、`Load` 提供獨立序列快照，`learning.RestoreTrainer` 重建隔離訓練器。
- `download.Fetch` 提供容量限制、取消、有限重試、版本檢查、續傳及來源回條。
- `feather.Scan` 以 callback 逐批讀取 Feather V2，保留整數、缺值、字典與 list。批次資料在 callback 期間有效，需保留時呼叫 `Retain`，使用完畢後 `Release`。
- `connectome.Build` 依 `DatasetManifest` 與 `ResourceLimits` 建立不可變的 `Graph` 與 `GraphReport`。`Node`、`IndexOf`、`NeuronIDs` 提供無損外部 ID 與連續索引的雙向對照；`StreamAnnotatedNodes`／`StreamAnnotatedEdges` 依固定順序串流選入視圖；`StreamRawSegments` 重新逐批讀取 weights 原件並保留每一列。重複 pair 以有界外部排序的相鄰 run 計數，不建立全量 pair map。

目前 `learning` 每次 `Step` 或 `Predict` 都從零神經狀態開始一段獨立序列，只讀取最後一步輸出。CPU 動態使用 float64，Insyra 編碼器、讀出與損失使用 float32。完整 API 可用 `go doc ./learning` 與 `go doc ./dynamics` 查閱。

## 開發驗證

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./...
go mod verify
```

在 macOS 或 Linux 可用 `scripts/verify.sh NEW_OUTPUT_DIRECTORY` 一次執行上述檢查、三組學習對照，以及真正跨程序的 CLI 續訓比對，保存環境報告、來源指紋與日誌。目錄須尚未存在。已下載三份官方原件時，`scripts/graph-evidence.sh NEW_OUTPUT_DIRECTORY` 會在計時下重跑真實接線圖建構並保存報告、環境與指紋。

## 開發入口

- [專案工作規則](AGENTS.md)：開發、驗證與 sub-agent 使用順序。
- [初始化紀錄](docs/initialization.md)：環境、依賴版本、查證命令與接續工作。
- [完整規格](docs/handoff/CoImNet_Implementation_Plan.zh-TW.md)：25 章規格與 14 個工作包。
- [需求定義](docs/handoff/requirements.json)與[實作狀態](docs/requirements-status.json)：85 項必要需求及各項驗證證據。
- [交接包說明](docs/handoff/README.zh-TW.md)與[研究來源](docs/handoff/sources.json)。

Go 最低版本為 `1.25.12`。指定依賴 Insyra `v0.3.2` 對應提交 `1f1cdb949c51a10be104965cf4cf30f8f0aaa648`，已在 `go.mod` 與 `go.sum` 固定版本與校驗值。上述核心與例子以 Go 獨立執行，不需要 Python。

## 框架目標

主要資料來源為 MaleCNS，另支援 FlyWire 比較。完整範圍涵蓋連續與脈衝神經動態、可訓練閾值、持續學習、化學調節、教師蒸餾及完整狀態保存。

任務驗證包含 OCR、語音轉文字、語言生成、文字轉影音、空間與導航，以及共用核心的多模態學習。這些是待驗證的研究目標。

這些任務的特定資料處理、訓練配方與模型只以明示範例或外部實驗呈現。通用核心、訓練器與快照介面不依賴某一個任務或個人模型。新增任務範例統一放在 `examples/`，既有 `experiment` 是人工延遲關聯的驗證範例套件。

依[主規格第 3 章](docs/handoff/CoImNet_Implementation_Plan.zh-TW.md#chapter-03)，實驗報告須區分軟體正確性、任務能力與生物證據。接線圖不等於已訓練模型，傳導物質預測不等於量測出的突觸作用，未知受體也不能當成沒有受體。突觸接點、神經元配對與參數量分開統計。獎勵、損失與化學調節分別建模，人工假設要明示。共同訊號格式或小型任務成功，不能證明一般語言、自然影音能力，固定編碼器的貢獻也須以對照確認。

## 授權與資料

儲存庫沿用既有 [MIT 授權](LICENSE)。接線資料、研究程式與外部模型依各自授權使用。大型資料與訓練結果存放於 Git 以外的本機目錄。
