# CoImNet

**Connectome-Imprinted Network** 是以真實果蠅神經接線建立可訓練模型的框架，使用 Go 與 Insyra。目標是讓使用者定義輸入訊號、訓練神經動態與閾值，並保存及恢復模型的學習狀態。

儲存庫提供通用 SDK、CLI 與驗證範例。使用者自己的任務程式、資料與訓練模型由使用者專案管理。本文的 `delayed`、`experiment` 套件與小型快照證據都是人工教學／測試範例，不是預訓練果蠅模型。

目前的連續核心屬於稀疏的連續時間循環神經網路。接線圖決定哪些神經元相連，神經動態與學習方法則由框架提供。詳見[模型定位與可選機制](docs/model-and-mechanisms.md)，以及[記憶體與時間量測](docs/resources.md)。

目前已實作 CPU 連續動態、完整與截斷時間梯度、Insyra 輸入與讀出、AdamW 訓練，以及人工延遲訊號範例。LIF 放電核心可以用同一套拓撲、延遲與時鐘執行，並以宣告的替代梯度訓練基礎放電閾值，也可以開啟慢速穩定，讓每顆神經元依自己的活動估計調整閾值偏移。連續與 LIF 都能建立持續個體，把電位、延遲歷史、突觸跡、適應、不應期與慢速穩定狀態一起保存和接續。模型包、個體快照與訓練快照是三種分開版本的保存物，彼此不能互相當成完整恢復來源。官方 MaleCNS 資料可下載、校驗、逐批讀取 Feather，並依明示的 manifest 建成 `raw_segments` 與 `annotated_neurons` 兩個具名視圖及統計報告。選入圖可保存為固定格式並在新程序驗證、讀回。`examples/realsubgraph` 示範將選出的 ALIN 子圖接上訓練核心。完整圖訓練、按類型混合、化學調節、五類任務與 GPU 核心尚未完成，完整需求以[開發進度](delivery-status.md)追蹤。

## 建置與範例

```sh
go build -o bin/coimnet ./cmd/coimnet
./bin/coimnet --help
./bin/coimnet doctor
./bin/coimnet examples run delayed
./bin/coimnet examples run lif-threshold
```

`delayed` 是三個人工神經元的五步延遲訊號任務，使用三組固定 seed。每組都執行訓練、凍結核心及打亂答案對照。輸入映射與讀出保持固定，只有核心連線權重可更新。JSON 輸出包含每組結果、設定及指紋，門檻未通過會傳回非零退出碼。

`lif-threshold` 把同一份延遲資料換成三顆 LIF 放電神經元，只比較基礎閾值可不可訓練。三組固定 seed 各跑全部可訓練、只訓練閾值與全部凍結三個對照，報告列出每組的保留資料 MSE、逐顆 `theta_base` 與放電率。更新預算用 `--updates` 調整，預設 300，範圍 1 到 100000。事前寫死的門檻沒過時仍輸出完整報告，並傳回非零退出碼。這是人工資料上的數值可學習性檢查，不是果蠅放電行為成績，詳見[範例說明](examples/lifthreshold/README.md)。

保存與接續單組人工範例訓練：

```sh
run_dir=$(mktemp -d)
./bin/coimnet train delayed --steps 600 --checkpoint "$run_dir/first.json"
./bin/coimnet resume --checkpoint "$run_dir/first.json" --steps 200 --out "$run_dir/second.json"
printf '[[0.7],[0],[0],[0],[0]]\n' > "$run_dir/observations.json"
./bin/coimnet predict --checkpoint "$run_dir/second.json" --input "$run_dir/observations.json"
```

訓練器可以限制更新範圍與更新方式：`learning.Options` 的 `Trainable` 群組旗標搭配 `Masks` 逐邊、逐節點指定哪些參數能動，被凍結的參數連動量、步數與權重衰減都不變；`Config.EdgeSigns` 讓有依據的邊固定興奮或抑制、只學幅度，符號永遠不會翻轉；`Ranges` 在每次更新後把數值投影回宣告的區間並回報被投影的數量。一步的順序是固定的：梯度先乘 `LossScale` 再除回（乘出非有限值就整步拒絕，不留半次狀態），累積滿 `AccumulateSteps` 次才平均，平均後才套 `ClipNorm`，再交給 AdamW 以 `Schedule`（`constant`／`step`／`cosine`，都可加線性暖身）算出的學習率更新。學習率只由已完成的更新次數決定，累積到一半的梯度也進快照，因此中途保存再恢復與一次跑完的結果逐位相同。

快照保存這個獨立序列模式的完整參數、最佳化器、資料 seed 與下一筆樣本位置，沒有持續個體或化學狀態。檔案上限為 64 MiB，以校驗碼、版本與形狀檢查後恢復。目的路徑已存在時拒絕覆寫，檔案系統須支援 hardlink 與目錄同步。

三種保存物各有自己的 schema，載入時互相拒絕，並說明讀到的是哪一種：模型包 `coimnet-model-package/v1` 只有拓撲指紋、設定、基礎參數、宣告單位與證據清單，可以用來建立新個體，不能當成恢復來源；個體快照 `coimnet-individual-checkpoint/v1` 另外保存持續神經狀態、最佳化器動量與更新次數；訓練快照 `coimnet-episode-checkpoint/v1` 保存訓練器狀態與資料游標。

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

[manifests/malecns-v1.0.json](manifests/malecns-v1.0.json) 明示官方欄位名稱、來源指紋、端點身份假設與選取條件（`annotations.status == "Traced"`，標為工程選取）。相對路徑以 manifest 所在目錄解析，原件保持只讀，讀取前後都比對 SHA-256。報告列出原始列數、選入節點、每一種排除原因、未對應註記的端點、unique／duplicate pair、自環、孤立節點、原始 `weight` 加總、傳導物質預測未知比例與受體 `not_derived` 狀態，每個比例都附分子、分母與依據欄位。記憶體、暫存空間、run 數與列數超限時直接失敗，不會偷偷縮圖。`weight` 是來源原始值，不是突觸數；重複 pair 預設保留每一列，只有 manifest 宣告可加總分割時才允許 `--edge-view aggregated_pairs`。加上 `--out-store graph.coimgraph` 會把 `annotated_neurons` 視圖與報告以不覆寫方式落盤，輸出改為 `{report, store}`；之後用 `./bin/coimnet data validate --store graph.coimgraph` 在新程序讀回，逐段比對 SHA-256、footer、結構不變量與 node index／edge order／report hash 後印出報告。同一張圖兩次落盤位元組相同。store 只記錄 weights 原件的路徑與指紋，不複製原件，讀回後串流 raw view 仍會先比對指紋。

不經訓練直接執行接線圖：

```sh
./bin/coimnet simulate run \
  --store data/malecns-v1.0/graph-v1.coimgraph \
  --protocol evidence/NAT-01/protocol-fullgraph-uniform.json \
  > run.json
./bin/coimnet simulate run \
  --store data/malecns-v1.0/graph-v1.coimgraph \
  --protocol evidence/NAT-02/protocol-fullgraph-derived.json \
  --params params-derive-v1.coimparams \
  > run-derived.json
```

`simulate run` 把 store 的節點與邊直接接上連續核心或 LIF 核心，用固定注入把刺激送進指定神經元，再以具名探針讀出指定神經元的活動。過程不經 encoder、readout、最佳化器或任何訓練步驟，也沒有可訓練矩陣。protocol 是嚴格 JSON，宣告核心設定（不含拓撲，節點與邊一律由圖提供）、注入、探針、刺激、門檻與參數來源。探針的 `reduce` 可選 `mean_output`、`sum_output`，LIF 另有 `spike_count` 與 `spike_fraction`；節點可直接給索引，也可用 `class`、`type`、`superclass`、`subclass`、`instance`、`soma_side` 的選擇器解析，解析結果寫進報告。報告含核心設定、圖、參數與 protocol 四個指紋、每步探針時序、沉默比例、每步全群放電比例、放電率分位數與穩定旗標。相同輸入兩次執行的 JSON 位元組相同；`--state-out` 保存狀態，`--state-in` 接續，分兩段跑的結果與一次跑完相同。

參數必須明示來源，目前有兩種。`engineering_uniform_positive` 不需要 `--params`：每條邊的權重是 `gain × 原始 weight`（因此全部是興奮性），每顆神經元共用同一組 bias、log_tau 與 theta_raw，延遲一律為零。`derived_release/v1` 必須加 `--params`，讀 `data derive` 產生的參數集，權重為 `weight_scale × 推導 sign × 推導強度`；protocol 要多一個 `derived` 區塊宣告 `unknown_sign`（`exclude` 權重為 0、`excitatory` 取 +|w|、`inhibitory` 取 −|w|）與大於零的 `weight_scale`，兩個欄位都沒有預設值，unknown 的邊數寫進報告。這個來源的節點純量仍由 protocol 的 `uniform` 區塊提供，所以該區塊的 `gain` 必須不寫或為零；命令會先比對參數集與 store 的 node index／edge order hash，不符就拒絕。

**兩種來源都是明示假設，不是生物參數。** uniform 的全興奮與統一純量是工程佔位；derived 的正負號是由「預測傳導物質機率」依規則推導的，不是量測到的突觸作用，而 bias、log_tau、theta_raw 與零延遲仍然是統一工程值。報告的 `assumptions` 會依來源原樣寫出這些話。要把觀察歸因於接線本身還需要 [ticket 14](docs/tickets/14-null-models-and-behavior.md) 的空模型對照。全圖實測與完整數據見 [evidence/NAT-01/verification.json](evidence/NAT-01/verification.json)（uniform）與 [evidence/NAT-02/verification.json](evidence/NAT-02/verification.json)（derived，含兩者的逐項比較）。

由發布資料推導動態參數：

```sh
./bin/coimnet data derive \
  --store data/malecns-v1.0/graph-v1.coimgraph \
  --rules rules.json \
  --out params.coimparams \
  > derive.json
./bin/coimnet data validate --params params.coimparams --store data/malecns-v1.0/graph-v1.coimgraph
```

`data derive` 讀四份官方原件（`body-stats`、`tbar-neurotransmitters`、`syn-partners`、`Neuprint_Meta.csv`），依規則檔 `coimnet-derivation-rules/v1` 產生每條邊的**正負號、信心度、傳導物質、配對突觸數**與**正規化強度**，以及每個神經元的 pre／post 突觸數與主要 ROI，寫成不覆寫的參數集檔 `coimnet-parameter-set/v1`。規則檔用相對路徑與 SHA-256 指定四份原件，開始排序前先逐一比對指紋。

**正負號是規則推導的，不是量測值。** 發布資料只有每個突觸前位置的傳導物質「預測機率」；命令取配對突觸的平均機率、選出機率最高的傳導物質，再用規則檔的對應表換成 `+1`／`-1`／unknown。果蠅 glutamate 多為抑制屬工程假設，規則檔的 `basis` 必須寫出「假設」或 "assumption" 才會通過驗證。**未知保持未知**：沒配到突觸、平均機率低於 `min_probability`、配對比例低於 `min_matched_fraction`，或規則本身把該傳導物質標為 unknown 的邊，正負號一律是 unknown，不補預設值；報告分開計算這四種原因。強度是 `gain × 原始 weight ÷ normalizer`，`normalizer` 可選 `none`、`post_total`（目標的 body-stats post）或 `pre_total`（來源的 pre），normalizer 為 0 的邊權重為 0 並計數。

推導報告寫在 JSON 輸出裡，也嵌在參數集檔中：四份來源指紋、規則 hash、每一步的計數與比例（附分子、分母與依據）、每種傳導物質的邊數、sign 分布、unknown 比例、強度分位數、`normalizer_zero_edges`、四個外部排序的 run 數與暫存位元組，以及耗時。整條流程有界：逐突觸資料一律走 `internal/extsort` 的固定寬度記錄，記憶體、暫存、run 數、Arrow 與列數都有上限，超限或取消時不產生輸出也不留暫存檔。`data validate --params` 在新程序讀回，逐段比對 SHA-256、footer 與結構不變量；加 `--store` 會再比對 node index／edge order hash，確認參數集確實屬於那張圖。同一份參數集兩次落盤位元組相同。

**全圖實測（2026-09-15）**：25,563,197 條邊全部推導完成，耗時 207.31 s、最大 RSS 3.87 GB（6 GiB 記憶體與 40 GiB 暫存上限都沒觸及），參數集檔 463,451,966 B。逐突觸座標的配對比例是 124,025,046／124,025,046 = 1.000（分母是兩端神經元都被選入的突觸列），沒有重複座標的 T-bar，也沒有配不到的突觸；正負號為 +13,636,628（53.3%）／−9,172,513（35.9%）／unknown 2,754,056（10.8%），unknown 全部來自「平均機率低於 0.5」1,038,293 與「規則把該傳導物質標為 unknown」1,715,763，沒有任何一條是因為配不到突觸。完整數字（每種傳導物質的邊數、強度分位數、四個排序的 run 數與暫存量）見 [evidence/NAT-02/verification.json](evidence/NAT-02/verification.json)，規則檔原件見 [evidence/NAT-02/rules-derive-v1.json](evidence/NAT-02/rules-derive-v1.json)。

產生的參數集用 `simulate run --params` 執行同一張圖：以 `unknown_sign: exclude`、`weight_scale: 21.6` 跑完 NAT-01 的同一份 protocol 後，沉默比例由 0.0207 升到 0.6440、最大全群放電比例由 0.5467 降到 0.0793，穩定旗標也由 `max_population_rate_exceeded` 變成沒有旗標。這是兩組參數假設在同一張接線圖上的差異，不是果蠅生理的結論。

比較原圖與空模型：

```sh
./bin/coimnet simulate compare \
  --store data/malecns-v1.0/graph-v1.coimgraph \
  --protocol compare.json \
  --params params-derive-v1.coimparams \
  --out-dir cells \
  > compare.json
```

`simulate compare` 用同一份 protocol、同一份刺激先跑原始接線，再跑每一種空模型的每一個 seed，每一格都是一份完整的 `simulate run` 報告。加上 `--out-dir` 會把每一格另存成 `cell-<編號>-<變體>.json`，那個目錄必須還不存在。

**空模型是記憶體裡的衍生物，不會另存成一張圖。** 三種空模型都不改 store 與參數集檔。`degree_preserving_rewire` 固定每個節點的出度與入度，用 double-edge swap 交換兩條邊的目標，嘗試次數是 `ceil(swap_factor × 邊數)`，提議只要會做出自環或重複 pair 就拒絕，兩種原因分開計數，每條邊的權重與正負號跟著自己那條邊走。`sign_shuffle` 只把正負號標籤在邊之間置換，`+1`／`-1`／unknown 各自的數量不變，而且只有 `derived_release/v1` 能用，因為 uniform 的邊全部是興奮性，沒有標籤可以換。`weight_shuffle` 只置換強度，正負號留在原來的邊上。

亂數是 `math/rand/v2` 的 PCG(seed, 0)，報告寫出 kind、seed、`prng`、嘗試次數、成功次數、兩種拒絕原因，以及衍生拓撲與參數的 SHA-256。拿原本的 store 加上這幾個欄位就能重算出同一份衍生物再比對 hash，所以不需要把假的「來源指紋」寫進第一層的圖格式。

**指標與門檻寫在 protocol 裡，跑之前就定案。** 具名神經元集合是多個選擇器的交集，報告記錄每個選擇器解析到什麼、節點數，以及節點索引的 SHA-256。集合名稱由使用者給，框架不預設任何集合代表某種行為。LIF 核心可用 `spike_fraction`、`mean_rate`、`latency_to_first_spike` 與 `activity_ratio_vs_baseline`，連續核心沒有事件，只能用 `mean_output`。窗是 `[start, end)` 的步數範圍，超出這次執行的步數就拒絕。

**沒有值的指標就是未定義，不是 0。** 集合在窗內完全沒有放電時 latency 沒有值，baseline 的放電率為 0 時 activity ratio 沒有值，門檻碰到未定義的指標一律 fail 並記下 `undefined`。門檻結果只寫進報告，不改退出碼。

多個 seed 的結果以分布呈現。每一種空模型對每個指標給 p0、p25、p50、p75、p100（最近秩，不內插），以及原圖的值落在該分布的百分位 `(小於的個數 + 0.5 × 相等的個數) ÷ 有定義的個數`。原圖或全部 seed 都沒有值時就不給百分位。每一格還會給每個指標與原圖的差 `deltas_from_original`，兩邊都有定義才有值。報告只陳述這些數字與事前宣告的門檻結果，不做沒有宣告過的檢定，也不宣稱任何生物行為。

真實全圖跑過兩次矩陣，每次十格（原圖加三種空模型 × seed 1、2、3），都是 165,122 個神經元與 25,563,197 條邊的 300 步。集合由註記解析成 `alin` 24 個、`descending_neuron` 1,314 個、`vnc_motor` 708 個節點。LIF 矩陣 890.94 s、最大 RSS 5,909,626,880 B；連續核心矩陣 929.97 s、最大 RSS 5,659,115,520 B；兩邊每一格的拓撲與參數 hash 逐格相同。指標的位置會隨核心改變：LIF 的 `descending_neuron_mean_rate` 原圖 0.0879，在三種空模型的三個 seed 分布中百分位都是 0（比每個 seed 都低），但連續核心的 `descending_neuron_mean_output` 原圖在 rewire 與 weight_shuffle 的分布中百分位是 1。三個 seed 的百分位只是位置，不是檢定。證據見 `evidence/NAT-03/`、`NAT-04/` 與 `NAT-05/`，指令封裝在 `scripts/compare-evidence.sh`。

真實子圖的選取與短訓練見 [ALIN 範例](examples/realsubgraph/README.md)。範例明示人工脈衝任務、初始化假設及更新範圍，輸出來源與參數指紋。

[多通道 adapter 範例](examples/multichannel/README.md) 將不同頻率的連續值、區間與脈衝轉成六欄輸入，接到既有核心與訓練器。執行 `go test ./examples/multichannel -run ExampleAdapt -count=1 -v` 可跑人工資料的完整流程。

## Go SDK

- `dynamics.NewContinuous` 建立同步稀疏連續模型。`Forward` 支援延遲，`Backward` 提供完整或固定視窗梯度。
- `dynamics.NewLIF` 用同一套拓撲、延遲與時鐘建立 LIF 放電核心。神經元達到閾值就產生一次事件並把電位重設，對外輸出是會衰減的突觸跡，不應期內保持重設值並忽略當步輸入。`Backward` 依宣告的 `fast_sigmoid` 替代梯度回推，重設分支不傳梯度。
- `learning.Config.LIF` 與 `Config.Dynamics` 二選一。選用 LIF 後，`Parameters.ThetaRaw` 是各神經元的閾值參數，經有界轉換得到 `theta_base`。`Trainable.Theta` 決定要不要訓練這一組，`Trainer.Spikes` 回傳每步的 0／1 事件。`LIFConfig.Homeostasis` 是可選的慢速穩定區塊，開啟後每顆神經元多一個活動估計與一個只會抬高或回落到零的閾值偏移，放電判定用的是 `theta_base + 適應 + 偏移`；不宣告這個區塊的設定，編碼與既有指紋完全不變。
- `learning.NewNetwork` 將 Insyra 編碼器及讀出接到核心，`LossGradient` 回傳整條路徑的梯度。
- `learning.NewTrainer` 使用可保存的 AdamW 狀態。凍結參數群組時，權重、動量、步數與衰減一起凍結。
- `signal` 提供具版本訊號、時鐘驗證與事件排序、觀察／答案／回饋分離及無損外部 ID 映射。`NewStreamingResampler` 支援連續值的因果取樣，`ResampleOffline` 另支援離線線性插值。`NewPulseAligner` 將脈衝對齊至當下或下一個神經步號，逐筆保留事件。`NewIntervalResampler` 依起訖時間取樣固定值區間，詳見[時間對齊與限制](docs/signal-resampling.md)。
- `signal.NewProjection` 保存輸入／輸出係數、選取條件與神經元指紋。`learning.BindProjections` 將映射接到實際圖，支援保存後重建及建立獨立替換模型，詳見[映射指南](docs/signal-projections.md)。
- `checkpoint.NewState`、`Save`、`Load` 提供獨立序列快照，`learning.RestoreTrainer` 重建隔離訓練器。
- `learning.NewIndividual` 對連續與 LIF 兩種核心建立隔離的持續個體，`ResetNeural`／`ResetParameters`／`ResetOptimizer` 各自只重設指定資料。`checkpoint.SaveIndividual`／`LoadIndividual` 保存持續神經狀態，LIF 個體另外保存突觸跡、適應、不應期與慢速穩定狀態，詳見[個體狀態與保存](docs/individual-state.md)。
- `checkpoint.NewModelPackage`、`SaveModelPackage`、`LoadModelPackage` 保存模型包，`NewIndividualFromPackage` 由模型包建立新個體，結果與用同一份設定、參數直接呼叫 `learning.NewIndividual` 相同。
- `download.Fetch` 提供容量限制、取消、有限重試、版本檢查、續傳及來源回條。
- `feather.Scan` 以 callback 逐批讀取 Feather V2，保留整數、缺值、字典與 list。批次資料在 callback 期間有效，需保留時呼叫 `Retain`，使用完畢後 `Release`。
- `connectome.Build` 依 `DatasetManifest` 與 `ResourceLimits` 建立不可變的 `Graph` 與 `GraphReport`。`Node`、`IndexOf`、`NeuronIDs` 提供無損外部 ID 與連續索引的雙向對照；`StreamAnnotatedNodes`／`StreamAnnotatedEdges` 依固定順序串流選入視圖；`StreamRawSegments` 重新逐批讀取 weights 原件並保留每一列。重複 pair 以有界外部排序的相鄰 run 計數，不建立全量 pair map。
- `simulate.Build` 以 `connectome.Graph`、`ParameterSet` 與 `Protocol` 建立核心無關的 `Runner`，`Run` 推進持續狀態並回傳 `RunReport`，`State`／`RestoreState` 保存與接續。`simulate.UniformPositive` 由原始 weight 產生 `engineering_uniform_positive` 參數集。套件不使用 Insyra，也不碰 `learning`。
- `connectome.Save`、`Load` 以固定區段格式落盤與讀回同一個 `Graph`：每段與 footer 都有 SHA-256，讀回時重算 node index／edge order／report hash 並檢查順序與索引範圍，不符即 `ErrStoreCorrupt`。`LoadWithReceipt` 另回傳實際驗證位元組的 SHA-256，供 CLI 報告使用。檔案、footer 與記憶體受 `StoreLimits` 限制。
- `params.DecodeRules` 讀取 `coimnet-derivation-rules/v1`，`params.Derive` 依規則由四份發布原件推導每條邊的正負號、信心度與強度，`Save`／`Load`／`LoadWithReceipt` 以區段加 footer 加 SHA-256 的格式落盤與讀回，`Set.CheckGraph` 比對 node index／edge order hash。套件不使用 Insyra，也不碰 `learning` 與 `simulate`。

`signal.NewSignal` 接受呼叫者已解碼的數值來源，連續、活動、脈衝與調節的具名訊號可用 `go test ./signal -run ExampleNewSignal -count=1 -v` 查看保存與讀回範例。訊號 JSON 的數值欄位拒絕 `null`，例如 `values:[null]` 不會被當成零。`quality.score` 與整個 `valid_range` 可用 `null` 表示未知，省略原本可省略的數值欄位則維持既有預設。從 JSON 建立訊號請使用 `DecodeSignal`，不要先以一般 JSON 解碼器讀入 `SignalSpec`，以免在驗證前就遺失缺值資訊。

`Trainer.Step`／`Predict` 與 `Network.Predict` 從零神經狀態開始獨立序列，只讀取最後一步輸出。持續個體則使用 `Individual.Advance`，接續電位與延遲歷史，回傳每一步輸出。CPU 動態使用 float64，Insyra 編碼器、讀出與損失使用 float32。完整 API 可用 `go doc ./learning` 與 `go doc ./dynamics` 查閱。

訊號套件的嚴格 JSON 會在解碼前拒絕非法 UTF-8 與未配對的 Unicode surrogate，避免來源名稱或外部 ID 被改寫。

訊號、快照、下載續傳資料與圖資料的嚴格 JSON 解碼皆限制巢狀路徑深度為 64 層，並拒絕 Unicode 大小寫別名重複欄位。例如 `schema_version` 與 `ſchema_version` 會被 Go 解碼器視為同一欄位，因此同時出現時拒絕輸入。

## 開發驗證

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./...
go mod verify
```

在 macOS 或 Linux 可用 `scripts/verify.sh NEW_OUTPUT_DIRECTORY` 一次執行上述檢查、三組學習對照，以及真正跨程序的 CLI 續訓比對，並執行 LIF 閾值範例及其預註冊門檻，保存環境報告、來源指紋與日誌。目錄須尚未存在。已下載三份官方原件時，`scripts/graph-evidence.sh NEW_OUTPUT_DIRECTORY` 會在計時下重跑真實接線圖建構並保存報告、環境與指紋。

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
