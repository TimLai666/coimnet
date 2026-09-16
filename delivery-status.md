# CoImNet 開發進度

## 目前階段

LIF 個體、慢速穩定與模型包（ticket 16、COR-04、STA-01）已在 fixture 驗證：`learning.NewIndividual` 對連續與 LIF 核心走同一條路徑，神經狀態改為指名核心的聯集，舊的連續個體檔仍可讀；LIF 可選的慢速穩定用每顆神經元的活動估計調整閾值偏移，手算時序與收斂數字記在 ticket；`coimnet-model-package/v1` 是第三種保存物，只有拓撲指紋、設定、參數、單位與證據清單，六種交叉載入都會被拒絕並指名讀到的是哪一種，LIF 個體跨程序恢復後續跑與不中斷逐項相等。COR-04、STA-01 標 passed，證據見 [COR-04](evidence/COR-04/verification.json)、[STA-01](evidence/STA-01/verification.json)。

受限更新與完整最佳化器流程（ticket 17、COR-10、LRN-03）已在 fixture 驗證：逐邊／逐節點遮罩讓凍結項在含 weight decay 的 50 步後逐位不變、動量與步數為零；固定符號邊改用對數幅度參數化，`learning_rate=1.0` 跑 1,000 步符號不翻轉、幅度 > 0，鏈鎖梯度與中央差分最差相對誤差 4.91e-05；範圍投影計數正確且不動動量；推導參數集的符號依 free／excitatory／inhibitory 政策帶入並回報計數。全零符號與舊快照逐位等於改前。第二階段：損失縮放前後參數逐位相同、溢位整步拒絕；累積三步等於平均梯度的一次 AdamW（手算）、視窗中途快照接續逐位相同；三種排程的學習率表手算、恢復後接續；裁切在平均之後、AdamW 之前。COR-10、LRN-03 標 passed，證據見 [COR-10](evidence/COR-10/verification.json)、[LRN-03](evidence/LRN-03/verification.json)。

局部可塑性（ticket 18 第一階段、LRN-04、LRN-05）與調節來源（ticket 20、SIG-03、MOD-01、MOD-08）已在 fixture 驗證：`plasticity` 套件的活動相關與脈衝時序規則各有手算時序（閘門關只衰退、延遲閘門用殘留參與紀錄、上限與 `w_min` 計數、固定符號不跨零），持續個體的 `AdvanceGated` 逐步套用新的有效權重，關閉時逐位還原，快照帶可選的 `plastic` 區塊；`modulation` 套件的四種來源（外部時序、具名集合活動、內在資源、重播）只產生非負有限的釋放率，獎懲映射把原始值、期望值、轉換值與套用值分開，負回饋變成清除項而不是負濃度；`scripts/check-target-flow.sh` 與 NaN 汙染測試證明目標只進損失。證據見 [LRN-04](evidence/LRN-04/verification.json)、[LRN-05](evidence/LRN-05/verification.json)、[SIG-03](evidence/SIG-03/verification.json)、[MOD-01](evidence/MOD-01/verification.json)、[MOD-08](evidence/MOD-08/verification.json)。

空模型對照與判讀協定（ticket 14、NAT-03／04／05）已在真實全圖驗證：`simulate compare` 用同一份刺激跑原圖與三種空模型（保留度數的重連、打亂正負號、打亂權重）各 3 個 seed，LIF 與連續核心各 10 格（每格 165,122 節點、25,563,197 邊、300 步，兩個矩陣各約 15 分鐘）。指標與門檻事前寫進 protocol；五條 LIF 門檻在原圖與九個空模型格全部通過，所以它們分不出真實與打亂的接線；同一份接線換核心後，原圖在空模型分布中的百分位方向可以相反，歸因結論必須連同核心、參數來源、空模型種類與指標定義一起陳述。報告只陳述指標與位置，不做行為宣稱。證據見 [NAT-03](evidence/NAT-03/verification.json)、[NAT-04](evidence/NAT-04/verification.json)、[NAT-05](evidence/NAT-05/verification.json)。

發布資料到動態參數的 adapter（ticket 13、NAT-02）已在真實全圖驗證：`data derive` 依明示規則檔把逐突觸傳導物質機率經座標配對到全部 25,563,197 條邊（配對比例 1.000、207 秒、RSS 3.87 GB），正負號 +53.3%／−35.9%／unknown 10.8%，unknown 只來自機率門檻與規則本身標 unknown，不補預設值；`simulate run --params` 以推導參數集跑 NAT-01 的同一協定，沉默比例由 2.1% 升到 64.4%、最大全群放電比例由 0.547 降到 0.079、不再觸發旗標。這是兩組參數假設在同一張接線圖上的差異，不是生理結論；歸因仍需 ticket 14 的空模型。證據見 [NAT-02](evidence/NAT-02/verification.json)。

原生模擬 runner（ticket 12、NAT-01）已在真實全圖驗證：`simulate run` 不經 encoder、readout 或訓練器，把 165,122 節點、25,563,197 條邊接上 LIF 核心跑 300 步，93.08 秒、RSS 4.38 GB、無非有限值；參數為明示的工程假設 `engineering_uniform_positive`（全興奮、統一 bias／log_tau／theta_raw），報告與文件皆寫明不是生物參數。刺激結束後活動自我維持並觸發 `max_population_rate_exceeded` 旗標，歸因需 ticket 14 的空模型。證據見 [NAT-01](evidence/NAT-01/verification.json)。

COR-01 狀態分離、STA-02 安全保存、STA-04 個體隔離通過 fixture 驗收。Mac／Ubuntu v25 同源 137 檔的完整 build、unit、race、vet、模組與 CLI 檢查，以及 27 個相關頂層測試／範例全部通過；快照雙向跨機器讀回並接續成功。Windows 僅交叉編譯通過。完整需求為 23／85，另有 6 項 NAT 增補需求（原生模擬），其中 NAT-01 到 NAT-05 已通過，NAT-06 尚未，證據見 [v25](evidence/cpu-reference-20260914/verification-v25.json) 與 [ticket 05](docs/tickets/05-resume.md)。

SIG-04 的輸入／輸出映射保存、重建、圖綁定與獨立替換通過 fixture 驗收。127 檔同源的 Mac／Ubuntu v22 完整檢查、選定輸入節點的跨程序續訓與 Windows 交叉編譯通過，詳見 [映射驗證](evidence/SIG-04/verification.json)。該階段累計為 18／85。SIG-01／02／05／06 維持通過。

真實子圖選取及短訓練範例已完成驗證（ticket 10、DAT-07）。原件與既有完整圖保持不變，證據見 [real-subgraph](evidence/real-subgraph-20260914/verification.json)。

標準化接線圖已在真實 MaleCNS v1.0 三份原件上完成建構、落盤與讀回驗證（ticket 08、09）：`connectome.Build` 與 `data import` 依明示 manifest 產生 `raw_segments`／`annotated_neurons` 視圖與報告，165,122 個選入節點、25,563,197 條邊，所有與獨立稽核共有的計數相等。Mac 與 Ubuntu v7 同一份 95 檔來源的建置、單元、race、vet、模組驗證與跨程序續訓全通過。CPU 參考流程、官方資料下載與 Feather 讀取維持 v4／v5 驗證。完整目標維持原始 W01–W14。

化學濃度、受體與效果（ticket 21 第一階段、MOD-02、MOD-03）與運行中學習與重播（ticket 23 第一階段、LRN-06、LRN-07）已在 fixture 驗證：濃度依 `lambda = exp(-dt/tau)` 衰退並收斂到 `tau*q`（幾何級數手算），區域傳輸不增加總量，負釋放率與非法傳輸都被拒絕；受體佔用率在對數域計算，`c = 1e300, n = 8` 仍為有限值，`unresponsive`／`unknown`／`hypothesized` 三種狀態分開報告且未知係數只在明示允許時才用並標為假設；效果映射在濃度為零時逐位等於未啟用，兩個核心的 `AdvanceModulated` 在 nil 與中性調節下逐位等於 `Advance`。運行中學習把作答、收回饋、更新三步分開，已輸出的答案 hash 在回饋與更新後不變，評估模式拒絕任何更新；重播存放區的先進先出與水塘淘汰、三種抽樣都有固定 seed 的手算序列，中途快照接續的抽樣序列與連續執行相同，測試分割永遠進不了重播。證據見 [MOD-02](evidence/MOD-02/verification.json)、[MOD-03](evidence/MOD-03/verification.json)、[LRN-06](evidence/LRN-06/verification.json)、[LRN-07](evidence/LRN-07/verification.json)。

## 階段目標

2026-09-15 使用者決定：把整個規格做完。原始 85 項到 2026-09-15 ticket 16 為止通過 26 項、NAT 六項通過五項，其餘依相依順序開票，先做能在 fixture 上驗證的，真實任務資料（TSK-11）與 GPU 後端（OPS-05）需要使用者提供資料或決定時明確標為受阻。路線：
15 強化（已驗證）→ 16 LIF 個體、慢速穩定、模型包（COR-04、STA-01、STA-03 部分）→ 17 遮罩、固定符號、完整最佳化器（COR-10、LRN-03、COR-07 證據）→ 18 局部可塑性（LRN-04、LRN-05、MOD-05 閘門）→ 19 runner 上的可塑性對照（NAT-06）→ 20 訊號來源與回饋三分離（SIG-03、MOD-01、MOD-08）→ 21 化學濃度、受體與調節效果（MOD-02、MOD-03、MOD-04）→ 22 記憶表現、控制器、對照與干預（MOD-06、MOD-07、MOD-10、COR-11）→ 23 運行中學習、重播、適應性評估（LRN-06、LRN-07、LRN-10）→ 24 遷移、敏感資料、SDK／CLI／組態／資源預估（STA-05、STA-06、OPS-01、OPS-02、OPS-03、OPS-06）→ 25 混合類型、向量節點、重算（COR-05、COR-06、LRN-02）→ 26 持續學習矩陣與模仿學習（LRN-08、LRN-09）→ 27 教師與蒸餾（TCH-01..06）→ 28 導航環境、多模態配對、共用核心、歸因（TSK-08、TSK-10、TSK-09、TSK-12）→ 29 真實任務（TSK-01..07、TSK-11，受阻於授權資料）→ 30 平台、裝置、全圖訓練、效能、治理、FlyWire（OPS-04、OPS-05、OPS-07、OPS-08、OPS-10、GOV-01/04/05/06、DAT-06）。

2026-09-15：票 21～30 的契約已全部定案並提交（`docs/tickets/21-*.md` 到 `30-*.md`），每票列 root 決策、分階段契約、驗收與依據；MOD-05 的受體來源與 MOD-09 分別併入 22 與 26。剩餘 31 項需求的規格抽取由 agy Gemini Flash 完成，紀錄在 root 的工作區。受阻項維持：TSK-11 真實任務資料、OPS-05 裝置後端（技術與存取）、DAT-06 的 FlyWire 真實資料、GOV-05 的發布授權，皆等使用者決定或提供。

2026-09-14 使用者定下兩條並列的一級路徑：Connectome 原生模擬（不經訓練直接執行）與可學習模式，見 [研究方向](docs/research-directions/connectome-native.md)、`AGENTS.md` 與 [NAT 增補需求](docs/requirements-addendum.json)。接下來依序做 [12 原生 runner](docs/tickets/12-native-runner.md)、[13 參數 adapter](docs/tickets/13-parameter-adapter.md)、[14 空模型與判讀](docs/tickets/14-null-models-and-behavior.md)，官方發布中能接的檔案（`body-stats`、`tbar-neurotransmitters`、`syn-partners`、`Neuprint_Meta.csv`）在 13 納入，盤點見 [發布盤點](docs/malecns-release-catalog.md)。

[10 真實子圖整合範例](docs/tickets/10-real-subgraph-example.md) 已驗證。98 檔同源的 Mac／Ubuntu v10 建置、單元、race、vet、模組與跨程序恢復驗證全部通過。ALIN 24 節點、110 邊的來源稽核、運算邊與初始化權重核對相等。

SIG-04 已依 §6.4 保存選取、來源、神經元指紋與投影重建資料（ticket 03）。ticket 05 已加入持續個體、選擇性重設、安全快照與隔離驗收，舊 episode 快照缺失／null scalar 靜默變零的 P1 缺口已於 2026-09-14 修正。下一步接續原規格的 LIF 放電核心。人工延遲關聯流程保留為 CPU 數值參考。框架不綁定使用者的單一模型或任務程式。

## 進行中

| id | 目標 | 負責者 | 狀態 | 驗證訊號 |
| --- | --- | --- | --- | --- |
| 01 | 開發者可核對環境與 Insyra 實際能力 | 主 agent / API 查核 agent | verified_scoped | Mac 與 Ubuntu 實際 doctor、Insyra 數值探測通過 |
| 02 | 使用者可計算稀疏前向與完整梯度 | sparse agent | verified_scoped | 手算、獨立稠密參考、形狀錯誤、有限差分已通過 |
| 03 | 使用者可提供與對齊具名訊號 | 主 agent / Luna 審查 | in_progress | SIG-01／02／04／05／06 fixture 已通過；SIG-03 控制器資料流待完成 |
| 04 | 研究者可訓練連續核心 | 主 agent | verified_scoped | 報告 v2 只置換訓練標籤並拒絕資料流重疊；Mac/Ubuntu v5 來源三個 seed 皆通過 |
| 05 | 使用者可中斷並接續學習 | sparse agent / 主 agent | verified_scoped | COR-01／STA-02／STA-04 fixture 通過；持續電位／延遲史、隔離重設、安全保存與新程序接續已驗證，STA-01／03 尚待完整機制 |
| 06 | 使用者可保存並續傳官方資料 | sparse agent / 主 agent | verified_scoped | 三份官方原件共 1,109,008,094 bytes，CRC32C 全通過；v4 CLI 另實測自動 CRC32C 驗證 |
| 07 | 使用者可逐批讀取 Feather 原件 | API agent / 主 agent | verified_scoped | annotations 211,577、NT 1,835,518、weights 151,856,684 列完整掃描，前後原件指紋一致 |
| 09 | 使用者可保存並讀回標準化接線圖 | 主 agent | verified_scoped | 容量、溢位、竄改／截斷／順序／索引與路徑替換回歸通過；真實圖 664 MB 在 Mac／Ubuntu 讀回再保存的獨立 SHA-256 均與原件相同，見 graph-store-v2 |
| 08 | 研究者可由官方原件建立可追溯標準化接線圖 | 主 agent（connectome）/ extsort subagent | verified_scoped | fixture 與真實資料皆通過；410 秒、RSS 3.40 GB、暫存 4.1 GB 後清空；raw 零重複 pair；durable GraphStore 切到 09 |
| 11 | 研究者可執行並訓練 LIF 放電核心 | 主 agent 指揮 / Opus 實作 | verified_scoped | 手算時序、tangent 參考與平滑模式有限差分通過，CLI `examples run lif-threshold` 三組 seed 只訓練閾值把保留 MSE 由 0.232 降到 0.052，COR-03／COR-09 已標 passed，COR-04 因慢速穩定未實作維持 specified |
| 12 | 研究者可不經訓練直接執行接線圖（LIF 持續狀態、原生 runner） | 主 agent 指揮 / Opus 實作 | verified_scoped | `LIFState`／`Advance` 與 `Forward` 逐位一致；`simulate` 手算 fixture、兩核心、決定性、分段接續、選擇器、門檻、容量與嚴格 JSON 測試通過；真實全圖 300 步 93 s／4.38 GB，NAT-01 passed；只在 macOS 實測，參數僅工程假設 |
| 13 | 研究者可由發布資料推導動態參數 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | `params` 套件與 `coimnet-parameter-set/v1` 檔案格式、`data derive`／`data validate --params`、`simulate run --params`（`derived_release/v1`，unknown 政策明示）；手算 fixture 由 root 獨立重算一致；真實全圖 25,563,197 條邊推導配對比例 1.000、sign +13.6M／−9.2M／unknown 2.75M，推導參數與均一參數同協定對照；NAT-02 passed；只在 macOS 實測，全圖只跑 exclude 政策 |
| 14 | 研究者可用空模型與判讀協定歸因 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 三種空模型（記憶體衍生物，seed 加 hash 重現）、具名集合、四種指標、事前門檻、`simulate compare`；fixture 手算由 root 重算一致；真實全圖 LIF 與連續核心各 10 格，NAT-03／04／05 passed；每種空模型 3 個 seed、只在 macOS、`population_sync` 未做 |
| 10 | 研究者可選取真實子圖並接上訓練核心 | 主 agent / Luna | verified_scoped | 獨立稽核及全部 ID／運算邊／初始化權重一致；兩平台各重跑一致、固定參數不變；跨平台最後參數指紋不同，僅作人工整合範例 |
| 15 | 強化：續傳、共用嚴格 JSON、邊順序守衛 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | `.part` 截斷續傳、死 pid 鎖回收、`signal` 改用 `strictjson`、推導的邊順序守衛皆有回歸測試 |
| 16 | 研究者可建立 LIF 個體、開啟慢速穩定並發布模型包 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 神經狀態聯集與舊檔升級、慢速穩定手算時序與收斂、模型包六種交叉拒絕、LIF 個體跨程序恢復逐項相等；COR-04、STA-01 passed；只有 fixture、只在 macOS |
| 17 | 使用者可以限制可更新參數、符號與範圍，並使用完整最佳化器流程 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 遮罩、固定符號（1,000 步不翻轉、有限差分 4.91e-05）、範圍投影、推導符號；損失縮放逐位相同、累積手算、三種排程手算與恢復接續、裁切順序；COR-10、LRN-03 passed；只有 fixture、只在 macOS |
| 18 | 研究者可以使用近期參與紀錄、學習閘門與兩種局部更新規則 | 主 agent 指揮 / Opus 5 實作 | in_progress | 第一階段驗證通過：兩種規則手算時序、閘門關／延遲閘門／衰退／上限／`w_min`、關閉逐位還原、快照往返，LRN-04、LRN-05 passed；第二階段（runner 對照，NAT-06）由 ticket 19 進行 |
| 20 | 研究者可以把觀察、目標與事後回饋分開，並選擇調節訊號的來源 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | `AvailableFeedback`、四種來源手算、獎懲映射四值分開、靜態 import 檢查腳本與 NaN 汙染測試；SIG-03、MOD-01、MOD-08 passed；控制器來源只保留名字（MOD-07） |
| 21 | 研究者可以運行有時間衰退的化學濃度、設定選擇性受體，並調節當下敏感度與有效閾值 | 主 agent 指揮 / Opus 5 實作 | in_progress | 第一階段驗證通過：濃度穩態／清除／中性／非負／傳輸手算、佔用率極端值無 NaN、三種受體狀態分離、效果中性逐位、`AdvanceModulated(nil)` 逐位等於 `Advance`；MOD-02、MOD-03 passed；第二階段（個體整合與快照）進行中 |
| 23 | 使用者可以在運行中依新經驗學習、使用受限容量的重播，並做適應性評估 | 主 agent 指揮 / Opus 5 實作 | in_progress | 第一階段驗證通過：`Act → Receive → Update` 順序、輸出 hash 不回寫、`Evaluate` 拒絕、重播 fifo／reservoir／三種抽樣手算與快照接續；LRN-06、LRN-07 passed；第二階段（適應性評估）待派工 |

## 目前阻礙

2026-09-13 使用者要求接回 Claude 的進度，分工優先 Spark `xhigh`；本次重新呼叫仍回報用量限制，改用 Luna `max`。Insyra 自訂 tape 梯度接合與最佳化器序列化缺少公開 API，已提 [#375](https://github.com/HazelnutParadise/insyra/issues/375)、[#376](https://github.com/HazelnutParadise/insyra/issues/376)。目前使用有完整數值測試的兩段 tape 接合，以及 CoImNet 自有可保存 AdamW。

嚴格 JSON 的深度與路徑配置缺口已修正，並將 Unicode 重複鍵檢查同步至訊號、快照、下載 metadata 與 manifest。圖檔補上配置前記憶體檢查、計數溢位、未初始化圖、未知 converter 與同次讀取 hash 回條；回歸測試及兩平台 v7 全套驗證通過。

`data download` 的續傳缺口（程序被砍後 meta 仍為 0、失效鎖不回收；2026-09-14 syn-partners 因此重抓）已於 [ticket 15](docs/tickets/15-hardening.md) 修正：每 64 MiB fsync 後寫回 `bytes`，`.part` 長於記錄者截斷後續傳，死 pid 的鎖回收並寫進回條。

Mac 可執行本機測試。Ubuntu 1 已實際連線並確認 RTX 4070 12 GB，Go 工具放在 `/tmp/coimnet-validation.irVFk8MV`。PC 的 App 主機連線存在，但本輪沒有可用的遠端指令工具，SSH 22 逾時，App UI 操作被工具限制拒絕。Windows 實機驗證尚未執行。GPU 硬體存在不等於稀疏訓練後端完成。

## 下一個可驗證成果與 ticket

研究方向的 12→13→14 序列已完成；2026-09-15 使用者決定把整個規格做完，路線見「階段目標」，ticket 15、16 與 17 第一階段已驗證，17 第二階段與 18 第一階段進行中。原先列出的候選（保留作紀錄）：(a) NAT-06 在 runner 之上開啟可塑性並做學習前後對照矩陣；(b) LIF 個體持續狀態（`learning.Individual` 的 LIF 變體與新 schema）與慢速穩定（COR-04）；(c) SIG-03 控制器資料流；(d) Ubuntu 上重跑 v26 以後的真實資料證據；(e) `signal/json.go` 的 JSON 深度防護與 `download` 續傳缺口。

LIF 個體持續狀態已由 ticket 16 完成（`dynamics.LIFState` 保存電位、突觸跡歷史、適應值、不應期計數與慢速穩定狀態，`coimnet-individual-checkpoint/v1` 以聯集指名核心，舊 episode 快照與模型包都不會被解讀成持續個體）。按類型混合（COR-05），混合要明示細胞分群依據，同一筆訊號不得重複計入。慢速穩定（homeostasis）補齊後 COR-04 才能標為通過。舊 episode 讀取器缺口已修（[ticket 05](docs/tickets/05-resume.md#已修正缺口舊-episode-必填欄位)）。SIG-03、可塑性、調節、全腦／GPU 仍依原始待辦。

圖儲存階段來源與日誌見 [macOS v7](evidence/cpu-reference-20260913/macos-v7/validation.log)、[Ubuntu v7](evidence/cpu-reference-20260913/ubuntu-v7/validation.log) 與 [來源比對](evidence/cpu-reference-20260913/verification-v7.json)。歷史驗證見 [macOS v6](evidence/cpu-reference-20260913/macos-v6/validation.log)（含 connectome、extsort，83 份來源指紋）、[macOS v5](evidence/cpu-reference-20260913/macos-v5/validation.log) 及 [Ubuntu v5](evidence/cpu-reference-20260913/ubuntu-v5/validation.log)。真實圖建構的命令、環境、指紋、報告與交叉核對見 [graph-v1](evidence/malecns-source-20260913/graph-v1/verification.json)。85 項完整需求目前有 23 項附上通過證據，其餘保留。LIF 階段的來源、日誌與環境見 [macOS v26](evidence/cpu-reference-20260914/macos-v26/validation.log) 與 [ticket 11](docs/tickets/11-lif-core.md#第三階段證據)。最新全套日誌見 [Mac v25](evidence/cpu-reference-20260914/macos-v25/validation.log)、[Ubuntu v25](evidence/cpu-reference-20260914/ubuntu-v25/validation.log) 及 [來源比對](evidence/cpu-reference-20260914/verification-v25.json)。

## 決策紀錄

2026-09-13：使用者授權開始規劃與實作，沿用既有 Go＋Insyra 及完整需求。先驗證梯度與可學習的小型流程，之後接真實資料，避免在未驗證核心上堆疊任務。

2026-09-13：官方 Feather 需要 nullable int64、dictionary 與 list 型別；Insyra 缺口已提 [#377](https://github.com/HazelnutParadise/insyra/issues/377)，資料 adapter 使用現有相依版本的 Arrow Go v17。查核見 [來源紀錄](docs/malecns-source-audit.md)。

2026-09-13：使用者授權在完成可驗證階段後主動提交與推送本專案，依 AGENTS.md 執行。

2026-09-13：ticket 08 的 root 決策：選取 predicate 用 `annotations.status == "Traced"` 並標為工程選取；端點身份以 manifest 明示並附官方頁面依據；比例保存分子、分母與欄位；節點順序為 namespace 內數值遞增；真實資料驗收限制 4 GiB 記憶體、16 GiB 暫存、256 run；durable GraphStore 切到 09；raw view 以重新讀取原件串流。詳見 [ticket 08](docs/tickets/08-connectome-graph.md)。同輪修正 `feather` 對零長度子陣列 nil buffer 的誤判。

2026-09-13：ticket 09 的 root 決策：圖儲存採自訂固定寬度區段加 JSON footer（非 Arrow IPC），每段與 footer 各有 SHA-256，檔案不含時間戳；發布沿用 checkpoint 的暫存檔＋hardlink＋目錄同步語意；讀回重算三個結果 hash 並檢查結構不變量；store 只保存 weights 原件路徑與指紋。不單獨標記需求通過，DAT-07 待真實子圖實驗。詳見 [ticket 09](docs/tickets/09-graph-store.md)。

2026-09-14：使用者決定框架必須同時支援 Connectome 原生模擬與可學習模式，底層不得綁死其一；五層分離；未訓練不等於沒有假設；歸因必須有空模型對照；官方發布中能接的檔案全部納入範圍。記錄於 `AGENTS.md` 與 [研究方向文件](docs/research-directions/connectome-native.md)，新增 NAT-01～06 增補需求。

2026-09-13：使用者明確界定 repo 僅提供框架，特定模型的訓練程式與模型限定為範例。維持原需求，五類任務與全圖訓練採範例／驗收流程。資源量測寫入 [resources](docs/resources.md)，現有神經網路的關係與可選機制寫入 [model-and-mechanisms](docs/model-and-mechanisms.md)。機制可選沿用原規格，不新增無法查證的全生物機制模式。

2026-09-14：ticket 03 的多通道範例保留各串流的來源序號，以共同神經步號填入矩陣，缺值用 presence 區分。同一步脈衝只在範例內明示加總，核心 API 保持通用。

2026-09-14：訊號持久化數值欄位拒絕明確 null，已宣告可空的品質分數／範圍與省略欄位預設保持原語意。SIG-01 依四種人工數值來源驗收，媒體解碼與生物映射分開追蹤。SIG-04 的明確集合局部測試不取代完整保存／替換流程。

2026-09-14：SIG-04 以獨立 Projection 保存加權映射，既有 Mapping 格式保持相容。InputNodes 指定集合時只配置對應的編碼器欄位，完整梯度經選定索引傳遞。替換保留核心、使用提供的兩側係數並建立新最佳化器。容量與來源條件見 [映射指南](docs/signal-projections.md)。

2026-09-14：本輪依使用者一次三項的要求完成 COR-01／STA-02／STA-04 fixture。新個體 profile 固定 CPU 精度及生命週期，不推論完整訓練工作恢復。跨 CPU 活化值最多四個相鄰 float64 值的載入容許範圍保留歷史原值；雙向真實傳檔驗證見 [跨機器證據](evidence/STA-02/crosscpu-verification.json)。舊 episode 缺失欄位缺口優先列入 ticket 05；checkpoint 分工的測試時序偏差已如實保存。

## 來源與接手

[共用設計](ENG.md)、[近期 tickets](docs/tickets/)、[主規格](docs/handoff/CoImNet_Implementation_Plan.zh-TW.md)、[85 項需求狀態](docs/requirements-status.json)。交接原件與既有 LICENSE 保持不變。所有完成狀態以實際證據為準，近期階段完成不等於整個框架完成。
