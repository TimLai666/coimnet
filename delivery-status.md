# CoImNet 開發進度

## 目前階段

2026-09-22 接手完成一個可驗證階段：ticket 26 的 PPO 更新入口與 ticket 29 的語音串流已完成狀態一致性、取消與恢復的回歸驗證。同一份 432 檔程式與腳本來源在 Mac／Ubuntu 通過 build、一般測試、race、vet、依賴與 CLI 續訓比對，見 [本輪證據](evidence/rl-asr-takeover-20260922/verification.json)。原始需求目前 63／85 標為 passed，NAT 增補為 6／6；這是既有證據的累計，本輪不重算為全部重新驗收。LRN-09 與 TSK-02 尚未通過完整需求驗收。

## 既有階段證據

以下保留各階段當時的驗證範圍與計數，現況以上方摘要及 `docs/requirements-status.json` 為準。

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

化學濃度、受體與效果（ticket 21 第一階段、MOD-02、MOD-03）與運行中學習與重播（ticket 23 第一階段、LRN-06、LRN-07）已在 fixture 驗證：濃度依 `lambda = exp(-dt/tau)` 衰退並收斂到 `tau*q`（幾何級數手算），區域傳輸不增加總量，負釋放率與非法傳輸都被拒絕；受體佔用率在對數域計算，`c = 1e300, n = 8` 仍為有限值，`unresponsive`／`unknown`／`hypothesized` 三種狀態分開報告且未知係數只在明示允許時才用並標為假設；效果映射在濃度為零時逐位等於未啟用，兩個核心的 `AdvanceModulated` 在 nil 與中性調節下逐位等於 `Advance`。第二階段把化學層接上持續個體：每一步固定走來源釋放 → 濃度 → 佔用率 → 效果陣列 → 調節後的核心一步 → 可塑性，連續與 LIF 各有四步手算表（LIF 的閾值效果抑制了原本會發生的放電），100 步後基礎參數逐位不變，中途快照帶濃度、資源與待處理回饋接續後逐位相同；MOD-04 標 passed，證據見 [MOD-04](evidence/MOD-04/verification.json)。運行中學習把作答、收回饋、更新三步分開，已輸出的答案 hash 在回饋與更新後不變，評估模式拒絕任何更新；重播存放區的先進先出與水塘淘汰、三種抽樣都有固定 seed 的手算序列，中途快照接續的抽樣序列與連續執行相同，測試分割永遠進不了重播。證據見 [MOD-02](evidence/MOD-02/verification.json)、[MOD-03](evidence/MOD-03/verification.json)、[LRN-06](evidence/LRN-06/verification.json)、[LRN-07](evidence/LRN-07/verification.json)。

runner 上的可塑性對照（ticket 19、NAT-06）已在真實全圖驗證：protocol 可宣告 `plasticity` 區塊（規則、邊選擇、閘門通道與比例），runner 在開啟時強制每步一次核心呼叫、每步先由快速狀態算有效權重再更新參與紀錄，`simulate compare` 新增 `original`／`plastic`／`learned_then_frozen` 三格；fixture 三神經元手算逐位相同，NAT-01 的 `protocol_hash` 由測試釘住不變；全腦（165,122 節點、25,563,197 邊、300 步、全部邊啟用、`hebbian_rate`）各跑兩次三格（`decay_p` 0.5 與 0.999），每格約 600 秒、最大 RSS 約 6.6 GB，`original` 格與 NAT-02 的全腦執行逐位相同；`decay_p` 0.999 時 ALIN 集合的 `mean_rate_after` 由 0.246 升到 0.305（凍結後 0.319），下行神經元的 `activity_ratio_after` 由 56.5 降到 54.4（凍結後 6.3），這些是同一刺激下的差值與百分位，報告不做文字判斷；`decay_p` 0.5 時關閉閘門 250 步後快速變化衰退到 8e-75，`learned_then_frozen` 與 `original` 逐位相同，是規則時間常數造成的。證據見 [NAT-06](evidence/NAT-06/verification.json)。

組態與資源預估（ticket 24 第一階段、OPS-03、OPS-06）已在 fixture 驗證：`config` 套件用共用的嚴格 JSON 解碼 `coimnet-config/v1`，從宣告的預設值出發，依檔案、環境變數、`--set` 的固定優先序覆寫並記錄每個葉節點的來源，所有名稱都要對到有版本的實作（未實作就拒絕，不替代），金鑰只以 `{"ref": "env:NAME"}` 保存、展開輸出永不出現值；`resources` 套件逐項估算記憶體並印出代入數字的公式，規格 17.1 的 `16E` 與 15.36 GB 兩個算術示例由測試釘住，超限時 `Check` 拒絕且不改計畫；`coimnet run --config … --dry-run` 展開組態、檢查模型檔與 generator、對 `max_memory_mib` 預估後印出 `coimnet-dry-run/v1` 報告，零副作用、教師呼叫次數 0，退出碼成功 0、拒絕 2、用法錯誤 1。全圖（165,122 節點、25,563,197 邊、f64、AdamW、不留反向歷史）預估 1,026,490,952 bytes，與 NAT-01 實測最大 RSS 4.38 GB 並列但不宣稱吻合。證據見 [OPS-03](evidence/OPS-03/verification.json)、[OPS-06](evidence/OPS-06/verification.json)。

按類型混合（ticket 25 第一階段、COR-05）已在 fixture 驗證：`dynamics.Mixed` 讓每個節點只跟一種規則，連續節點輸出 `phi(v)`、LIF 節點輸出突觸跡，兩種目標都用同一條 `I = external + Σ w·out(t − delay)`，延遲 0 讀本步開始時的輸出，LIF 事件只經突觸跡進下游一次；3 節點手算圖、不雙重計入、4 節點重新編號順序無關都逐位相符；全連續指派逐位等於 `Continuous`、全 LIF 指派逐位等於 `LIF`（`-race` 建置下既有 LIF 核心自己的膜電位差一個 ULP，混合核心把膜電位運算放進不內聯函式固定融合，差異由 build tag 常數釘住並在測試裡檢核）；梯度以平滑測試模式的中央差分與硬事件的手算兩步梯度驗證。`learning` 多了第三個核心、`LIFIndex` 對照與混合個體 profile，`checkpoint` 接受第三種神經狀態；模型包暫時拒絕混合核心（指紋與必填走訪尚未支援）。證據見 [COR-05](evidence/COR-05/verification.json)。

2026-09-17 起派工方式改為：實作票切成 200 行以內的小票派給 opencode 免費模型（`opencode/big-pickle`），主 agent 逐張讀 diff、自己重跑驗證再提交；Opus 只留給 review 備援。教師與蒸餾（ticket 27 第一階段、TCH-01、TCH-02）已在 fixture 驗證：`teacher` 套件的統一契約把回應當資料（含指令字樣的答案原樣保存、不執行），JSONL 紀錄檔拒絕任何屬於保留測試輸入的 hash，離線與重播教師把 `http.DefaultTransport` 換成一律失敗的傳輸仍能回答，HTTP 教師的逾時、5xx 精確重試、4xx 不重試、限速、預算（預設零）、同請求 ID 去重、只送允許欄位、金鑰只放 header 全部對本機 `httptest` 伺服器驗證，真實遠端服務明寫未驗證；封鎖教師對每次呼叫回錯並計數。證據見 [TCH-01](evidence/TCH-01/verification.json)、[TCH-02](evidence/TCH-02/verification.json)。

記憶表現的穩定化寫入（ticket 22 第一階段、MOD-06 後半）已在 fixture 驗證：`plasticity.Model.EffectiveWith` 在上方掛一層可選 `slow`，有效權重為 `(w_base + slow) + plastic`（固定符號邊為 `s * max(|w_base| + slow + plastic, w_min)`），slow 為 nil 時逐位等於既有 `Effective`；`learning.Individual.Consolidate` 依受體平均佔用率開閘，同 episode 去重（`ErrAlreadyConsolidated`）、預算用盡（`ErrBudgetExhausted`）回報而不動狀態，寫入量 `slow += Rate * plastic` 後 `plastic *= Retain` 與三層 `base_l2`／`slow_l2`／`plastic_l2` 都是手算值；把整個 slow 層折進基礎權重的孿生個體輸出、塑膠報告與電位逐位相同，證明 slow 進入整合而不只是裝飾；快照加 `plastic.slow`（`SlowState`）與 `optimizer.episodes`（每次 `TrainEpisode` 成功遞增、`ResetOptimizer` 歸零），checkpoint 往返逐一相等，中途快照在跨 JSON 接續後再推進一 episode 的寫入與去重判斷和連續執行相同。MOD-06 仍待 `evidence/MOD-06/` 證據目錄、MOD-05 閘門與時間窗等第一階段其餘卡片完成後才標 passed。

受體驅動的學習閘門與時間窗、記憶表現與穩定化（ticket 22 第一階段、MOD-05、MOD-06）已在 fixture 驗證：可塑性規則可指名受體，每步由該受體的平均佔用率算閘門（`GateScale × occ`）與參與紀錄的衰退（`clamp(base + span × occ, min, max)`），閘門關時快速變化只衰退不新增，明確閘門序列與受體閘門不得並用；讀出增益只乘在讀出路徑，核心、可塑、化學狀態與參數都逐位不變，清除後輸出與孿生個體逐位相同，文件明寫這是表現受抑制而不是遺忘；穩定化把快速變化按比例寫進較慢的 `slow` 層，只在受體佔用率達門檻、本 episode 未寫入且預算未用完時執行，同 episode 第二次拒絕且狀態逐位不變，報告分開列基礎、慢速、快速三層的 L2，梯度更新不動 `slow`。證據見 [MOD-05](evidence/MOD-05/verification.json)、[MOD-06](evidence/MOD-06/verification.json)。

平台矩陣與治理（ticket 30 第一階段、OPS-04、GOV-01、GOV-04、GOV-05、GOV-06）已驗證：`scripts/platform-matrix.sh` 對 linux/amd64、linux/arm64、darwin/arm64、windows/amd64 交叉編譯並 vet，只有本機 darwin/arm64 真正執行過 unit、race、vet 與 mod verify，JSON 裡 compiled 與 executed 分開標記；`dynamics.Config.Workers` 的並行分區在連續與 LIF 核心都與單執行緒逐位相同（前向與梯度），每步配置量不隨步數增加；治理測試檢查需求 id 集合、狀態值、passed 必有證據、Root 決策有日期；README 能力表四欄與科學界線、授權盤點（182 個模組、68 個未知）與 `blocked_permission` 的發布清單、`docs/INDEX.md` 的 39 個相對連結都經指令核對。證據見 [OPS-04](evidence/OPS-04/verification.json)、[GOV-01](evidence/GOV-01/verification.json)、[GOV-04](evidence/GOV-04/verification.json)、[GOV-05](evidence/GOV-05/verification.json)、[GOV-06](evidence/GOV-06/verification.json)。

適應性評估（ticket 23 第二階段、LRN-10）已在 fixture 驗證：評估協定預先宣告可用回饋、適應與評分分割、題間重設政策；fixed 模式只推論，adaptive 模式在適應分割允許宣告的局部更新但基礎參數凍結；三項污染檢查（參數逐位不變、評分分割拒絕回饋、評分題目送進重播庫全部被拒）與打亂順序逐位相同都寫進報告，CLI `examples run evaluate` 把模式寫進結果檔。全重設政策下兩種模式的成績相同，差異要用不重設可塑狀態的政策才看得到，本次沒有跑。證據見 [LRN-10](evidence/LRN-10/verification.json)。

遷移與敏感資料（ticket 24 第三階段、STA-05、STA-06）已在 fixture 驗證：`checkpoint.Migrate` 讀原件、寫新檔、不覆寫任何檔，報告前後 SHA-256、欄位變更與資訊損失（唯一的真實遷移是聯集出現前的連續個體檔，兩條欄位變更、無損失；同 schema 逐位複製；其他組合明確拒絕），CLI `checkpoint migrate` 把報告印成 JSON；`internal/redact` 遮罩 sk- 金鑰、Bearer、token／api_key 與 JSON 裡的 secret、password，原始文字、音訊、影像只留長度與 SHA-256 前 8 碼，跨 Write 邊界也不漏；`scripts/scan-commit.sh` 擋金鑰樣式、大檔、data/ 原件與教師回應原文，pre-commit 範本要手動 `git config core.hooksPath .githooks`；根套件測試證明沒有遙測字串、只有 download、teacher 與 internal/cli（`data sources` 的 HEAD 檢查，既有偏差）import net/http。證據見 [STA-05](evidence/STA-05/verification.json)、[STA-06](evidence/STA-06/verification.json)。

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
| 05 | 使用者可中斷並接續學習 | sparse agent / 主 agent | verified_scoped | COR-01／STA-02／STA-04 fixture 通過；持續電位／延遲史、隔離重設、安全保存與新程序接續已驗證，STA-01／03 尚待完整機制；STA-03（2026-09-19）：含可塑性快速權重、化學狀態、AdamW 最佳化器與資料游標的個體在子程序接續後與連續執行逐位相同，錯游標反例會分歧；順手修掉 checkpoint 拒絕受體閘門規則（`*int` 欄位）的載入 bug；`evidence/STA-03/`（STA-03 passed），STA-01 待完成 |
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
| 18 | 研究者可以使用近期參與紀錄、學習閘門與兩種局部更新規則 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 兩種規則手算時序、閘門關／延遲閘門／衰退／上限／`w_min`、關閉逐位還原、快照往返，LRN-04、LRN-05 passed；第二階段（runner 對照）由 ticket 19 完成 |
| 19 | 研究者可以在原生模擬之上開啟可塑性，並報告原有輸出被增強、修改或破壞 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | protocol `plasticity` 區塊、runner 逐步有效權重、`simulate compare` 三格（original／plastic／learned_then_frozen）；fixture 手算逐位相同、NAT-01 `protocol_hash` 不變；全腦兩次各三格（`decay_p` 0.5 與 0.999，每格約 600 s、RSS 6.6 GB），報告只給差值與百分位；NAT-06 passed；只在 macOS、`stdp_pair` 只有 fixture |
| 20 | 研究者可以把觀察、目標與事後回饋分開，並選擇調節訊號的來源 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | `AvailableFeedback`、四種來源手算、獎懲映射四值分開、靜態 import 檢查腳本與 NaN 汙染測試；SIG-03、MOD-01、MOD-08 passed；控制器來源只保留名字（MOD-07） |
| 21 | 研究者可以運行有時間衰退的化學濃度、設定選擇性受體，並調節當下敏感度與有效閾值 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 濃度穩態／清除／中性／非負／傳輸手算、佔用率極端值無 NaN、三種受體狀態分離、效果中性逐位、`AdvanceModulated(nil)` 逐位等於 `Advance`；個體每步順序在連續與 LIF 各有手算表、100 步 `Parameters` 逐位不變、中途快照接續逐位相同、`modulation` 不再依賴 `simulate`；MOD-02、MOD-03、MOD-04 passed；來源對所有區域一致、只有 fixture |
| 22 | 研究者可以讓化學狀態調節局部學習與記憶表現、施加具名干預、訓練小型控制器並和簡單替代模型對照 | 主 agent 指揮 / opencode big-pickle 小票實作 | in_progress | 第一、二階段驗證通過：受體驅動的閘門與時間窗、讀出增益（不是遺忘）、穩定化寫入 `slow`（MOD-05、MOD-06 passed）；具名干預：宣告的八條驗證規則、個體上的節點類（clamp／silence／force_spike）與通道類（remove／fix／block／swap／shuffle）干預、原生 runner 的節點類干預、未授權不改任何狀態、孿生比較日誌的四種差值；`evidence/COR-11/` 25 個頂層測試 race 全綠（COR-11 passed，2026-09-19）；第三階段（控制器與對照）待派工：控制器來源第一次派工偏離契約已丟棄，重派排程中 |
| 23 | 使用者可以在運行中依新經驗學習、使用受限容量的重播，並做適應性評估 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 第一階段驗證通過：`Act → Receive → Update` 順序、輸出 hash 不回寫、`Evaluate` 拒絕、重播 fifo／reservoir／三種抽樣手算與快照接續；LRN-06、LRN-07 passed；第二階段（適應性評估）切成小票派給 opencode 進行中；第二階段驗證通過：fixed／adaptive 兩種模式、三種重設政策、污染檢查三項（基礎參數不變、評分分割拒絕回饋、重播拒絕 8／8）、打亂順序逐位相同、CLI 把模式寫進結果；LRN-10 passed（全重設政策下兩種模式成績相同，差異未展示） |
| 24 | 使用者可以用嚴格展開的組態、啟動前的資源預估、完整 CLI 與 SDK，並安全遷移格式與保護敏感資料 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 三個階段驗證通過（2026-09-19）：組態嚴格展開與資源預估（OPS-03、OPS-06）；SDK 八個可執行範例、87 組非法輸入不 panic、五個取消釋放測試（OPS-01）；CLI `model inspect`／`model validate`、六階段 `benchmark`（匯入、前向、反向、局部學習、調節、快照，附 uptime）、`export`（json／markdown，未支援格式列出清單）、`report`（登錄、成績、限制、來源、完成度的 JSON＋Markdown）與各命令端到端及失敗測試（OPS-02）；遷移不覆寫原件、遮罩與掃描腳本、無遙測（STA-05、STA-06）；`evidence/OPS-01/`、`evidence/OPS-02/`、`evidence/OPS-03/`、`evidence/OPS-06/`、`evidence/STA-05/`、`evidence/STA-06/`；`run`／`evaluate`／`ablate` 的完整組態執行仍由 23／22 的骨架承接（見 OPS-02 limitations） |
| 25 | 研究者可以依神經元類型混合連續與脈衝規則、使用向量節點與共享參數，並以重算降低反向歷史記憶體 | 主 agent 指揮 / Opus 5 實作 | in_progress | 第一階段驗證通過：`dynamics.Mixed` 每節點一種規則、單一輸出契約、不雙重計入、編號順序無關、全 0／全 1 逐位等於既有核心（`-race` 下 LIF 核心自身的 1 ULP 差由 build tag 常數釘住）、平滑模式有限差分與手算兩步梯度；`learning` 第三核心與混合個體快照；COR-05 passed；第二階段（向量節點、共享參數）與第三階段（重算）待派工 |
| 27 | 使用者可以用離線與 HTTP 教師蒸餾學生，並在完全移除教師的情況下評估它 | 主 agent 指揮 / opencode big-pickle 小票實作 | in_progress | 第一、二階段驗證通過：統一教師契約、離線／重播／HTTP／封鎖教師（TCH-01、TCH-02 passed）；蒸餾：學生自編碼（教師 token id 拒絕）、保留集從不送教師與去重、合法對齊（identical／declared_map）的分布蒸餾與 log-sum-exp 穩定、top-k 只標 partial、接上 `StepFrom` 的訓練（held-out 一致率 0→1）；`evidence/TCH-03/`、`evidence/TCH-04/` race 全綠（TCH-03、TCH-04 passed，2026-09-19）；第三階段：學生獨立評估（student 模式）票已寫好待派，teacher_assisted 與工具安全證據待派 |
| 30 | 使用者可以在目標平台使用 CPU 參考路徑、在真實資料上做全圖前向／反向與訓練、核對治理文件與授權，並匯入 FlyWire | 主 agent 指揮 / opencode big-pickle 小票實作、agy Gemini Flash 證據整理 | in_progress | 第一階段驗證通過：平台矩陣把 compiled 與 executed 分開標記（darwin/arm64 實際執行 unit／race／vet／mod verify，linux 與 windows 只交叉編譯與 vet）、`Workers` 並行在連續與 LIF 核心逐位相同且每步配置量不隨步數增加、治理測試四項、README 能力表與科學界線、授權盤點與 `blocked_permission` 發布清單、INDEX 連結檢查；OPS-04、GOV-01、GOV-04、GOV-05、GOV-06 passed；第二～四階段待派工，第五階段裝置後端（OPS-05）與 FlyWire（DAT-06）受阻於使用者；第二階段進行中（2026-09-19）：FlyWire adapter（CSV／gzip → Feather 與 manifest，命名空間 `flywire-<version>`，root id 到 2^63−1 精確往返、溢位拒絕，`docs/flywire-source-audit.md` 標 blocked_data）已提交，拼接禁止（`MappingEvidence`）票已寫好；第五階段 `backend` 能力宣告、明示回退與更新 ledger 已提交（OPS-05 可做部分）；第四階段乾淨環境腳本待 `report` 命令；第二階段 fixture 部分驗證通過（2026-09-19）：FlyWire adapter、`MappingEvidence` 拼接禁止與 named set 命名空間檢查，`evidence/DAT-06/`（DAT-06 passed，profile fixture；真實 FlyWire 檔案 blocked_data）；第四階段 `scripts/clean-env-verify.sh` 已落地（14 步 passed、3 步 blocked_data），效能報告與 OPS-10 證據待派 |
| 26 | 研究者可以量測先學 A 再學 B 的保持與適應、完成專家模仿與行動回饋學習，並執行有來源的生物啟發干預協定 | Codex 主 agent / Luna max | in_progress | 第一階段 LRN-08 與第三階段 MOD-09 已有證據；第二階段 PPO 更新入口的零初始狀態、安全拒絕、版本識別與候選個體更新通過 Mac／Ubuntu 局部驗證；任意狀態梯度、3 seed 各 200 次更新與 CLI 仍待驗收 |
| 28 | 研究者可以在二維環境訓練導航與位置記憶、建立配對／缺失／非同步多模態資料、讓多任務共用同一核心，並比較核心、外圍與接線本身的貢獻 | 主 agent 指揮 / opencode big-pickle 小票實作 | in_progress | 第一、二階段進行中（2026-09-19）：`nav2d` 環境（視野錐、碰撞、終止與時間上限分開、BFS 專家）與四個任務變體、`multimodal` 配對資料契約（值＋presence）已落地並提交；合成產生器、評估與對照待派 |
| 29 | 使用者可以用同一核心做 OCR、語音轉文字、文字生成與文字條件影音生成，並把有授權的真實任務資料帶入框架 | Codex 主 agent / Luna max | in_progress | OCR fixture 已有 TSK-01 證據；整檔 CTC 與因果串流的短尾、取消還原及語義設定恢復通過 Mac／Ubuntu 局部驗證；資料匯入、分割、延遲、TSK-11 真實授權資料與後續生成階段保留 |

## 目前阻礙

2026-09-22 跨平台整合已通過。Ubuntu 初次完整測試的 6 個失敗來自測試的浮點捨入假設、固定梯度數字，以及恢復測試把保存值換成 Mac 字面常數。修正限定於五個測試檔，未改核心運算；保留平均、裁切與 AdamW 的嚴格手算對照及快照逐位一致檢查。兩台機器已用同一份最終來源重跑完整驗證通過，原始失敗與診斷日誌一併保存。

2026-09-21 的 Spark 呼叫回 `Unknown model gpt-5.3-codex-spark`，本輪修正與審查改用 Luna `max`。使用者另授權善用 OpenCode，PPO 使用文件交給 `opencode/big-pickle` 免費模型。Insyra 自訂 tape 梯度接合與最佳化器序列化缺少公開 API，已提 [#375](https://github.com/HazelnutParadise/insyra/issues/375)、[#376](https://github.com/HazelnutParadise/insyra/issues/376)。目前使用有完整數值測試的兩段 tape 接合，以及 CoImNet 自有可保存 AdamW。

嚴格 JSON 的深度與路徑配置缺口已修正，並將 Unicode 重複鍵檢查同步至訊號、快照、下載 metadata 與 manifest。圖檔補上配置前記憶體檢查、計數溢位、未初始化圖、未知 converter 與同次讀取 hash 回條；回歸測試及兩平台 v7 全套驗證通過。

`data download` 的續傳缺口（程序被砍後 meta 仍為 0、失效鎖不回收；2026-09-14 syn-partners 因此重抓）已於 [ticket 15](docs/tickets/15-hardening.md) 修正：每 64 MiB fsync 後寫回 `bytes`，`.part` 長於記錄者截斷後續傳，死 pid 的鎖回收並寫進回條。

Mac 可執行本機測試。Ubuntu 1 已實際連線並確認 RTX 4070 12 GB，Go 工具放在 `/tmp/coimnet-validation.irVFk8MV`。PC 的 App 主機連線存在，但本輪沒有可用的遠端指令工具，SSH 22 逾時，App UI 操作被工具限制拒絕。Windows 實機驗證尚未執行。GPU 硬體存在不等於稀疏訓練後端完成。

## 下一個可驗證成果與 ticket

優先接續 [ticket 26 第二階段](docs/tickets/26-continual-matrix-imitation-ppo-and-bio-inspired-protocols.md)：把已驗證的 PPO 更新入口接上取樣策略與正確的時間上限 bootstrap，完成 3 seed 各 200 次更新、隨機策略對照與同 seed 重現，並提供 `examples run gridnav --method imitation|ppo`。任意非零初始神經狀態需要先有相符的梯度路徑，不能沿用從零開始的 `StepFrom` 假裝支援。

[ticket 29 第二階段](docs/tickets/29-real-tasks-ocr-asr-text-and-media-generation.md) 接續語音串流的資料匯入、說話者／場次分割與延遲報告。既有測試使用程式產生的三種音調，不能當成自然語音能力證據。真實授權資料與全圖／GPU 訓練維持原驗收條件。

歷史 Mac／Ubuntu v25 是 2026-09-14 的 137 檔來源驗證，不能代表目前 checkout；新階段證據必須帶當次來源指紋。需求累計以 `docs/requirements-status.json` 為準。

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
