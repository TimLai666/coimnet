# CoImNet

**Connectome-Imprinted Network** 是以真實果蠅神經接線建立可訓練模型的框架，使用 Go 與 Insyra。目標是讓使用者定義輸入訊號、訓練神經動態與閾值，並保存及恢復模型的學習狀態。

儲存庫提供通用 SDK、CLI 與驗證範例。使用者自己的任務程式、資料與訓練模型由使用者專案管理。本文的 `delayed`、`experiment` 套件與小型快照證據都是人工教學／測試範例，不是預訓練果蠅模型。

目前的連續核心屬於稀疏的連續時間循環神經網路。接線圖決定哪些神經元相連，神經動態與學習方法則由框架提供。詳見[模型定位與可選機制](docs/model-and-mechanisms.md)，以及[記憶體與時間量測](docs/resources.md)。

目前已實作 CPU 連續動態、完整與截斷時間梯度、Insyra 輸入與讀出、AdamW 訓練，以及人工延遲訊號範例。LIF 放電核心可以用同一套拓撲、延遲與時鐘執行，並以宣告的替代梯度訓練基礎放電閾值，也可以開啟慢速穩定，讓每顆神經元依自己的活動估計調整閾值偏移。連續與 LIF 都能建立持續個體，把電位、延遲歷史、突觸跡、適應、不應期與慢速穩定狀態一起保存和接續。模型包、個體快照與訓練快照是三種分開版本的保存物，彼此不能互相當成完整恢復來源。官方 MaleCNS 資料可下載、校驗、逐批讀取 Feather，並依明示的 manifest 建成 `raw_segments` 與 `annotated_neurons` 兩個具名視圖及統計報告。選入圖可保存為固定格式並在新程序驗證、讀回。`examples/realsubgraph` 示範將選出的 ALIN 子圖接上訓練核心。完整圖訓練、按類型混合、化學調節、五類任務與 GPU 核心尚未完成，完整需求以[開發進度](delivery-status.md)追蹤。

## 能力狀態

未訓練的模型只能稱為初始化模型，人工圖與真實圖的名稱要能辨識（fixture 人工圖／real-subgraph 真實子圖／male-full 真實全圖），工具輔助的結果不計為核心能力。下表只列 [docs/requirements-status.json](docs/requirements-status.json) 標為 `passed` 且證據檔存在的需求群，每一格的規模依證據內容標注；模型類能力中，真實全圖只有 NAT-01～06 那幾列使用 `male-full`，其餘一律 `fixture` 或 `real-subgraph`，純資料管線與工具類能力則標其實際資料規模。

| 能力 | 模型狀態 | 資料規模 | 角色 | 證據 |
| --- | --- | --- | --- | --- |
| 連續核心訓練：可訓練稀疏連續動態、真實連線計算、完整／截斷梯度、受限更新與完整最佳化器（COR-02、COR-07、COR-08、COR-10、LRN-01、LRN-03） | 已訓練（人工資料） | fixture | 核心能力 | [COR-02](evidence/COR-02/verification.json)・[COR-07](evidence/COR-07/verification.json)・[COR-08](evidence/COR-08/verification.json)・[COR-10](evidence/COR-10/verification.json)・[LRN-01](evidence/LRN-01/verification.json)・[LRN-03](evidence/LRN-03/verification.json) |
| 反向歷史重算：訓練步不保存核心歷史、反向逐段重算，梯度與完整歷史逐位相同，不重複學習、隨機事件或教師呼叫（LRN-02） | 已訓練（人工資料） | fixture | 核心能力 | [LRN-02](evidence/LRN-02/verification.json) |
| LIF 放電核心：替代梯度閾值訓練、慢速穩定與個體持續狀態（COR-03、COR-04、COR-09） | 已訓練（人工資料） | fixture | 核心能力 | [COR-03](evidence/COR-03/verification.json)・[COR-04](evidence/COR-04/verification.json)・[COR-09](evidence/COR-09/verification.json) |
| 混合核心：連續／脈衝依神經元類型混合執行（COR-05） | 初始化模型（未訓練） | fixture | 核心能力 | [COR-05](evidence/COR-05/verification.json) |
| 向量節點與共享參數：純量或多維節點、共享或逐項參數、分開回報容量（COR-06） | 已訓練（人工資料） | fixture | 核心能力 | [COR-06](evidence/COR-06/verification.json) |
| 訊號、時間對齊與映射：具名訊號、因果／離線取樣、輸入輸出映射保存與替換（SIG-01、SIG-02、SIG-04、SIG-05、SIG-06） | 初始化模型（資料層） | fixture | 核心能力 | [SIG-01](evidence/SIG-01/verification.json)・[SIG-02](evidence/SIG-02/verification.json)・[SIG-04](evidence/SIG-04/verification.json)・[SIG-05](evidence/SIG-05/verification.json)・[SIG-06](evidence/SIG-06/verification.json) |
| 個體與保存：解剖／參數／個體／訓練器狀態分離、三種保存物、安全快照與隔離個體（COR-01、STA-01、STA-02、STA-04） | 已訓練（人工資料） | fixture | 核心能力 | [COR-01](evidence/COR-01/verification.json)・[STA-01](evidence/STA-01/verification.json)・[STA-02](evidence/STA-02/verification.json)・[STA-04](evidence/STA-04/verification.json) |
| 精確中斷接續：連續執行對照下同時恢復參數、最佳化器、快速權重、化學與資料游標（STA-03） | 已訓練（人工資料） | fixture | 核心能力 | [STA-03](evidence/STA-03/verification.json) |
| 調節來源與化學：觀察／目標／回饋分離、四種調節來源、獎懲映射、時間衰退的濃度、選擇性受體與效果（SIG-03、MOD-01、MOD-02、MOD-03、MOD-04、MOD-08） | 初始化模型（未訓練） | fixture | 核心能力 | [SIG-03](evidence/SIG-03/verification.json)・[MOD-01](evidence/MOD-01/verification.json)・[MOD-02](evidence/MOD-02/verification.json)・[MOD-03](evidence/MOD-03/verification.json)・[MOD-04](evidence/MOD-04/verification.json)・[MOD-08](evidence/MOD-08/verification.json) |
| 局部可塑性規則：活動相關與脈衝時序規則、近期參與紀錄與學習閘門（LRN-04、LRN-05） | 初始化模型（未訓練） | fixture | 核心能力 | [LRN-04](evidence/LRN-04/verification.json)・[LRN-05](evidence/LRN-05/verification.json) |
| 運行中學習與重播：作答／回饋／更新分離、受限容量重播與適應性評估（LRN-06、LRN-07） | 已訓練（人工資料） | fixture | 核心能力 | [LRN-06](evidence/LRN-06/verification.json)・[LRN-07](evidence/LRN-07/verification.json) |
| 持續學習矩陣：階段後全任務評估、遺忘、固定化學與狀態切換對照、independent 對照、事前 bootstrap 比較（LRN-08） | 已訓練（人工資料） | fixture | 核心能力 | [LRN-08](evidence/LRN-08/verification.json) |
| 環境學習：專家模仿與取樣式循環 PPO、狀態／版本一致、時間上限的下一步價值、三個 seed 的訓練前與隨機對照（LRN-09） | 已訓練（人工走廊） | fixture | 核心能力 | [LRN-09](evidence/LRN-09/verification.json) |
| 二維導航與位置記憶：有限視野、隱藏狀態、碰撞與轉向、目標記憶、新地圖評估，前饋／重連／隨機對照（TSK-08） | 已訓練（人工地圖） | fixture | 核心能力 | [TSK-08](evidence/TSK-08/verification.json) |
| 多模態配對資料：配對／缺失／非同步樣本、ID 不進模型、缺失與零值分離、未見組合分割與評估（TSK-10；留出組合的影像↔文字檢索為 0，不宣稱跨模態概念） | 已訓練（合成資料） | fixture | 核心能力 | [TSK-10](evidence/TSK-10/verification.json) |
| 核心、外圍與接線的歸因對照：凍結核心、只訓練核心、一般網路、重連、移除後重訓、容量匹配調節器共七組，事前配對 bootstrap 與排查清單（TSK-12；本次 normal 基準沒學會，不支持依賴或接線較優的結論） | 已訓練（人工地圖） | fixture | 核心能力 | [TSK-12](evidence/TSK-12/verification.json) |
| 有來源的生物啟發干預協定：脈衝時機曲線與受體驅動的表現抑制／恢復對照，定量生物命名缺四項證據即拒絕（MOD-09） | 已訓練（人工資料） | fixture | 核心能力 | [MOD-09](evidence/MOD-09/verification.json) |
| 小型可訓練調節控制器與對照流程：控制器手算前向／有限差分／容量報告、無題目旁路、容量匹配對照，`examples run ablate` 五組同資料同預算對照（MOD-07、MOD-10） | 已訓練（人工資料） | fixture | 核心能力 | [MOD-07](evidence/MOD-07/verification.json)・[MOD-10](evidence/MOD-10/verification.json) |
| 教師蒸餾：學生自編碼的標籤／行動蒸餾、保留集隔離、合法對齊的分布蒸餾（TCH-03、TCH-04） | 已訓練（人工資料） | fixture | 核心能力 | [TCH-03](evidence/TCH-03/verification.json)・[TCH-04](evidence/TCH-04/verification.json) |
| OCR 單行辨識：log-space CTC 與枚舉對照、rune 層級 CER／WER、程式生成字形的訓練測試分離、投影切行的頁面區塊契約、核心斷開檢查（TSK-01；真實授權資料 blocked_data，需使用者提供含授權欄位的資料集，目前只驗到 fixture） | 已訓練（程式生成字形） | fixture | 核心能力 | [TSK-01](evidence/TSK-01/verification.json) |
| 原生模擬 runner：不經訓練直接執行標準接線圖並讀出指定神經元（NAT-01） | 初始化模型（未訓練） | male-full | 核心能力 | [NAT-01](evidence/NAT-01/verification.json) |
| 動態參數 adapter：由發布資料推導正負號、信心度與強度並回報未知計數（NAT-02） | 初始化模型（未訓練） | male-full | 核心能力 | [NAT-02](evidence/NAT-02/verification.json) |
| 空模型對照與判讀：保留度數重連、正負號／權重置換、具名集合與事前門檻（NAT-03、NAT-04、NAT-05） | 初始化模型（未訓練） | male-full | 核心能力 | [NAT-03](evidence/NAT-03/verification.json)・[NAT-04](evidence/NAT-04/verification.json)・[NAT-05](evidence/NAT-05/verification.json) |
| 原生模擬上的可塑性對照：original／plastic／learned_then_frozen 三格比較（NAT-06） | 初始化模型（未訓練，快速變化不寫回參數集） | male-full | 核心能力 | [NAT-06](evidence/NAT-06/verification.json) |
| 官方資料工具：下載與校驗、Feather 逐批讀取、標準化接線圖建構、外部 ID 保真、聚合與未知欄位保留（DAT-01、DAT-02、DAT-03、DAT-04、DAT-05、DAT-08） | 不適用（資料管線） | 官方全量原件（非模型執行） | 工具輔助 | [DAT-01](evidence/DAT-01/verification.json)・[DAT-02](evidence/DAT-02/verification.json)・[DAT-03](evidence/DAT-03/verification.json)・[DAT-04](evidence/DAT-04/verification.json)・[DAT-05](evidence/DAT-05/verification.json)・[DAT-08](evidence/DAT-08/verification.json) |
| FlyWire 獨立匯入：公開釋出 CSV／gzip 轉 Feather 與 manifest、`flywire-<version>` 獨立命名空間與大 root id 字串精確往返、無映射證據一律拒絕與 MaleCNS 拼接（DAT-06；真實 FlyWire 檔案 blocked_data，需帳號與條款同意，目前只驗到 fixture） | 不適用（資料管線） | fixture | 工具輔助 | [DAT-06](evidence/DAT-06/verification.json) |
| 真實子圖範例：ALIN 子圖選取與人工脈衝短訓練（DAT-07） | 已訓練（人工脈衝短訓練） | real-subgraph | 工具輔助（範例） | [DAT-07](evidence/DAT-07/verification.json) |
| 教師工具：離線 JSONL 答案、重播、HTTP 存取與封鎖教師（TCH-01、TCH-02） | 不適用（教師工具） | fixture | 工具輔助（教師） | [TCH-01](evidence/TCH-01/verification.json)・[TCH-02](evidence/TCH-02/verification.json) |
| 學生獨立評估與外部回應安全：student 模式封鎖教師與網路、teacher_assisted 分開報告、20% 錯標籤穩健性、工具允許清單與參數 schema、回應只當資料不外傳未授權欄位（TCH-05、TCH-06）。這些教師與工具功能不把 CoImNet 擴大成任意電腦操作 agent：回應只當資料，不執行其中的指令 | 不適用（評估與工具） | fixture | 核心能力與工具輔助 | [TCH-05](evidence/TCH-05/verification.json)・[TCH-06](evidence/TCH-06/verification.json) |
| SDK 與 CLI 介面：八個可執行 SDK 範例、88 個非法輸入不 panic 的案例、五個取消釋放測試，以及 doctor／model inspect／model validate／run --dry-run／benchmark／export／report／examples／checkpoint migrate 的端到端與失敗測試（OPS-01、OPS-02） | 不適用（介面層） | fixture | 工具輔助 | [OPS-01](evidence/OPS-01/verification.json)・[OPS-02](evidence/OPS-02/verification.json) |
| 環境與自動驗證：鎖定版本建置、Insyra 實際參與運算、自動防退步檢查（GOV-02、GOV-03、OPS-09） | 不適用（環境與測試） | 不適用 | 工具輔助 | [GOV-02](evidence/GOV-02/verification.json)・[GOV-03](evidence/GOV-03/verification.json)・[OPS-09](evidence/OPS-09/verification.json) |

以上都是「能力已驗證」的陳述，不表示這些能力在同一張圖或同一台機器上已全部組合完成；組合性的完整執行仍以 [delivery-status.md](delivery-status.md) 為準。

## 科學界線

下列界線逐條對應[主規格 3.2](docs/handoff/CoImNet_Implementation_Plan.zh-TW.md#chapter-03)：

- 接線圖不是已訓練好的全能模型：接線只決定哪些神經元相連，動態、參數與學習方法是明示的額外假設，未經訓練的圖只能稱為初始化模型。
- 神經傳導物質預測不等於已量測的突觸作用正負：推導的正負號由「預測機率」依規則換出，不是量測值，未知的邊必須保持未知。
- 未知受體不是沒有受體：查無受體表現資料時以 `unresponsive`／`unknown`／`hypothesized` 分開報告，採用未知係數必須明示為假設。
- 數千萬個突觸接點不等於同樣數量的獨立參數：接點數、神經元配對數與參數量分開統計，接線規模不能自動當成可訓練參數量。
- 獎勵、懲罰、損失函數、神經調節物質、內分泌荷爾蒙是不同概念：可以互相映射，但不能改名後宣稱生物合理。
- 固定編碼器不等於能力歸因已完成：固定輸入輸出映射的貢獻必須以對照（例如空模型）確認才算歸因。
- 專案名稱不構成新的學術分類或優於既有方法的證據：CoImNet 只是框架名稱，不是已驗證的生物或學術宣稱。
- 全腦或 GPU 測試受硬體限制時標示受阻：缺少資料或硬體的項目標 `blocked_*`，不拿小型資料或人工圖的成功替代全圖或平台執行。

## 建置與範例

```sh
go build -o bin/coimnet ./cmd/coimnet
./bin/coimnet --help
./bin/coimnet doctor
./bin/coimnet examples run delayed
./bin/coimnet examples run lif-threshold
./bin/coimnet examples run gridnav --method ppo
```

`delayed` 是三個人工神經元的五步延遲訊號任務，使用三組固定 seed。每組都執行訓練、凍結核心及打亂答案對照。輸入映射與讀出保持固定，只有核心連線權重可更新。JSON 輸出包含每組結果、設定及指紋，門檻未通過會傳回非零退出碼。

`lif-threshold` 把同一份延遲資料換成三顆 LIF 放電神經元，只比較基礎閾值可不可訓練。三組固定 seed 各跑全部可訓練、只訓練閾值與全部凍結三個對照，報告列出每組的保留資料 MSE、逐顆 `theta_base` 與放電率。更新預算用 `--updates` 調整，預設 300，範圍 1 到 100000。事前寫死的門檻沒過時仍輸出完整報告，並傳回非零退出碼。這是人工資料上的數值可學習性檢查，不是果蠅放電行為成績，詳見[範例說明](examples/lifthreshold/README.md)。

`gridnav` 示範在人工走廊進行 PPO 或專家模仿，輸出三個 seed 的訓練與評估報告。PPO 預設每個 seed 更新 200 次，平均回報須同時超過訓練前與隨機基線。使用 `--method imitation --updates 500` 可執行既有模仿一致率改善協定，詳見[走廊範例](experiment/gridnav/README.md)。

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

個體快照可以遷移到新檔而不動原件：`./bin/coimnet checkpoint migrate --src old.json --dst migrated.json` 讀 `--src`、寫 `--dst`（已存在的路徑拒絕、同路徑拒絕），印出含前後 SHA-256、欄位變更清單與資訊損失的 JSON 報告；聯集出現以前的連續個體檔會升級成 `coimnet-individual-checkpoint/v1` 聯集形，同版本則逐位複製。

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

在原生模擬之上開啟可塑性：

```sh
./bin/coimnet simulate compare \
  --store data/malecns-v1.0/graph-v1.coimgraph \
  --protocol evidence/NAT-06/compare-fullgraph-derived-plastic.json \
  --params params-derive-v1.coimparams \
  --out-dir cells \
  > compare.json
```

protocol 多一個 `plasticity` 區塊就會在執行中開啟局部可塑性，`simulate run` 與 `simulate compare` 都吃這個區塊，兩個命令都沒有新旗標。`rule` 選 `hebbian_rate`（LIF 核心另有 `stdp_pair`）並帶 `decay_e`、`decay_p`、`plastic_max` 與 `w_min`；`all` 與 `edges` 選擇器二擇一決定哪些連線參與，選擇器解析出一組神經元後啟用「兩端都在這組裡」的邊；`gate_channel` 與 `gate_scale` 直接從刺激矩陣的一個通道讀學習閘門，所以不必另外給一份輸入。閘門通道必須是刺激宣告的通道，而且不能是任何注入使用的通道，否則同一個通道既驅動神經元又開閘門，事後分不出是哪一個造成的。

每一步的權重是「參數集的權重加上該條邊有上限的快速變化」，在推進之前重算；固定符號的邊被 `w_min` 擋在零的同一側，永遠不會翻成相反的作用；快速變化不寫回參數集。因為下一步的權重取決於這一步的快速變化，開啟後分塊大小強制為 1，報告的 `plasticity` 區塊會寫出規則、啟用邊數、閘門通道、`w_min` 與 `plastic_max` 各擋下幾次、快速變化的 L2 範數（執行前後）、分塊大小與這個耗時代價。沒有宣告 `plasticity` 的 protocol 逐位元等於這個區塊出現以前的執行與報告。

`simulate compare` 的 `learning_variants` 用同一份刺激跑三格：`original` 是把 `plasticity` 區塊拿掉的執行，也是所有差值的基準，報告逐位元等於沒有宣告區塊的那一格；`plastic` 開啟區塊；`learned_then_frozen` 以 `plastic` 跑完的快速變化把權重固定住、不再更新，重跑同一份刺激。三格加上空模型那幾格共用同一組指標與門檻，空模型那幾格不帶可塑性。

**報告只給差值與百分位，不做文字判斷。** 每一格的 `deltas_from_original` 是它與 `original` 的逐指標差，兩邊都有定義才有值；「增強、修改或破壞」由讀者從差值和門檻結果自己判斷，框架不寫這種句子。**規則與四個常數是明示的工程假設**，不是量測到的果蠅可塑性，報告的 `assumptions` 會原樣寫出來。

真實全圖跑過兩次三格矩陣（2026-09-16），同一份 store、同一份參數集與同一份刺激，只有 `decay_p` 不同：0.5 與 0.999。每一格都是 165,122 個神經元、25,563,197 條邊、300 步，`all: true` 啟用全部 25,563,197 條邊。刺激是 NAT-02 的同一個脈衝（通道 0，第 10–29 步）加上通道 1 的閘門（第 30–49 步常數 1）。兩次都是 1252.44 s 與 1217.28 s、最大 RSS 6.61 GB 與 6.56 GB，可塑性那一格各約 610 s 與 599 s。`original` 那一格的四個探針序列、monitors 與 assumptions 與 NAT-02 的全腦單次執行逐位相同。

`decay_p` 0.999 時快速變化撐到最後（L2 範數 11.90），`learned_then_frozen` 拿到的正是那組值：ALIN 的 `mean_rate` 在閘門之後 +0.0585（凍結後 +0.0728），`descending_neuron` 的 `spike_fraction` −0.0723（凍結後 −0.0502），`vnc_motor` 的首次放電延遲在凍結那一格少 3 步。`decay_p` 0.5 時閘門關上之後 `plastic` 每步衰減一半，250 步後整個陣列只剩 8.08e-75，所以 `learned_then_frozen` 的報告與 `original` 位元組相同，十四個有定義的差值全部是 0（剩下那一個三格都未定義），但 `plastic` 那一格仍然有差。**快速變化撐不撐得到最後由 `decay_p` 決定**，不是實作的選擇。完整數字、假設與限制見 [evidence/NAT-06/verification.json](evidence/NAT-06/verification.json)，指令封裝在 `scripts/plasticity-evidence.sh`。

真實子圖的選取與短訓練見 [ALIN 範例](examples/realsubgraph/README.md)。範例明示人工脈衝任務、初始化假設及更新範圍，輸出來源與參數指紋。

[多通道 adapter 範例](examples/multichannel/README.md) 將不同頻率的連續值、區間與脈衝轉成六欄輸入，接到既有核心與訓練器。執行 `go test ./examples/multichannel -run ExampleAdapt -count=1 -v` 可跑人工資料的完整流程。

## Go SDK

新增的任務 SDK 入口：[PPO 更新](learning/rl/README.md)、[音訊轉文字與串流恢復](tasks/asr/README.md)。兩者目前是局部框架能力，完整 LRN-09／TSK-02 驗收狀態見需求追蹤。

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

`Trainer.Step`／`Predict` 與 `Network.Predict` 從零神經狀態開始獨立序列，`Predict` 回傳最後一步輸出。設定 `ReadoutEveryStep` 後可用 `PredictAll` 取得每步輸出，並以 `StepFrom` 傳入每步的輸出梯度。持續個體則使用 `Individual.Advance`，接續電位與延遲歷史，回傳每一步輸出。CPU 動態使用 float64，Insyra 編碼器、讀出與損失使用 float32。完整 API 可用 `go doc ./learning` 與 `go doc ./dynamics` 查閱。

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

commit 前的敏感資料掃描可以選擇安裝，不會自動啟用：`git config core.hooksPath .githooks` 之後，每次 `git commit` 會先執行 `scripts/scan-commit.sh`，掃描暫存 diff 的金鑰樣式、超過 5 MiB 的檔、`data/` 原件目錄新增與 `teacher_response` 原文，違規就中止提交；`git config --unset core.hooksPath` 取消。

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
