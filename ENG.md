# CoImNet 工程設計

完整目標與驗收依 [交接規格](docs/handoff/CoImNet_Implementation_Plan.zh-TW.md)。此檔只記錄跨功能的實作決策，開發順序見 [目前進度](delivery-status.md)。

## 第一個可執行流程

使用者提供具版本的圖、訊號與設定，CPU 核心同步推進狀態。訓練器持有答案，計算讀出誤差並沿時間回推到核心及輸入映射。推論只接觀察，不接答案。第一個例子用明示為人工的延遲關聯，驗證核心能學習，再接真實資料。

```text
官方資料 -> 格式驗證/命名空間 -> 不可變解剖圖 -> 稀疏核心
觀察 -> 訊號/時間驗證 -> 輸入映射 -> 核心狀態 -> 選擇性讀出 -> 輸出
答案 ------------------------------> 訓練器損失
                  輸入映射 <- 核心 <- 讀出 <- 梯度
事後回饋 -> 有取得時間的紀錄 -> 後續學習/調節
模型/個體/訓練器 -> 各自版本化快照 -> 驗證後恢復
```

檔案與遠端回應都是不可信輸入，解析後先驗證再建立狀態。原始解剖紀錄、基礎參數、個體與訓練器各自擁有資料。建構與匯出須複製可變切片，避免不同個體共享更新。

## 共用決策

- 框架與任務分開：SDK 負責接線、動態、訊號、學習與狀態保存。特定任務程式與訓練結果由使用者專案持有，repo 內僅保留明示範例與必要驗證證據。新增任務範例放 `examples/`，既有 `experiment` 是人工範例套件。五類任務驗收依這個方式實現，不把個人模型作為框架本體。
- 可選機制沿用主規格第 8、10、11 章：連續／LIF／按類型混合、適應、局部可塑性與調節分開選擇。只有有實作、有資料或明示假設且通過相容性檢查的組合才能執行。關閉機制須還原參考行為，快照須保存啟用機制及狀態。沒有「載入全部生物機制」的承諾；定位、證據與驗收原則見 [機制說明](docs/model-and-mechanisms.md)。
- Go 與 Insyra v0.3.2 固定。Insyra 公開 API 與編譯探測通過後才採用，缺口先查核再依授權提 issue。
- CPU 參考使用 float64、固定邊順序與同步更新。運算/圖儲存為 O(N+E)，不建立全圖 N×N 矩陣。小型稠密運算只用於獨立測試參考。
- 第一批用純量連續動態與逐邊參數建立完整梯度。向量/共享參數在稀疏數值介面驗證，隨後接入動態。LIF、調節、真實資料與 GPU 仍是完整必做範圍。
- 未支援的設定返回具體錯誤，不退回別種模型或偷偷縮圖。人工圖與資料產生器在設定、輸出與報告明示名稱。
- 所有可失敗更新先驗證輸入並計算候選結果，再提交狀態；取消、NaN、Inf 或形狀錯誤不得留半次更新。
- 初期手動稀疏反向計算屬主規格允許方案。Insyra 必須實際參與張量與可用數值/學習操作，不能只是檔案讀取。若其 tape 無公開接合方法，使用明示且測過的完整向量梯度契約，禁止 unsafe/反射或假裝 detach 後仍可微。
- 已查證 Insyra v0.3.2 的 tape 沒有外部運算接合 API。使用兩段新建 tape：讀出損失產生核心上游梯度，手動時間回推取得編碼器輸出梯度，再用內積將梯度傳回編碼器。相關缺口追蹤 [#375](https://github.com/HazelnutParadise/insyra/issues/375)、[#376](https://github.com/HazelnutParadise/insyra/issues/376)。
- 第一版快照明確命名為 `episode-training/v1`，每個訓練步驟重設神經狀態。資料由固定版本、seed 與樣本索引決定。此格式不表示已保存持續個體、延遲歷史、快速權重或化學狀態，後續完整狀態須用不同 schema。
- 接線圖建構器只消費 manifest 明示的內容：官方欄位名稱、來源指紋、端點身份假設（含 evidence）、選取 predicate 與重複語意。未宣告身份時拒絕 annotated view；未宣告可加總分割時拒絕聚合。節點 canonical 順序為同一 namespace 內的數值遞增，索引寬度依節點數選 32／64 位元。endpoint、raw pair 與 annotated edge 三條統計都走 `internal/extsort` 的有界 run＋k-way merge，重複只計數不合併；raw view 不在記憶體保存 151M 列，改以重新讀取原件並前後比對指紋。manifest hash 排除本機路徑，結果識別以來源 SHA-256、predicate hash、converter version 與 node index／edge order hash 為準。
- 圖儲存（`coimnet-graph-store/v1`）用自訂固定寬度區段加 JSON footer：`node_ids`、`node_meta`、`edges`、`batch_starts`、`report` 各有 offset／length／SHA-256，footer 另有 SHA-256 與 magic 尾記；檔案不含時間戳，同一張圖位元組相同。發布沿用 checkpoint 的暫存檔＋hardlink＋目錄同步語意（實作各自保有）。讀回逐段校驗並重算三個結果 hash，結構不合法（順序、索引、列號、非有限值）即使 hash 一致也拒絕。store 不複製 weights 原件，只保存路徑、指紋與掃描設定。
- `LoadWithReceipt` 從同一個已開啟檔案取得通過校驗的位元組並計算整檔 SHA-256，CLI 不重新開啟路徑取 hash。載入回條不宣告寫入耐久性。圖陣列、metadata、字串暫存、footer／report 輸入長度及 strict decoder 輸入副本、五段讀取緩衝在配置前計入限制；乘積先檢查溢位，各項分開保留以避免加總溢位。此帳面限制不含 Go 配置餘量、解碼後 JSON 物件與執行環境，不能當成 RSS 上限。未知 converter 版本拒絕讀取。
- 所有嚴格 JSON 入口限制 64 層路徑深度；重複鍵依 Unicode simple fold 比對，涵蓋 `encoding/json` 接受的大小寫別名。錯誤路徑使用堆疊，僅回報錯誤時組字串。
- 訊號套件的數值 JSON 欄位保留型別資訊到驗證結束，明確 `null` 必須拒絕，不能由 Go 解碼器轉成零。已宣告可空的品質分數及有效範圍保持可空，省略欄位沿用既有預設。嚴格檔案入口使用 `DecodeSignal`／各容器解碼器，`SignalSpec` 是呼叫端建構資料。錯誤發生時不替換既有物件。
- 真實子圖範例沿用 manifest 選取、GraphStore 與既有訓練 API。框架不因範例新增模型工廠或圖格式；範例保存選取與初始化假設，人工任務結果與生物行為驗收分開。範例範圍、錯誤及證據由 [ticket 10](docs/tickets/10-real-subgraph-example.md) 管理。
- 訊號時間點重取樣沿用 `Signal` 與 `Clock`，以整數比率對齊共用零起點。即時入口只接受 `CausalHold`，離線入口可用 `OfflineLinear`，回傳結果標明模式。來源 watermark 封閉其以前的樣本群，同時樣本依序號覆寫，結尾以明示 `HoldLast` 延伸。輸入限持續時間為零的連續／活動／調節樣本，脈衝使用獨立的 `PulseAligner`，固定值區間使用 `IntervalResampler`。容量、錯誤與驗收責任見 [ticket 03](docs/tickets/03-signals.md)，公開用法見 [訊號取樣](docs/signal-resampling.md)。
- 脈衝時間點以 `ceil(sourceTime * SimulationSteps / SourceTicks)` 對齊，不提前事件。保留每筆數值與來源序號，同一步的碰撞事件逐筆輸出。watermark 嚴格超過目的步的來源位置才整組輸出，`Finish` 只排空事件。時間運算使用精確整數中間值，容量與失敗原子性由 [ticket 03](docs/tickets/03-signals.md) 驗收。
- 固定值區間採 `[start, start+duration)`，重疊時以較晚起點及較大序號優先，較新區間到期後恢復仍有效的舊區間。輸出為已宣告範圍內的時間點，空白省略、結尾不延長區間。即時使用的前提是起點已知值與持續時間。容量及錯誤沿用取樣契約，已到期區間釋放後仍保留最後封閉來源的序號與 metadata，避免重複輸入或切換通道。
- 數值反向使用平滑數學公式的解析導數。小步長的輸入係數用 `-Expm1(-dt/tau)` 計算，避免 `1-exp(...)` 消去有效數字。有限差分須選可解析的尺度，不能以浮點捨入後差分為零要求解析梯度歸零。

- 多通道整合保留獨立來源 Observation 與序號，以共同神經步號填入固定欄位矩陣。缺值用獨立 presence 欄位表示，同一步脈衝按宣告順序加總。自訂 adapter 及人工投影放在 `examples/multichannel`，直接使用既有 `signal`／`learning` API，核心不依賴任務格式。範例整段完成後回傳，不能用於即時延遲量測。欄位、容量、錯誤與相容性驗收見 [ticket 03](docs/tickets/03-signals.md)。

- LIF 放電核心與連續核心並列，`learning.Config` 的 `Dynamics` 與 `LIF` 二選一，節點數與邊數改由選中的核心取得，內部用私有介面包住兩種核心，不公開新抽象。膜電位沿用相同的指數漏電與可訓練 `log_tau`，對外輸出改為衰減突觸跡 `x`，讀出與 upstream 都對 `x`，float32 邊界與既有路徑相同。基礎閾值用 `theta_base = theta_min + (theta_max - theta_min) * sigmoid(theta_raw)` 保持在設定範圍內，建構時要求 `v_reset < theta_min < theta_max`。`theta_raw` 是新的可訓練參數群組，遮罩為 `Trainable.Theta`，連續模型設為 true 回錯。反向只驗證宣告的 `fast_sigmoid` 替代梯度 `psi(u) = 1/(1 + scale*|u|)^2`：重設分支與不應期步不傳梯度，硬放電事件不做有限差分，有限差分只用在套件私有的測試專用平滑模式，該模式不是產品模式。快照沿用 `coimnet-episode-training/v1`，新增欄位全部 `omitempty`，AdamW 攤平順序固定為 `weights, bias, log_tau, theta_raw, encoder, readout`，連續模型的 `theta_raw` 長度為 0，既有快照的位置與序列化結果不變。LIF 個體持續狀態已於 ticket 16 補齊，見下一個項目。範例協定放在 `experiment.RunLIFThreshold`，`examples/lifthreshold` 與 CLI `examples run lif-threshold` 共用同一份實作。決策、驗收與證據見 [ticket 11](docs/tickets/11-lif-core.md)。

- 神經狀態改為聯集，LIF 取得自己的個體 profile，慢速穩定成為可選機制，模型包成為第三種保存物。`learning.IndividualSnapshot.Neural` 是 `NeuralState{Core, Continuous, LIF}`：`core` 指名核心，兩半只能出現宣告的那一半，缺一半或多一半都是錯誤；`checkpoint.LoadIndividual` 讀到聯集出現前的舊檔（`neural` 直接是連續 `State`）時視為 `core: continuous`，有提交的 fixture 為證。`coreModel` 介面加 `profile`／`newState`／`validateState`／`advance`，`NewIndividual`／`RestoreIndividual`／`Advance`／`TrainEpisode`／`ResetNeural` 對兩種核心走同一條路徑，LIF 個體的 profile 是 `lif-f64-insyra-f32-persistent-inference-episode-learning/v1`。慢速穩定是 `LIFConfig.Homeostasis *LIFHomeostasis`（指標加 `omitempty`，未宣告的設定編碼與既有 hash 完全不變）：每顆神經元一個活動估計 `r` 與一個閾值偏移 `h`，`r(t+1) = r(t) + (dt/tau_rate) * (spike(t+1) - r(t))`、`h(t+1) = clamp(h(t) + eta*dt*(r(t+1) - target_rate), 0, h_max)`、`theta_eff = theta_base + a + h`；`r`、`h` 是狀態不是參數，反向視為常數，關閉時兩個陣列不存在且前向與反向逐位等於未宣告的設定。模型包是 `checkpoint.ModelPackage`（`coimnet-model-package/v1`），帶拓撲指紋（節點數、邊數，以及 sources 全部再 targets 全部、各以 little-endian uint32 編碼後的 SHA-256，編碼與 `simulate` 的 topology hash 相同，但在 `checkpoint` 私下重寫，避免 `checkpoint` 依賴 `simulate`）、`learning.Config`、`learning.Parameters`、宣告單位、證據路徑清單與可 seed 的 schema 版本；`SaveModelPackage`／`LoadModelPackage` 沿用個體快照的 envelope、checksum、嚴格解碼、必填走訪與排他 hardlink，載入時以 `learning.NewTrainer` 重建模型並重算指紋，指紋與設定不符就拒絕。`NewIndividualFromPackage` 放在 `checkpoint` 而不是 `learning`，因為模型包型別住在 `checkpoint`，反向依賴會成環。三種保存物在六種交叉載入全部互相拒絕並指名讀到的是哪一種，模型包的訊息另外說明它只有設定與參數、不能當成完整恢復。決策、驗收與證據見 [ticket 16](docs/tickets/16-lif-individual-and-model-package.md)、`evidence/COR-04/` 與 `evidence/STA-01/`。
- 原生模擬 runner 放在 `simulate`，與 `learning` 分離：不使用 Insyra，不建 encoder／readout／最佳化器，注入是固定線性映射 `input[node] += gain * stimulus[t][channel]`，探針是具名節點集合加宣告的 reduce。核心以套件私有介面包住連續與 LIF，不公開第三種模型抽象；`Protocol` 的核心設定只帶純量，節點、邊、延遲一律由 `connectome.Graph` 提供，protocol 宣告拓撲即報錯。選擇器限 class／type／superclass／subclass／instance／soma_side，null 註記不匹配，零命中除非 `allow_empty` 否則拒絕，解析結果（欄位、值、數量、首末索引）寫進報告。protocol 與狀態快照沿用 `internal/strictjson`（位元組上限由呼叫端給定：protocol 1 MiB、狀態快照 64 MiB；64 層、大小寫別名重複鍵、未知欄位、尾隨資料皆拒絕），該套件由 `connectome` 的 manifest 規則抽出，`DecodeManifest` 與 `Load` 改為呼叫它，行為不變。參數集必須明示 `Source`，目前有兩種。`engineering_uniform_positive`：`weight = gain * 原始 weight`、全興奮、統一 bias／log_tau／theta_raw、延遲零。`derived_release/v1`：由 `params` 的參數集檔提供每條邊的 sign 與強度，`weight = weight_scale * sign * 推導強度`，sign 為 unknown 的邊依 protocol 的 `derived{unknown_sign ∈ exclude|excitatory|inhibitory, weight_scale > 0}` 處理（exclude 為 0、excitatory 為 +|w|、inhibitory 為 −|w|，乘積為零一律存正零）並分開計數；節點純量仍取 uniform 區塊，因此該區塊的 `gain` 必須不寫或為零，避免出現第二個被忽略的強度旋鈕。`FromDerived(set, setSHA256, protocol)` 多收參數集檔的 SHA-256，因為票面同一句要求 hash 納入該指紋，而 `params.Set` 不帶它。hash 沿用「來源標籤加四組陣列」的編碼，derived 另在來源與陣列之間寫入參數集檔 SHA-256、規則 hash、政策與 scale，因此 uniform 既有的 hash 不變；兩種來源都要求 `params.Source == protocol.ParameterSource`。CLI 的 `simulate run --params` 在 derived 為必填、在 uniform 被拒絕，以 `params.LoadWithReceipt` 讀檔（上限沿用 `--max-store-bytes`／`--max-footer-bytes`／`--max-memory-bytes`），建構前先 `CheckGraph`。`RunReport` 增加 `parameter_set_sha256`、`rules_hash`、`unknown_sign_policy`、`unknown_sign_edges`、`weight_scale` 五個欄位，uniform 來源不輸出；`assumptions` 對 derived 寫明「符號由預測傳導物質機率依規則推導、unknown 依宣告政策處理、bias／log_tau／theta_raw 仍是統一工程值」。記憶體只做帳面估算（邊陣列本地一份加核心複製一份，各 24 B/邊，加每節點 80 B、刺激、分塊輸入／輸出／事件緩衝與探針時序），超限回 `ErrCapacity`，不縮圖也不減步數。`dynamics` 對單次 `Advance` 的輸出有 `MaxStateValues` 元素上限，全腦一次只能推進 6 步，因此 runner 內部分塊推進，分塊邊界不改變任何數值（有測試）。門檻只產生 `stability_flags`，不改參數也不改退出碼。相同輸入兩次執行的 `RunReport` JSON 位元組相同，中途保存狀態再接續與一次跑完結果相同。決策、驗收與證據見 [ticket 12](docs/tickets/12-native-runner.md)。

- 動態參數來源放在 `params`，與 `simulate` 平行的第三層：依賴 `connectome`、`feather`、`internal/extsort` 與 `internal/strictjson`，不依賴 `learning`，不用 Insyra。推導輸入不改 manifest，改用獨立規則檔 `coimnet-derivation-rules/v1`（相對路徑加 SHA-256、七種傳導物質齊全的 sign mapping 與 basis、glutamate basis 必須寫出「假設」或 "assumption"、`min_probability`／`min_matched_fraction` 限 [0,1]、`gain` 有限且大於零、`normalizer ∈ none|post_total|pre_total`），理由是 manifest 與 GraphStore 屬第一層不可變結構，分開後既有 manifest hash 不變。規則 hash 為 domain 字串加規則的 canonical JSON 的 SHA-256。固定寬度記錄：tbar 41 B（`(x,y,z)` 各以 XOR 符號位轉 big-endian 共 12 B、null 機率旗標 1 B、七個 float32 28 B）、syn-partners 20 B（鍵 12 B、來源與目標索引各 4 B）、pair 36 B（來源、目標、七個 float32）、ROI 6 B（節點索引 4 B、ROI id 2 B）。四個排序共用記憶體限制：先保留已知輸出陣列（每邊 18 B 加分位數副本 8 B、每節點 20 B），排序緩衝取剩餘的一半，其餘留給 ROI 字典與區段緩衝；暫存預算平均分給四個排序與兩個中介檔。`extsort.Merge` 是 push 介面，兩個排序串流無法同時拉取，因此 tbar 合併結果與 pair 聚合結果各自寫成一個已排序的中介檔（狀態位元組區分 usable／ambiguous／null probability），另一邊以循序游標 merge join，不開 goroutine 也不建全量索引。canonical edge 順序為 (source index, target index, absolute row) 遞增，重複 pair 的邊相鄰，因此可直接 merge join，並以 pair 的 raw weight 總和計算配對比例。參數集檔 `coimnet-parameter-set/v1` 沿用 GraphStore 精神：magic 加八個區段（`edge_weight`、`edge_sign`、`edge_sign_confidence`、`edge_transmitter`、`edge_matched_synapses`、`node_totals`、`node_primary_roi`、`derivation_report`）各有 offset／length／SHA-256，footer 另有 SHA-256 與 magic 尾記，不含時間戳，發布用暫存檔加 hardlink 加目錄同步且不覆寫；讀回逐段校驗並檢查值域，`CheckGraph` 比對 node index／edge order hash。unknown 保持 unknown，四個 unknown 原因（沒配對、低於機率門檻、低於配對比例門檻、規則對應為 unknown）互斥且依此順序判定，門檻比對用已存入檔案的 float32 信心度，讀者可逐位元重現。真實資料（2026-09-15，`evidence/NAT-02/derive-v1/`）：25,563,197 條邊全部推導完成，tbar 45,656,140 個鍵沒有重複鍵也沒有 null 機率，兩檔座標配對比例為 124,025,046／124,025,046 = 1.000（分母是兩端都選入的突觸列），`pairs_not_in_graph` 與 `edges_without_match` 皆為 0，sign 為 +13,636,628／−9,172,513／unknown 2,754,056（10.77%，只來自「低於 min_probability 0.5」1,038,293 與「規則對應 unknown」1,715,763），權重分位數 p0 8.02e-06、p50 0.00231、p100 1，`normalizer_zero_edges` 為 0；牆鐘 207.31 s、max RSS 3,870,539,776 B、四個排序共 9.6 GB 暫存加兩個中介檔 3.6 GB（6 GiB 記憶體與 40 GiB 暫存上限都未觸及），參數集檔 463,451,966 B。同一份推導以兩個不同 build 各跑一次，除了耗時、峰值記憶體與因此改變的檔案 hash 之外，每一個計數、比例、直方圖與分位數都相同。決策、驗收與證據見 [ticket 13](docs/tickets/13-parameter-adapter.md)。
- 空模型、判讀協定與比較矩陣全部放在 `simulate`，`connectome` 與 `params` 不動。空模型是記憶體內的衍生物，不另存 GraphStore 或參數集檔，理由是第一層是不可變結構，落盤的衍生圖會把假的來源指紋寫進那個格式；報告改記 `null_model{kind, seed, prng, swap_factor, attempts, applied, rejected{self_loop, duplicate}, topology_hash, parameter_hash}`，讀者用原 store 加 seed 重算後比對 hash。PRNG 固定 `math/rand/v2` 的 `rand.New(rand.NewPCG(seed, 0))`，報告寫 `"pcg"`。`degree_preserving_rewire` 的嘗試次數為 `ceil(swap_factor × E)`，每次抽 `IntN(E)` 與 `IntN(E-1)` 後把第二個索引跳過第一個，確保兩個相異索引只花兩次抽樣，因此抽樣序列只由 seed 與嘗試次數決定；先判自環再判重複 pair，兩種原因分開計數，`applied + self_loop + duplicate == attempts`。pair 集合是線性探測的開放定址 uint64 雜湊表，槽位存 `key+1`（0 代表空槽，因此自環 0→0 也能存），key 為 `source<<32|target`，hash 用 splitmix64 finaliser，大小取 `2E` 以上的最小 2 的冪（至少 2），因此永遠不超過半載；刪除用 Knuth 6.4R 的 backward shift，不用墓碑，因為每次成功交換要刪兩次。原圖若本來就有重複 pair 就直接拒絕，因為集合無法表示重數（MaleCNS v1.0 的 `duplicate_pairs` 為 0）；原圖自有的自環可以被換掉但不會被造出來。兩種 shuffle 是 Fisher–Yates，`attempts` 與 `applied` 都是抽樣次數 `n-1`，`swap_factor` 必須不寫或為零。`sign_shuffle` 置換 `EdgeSign` 的副本再走 `FromDerived` 的 unknown 政策，`weight_shuffle` 置換強度陣列（uniform 是 `gain × 原始 weight`，derived 是 `EdgeWeight`），兩者都以值複製 `params.Set` 並只換掉那一個陣列，原圖、原參數集與呼叫端陣列都不變（有 hash 前後比對的測試）。`DeriveNullModel` 與 `OriginalVariant` 比票面多收參數集檔的 SHA-256，理由與 `FromDerived` 同一句：hash 要指名檔案而 `params.Set` 不帶它。`topology_hash` 是 sources 全部再 targets 全部、各以 little-endian uint32 編碼後的 SHA-256。`Build` 改成串流原圖拓撲後呼叫 `BuildVariant`，兩者共用同一條驗證、記帳、建核心與解析路徑；`RunReport` 一律多出 `topology_hash`，`null_model` 只在空模型出現，uniform 與 derived 既有欄位與 `protocol_hash`／`parameter_hash` 都不變。具名集合是多個選擇器的交集，解析結果記錄每個選擇器、節點數與「遞增節點索引以 little-endian uint32 編碼」的 SHA-256，交集為空只有在每個選擇器都宣告 `allow_empty` 時才接受。四種指標只在 LIF 可用，連續核心只允許 `mean_output`（LIF 也可用，讀的是突觸跡）；`spike_fraction` 是窗內至少放電一次的節點數／集合節點數，`mean_rate` 是窗內放電事件數／(節點數×窗長)，`latency_to_first_spike` 的 numerator／denominator 是「首次放電步」與「窗起點」而值是兩者相減，沒有放電時整個結果歸零並標 `defined=false`，`activity_ratio_vs_baseline` 是窗與 baseline 兩個 `mean_rate` 相除、分母為零時未定義但仍寫出已算出的分子。分母為零一律未定義，不寫 0。門檻只有 `>=`、`>`、`<=`、`<`，未定義的指標一律 fail 並記 `reason: "undefined"`，門檻結果不改退出碼。集合追蹤由 `Runner.TrackSets(sets, windows)` 宣告，`Run` 逐步對每個窗內的步數累積每個追蹤節點的放電計數、集合首次放電步與輸出總和，探針語意不變，量測值由 `Measurements()` 讀回並在成功的 `Run` 之後才更新。記帳同樣只是帳面估算：變體是 `4 × 8 × E`（sources、targets、權重與 shuffle 副本），rewire 另加 `槽數 × 8`；追蹤在該次 run 已記的位元組之上再加每個集合每個窗 `4 × 節點數 + 64`，總和超過 `Limits.MaxMemoryBytes` 就回 `ErrCapacity`。`CompareProtocol`（`coimnet-simulate-compare/v1`，1 MiB 上限、strictjson）宣告一份 run protocol 加集合、指標、門檻、空模型與 seeds，空模型項目不得自帶 seed；`Compare` 先跑原圖一格，再依宣告順序跑每種 kind × 每個 seed 一格，每格是獨立的 Runner 與完整 `RunReport`。`MetricSummary.PerKind` 用切片而非票面的 map，保留宣告順序（Go 的 map JSON 會改成鍵排序）。分位數用 `params.quantiles` 的最近秩規則（`rank = ceil(q×n)` 夾在 `[1,n]`，取第 rank 個），與 `Monitors.RateQuantiles` 的線性內插不同，因為每個分位數都要是某個 seed 真的產生過的值；百分位是 `(小於的個數 + 0.5 × 相等的個數) / 有定義的個數`，原圖或全部 seed 未定義時不給。相同輸入兩次 `Compare` 的 JSON 除了每格的 `wall_seconds` 位元組相同。CLI `simulate compare --store --protocol [--params] [--out-dir]` 印出報告，`--out-dir` 必須不存在、在載入與參數檢查之後、跑矩陣之前以 `Mkdir` 佔用，每格另存 `cell-<index>-<variant>.json`；`--params` 規則與 `simulate run` 相同。`CompareCell` 另有 `deltas_from_original`，依指標順序記每一格與原圖的差，兩邊都有定義才有值，原圖那一格的差在指標有定義時為 0；差不在 `MetricSummary`，因為分位數與百分位描述分布，差描述單一格。真實資料（2026-09-15，`evidence/NAT-03/compare-lif-v1/` 與 `evidence/NAT-05/compare-continuous-v1/`）：同一份 store 與 `params-derive-v1.coimparams`，同一份刺激，兩個核心各跑十格（原圖加三種 kind × seed 1、2、3），每格 165,122 個神經元、25,563,197 條邊、300 步。`degree_preserving_rewire` 在 `swap_factor` 1 下每個 seed 嘗試 25,563,197 次，seed 1 成功 25,235,685、拒絕 724 自環與 326,788 重複，seed 2 成功 25,234,331、拒絕 715 與 328,151，seed 3 成功 25,233,927、拒絕 768 與 328,502；三者的 `applied + self_loop + duplicate` 都正好等於嘗試次數。兩種 shuffle 每個 seed 抽 25,563,196 次（n−1），沒有拒絕。LIF 矩陣 890.94 s 牆鐘（十格合計 852.24 s）、最大 RSS 5,909,626,880 B、峰值記憶體 9,543,671,152 B；連續核心矩陣 929.97 s（十格合計 890.17 s）、最大 RSS 5,659,115,520 B、峰值 10,513,997,816 B；兩者都沒碰到預設 8 GiB 的帳面上限。兩個矩陣是兩個獨立行程，十格的變體名稱、seed、`topology_hash`、`parameter_hash` 與整個 `null_model` 區塊（含三種計數）逐格相同，所以「原 store 加 seed 可重算」這句有實測；LIF 第 0 格的 `protocol_hash` 與 probe、monitor 序列也與 NAT-02 的單次全腦執行完全相同。`population_sync` 仍未實作。決策、驗收與證據見 [ticket 14](docs/tickets/14-null-models-and-behavior.md)。

## 功能流程、錯誤與驗證責任

| 功能/負責 ticket | 正常流程與測試入口 | 空值/零與錯誤處理 | 互動與失敗情況 |
| --- | --- | --- | --- |
| 01 檢查開發環境 | 命令報告實際 build info 與能力，Insyra 編譯/數值探測 | 找不到工具/後端顯示缺少，未知能力不可判通過 | 無網路/裝置不可阻止 CPU，取消要退出 |
| 02 計算稀疏傳播與梯度 | 公開 Go 算子對手算及獨立稠密參考 | 零邊合法、非法索引/形狀/非有限值拒絕；乘法溢位不配置巨大記憶體 | batch、向量、共享梯度加總、重複/自我邊依穩定順序；失敗不改輸入 |
| 03 提供與對齊訊號 | 訊號驗證/時鐘/映射公開 API | 空觀察可保持靜默、空名稱/負時間/形狀錯誤拒絕；零資料與缺失分開 | 同時事件穩定排序、重複序號/倒退拒絕、未知細胞拒絕；回饋不可提前取得 |
| 04 訓練連續核心 | SDK/例子從觀察至輸出/損失/參數更新 | 空序列明確拒絕、零邊可執行、錯參數/NaN/取消保留原狀 | 同步狀態、完整/有限回推分開；凍結核心、固定外圍、打亂標籤對照 |
| 05 保存並接續學習 | 新程序恢復與連續執行比較 | 損壞/未知版本/形狀/拓撲不符拒絕 | 不覆寫、原子寫入、並行衝突失敗、取消清理；參數/最佳化器/游標/隨機一致 |
| 06 取得官方資料 | SDK/CLI 至暫存、校驗、原子發布與來源回條 | 缺少長度/ETag、過大、磁碟不足或內容不符拒絕 | 取消與中斷可續傳，來源改版或忽略 Range 拒絕拼接，並行/既有成果不覆寫 |
| 07 讀取 Feather 原件 | SDK callback / CLI 逐批讀取及 schema 報告 | 錯格式、未知型別、重複欄位、損壞內容與容量超限拒絕 | 取消或 callback 錯誤結束並釋放資源，部分讀取不得標示完整；保留 int64、字典、list 與 null |
| 09 保存並讀回接線圖 | `connectome.Save`／`Load`／`LoadWithReceipt` 與 CLI `data import --out-store`、`data validate` | 空圖可往返；零長度字串、null 欄位、null weight 保留；零或負限制、空路徑、nil／未初始化圖拒絕 | 目標已存在、父目錄缺、取消時不發布不留暫存；竄改區段／footer、截斷、錯 magic／版本、順序或索引不合法拒絕；檔案／footer／記憶體超限回 `ErrCapacity`；發布後目錄同步失敗回報耐久性未確認 |
| 08 建立標準化接線圖 | `connectome.Build` 由 manifest 與三份原件產生兩個視圖與報告；CLI `data import` 輸出報告 | 空 rows／零選入節點產生空視圖與未定義比例；null／負 ID、缺註記、predicate false 各以單一原因排除；零／負限制、無效 manifest／predicate、缺身份證據、聚合無證據拒絕 | 指紋前後不符、schema／型別不符、NaN、容量、取消都不發布結果並清掉暫存；相同輸入與 predicate 重跑得到相同索引、順序與 hash；建構與匯出不共享可變切片 |

每列對應單一 ticket 的驗收，測試由公開函式或命令進入，不針對私有實作逐函式寫鏡像測試。後續功能在實作前依原規格補相同檢查與可驗證 ticket。

## 數值與研究測試

稀疏算子先用規格第 9.2 節的手算值，再以隨機小圖、batch、向量與共享參數對照獨立參考。平滑完整路徑以中心有限差分驗證每組參數與輸入。替代梯度只驗證宣告公式，不拿硬放電做有限差分。

`evidence/gradient-audit-20260913.json` 稽核 bridge 的 `epsilon=1e-2, 2e-3, 1e-3, 1e-4`；目前測試值 `2e-3` 的六組梯度最大絕對誤差為 `9.288636276e-7`，最大相對誤差為 `1.583954284e-3`，通過本稽核選定的 `audit_combined_atol_1e-5_rtol_1e-4` 比較判準 `absolute_error <= 1e-5 + 1e-4*abs(fd)`，此判準不是第 9.4 節逐字公式。第 9.4 節的 `atol=1e-5, rtol=1e-4` 是 float32 前向與參考值比較的工程預設；平滑 float64 有限差分使用相對誤差起始門檻 `1e-4`，近零梯度另看絕對誤差。純 float64 core 的 tanh、softplus 各組在四個 epsilon 的 `relative_error_gt_1e-4_count` 均為 0，近零絕對檢查計數也均為 0；bridge 跨越 Insyra float32 邊界，`epsilon=1e-4` 有三個稽核合併判準失敗，因此保留 `2e-3`，不把結果推廣成所有配置或純 float64 保證。

人工延遲關聯固定資料產生器、訓練/驗證/測試 seed、更新預算與門檻後才跑保留資料。報告所有 seed、更新前後指標、梯度、凍結與打亂對照。數值通過不等同真實圖或五類任務成功。

延遲關聯報告 v2 的打亂對照只對訓練標籤做固定 Fisher–Yates 置換，保留原標籤分布，並記錄方法與種子。既有 v1 報告使用獨立產生的標籤，不能當成 v2 置換對照的證據。訓練與保留資料的 counter 範圍不得重疊，即使 seed 不同也須檢查。

建置、完整單元/race/vet、相依性驗證及已支援例子全部執行。真實子圖、全圖、各目標平台和 GPU 分開驗收。CPU 正確性是裝置比較基準。

## 假設與資源

本機優先、CPU、離線教師、零雲端費用。大型資料下載先查大小、可用空間與預算。全圖訓練須預估包括 E×B×T 的狀態歷史，超限拒絕而非自動縮小。外部資料 schema、Insyra 自訂梯度介面與裝置能力須由實測確認。

實測資源與適用設定見 [資源說明](docs/resources.md)。目前反向視窗只控制梯度傳遞，前向 trace 仍保存完整輸入序列歷史，不能宣稱縮短視窗即可降低歷史配置量。啟動前完整記憶體預估（OPS-06）、重算節省歷史（LRN-02）及全圖訓練（OPS-07）依原需求追蹤，文件估算不取代這些功能。

目前沒有資料庫或已發布格式，不需要資料庫遷移。建立第一版外部格式後，版本變更必須保存舊原件並提供相容性驗證。

減法審查：沿用原始完整規格與需求追蹤，不再複製 85 條定義。只為立即可做的使用流程細分 tickets，不建空套件或假命令。

## 投影映射實作契約

保留既有 `Mapping` 格式。新增 `signal.Projection` 保存方向、通道形狀、來源／人工標記／證據、選取條件、已解析外部 ID、依序排列的神經元清單指紋及 row-major 係數。明確集合保留指定順序，細胞類型／區域選取使用呼叫者提供的 metadata，按 namespace／external ID 排序，未知空欄位不推論。固定隨機係數使用明示版本的演算法、seed 與 scale，保存實際係數並驗證重建一致。

輸入投影係數為 `[channel, selected neuron]`，輸出為 `[selected neuron, channel]`。`learning.Config.InputNodes` 的 nil 保留既有全節點語意，指定集合時只為所選神經元配置 encoder 欄位，前向 scatter、反向 gather 維持完整梯度。映射載入／接合核對 caller 提供且與動態索引同序的神經元 metadata，不假造圖身份。替換建立獨立 Config／Parameters／Trainer，保留呼叫者核心參數、替換 encoder／readout 並明確重新初始化最佳化器，不變更原訓練器。

訊號套件在 JSON 解碼前檢查 UTF-8 與 Unicode surrogate 配對，避免 encoding/json 靜默改寫身份或來源字串。NeuronID 的建構／保存也拒絕非法 UTF-8。metadata 投影在載入時核對標準排序，不能等到 Bind 才發現無法重建。

JSON 沿用 16 MiB／64 層嚴格入口，投影最多選取 1,048,576 個神經元及 1,048,576 個係數。乘積與上限先檢查再配置隨機係數。限制為邏輯數量及輸入位元組，不是程序 RSS 上限。大型投影需另行設計儲存格式，不以本 fixture 宣告全腦投影已驗證。


## 持續個體與同步保存

持續個體採 `learning.Individual`，擁有獨立的參數、神經狀態及 episode 最佳化器，連續與 LIF 兩種核心都適用。不可變設定與原始接線各自保存。`Advance` 以 Insyra 編碼／讀出並保留延遲歷史，`TrainEpisode` 沿用獨立序列梯度，兩者的生命週期明確分開。所有操作共用同一把鎖。帶 context 的修改可取消，候選結果完成後才提交；快照等候操作完成，再取出深複製。

`dynamics.State` 是已啟用連續機制的最小接續資料，保留電位、步數及必要延遲輸出史，不保留已無用途的完整 trace。`dynamics.LIFState` 是 LIF 核心的對應資料，另外保留突觸跡歷史、適應值、不應期計數，以及慢速穩定開啟時的活動估計與閾值偏移。神經、參數、最佳化器的重設由不同 API 指定，解剖資料保持不可變。新 schema 保存已支援 profile、精度／生命週期與設定指紋，不把舊 episode 快照解讀為持續個體。公開契約、失敗處理、容量與三項驗收見 [ticket 05](docs/tickets/05-resume.md)。

載入時，最新歷史輸出與電位經活化函數的計算值最多容許相鄰四個 float64 值的差異（4 ULP），不改寫保存的歷史。Go 1.26.5 的 Mac arm64／Ubuntu amd64 實測曾出現 tanh 1 ULP、softplus 2 ULP 差異；此容許範圍只用於狀態驗證，不保證跨處理器後續運算逐位元相等。
