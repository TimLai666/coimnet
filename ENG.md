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

- `Individual.Intervene` 的私有對照沿用同一持續快照，另複製當下 chemistry／plasticity 的 runtime freeze 旗標。先完成無介入對照，再設定 subject 的暫時網路與 hooks 並推進，避免對照失敗或取消時個體已提交。一般 `RestoreIndividual` 繼續預設解凍，保存格式、正常動態及 frozen channel phase 的既有優先序保持。公開未生效／阻斷控制與取消原子性由[ticket 127](docs/tickets/127-frozen-intervention.md)驗收。

- [Ticket 66](docs/tickets/66-real-ocr-chemical-comparison.md) 沿用 OCR 的 Snapshot／Inputs 與通用 Trainer，加入固定 ChemicalEpisode。損失、初值、分割、順序與更新預算相同，先驗證與原 SDK 逐位一致。比較的是工程設定對單字元辨識的效果，保留全部結果，不依測試選參數，也不宣稱盲測或生物定量正確性。

- OCR 新輸入模式以明示 opt-in 設定加入。舊欄位／CTC 模式與已存模型保持；aligned-columns 補一個傳遞步並同時對齊讀出與外部梯度；single-character 以完整圖片的一次輸入及一個零傳遞步，僅最後讀出計算單字元分類。所有模式保留既有稀疏連續核心與 Insyra 梯度，資料、容量、目標與人工核心範圍明示。正式品質設定在測試前凍結，已用於開發的 200 張測試圖片排除。見 [ticket 64](docs/tickets/64-real-task-quality.md)。

- GPU episode 的裝置 AdamW 以明示 opt-in 模式加入既有 Trainer。成功提交前讀回完整候選並驗證，提交後更新主機鏡像，既有 Snapshot 不改成可失敗操作。參數和動量常駐，但神經狀態、Insyra 編碼／讀出與稀疏算子參數傳輸仍在主機，量測須分開且不宣稱加速。原規格 §17.4 的完整神經狀態常駐是可行時的方向，不將 GPU 全圖或第二平台另加為 OPS-05 門檻。見 [ticket 61](docs/tickets/61-gpu-device-training.md)。

- OCR 圖片前處理採有上限的標準 PNG／JPEG 解碼、明示灰階／墨色／nearest 縮放及來源 metadata，單欄頁面分行仍由同一 Recognizer 辨識。新增可選可訓練 identity 初值 encoder，缺省 false 保持舊模型，保存 mode 與 optimizer，配對比較獨立揭露 encoder 容量。影音範例沿用 task bundle，把來源、特徵／縮放／解碼設定與完整 snapshot 一起保存，新程序讀回後使用同一 predict／score／preview，沒有隱藏重訓。見 [ticket 62](docs/tickets/62-ocr-images-page-and-encoder.md)、[ticket 63](docs/tickets/63-realmedia-model-restore.md)。

- 任務模型保存沿用網路磁碟 v2 bundle，將任務設定、詞表與完整訓練狀態一起保存。任務 Load 重新核對設定與網路、最佳化器，不用缺失欄位建立新模型。三類任務公開 Save／Load 與真實資料範例見 [ticket 59](docs/tickets/59-task-model-save-restore.md)。GPU 參數更新先採獨立 float32 裝置格式，候選更新與提交分開，既有 CPU／episode GPUTrainer 保持。所有宣告參數分塊運算，量測常駐狀態與傳輸量，見 [ticket 60](docs/tickets/60-device-adam-update-restore.md)。

- 真實全圖的分段重算效能直接量測既有 SDK 的 ForwardRecompute、BackwardRecomputeStats 與啟用 Recompute 的 Trainer.Step，因 benchmark CLI 的既有路徑未啟用重算。固定完整圖、原始邊順序、參數與事前設定，只有兩份核心檔透過 Go overlay 對照修改前版本。載入、建構、明示 GC 與摘要分開計時，全部原始程序、輸出／梯度／更新／Adam 指紋、配置量與整個程序的 RSS／足跡都保留。效能結果不改模型或任務驗收狀態，見 [ticket 58](docs/tickets/58-fullgraph-recompute-efficiency.md)。

- 網路磁碟的模型保存沿用 v2 bundle 及三種既有 Bundle SDK。排他建立目錄後先保存資料，最後寫完整完成清單，載入端拒絕缺失或截斷清單。完成清單完整寫出後即保留成果，後續取消與同步失敗分開回報。單檔發布的既有契約保持。驗收見 [ticket 55](docs/tickets/55-network-model-bundles.md)。

- 生物啟發協定於共用 RunBioInspired 入口的既有驗證後複製全部可變設定，再計算雜湊及執行。每次報告各自持有種子、脈衝步、來源登錄與定量證據，保留 nil／空值及原驗證順序。回傳後修改設定與報告互不影響，不保證執行期間並行修改輸入安全。見 [ticket 54](docs/tickets/54-bioinspired-config-ownership.md)。

- 生物啟發協定沿用既有驗證順序與錯誤，NPF Suppressed 只接受嚴格介於 0 與 1 的值，Tolerance 只接受嚴格正且有限的值。合法報告及模型保持，見 [ticket 52](docs/tickets/52-bioinspired-finite-validation.md)。
- 本機必要資料與執行結果放在專案內已忽略的 data／runs，過程暫存放 runs/.tmp 並於證據保存後清除。搬移前後比對逐檔 SHA-256，歷史 provenance 不改寫。realnav 解析符號連結後，以最近的 `.git` 為 repository 邊界，只允許該 repository 的 runs 新子目錄或既有支援的外部目錄，保持來源重疊與既有輸出拒絕；不引入執行時 Git 依賴。見 [ticket 53](docs/tickets/53-project-local-data-and-runs.md)。

- 歸因與連續任務的 Comparison.Interval 沿用原 Validate 錯誤位置，只接受嚴格介於 0 與 1 的數值。NaN 與正負無限大在建模／訓練前被拒絕。nil Comparison、取消／nil 的優先順序、格式與合法報告保持，驗收見 [ticket 50](docs/tickets/50-comparison-finite-validation.md)。連續任務入口於驗證通過後、雜湊與 builder 前複製 Tasks 及 Params、Stages、Seeds、Comparison 與 StateSwitch 的內外化學列，讓呼叫端與每次執行分開持有資料。標準庫複製保留 nil 與非 nil 空列，不新增並行修改輸入的保證，驗收見 [ticket 51](docs/tickets/51-continual-protocol-ownership.md)。

- 導航範例的環境設定由 nav2d.Config.Resolve 共用既有預設值與驗證。New 與新導航／歸因報告共用解析結果，config_hash 計算已展開的報告設定。WallDensity、StepPenalty、CollisionPenalty、GoalReward 在各自的範圍檢查前拒絕 NaN 與正負無限大。零值預設、有限值的原錯誤與檢查順序、空任務限制、訓練與格式版本保持，歷史報告不重寫。RunNav2D 在計算雜湊前複製 Seeds 與 Policies，單任務、呼叫端及 suite 各任務分開持有兩份切片。驗收見 [ticket 47](docs/tickets/47-nav2d-effective-config-reports.md)、[ticket 48](docs/tickets/48-nav2d-finite-config-validation.md)與 [ticket 49](docs/tickets/49-nav2d-report-config-ownership.md)。

- 模型包的可選 Capacity 由共同驗證入口核對六個整數欄位。沿用已驗證 Trainer 的容量報告，FreeParameterCount 設為 ParameterCount，符合不帶最佳化器遮罩的 Network.Capacity 語意。不再建立額外網路。nil 舊欄位保持省略，不補寫容量。單檔、bundle、保存及建立個體共用此檢查，驗收見 [ticket 46](docs/tickets/46-model-package-capacity-validation.md)。

- `coimnet-名稱/v數字` 形式的版本來源集中列在 [文件索引](docs/INDEX.md)，分成正式格式、規則與雜湊識別、驗證工具格式、測試資料與反例。Go 字串以 AST 解碼，另核對 shell／Python 工具及四份治理 JSON 的版本欄位；每個版本只列一次，來源檔案及行號須相符。索引檢查併入既有 `go test`，無需 Git 或執行時註冊表，驗收見 [ticket 45](docs/tickets/45-format-index-consistency.md)。

- Ticket 34 的最終本機證據見 [verification.json](evidence/TSK-11/stimulus-memory-20261003/verification.json)。九組模型採完整歷史遮罩更新，200 次更新同時核對各參數的 optimizer step。自主行走的 recorded current row 保留 raw stimulus，只有後續生成列寫零。完整一般及 race 通過，模仿誤差下降但模型均為 0／13 命中，沒有導航改善或生物機制證據。

- 刺激記憶工程範例與既有軌跡 v1 分開。逐列刺激在同一次 CSV 讀取及來源指紋驗證中取得，已發生 LED 與預定事件分開，模型只接已送出刺激。完整試次從零開始保留神經狀態，後段遮罩損失透過既有 `StepFrom` 回傳完整歷史。凍結反事實評估先套用原控制轉換，再只清除第一個來源 relocation 列之前的模型可見刺激。自主續走保留原始 raw stimulus 區塊，確保後段刺激相同。搬移界線由同次來源讀取建立的獨立 map 提供，不寫進模型特徵。配對報告為 `coimnet-realnav-memory-counterfactual/v1`，工程解析尺度固定為 1e-6 cm，所有非零值均保存。詳見 [ticket 35](docs/tickets/35-frozen-stimulus-history-counterfactual.md)。核心與舊資料格式不變，固定切分、時鐘、搬移假設、模型及控制預算見 [ticket 34](docs/tickets/34-real-trajectory-stimulus-memory-proposal.md#實作契約與分工2026-10-03)。

- 框架與任務分開：SDK 負責接線、動態、訊號、學習與狀態保存。特定任務程式與訓練結果由使用者專案持有，repo 內僅保留明示範例與必要驗證證據。新增任務範例放 `examples/`，既有 `experiment` 是人工範例套件。五類任務驗收依這個方式實現，不把個人模型作為框架本體。
- 真實果蠅軌跡範例使用 `tasks/nav2d/trajectory` 的逐批匯入與 `(fname, fly)` 試次身分。先按整個試次分割，再從同試次、同區段且時間連續的已觀測列建下一步位移樣本；獎勵、條件、未來位置不進模型。`examples/realnav` 的快照綁定原檔指紋、試次分割與前處理契約，推論於新程序重建分割後評估。這是觀測式行為預測範例，不能作為閉環導航或生物接線機制的證據，驗收見 [TSK-11](evidence/TSK-11/verification.json)。
- 真實軌跡的既有推論與自主評估共用整試次分割檢查：兩組互斥、組內不得重複、完整涵蓋過濾後來源且無未知試次。檢查在樣本建立與輸出發布之前完成，合法快照格式及計分不變，驗收見 [ticket 33](docs/tickets/33-real-trajectory-inference-isolation.md)。
- 真實軌跡的自主行走採獨立工程評估入口，沿用已保存的整試次分割、模型與前處理。目標中心由與資料指紋相同的讀取傳給計分器，不進模型；每試次只以最早合法相鄰列對初始化，後續觀察由模型位移及圓形場地更新產生。模型決策步不換算為動物秒數，碰撞後回傳零實現位移；命中率不當作真實回巢或生物接線證據。固定判準與驗收見 [ticket 32](docs/tickets/32-real-trajectory-autonomous-rollout.md)。
- 可選機制沿用主規格第 8、10、11 章：連續／LIF／按類型混合、適應、局部可塑性與調節分開選擇。只有有實作、有資料或明示假設且通過相容性檢查的組合才能執行。關閉機制須還原參考行為，快照須保存啟用機制及狀態。沒有「載入全部生物機制」的承諾；定位、證據與驗收原則見 [機制說明](docs/model-and-mechanisms.md)。
- Go 與 Insyra v0.3.4 固定。Insyra 公開 API 與編譯探測通過後才採用，缺口先查核再依授權提 issue。
- 2026-09-24 GPU 後端選 WebGPU 作為跨 Mac Metal、Linux Vulkan 與 Windows 的第一條裝置路徑；CPU float64 仍是數值參考。2026-09-25 已在 Mac Metal 驗證 float32 稀疏 drive 與局部反向導數，並以純量零延遲連續核心 adapter 在逐步前向／反向和 CPU 對照。前向拓樸依 target、反向依 source 分組，保留各組內邊宣告順序；漏寫梯度的 Metal shader 迴圈改為每邊獨立寫入並連續重跑驗證。可選的 `learning.NewGPUTrainer`／`RestoreGPUTrainer` 已讓獨立 episode 的 `Step`／`StepFrom` 使用這條核心，並通過 CPU 參數／AdamW 數值對照與 JSON 快照新程序續訓。編碼器與讀出仍走既有 Insyra 路徑，是否由 Insyra 在裝置執行未量測；神經狀態、活化與 AdamW 在 CPU，沒有裝置上參數更新或常駐神經狀態。LIF、混合、延遲、向量／矩陣、重算與完整圖均未支援／驗證，因此 OPS-05 仍未通過。裝置選取、實際執行與回退須分別報告；Insyra dense MatMul 不算稀疏核心加速。Insyra v0.3.4 已提供 CPU `EdgeSum`，稀疏裝置運算與常駐裝置狀態仍待上游提供公開契約，追蹤 [#379](https://github.com/HazelnutParadise/insyra/issues/379)。
- CPU 參考使用 float64、固定邊順序與同步更新。運算/圖儲存為 O(N+E)，不建立全圖 N×N 矩陣。小型稠密運算只用於獨立測試參考。
- Doctor 的 GPU 六項字串使用 `implemented_with_constraints`，以新增的 `core.gpu.details` 明列 WebGPU 純量零延遲連續核心及獨立 episode 的條件、CPU 神經狀態／活化／AdamW、未提供的裝置更新／狀態保存與未驗證的完整圖。頂層 `gpu` 只記錄本次作業系統硬體探測。核心與 Insyra encoder／readout 裝置執行維持 `not_probed`。Doctor 不初始化 GPU 或新增訓練流程，既有 v1 欄位與 API 簽名保持，驗收由 [ticket 44](docs/tickets/44-doctor-gpu-capabilities.md) 負責。
- 第一批用純量連續動態與逐邊參數建立完整梯度。向量/共享參數在稀疏數值介面驗證，隨後接入動態。LIF、調節、真實資料與 GPU 仍是完整必做範圍。
- 未支援的設定返回具體錯誤，不退回別種模型或偷偷縮圖。人工圖與資料產生器在設定、輸出與報告明示名稱。
- 所有可失敗更新先驗證輸入並計算候選結果，再提交狀態；取消、NaN、Inf 或形狀錯誤不得留半次更新。
- LIF 放電事件的反向保留適應狀態跨時間傳遞。不應期內事件導數為零，但突觸跡與適應遞推照常執行。只有完整事件導數非有限而原始相關運算元皆有限時，精確重算事件乘加與替代導數的乘積。普通數值保留原算式，真正超限維持拒絕。一般、重算、化學耦合及混合核心共用私有的例外恢復，見 [ticket 114](docs/tickets/114-lif-event-adjoint.md)。
- 膜時間常數的導數保留各核心原算式與正常累加順序。只有結果非有限、但原始電位、drive、adjoint 與衰退導數皆有限時，精確重算完整差乘積後做一次 float64 捨入；真正超限與原始非有限值依既有梯度檢查拒絕。精確重算不放進正常路徑，以舊新逐位對照驗證保存模型接續的相容性。範圍與驗收見 [ticket 113](docs/tickets/113-membrane-tau-derivative.md)。
- 初期手動稀疏反向計算屬主規格允許方案。Insyra 必須實際參與張量與可用數值/學習操作，不能只是檔案讀取。若其 tape 無公開接合方法，使用明示且測過的完整向量梯度契約，禁止 unsafe/反射或假裝 detach 後仍可微。
- Insyra v0.3.4 的 `Tape.BackwardFrom` 用於兩段 tape 的外部梯度：讀出 tape 接收與 prediction 同形狀的種子，核心 float64 時間回推後，編碼器 tape 接收與 encoded 同形狀的種子。保留原有 float32 轉換、輸入節點順序與核心反向契約。驗收見 [ticket 31](docs/tickets/31-direct-upstream-gradient.md)。`Tape.Custom` 尚未採用。最佳化器保存由 CoImNet 的 AdamW 負責，追蹤 [#376](https://github.com/HazelnutParadise/insyra/issues/376)。`nn.EdgeSum` 的精確加總後一次捨入，與 CoImNet float64／WebGPU 依邊順序加總的數值契約不同，不直接替換核心。CoImNet 目前沒有呼叫 Insyra `Tape.Tanh`，該 API 的 float32 捨入變更不影響現有活化路徑。若之後採用，須另驗證數值與舊模型接續相容性。
- 第一版快照明確命名為 `episode-training/v1`，每個訓練步驟重設神經狀態。資料由固定版本、seed 與樣本索引決定。此格式不保存持續個體的神經歷史或快速權重；若 `Config.Chemistry` 宣告 `ChemicalEpisode`，則保存獨立回合的化學起點、來源時鐘與來源視窗、資源、回饋及讀出表現調節。完整持續個體另用個體 schema。
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

- LIF 放電核心與連續核心並列，`learning.Config` 的 `Dynamics` 與 `LIF` 二選一，節點數與邊數改由選中的核心取得，內部用私有介面包住兩種核心，不公開新抽象。膜電位沿用相同的指數漏電與可訓練 `log_tau`，對外輸出改為衰減突觸跡 `x`，讀出與 upstream 都對 `x`，float32 邊界與既有路徑相同。基礎閾值用 `theta_base = theta_min + (theta_max - theta_min) * sigmoid(theta_raw)` 保持在設定範圍內，建構時要求 `v_reset < theta_min < theta_max`。有限上下界的中間跨度溢位時，先以 sigmoid 係數縮放上下界再組合閾值與斜率。正常範圍保持原算式。供放電比較使用的最終閾值若不可表示，在發布模擬結果或持續狀態前回錯。化學非零偏移先完成既定的 `max(theta + shift, theta_min)` 投影，再檢查最終值。基礎閾值、適應與慢速穩定的部分和若正向溢位、化學偏移為負，使用原始四個有限加數精確相加後取為 float64，再沿用投影與最終有限值檢查。正常浮點順序保持，見[ticket 112](docs/tickets/112-chemical-threshold-sum.md)。有限負加數相加溢位時，仍可依此公式取得有限下限；下限導數為零。明確非有限輸入與投影後不可表示值保持拒絕，見[ticket 110](docs/tickets/110-chemical-threshold-floor.md)。此契約由[ticket 103](docs/tickets/103-lif-threshold-range.md)驗證。`theta_raw` 是新的可訓練參數群組，遮罩為 `Trainable.Theta`，連續模型設為 true 回錯。反向只驗證宣告的 `fast_sigmoid` 替代梯度 `psi(u) = 1/(1 + scale*|u|)^2`：重設分支與不應期步不傳梯度，硬放電事件不做有限差分，有限差分只用在套件私有的測試專用平滑模式，該模式不是產品模式。快照沿用 `coimnet-episode-training/v1`，新增欄位全部 `omitempty`，AdamW 攤平順序固定為 `weights, bias, log_tau, theta_raw, encoder, readout`，連續模型的 `theta_raw` 長度為 0，既有快照的位置與序列化結果不變。LIF 個體持續狀態已於 ticket 16 補齊，見下一個項目。範例協定放在 `experiment.RunLIFThreshold`，`examples/lifthreshold` 與 CLI `examples run lif-threshold` 共用同一份實作。決策、驗收與證據見 [ticket 11](docs/tickets/11-lif-core.md)。

- 神經狀態改為聯集，LIF 取得自己的個體 profile，慢速穩定成為可選機制，模型包成為第三種保存物。`learning.IndividualSnapshot.Neural` 是 `NeuralState{Core, Continuous, LIF}`：`core` 指名核心，兩半只能出現宣告的那一半，缺一半或多一半都是錯誤；`checkpoint.LoadIndividual` 讀到聯集出現前的舊檔（`neural` 直接是連續 `State`）時視為 `core: continuous`，有提交的 fixture 為證。`coreModel` 介面加 `profile`／`newState`／`validateState`／`advance`，`NewIndividual`／`RestoreIndividual`／`Advance`／`TrainEpisode`／`ResetNeural` 對兩種核心走同一條路徑，LIF 個體的 profile 是 `lif-f64-insyra-f32-persistent-inference-episode-learning/v1`。慢速穩定是 `LIFConfig.Homeostasis *LIFHomeostasis`（指標加 `omitempty`，未宣告的設定編碼與既有 hash 完全不變）：每顆神經元一個活動估計 `r` 與一個閾值偏移 `h`，`r(t+1) = r(t) + (dt/tau_rate) * (spike(t+1) - r(t))`、`h(t+1) = clamp(h(t) + eta*dt*(r(t+1) - target_rate), 0, h_max)`、`theta_eff = theta_base + a + h`；活動率原乘加若先得到非有限值，以同一乘加的 FMA 重算後再做有限檢查，保留原有限路徑及真正不可表示值的拒絕，見[ticket 122](docs/tickets/122-homeostasis-rate.md)。`r`、`h` 是狀態不是參數，反向視為常數，關閉時兩個陣列不存在且前向與反向逐位等於未宣告的設定。獨立測試參考維護前向 r/h，但不傳回它們的導數，adaptation 導數保持；驗收矩陣見[ticket 123](docs/tickets/123-homeostasis-gradient-reference.md)。化學回授並用的獨立完整梯度與首次 Adam 更新沿用同一停止梯度契約，見[ticket 124](docs/tickets/124-homeostasis-chemical-reference.md)。持續個體以原始神經活動維護來源視窗，讀出增益不進神經或化學狀態；獨立末狀態與分段恢復驗收見[ticket 125](docs/tickets/125-chemical-homeostasis-state.md)。模型包是 `checkpoint.ModelPackage`（`coimnet-model-package/v1`），帶拓撲指紋（節點數、邊數，以及 sources 全部再 targets 全部、各以 little-endian uint32 編碼後的 SHA-256，編碼與 `simulate` 的 topology hash 相同，但在 `checkpoint` 私下重寫，避免 `checkpoint` 依賴 `simulate`）、`learning.Config`、`learning.Parameters`、宣告單位、證據路徑清單與可 seed 的 schema 版本；`SaveModelPackage`／`LoadModelPackage` 沿用個體快照的 envelope、checksum、嚴格解碼、必填走訪與排他 hardlink，載入時以 `learning.NewTrainer` 重建模型並重算指紋，指紋與設定不符就拒絕。`NewIndividualFromPackage` 放在 `checkpoint` 而不是 `learning`，因為模型包型別住在 `checkpoint`，反向依賴會成環。三種保存物在六種交叉載入全部互相拒絕並指名讀到的是哪一種，模型包的訊息另外說明它只有設定與參數、不能當成完整恢復。決策、驗收與證據見 [ticket 16](docs/tickets/16-lif-individual-and-model-package.md)、`evidence/COR-04/` 與 `evidence/STA-01/`。
- 原生模擬 runner 放在 `simulate`，與 `learning` 分離：不使用 Insyra，不建 encoder／readout／最佳化器，注入是固定線性映射 `input[node] += gain * stimulus[t][channel]`，探針是具名節點集合加宣告的 reduce。核心以套件私有介面包住連續與 LIF，不公開第三種模型抽象；`Protocol` 的核心設定只帶純量，節點、邊、延遲一律由 `connectome.Graph` 提供，protocol 宣告拓撲即報錯。選擇器限 class／type／superclass／subclass／instance／soma_side，null 註記不匹配，零命中除非 `allow_empty` 否則拒絕，解析結果（欄位、值、數量、首末索引）寫進報告。protocol 與狀態快照沿用 `internal/strictjson`（位元組上限由呼叫端給定：protocol 1 MiB、狀態快照 64 MiB；64 層、大小寫別名重複鍵、未知欄位、尾隨資料皆拒絕），該套件由 `connectome` 的 manifest 規則抽出，`DecodeManifest` 與 `Load` 改為呼叫它，行為不變。參數集必須明示 `Source`，目前有兩種。`engineering_uniform_positive`：`weight = gain * 原始 weight`、全興奮、統一 bias／log_tau／theta_raw、延遲零。`derived_release/v1`：由 `params` 的參數集檔提供每條邊的 sign 與強度，`weight = weight_scale * sign * 推導強度`，sign 為 unknown 的邊依 protocol 的 `derived{unknown_sign ∈ exclude|excitatory|inhibitory, weight_scale > 0}` 處理（exclude 為 0、excitatory 為 +|w|、inhibitory 為 −|w|，乘積為零一律存正零）並分開計數；節點純量仍取 uniform 區塊，因此該區塊的 `gain` 必須不寫或為零，避免出現第二個被忽略的強度旋鈕。`FromDerived(set, setSHA256, protocol)` 多收參數集檔的 SHA-256，因為票面同一句要求 hash 納入該指紋，而 `params.Set` 不帶它。hash 沿用「來源標籤加四組陣列」的編碼，derived 另在來源與陣列之間寫入參數集檔 SHA-256、規則 hash、政策與 scale，因此 uniform 既有的 hash 不變；兩種來源都要求 `params.Source == protocol.ParameterSource`。CLI 的 `simulate run --params` 在 derived 為必填、在 uniform 被拒絕，以 `params.LoadWithReceipt` 讀檔（上限沿用 `--max-store-bytes`／`--max-footer-bytes`／`--max-memory-bytes`），建構前先 `CheckGraph`。`RunReport` 增加 `parameter_set_sha256`、`rules_hash`、`unknown_sign_policy`、`unknown_sign_edges`、`weight_scale` 五個欄位，uniform 來源不輸出；`assumptions` 對 derived 寫明「符號由預測傳導物質機率依規則推導、unknown 依宣告政策處理、bias／log_tau／theta_raw 仍是統一工程值」。記憶體只做帳面估算（邊陣列本地一份加核心複製一份，各 24 B/邊，加每節點 80 B、刺激、分塊輸入／輸出／事件緩衝與探針時序），超限回 `ErrCapacity`，不縮圖也不減步數。`dynamics` 對單次 `Advance` 的輸出有 `MaxStateValues` 元素上限，全腦一次只能推進 6 步，因此 runner 內部分塊推進，分塊邊界不改變任何數值（有測試）。門檻只產生 `stability_flags`，不改參數也不改退出碼。相同輸入兩次執行的 `RunReport` JSON 位元組相同，中途保存狀態再接續與一次跑完結果相同。決策、驗收與證據見 [ticket 12](docs/tickets/12-native-runner.md)。

- 動態參數來源放在 `params`，與 `simulate` 平行的第三層：依賴 `connectome`、`feather`、`internal/extsort` 與 `internal/strictjson`，不依賴 `learning`，不用 Insyra。推導輸入不改 manifest，改用獨立規則檔 `coimnet-derivation-rules/v1`（相對路徑加 SHA-256、七種傳導物質齊全的 sign mapping 與 basis、glutamate basis 必須寫出「假設」或 "assumption"、`min_probability`／`min_matched_fraction` 限 [0,1]、`gain` 有限且大於零、`normalizer ∈ none|post_total|pre_total`），理由是 manifest 與 GraphStore 屬第一層不可變結構，分開後既有 manifest hash 不變。規則 hash 為 domain 字串加規則的 canonical JSON 的 SHA-256。固定寬度記錄：tbar 41 B（`(x,y,z)` 各以 XOR 符號位轉 big-endian 共 12 B、null 機率旗標 1 B、七個 float32 28 B）、syn-partners 20 B（鍵 12 B、來源與目標索引各 4 B）、pair 36 B（來源、目標、七個 float32）、ROI 6 B（節點索引 4 B、ROI id 2 B）。四個排序共用記憶體限制：先保留已知輸出陣列（每邊 18 B 加分位數副本 8 B、每節點 20 B），排序緩衝取剩餘的一半，其餘留給 ROI 字典與區段緩衝；暫存預算平均分給四個排序與兩個中介檔。`extsort.Merge` 是 push 介面，兩個排序串流無法同時拉取，因此 tbar 合併結果與 pair 聚合結果各自寫成一個已排序的中介檔（狀態位元組區分 usable／ambiguous／null probability），另一邊以循序游標 merge join，不開 goroutine 也不建全量索引。canonical edge 順序為 (source index, target index, absolute row) 遞增，重複 pair 的邊相鄰，因此可直接 merge join，並以 pair 的 raw weight 總和計算配對比例。參數集檔 `coimnet-parameter-set/v1` 沿用 GraphStore 精神：magic 加八個區段（`edge_weight`、`edge_sign`、`edge_sign_confidence`、`edge_transmitter`、`edge_matched_synapses`、`node_totals`、`node_primary_roi`、`derivation_report`）各有 offset／length／SHA-256，footer 另有 SHA-256 與 magic 尾記，不含時間戳，發布用暫存檔加 hardlink 加目錄同步且不覆寫；讀回逐段校驗並檢查值域，`CheckGraph` 比對 node index／edge order hash。unknown 保持 unknown，四個 unknown 原因（沒配對、低於機率門檻、低於配對比例門檻、規則對應為 unknown）互斥且依此順序判定，門檻比對用已存入檔案的 float32 信心度，讀者可逐位元重現。真實資料（2026-09-15，`evidence/NAT-02/derive-v1/`）：25,563,197 條邊全部推導完成，tbar 45,656,140 個鍵沒有重複鍵也沒有 null 機率，兩檔座標配對比例為 124,025,046／124,025,046 = 1.000（分母是兩端都選入的突觸列），`pairs_not_in_graph` 與 `edges_without_match` 皆為 0，sign 為 +13,636,628／−9,172,513／unknown 2,754,056（10.77%，只來自「低於 min_probability 0.5」1,038,293 與「規則對應 unknown」1,715,763），權重分位數 p0 8.02e-06、p50 0.00231、p100 1，`normalizer_zero_edges` 為 0；牆鐘 207.31 s、max RSS 3,870,539,776 B、四個排序共 9.6 GB 暫存加兩個中介檔 3.6 GB（6 GiB 記憶體與 40 GiB 暫存上限都未觸及），參數集檔 463,451,966 B。同一份推導以兩個不同 build 各跑一次，除了耗時、峰值記憶體與因此改變的檔案 hash 之外，每一個計數、比例、直方圖與分位數都相同。決策、驗收與證據見 [ticket 13](docs/tickets/13-parameter-adapter.md)。
- 空模型、判讀協定與比較矩陣全部放在 `simulate`，`connectome` 與 `params` 不動。空模型是記憶體內的衍生物，不另存 GraphStore 或參數集檔，理由是第一層是不可變結構，落盤的衍生圖會把假的來源指紋寫進那個格式；報告改記 `null_model{kind, seed, prng, swap_factor, attempts, applied, rejected{self_loop, duplicate}, topology_hash, parameter_hash}`，讀者用原 store 加 seed 重算後比對 hash。PRNG 固定 `math/rand/v2` 的 `rand.New(rand.NewPCG(seed, 0))`，報告寫 `"pcg"`。`degree_preserving_rewire` 的嘗試次數為 `ceil(swap_factor × E)`，每次抽 `IntN(E)` 與 `IntN(E-1)` 後把第二個索引跳過第一個，確保兩個相異索引只花兩次抽樣，因此抽樣序列只由 seed 與嘗試次數決定；先判自環再判重複 pair，兩種原因分開計數，`applied + self_loop + duplicate == attempts`。pair 集合是線性探測的開放定址 uint64 雜湊表，槽位存 `key+1`（0 代表空槽，因此自環 0→0 也能存），key 為 `source<<32|target`，hash 用 splitmix64 finaliser，大小取 `2E` 以上的最小 2 的冪（至少 2），因此永遠不超過半載；刪除用 Knuth 6.4R 的 backward shift，不用墓碑，因為每次成功交換要刪兩次。原圖若本來就有重複 pair 就直接拒絕，因為集合無法表示重數（MaleCNS v1.0 的 `duplicate_pairs` 為 0）；原圖自有的自環可以被換掉但不會被造出來。兩種 shuffle 是 Fisher–Yates，`attempts` 與 `applied` 都是抽樣次數 `n-1`，`swap_factor` 必須不寫或為零。`sign_shuffle` 置換 `EdgeSign` 的副本再走 `FromDerived` 的 unknown 政策，`weight_shuffle` 置換強度陣列（uniform 是 `gain × 原始 weight`，derived 是 `EdgeWeight`），兩者都以值複製 `params.Set` 並只換掉那一個陣列，原圖、原參數集與呼叫端陣列都不變（有 hash 前後比對的測試）。`DeriveNullModel` 與 `OriginalVariant` 比票面多收參數集檔的 SHA-256，理由與 `FromDerived` 同一句：hash 要指名檔案而 `params.Set` 不帶它。`topology_hash` 是 sources 全部再 targets 全部、各以 little-endian uint32 編碼後的 SHA-256。`Build` 改成串流原圖拓撲後呼叫 `BuildVariant`，兩者共用同一條驗證、記帳、建核心與解析路徑；`RunReport` 一律多出 `topology_hash`，`null_model` 只在空模型出現，uniform 與 derived 既有欄位與 `protocol_hash`／`parameter_hash` 都不變。具名集合是多個選擇器的交集，解析結果記錄每個選擇器、節點數與「遞增節點索引以 little-endian uint32 編碼」的 SHA-256，交集為空只有在每個選擇器都宣告 `allow_empty` 時才接受。四種指標只在 LIF 可用，連續核心只允許 `mean_output`（LIF 也可用，讀的是突觸跡）；`spike_fraction` 是窗內至少放電一次的節點數／集合節點數，`mean_rate` 是窗內放電事件數／(節點數×窗長)，`latency_to_first_spike` 的 numerator／denominator 是「首次放電步」與「窗起點」而值是兩者相減，沒有放電時整個結果歸零並標 `defined=false`，`activity_ratio_vs_baseline` 是窗與 baseline 兩個 `mean_rate` 相除、分母為零時未定義但仍寫出已算出的分子。分母為零一律未定義，不寫 0。門檻只有 `>=`、`>`、`<=`、`<`，未定義的指標一律 fail 並記 `reason: "undefined"`，門檻結果不改退出碼。集合追蹤由 `Runner.TrackSets(sets, windows)` 宣告，`Run` 逐步對每個窗內的步數累積每個追蹤節點的放電計數、集合首次放電步與輸出總和，探針語意不變，量測值由 `Measurements()` 讀回並在成功的 `Run` 之後才更新。記帳同樣只是帳面估算：變體是 `4 × 8 × E`（sources、targets、權重與 shuffle 副本），rewire 另加 `槽數 × 8`；追蹤在該次 run 已記的位元組之上再加每個集合每個窗 `4 × 節點數 + 64`，總和超過 `Limits.MaxMemoryBytes` 就回 `ErrCapacity`。`CompareProtocol`（`coimnet-simulate-compare/v1`，1 MiB 上限、strictjson）宣告一份 run protocol 加集合、指標、門檻、空模型與 seeds，空模型項目不得自帶 seed；`Compare` 先跑原圖一格，再依宣告順序跑每種 kind × 每個 seed 一格，每格是獨立的 Runner 與完整 `RunReport`。`MetricSummary.PerKind` 用切片而非票面的 map，保留宣告順序（Go 的 map JSON 會改成鍵排序）。分位數用 `params.quantiles` 的最近秩規則（`rank = ceil(q×n)` 夾在 `[1,n]`，取第 rank 個），與 `Monitors.RateQuantiles` 的線性內插不同，因為每個分位數都要是某個 seed 真的產生過的值；百分位是 `(小於的個數 + 0.5 × 相等的個數) / 有定義的個數`，原圖或全部 seed 未定義時不給。相同輸入兩次 `Compare` 的 JSON 除了每格的 `wall_seconds` 位元組相同。CLI `simulate compare --store --protocol [--params] [--out-dir]` 印出報告，`--out-dir` 必須不存在、在載入與參數檢查之後、跑矩陣之前以 `Mkdir` 佔用，每格另存 `cell-<index>-<variant>.json`；`--params` 規則與 `simulate run` 相同。`CompareCell` 另有 `deltas_from_original`，依指標順序記每一格與原圖的差，兩邊都有定義才有值，原圖那一格的差在指標有定義時為 0；差不在 `MetricSummary`，因為分位數與百分位描述分布，差描述單一格。真實資料（2026-09-15，`evidence/NAT-03/compare-lif-v1/` 與 `evidence/NAT-05/compare-continuous-v1/`）：同一份 store 與 `params-derive-v1.coimparams`，同一份刺激，兩個核心各跑十格（原圖加三種 kind × seed 1、2、3），每格 165,122 個神經元、25,563,197 條邊、300 步。`degree_preserving_rewire` 在 `swap_factor` 1 下每個 seed 嘗試 25,563,197 次，seed 1 成功 25,235,685、拒絕 724 自環與 326,788 重複，seed 2 成功 25,234,331、拒絕 715 與 328,151，seed 3 成功 25,233,927、拒絕 768 與 328,502；三者的 `applied + self_loop + duplicate` 都正好等於嘗試次數。兩種 shuffle 每個 seed 抽 25,563,196 次（n−1），沒有拒絕。LIF 矩陣 890.94 s 牆鐘（十格合計 852.24 s）、最大 RSS 5,909,626,880 B、峰值記憶體 9,543,671,152 B；連續核心矩陣 929.97 s（十格合計 890.17 s）、最大 RSS 5,659,115,520 B、峰值 10,513,997,816 B；兩者都沒碰到預設 8 GiB 的帳面上限。兩個矩陣是兩個獨立行程，十格的變體名稱、seed、`topology_hash`、`parameter_hash` 與整個 `null_model` 區塊（含三種計數）逐格相同，所以「原 store 加 seed 可重算」這句有實測；LIF 第 0 格的 `protocol_hash` 與 probe、monitor 序列也與 NAT-02 的單次全腦執行完全相同。`population_sync` 仍未實作。決策、驗收與證據見 [ticket 14](docs/tickets/14-null-models-and-behavior.md)。
- 原生模擬之上的局部可塑性全部放在 `simulate`，直接用 18 的 `plasticity` 套件，`plasticity/` 與 `learning/` 都沒有動。`Protocol.Plasticity *Plasticity`（指標加 `omitempty`，未宣告的 protocol 編碼與既有 hash 完全不變，NAT-01 的 `befa9f1d…` 由測試釘住）帶 `rule`、`edges` 選擇器或 `all` 二擇一、`gate_channel`、`gate_scale` 與 `frozen`。規則先由 `plasticity.New` 在一條邊的核心上驗證，再由同一個原生驗證入口拒絕非 nil 的 `GateReceptor`／`DecayEReceptor`，包含明確的索引 0 與 `frozen` 宣告。原生 runner 沒有化學綁定，使用固定 `Rule.DecayE`，因此 protocol 與 `Build` 都不接受受體宣告。閘門直接從刺激矩陣讀：`gate(t) = gate_scale × stimulus[t][gate_channel]`，所以不需要第二份輸入也不需要新旗標；閘門通道必須在刺激的通道範圍內，而且不得是任何 `injections` 或 `injection_groups` 的通道，否則同一個通道既驅動神經元又開閘門、事後分不開。`stdp_pair` 在連續核心於 `Validate` 就被拒絕（與 `learning.EnablePlasticity` 同一個選擇）。`edges` 是節點選擇器，啟用「兩端都落在解析出的集合內」的邊，也就是那個族群內部的連線；票面只寫「Selector 或 all」，邊的定義是本票的決定，零命中由 `plasticity.New` 拒絕。runner 在宣告區塊時把 `maxChunk` 強制為 1，每步先 `Effective(params.Weights, params.Signs, fast)` 算出 `w_eff` 交給核心，核心推進一步後再以該步的 pre／post 更新 eligibility、pair 跡與 plastic，順序與 `learning.Individual.AdvanceGated` 完全相同（LIF 的 pre 是突觸跡、post 是當步事件；連續核心兩者都是輸出），`frozen` 只做前半段。快速狀態與神經狀態一樣「整個 `Run` 成功才提交」，失敗的 `Run` 兩者都不動；`PlasticState()`／`RestorePlasticState()` 是 `learned_then_frozen` 用的存取點，後者以 `plasticity.ValidateState` 檢查規則與啟用邊。`RunReport.Plasticity *PlasticityReport{rule, enabled_edges, gate_channel, frozen, clamped_by_w_min, clamped_by_plastic_max, plastic_l2_before, plastic_l2_after, chunk_size, wall_clock_penalty_note}`（omitempty），`assumptions` 在宣告區塊時多一句寫出規則、四個常數、閘門、「固定符號邊被 `w_min` 擋住不跨零」與「快速變化不寫回參數集」。記帳在既有帳面上再加：保留的 sources／targets 各 8 B/邊（`plasticity.Step` 要讀兩端）、每步的 `w_eff` 8 B/邊、模型的啟用邊索引 8 B/邊，以及每條啟用邊 4 個（`stdp_pair` 8 個）float64。`ParameterSet` 多 `Signs []int8`（`omitempty`），由 `FromDerived` 寫入「實際採用的符號」（unknown 邊在 `exclude` 政策下是 0，因此是自由邊），uniform 來源不帶；它刻意不進 `fingerprint()`，因為那個編碼是既有執行記錄的契約，而 derived 的 hash 已經指名參數集檔、規則 hash、unknown 政策與 scale，符號就是這四者的函數。`CompareProtocol.LearningVariants []string`（omitempty）與 run 的 `plasticity` 區塊必須同時宣告，`original` 必須排第一（所有 `deltas_from_original` 由它取），`learned_then_frozen` 必須排在 `plastic` 之後。三格都跑在原圖上：`original` 把區塊拿掉，因此整份 `RunReport`（含 `protocol_hash`）與 ticket 14 的那一格逐位元相同，這正是 root 決策 4「關閉即還原」可以完全逐位比對的位置；`plastic` 照宣告跑；`learned_then_frozen` 以同一份宣告加 `frozen: true` 建格，並把 `plastic` 跑完的快速狀態灌進去，所以它的 `w_eff` 正好是 `base + plastic`。空模型那幾格仍是 ticket 14 的格子且不帶可塑性——把學習拿去跟打亂的接線比是另一個問題、另一份 protocol。`reproduction` 只在宣告 `learning_variants` 時多接一段，既有比較的字串一字不變。宣告區塊之後只有兩個欄位一定不同：`protocol_hash`（它就是宣告的指紋）與多一句的 `assumptions`；其餘每一個 probe 值、monitor、旗標、參數與拓撲 hash 與步數都逐位元相同，測試以「拿掉這三個欄位後位元組相等，再單獨比對 assumptions 只是多了那一句」表示。CLI 兩個子命令都沒有新旗標，區塊只走 protocol，`--help` 各自寫出區塊與三格。真實資料（2026-09-16，`evidence/NAT-06/compare-plastic-v1/` 與 `evidence/NAT-06/compare-plastic-persistent-v1/`）：同一份 store 與 `params-derive-v1.coimparams`，NAT-02 的脈衝加上刺激通道 1 的閘門（第 30–49 步常數 1），`hebbian_rate`、`decay_e` 0.5、`plastic_max` 0.01、`w_min` 0.0001、`gate_scale` 1，啟用全部 25,563,197 條邊，兩份 protocol 只差在 `decay_p`（0.5 與 0.999），各跑三格 300 步。1252.44 s／最大 RSS 6,605,570,048 B（三格 137.51／611.82／494.04 s）與 1217.28 s／6,558,416,896 B（89.58／598.64／519.41 s），兩次退出碼都是 0；事前用 25,563,197 條邊量出每步可塑性額外成本約 0.30 s（`Effective` 0.12 s、`Step` 0.19 s）加 NAT-01 的每步 0.31 s，估每格約三分鐘，實測遠低於 90 分鐘門檻，沒有縮短 protocol。`clamped_by_w_min` 兩次都是 0，`clamped_by_plastic_max` 是 33,391,125 與 34,549,873。`original` 那一格的探針序列、monitors、assumptions、`core_config_hash` 與 `parameter_hash` 與 NAT-02 的全腦單次執行逐位相同（只有 `protocol_hash` 因為刺激多了閘門通道而不同），而且兩次執行（不同 build 的 binary）的第 0 格位元組相同。`decay_p` 0.999 時快速變化的 L2 由 0 長到 11.902496517509498，`learned_then_frozen` 前後都是同一個值：ALIN 的 `mean_rate`（閘門後）+0.0585／凍結 +0.07283333333，`descending_neuron` 的 `spike_fraction` −0.07229832572／−0.0502283105，`vnc_motor` 的 latency 在凍結那一格少 3 步。`decay_p` 0.5 時閘門關上後 plastic 每步減半，250 步後只剩 8.083189933121256e-75，`learned_then_frozen` 的整份報告在三個預期欄位之外與 `original` 位元組相同、十四個有定義的差值全為 0（第十五個三格都未定義），但 `plastic` 那一格仍有差——快速變化撐不撐得到最後由 `decay_p` 決定。決策、驗收與證據見 [ticket 19](docs/tickets/19-plasticity-on-runner.md)。
- 受限更新與完整最佳化器流程放在既有的 `learning.Trainer`，不另開訓練器。可更新範圍是
  `Options.Trainable` 的群組旗標 AND `Options.Masks *UpdateMasks{Edges, Nodes}` 的逐項遮罩
  （邊遮罩對 weights，節點遮罩對 bias／log_tau／theta_raw），被遮罩的參數連 Adam 動量、步數、
  權重衰減與範圍投影都整個跳過。`Config.EdgeSigns []int8` 把有依據的邊改成對數幅度
  參數化（`w = sign * exp(rho)`，`Parameters.Core.Weights` 從此存 raw，核心一律經
  `EffectiveWeights`，梯度乘 `w` 鏈鎖），符號永不翻轉、幅度不為零，下溢由
  `Config.MinLogMagnitude`（預設 −20）投影擋住。`Options.Ranges *ParameterRanges` 在更新後
  以投影套用並在 `StepResult.Projected` 分群組計數，投影不動動量。一步的順序固定為：
  梯度乘 `Options.LossScale` 再除回（乘積非有限就整步拒絕，不留半次狀態，預設 0 表示 1）→
  加進累積器 → 只有累積滿 `Options.AccumulateSteps` 才平均 → 對平均值套 `ClipNorm` →
  AdamW 用 `LearningRateAt(options, updates)` 的學習率（權重衰減也用同一個）→ 範圍投影。
  累積視窗為 1 時直接用當步梯度，不額外配置，數值與本票之前逐位相同。
  `Options.Schedule *Schedule{Kind constant|step|cosine, WarmupUpdates, DecayUpdates,
  FinalFactor, StepEvery, StepFactor}` 的學習率是 `Updates` 的純函數，恢復後自然接續；
  暖身對三種 kind 都適用，該 kind 不讀的欄位寫了值會被 `NewTrainer` 拒絕，避免出現被忽略的旋鈕。
  快照新增 `TrainingSnapshot.Accumulator *GradientAccumulator{Sum, Count}`，只有視窗未滿時
  存在（`Sum` 長度等於參數數、`Count` 落在 `[0, AccumulateSteps)`，`Count` 為 0 時 `Sum` 必須全零），
  `StepResult` 新增 `Applied` 與 `Accumulated`，`GradientNorm` 一律是該步自己的梯度範數。
  `masks`／`ranges`／`edge_signs`／`min_log_magnitude`／`loss_scale`／`accumulate_steps`／
  `schedule`／`accumulator` 全部 `omitempty`，舊快照載入後等於「不限制、不縮放、不累積、不排程」。
  決策、驗收與證據見 [ticket 17](docs/tickets/17-constrained-updates-and-optimizer.md)、
  `evidence/COR-10/` 與 `evidence/LRN-03/`。

梯度累積的候選 Sum 在每項有效遮罩的加總後檢查有限性。不可表示的總和在部分視窗提交或完整平均之前回錯，保留原參數、最佳化器、累積器與個體回合計數。正常加總順序、平均、裁切與保存格式保持。公開入口與相容性由[ticket 98](docs/tickets/98-training-failure-composition.md)驗證。

## 功能流程、錯誤與驗證責任

### 六步任務期限的訓練對照

[ticket 40](docs/tickets/40-six-step-deadline-training-comparison.md) 在測試範例內加入六步就結束任務的訓練組，與原逾時後計入預估價值的設定比較。兩組沿用既有收集器、模型、隨機串流與更新器。期限轉換使用獨立拷貝，原抵達、逾時與後續預估值另行記錄。評估沿用 ticket 37 的左右配對提示測驗及判準，公開預設設定與舊證據保持不變。

### 固定 PPO 梯度診斷（2026-10-04）

[ticket 39](docs/tickets/39-ppo-gradient-diagnostic.md) 只在測試範例內重播 ticket 38 的固定回合。每次由原模型副本的 `Individual.Advance` 計算輸出，與 `rl.Update` 的前向一致。凍結 collector 的 GAE、目標值與舊行動機率，以 `rl.Loss` 分離三項 upstream，再用 `Network.LossGradientFrom` 保存完整梯度。總 upstream 不做回合平均。weights、bias 與 readout 是可更新群組，encoder 與 log_tau 凍結。零範數的角度不定義。分離梯度只用於觀察，完整 upstream 的 `StepFrom` 須和原更新的參數、Adam、累積器與步數完全相同。方法、精度容許範圍及有限差分驗收由票面固定，沒有調整訓練或公開契約。

### 語音資料與串流

`tasks/asr.Stream` 沿用辨識器的 log-mel 前處理與持續個體，快照需要同時識別取樣率、前處理版本與字母表，僅比對核心形狀不足以安全恢復。每次 `Feed`／`Flush` 成功才提交狀態，取消或數值錯誤保留呼叫前的完整狀態。已分析過的重疊樣本不能重複補窗，尚未分析過的短音訊則在 `Flush` 補零成一窗。驗收歸 ticket 29。

`Stream.Save`／`Recognizer.LoadStream` 使用 `coimnet-asr-stream/v1` 的單檔 envelope，包含完整 `coimnet-individual-checkpoint/v1` 文件與待處理音訊、CTC 狀態、前處理身分；兩層各自核對 schema 與 SHA-256，外層檢查必填欄位、null 與未知欄位，內層沿用 checkpoint 個體的原始 JSON、數值與神經狀態驗證。待處理樣本先按視窗長度限量掃描，不可能的輸出字數先與神經步數比較，之後才配置 rune 陣列。64 MiB 上限與排他發布沿用 checkpoint，認證或簽章不在此格式內。檔案恢復仍要經 `RestoreStream` 比對辨識器語意，跨程序人工音訊接續與不中斷結果相同；此契約不表示已能保存超過上限的全圖個體。驗收歸 ticket 29。

ASR 資料清單只指定使用者提供的 PCM16 WAV、文字稿、取樣率、聲道、說話者、場次與授權來源。`ReadDataset` 以 `os.Root` 固定清單的資料夾根目錄，清單與錄音都透過該根目錄開檔，拒絕指向外部的連結；核對音檔與宣告一致，按混音、線性重取樣、峰值正規化的順序產生樣本，並保存原檔指紋與處理報告。讀取單筆 WAV 時，來源 SHA-256 與解碼必須使用同一份位元組；在配置處理緩衝前檢查來源大小及輸出長度。預設每檔原始資料 64 MiB、每檔處理緩衝估算 512 MiB、全批處理緩衝估算總額 2 GiB，可用 `ReadDatasetWithLimits` 明確調整；這是資料配置估算，不是 RSS 上限或串流語料讀取器。`SplitBySpeakerSession` 以共享說話者或場次形成的連通群分割，只有一群時拒絕分割；不同大小的群組在最多 2,000,000 個計數容量及 10,000,000 個工作單位內找全域最接近的可行分割，超過上限回錯，不交付偏差未知的近似結果。`MeasureStreaming` 與 `EvaluateStreaming` 的耗時只量本機離線逐塊計算，文字、CER／WER 與耗時分開記錄，不把未量到的影格數寫成 0。契約與尚待驗收項見 [ticket 29](docs/tickets/29-real-tasks-ocr-asr-text-and-media-generation.md)。

### PPO 更新（2026-09-21）

PPO 的機率與損失共用位移後的 log-softmax。`-Inf` 可作動作遮罩，至少一個動作須有有限分數。遮罩動作的 `LogProb` 可為 `-Inf`，`Loss` 回成功時的分項、比值與梯度須全部有限。NaN、+Inf、全部遮罩、非有限純量及不可表示的計算回錯誤。關閉的價值項不計算差值平方。公開介面、模型設定與狀態限制保持，驗收見 [ticket 41](docs/tickets/41-ppo-objective-numerical-correctness.md)。

`learning/rl.Update` 回傳獨立的新個體，保留原個體及回傳個體的現行神經狀態。策略版本識別設定與參數。fresh-zero rollout 沿用 `Trainer.StepFrom`。CPU 純量連續核心的合法持續狀態使用 `StepFromState`，包含延遲歷史，每個 epoch 的算分與梯度從同一份 InitialNeural 出發。段起點的電位與歷史視為常數，只回推目前段，段內沿用 Truncation。`BurnIn` 只遮掉前綴的直接損失，後續梯度仍可回傳到段內前綴。State 最新歷史值依原驗證容許 4 ULP，但不重建或改寫保存值。新 trace 擁有副本。非零狀態的 LIF、mixed 與 Recompute 明確拒絕，保留既有可用的 fresh-zero 路徑。vector 與 GPU 的個體持續狀態原本就未提供。可塑性、化學機制及 `MiniBatch=1` 限制保持。所有 rollout 驗證後才更新候選副本。StepFromState 在提交前以同一保存狀態驗證候選參數，失敗保留原參數、最佳化器與未完成累積視窗。API 為新增方法，沒有保存格式或資料遷移。契約與驗收見 [ticket 43](docs/tickets/43-stateful-continuous-ppo.md)。環境學習另以 `RunPPO` 的取樣與學習成績驗收，不以更新入口測試取代。

ticket 26 的 `RunPPO` 沿用此更新器，人工模型與走廊收集器放在既有 `experiment` 範例中。collector 在個體副本上收集，以獨立 RNG 依動作機率取樣，時間上限的 bootstrap 使用同一遞迴狀態處理下一筆觀察。評估與隨機對照各有獨立 RNG，不改訓練個體。報告固定協定、初始與最終版本、完整快照摘要、各 seed 曲線與失敗紀錄；驗收門檻、檔案責任及重現命令以 [ticket 26](docs/tickets/26-continual-matrix-imitation-ppo-and-bio-inspired-protocols.md#環境學習整合契約2026-09-22) 為準。

[ticket 36](docs/tickets/36-synthetic-goal-cue-audit.md) 在測試範例內補凍結策略的 sampled／greedy 提示對照，不改公開 PPO 契約或訓練設定。每回合獨立 PCG 使用相同環境種子與 0x1004 串流，原個體完整快照保持不變，重訓摘要必須等於 RunPPO。描述性工程判準與導航能力分開：20 步允許走遍兩端，通過抵達與提示反轉比較不足以證明提示記憶必要性。

[ticket 37](docs/tickets/37-short-goal-cue-audit.md) 使用 6 步，排除至少 9 步的兩端搜尋。環境種子 1000～1039 按左右排序配成 20 對，每對左右、前後及提示干預共用 PCG(1000 + pair_index, 0x1005) 初始狀態。清除提示後，同一對的策略直到任一回合終止前逐步相同，因此最多成功一側。重用 ticket 36 的訓練與評估輔助函式，完整快照與相同設定 RunPPO 比對，未新增公開 API。

[ticket 38](docs/tickets/38-ppo-training-feedback-diagnostic.md) 固定同一六步設定，只保存 collector 與原更新路徑的 600 回合診斷。每 20 更新在個體副本上執行固定左右的三種提示 greedy 探測，不消耗訓練 RNG。每次曲線、終止統計與最終快照須等於 RunPPO，完整舊報告也與 ticket 37 相同。GAE 與回饋分開判讀，觀察性偏向不能當作唯一因果結論，沒有修改更新器或公開契約。

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

蒸餾與 CTC 的 log 機率在分數位移後正規化。蒸餾溫度作用於位移後差值，KL 與 CE 使用相同的私有計算。partial 機率、類別映射及混合設定保持。CTC 保留路徑合併與無合法路徑策略，每影格的條件路徑機率總和須在 1e-8 內接近 1，超出時回數值精度錯誤。NaN／Inf 輸入或不可表示的數值計算回錯誤，清空損失、梯度及報告。ZeroOnImpossible 只處理無合法路徑，不吞掉數值錯誤。純計算不改輸入，驗收見 [ticket 42](docs/tickets/42-distribution-ctc-numerical-correctness.md)。

稀疏算子先用規格第 9.2 節的手算值，再以隨機小圖、batch、向量與共享參數對照獨立參考。平滑完整路徑以中心有限差分驗證每組參數與輸入。替代梯度只驗證宣告公式，不拿硬放電做有限差分。

`evidence/gradient-audit-20260913.json` 稽核 bridge 的 `epsilon=1e-2, 2e-3, 1e-3, 1e-4`；目前測試值 `2e-3` 的六組梯度最大絕對誤差為 `9.288636276e-7`，最大相對誤差為 `1.583954284e-3`，通過本稽核選定的 `audit_combined_atol_1e-5_rtol_1e-4` 比較判準 `absolute_error <= 1e-5 + 1e-4*abs(fd)`，此判準不是第 9.4 節逐字公式。第 9.4 節的 `atol=1e-5, rtol=1e-4` 是 float32 前向與參考值比較的工程預設；平滑 float64 有限差分使用相對誤差起始門檻 `1e-4`，近零梯度另看絕對誤差。純 float64 core 的 tanh、softplus 各組在四個 epsilon 的 `relative_error_gt_1e-4_count` 均為 0，近零絕對檢查計數也均為 0；bridge 跨越 Insyra float32 邊界，`epsilon=1e-4` 有三個稽核合併判準失敗，因此保留 `2e-3`，不把結果推廣成所有配置或純 float64 保證。

人工延遲關聯固定資料產生器、訓練/驗證/測試 seed、更新預算與門檻後才跑保留資料。報告所有 seed、更新前後指標、梯度、凍結與打亂對照。數值通過不等同真實圖或五類任務成功。

延遲關聯報告 v2 的打亂對照只對訓練標籤做固定 Fisher–Yates 置換，保留原標籤分布，並記錄方法與種子。既有 v1 報告使用獨立產生的標籤，不能當成 v2 置換對照的證據。訓練與保留資料的 counter 範圍不得重疊，即使 seed 不同也須檢查。

建置、完整單元/race/vet、相依性驗證及已支援例子全部執行。真實子圖、全圖、各目標平台和 GPU 分開驗收。CPU 正確性是裝置比較基準。

Mixed／LIF 的化學關閉配置量測在一般建置執行。race 建置的 `sync.Pool` 會隨機丟棄 `fmt.Sprintf` 的 printer，無法用兩次獨立配置計數驗收額外成本。同核心的關閉／中性輸出與梯度逐位對照在兩種建置都執行。跨 Mixed／LIF 的比較沿用原有 compiler-rounding 契約。

普通與競態建置的 LIF 人工案例，以相同編碼後輸入和讀出上游對照公開 Network／LIF，並以獨立完整前向／反向取整參考及原執行檔指令判讀差異。來源層的 candidate 融合項可以不同，既有公式與同建置逐位重現判準保持。不得把人工案例的預測相同解讀為長期任務品質相同。參考與限制見 `evidence/LRN-01/lif-float-audit-20261015/reproduction.md`。

## 假設與資源

本機優先、CPU、離線教師、零雲端費用。大型資料下載先查大小、可用空間與預算。全圖訓練須預估包括 E×B×T 的狀態歷史，超限拒絕而非自動縮小。外部資料 schema、Insyra 自訂梯度介面與裝置能力須由實測確認。

實測資源與適用設定見 [資源說明](docs/resources.md)。反向視窗（`Options.Truncation`）只控制梯度傳遞，前向 trace 仍保存完整輸入序列歷史，縮短視窗不會降低歷史配置量；每一步的梯度實際往回追溯幾步寫在 `StepResult.GradientHorizonSteps`（視窗短於輸入列數時是視窗，否則是整段），更早的列仍影響預測與狀態，但收不到梯度。要降低反向歷史用 `Options.Recompute{SegmentSteps}`（LRN-02）：`Step`／`StepFrom` 不保存核心 trace，前向逐段推進，反向從每段起點的檢查點重跑該段，梯度、參數、最佳化器狀態與更新次數和完整歷史逐位相同；`Predict`、`PredictAll`、`Spikes` 一律走完整前向。只有連續與 LIF 核心支援，混合與向量核心在 `NewTrainer`、`RestoreTrainer`、`Individual.ResetOptimizer` 寫入前就拒絕。重算省下的是核心歷史（`T = 256`、`S = 16` 時從 257 列降到 81 列），網路層仍保留整段的核心輸入、核心輸出、上游梯度與輸入梯度，所以實測最大 RSS 只省 24–34%，不是數量級；有限序列前向由連續與 LIF 的 `ForwardRecompute` 逐段執行，保留目前段落及已發生的延遲前史，容量不經持續狀態 API。完整輸出矩陣仍需 T×N 空間，LIF 段落另有 O(T) 的列指標，這不是整個訓練器的固定記憶體上限。持續個體的 `MaxStateValues` 保護另行保留，驗收見 [ticket 56](docs/tickets/56-recompute-sequence-capacity.md)。未宣告 `Config.Chemistry` 時，訓練回合從零神經狀態開始，不推進可塑性、化學狀態或亂數。宣告化學起點時，連續／LIF 重算保存並重播私有化學與來源狀態，不提交持續個體，契約見下方「受體位置與一般化學訓練」。原有無化學重算證據見 [LRN-02](evidence/LRN-02/verification.json)。啟動前記憶體預估（OPS-06）已由 `resources.Estimate` 提供；全圖訓練（OPS-07）依原需求追蹤，文件估算不取代這項功能。

連續核心的分段重算在單段及必要延遲前史內保存活化輸出，供驅動與權重梯度共用。每次段落前向建立輸出列時，每個節點活化只計算一次，相鄰相同延遲的邊共用選出的列，連續相同目標的邊在 float64 本機變數依原順序累加，換目標前及結尾寫回。已驗證等長的連線陣列在迴圈前切片，避免重複索引檢查。宣告邊順序、乘加式及導數保持。檢查點仍只保存電位，`RecomputeStats` 仍只統計電位列，新增輸出暫存的實際成本須另量測。零邊反向略過輸出暫存，零邊前向維持原本的公開輸出計算。有邊的 `ForwardRecompute` 回傳新產生的輸出列，不另活化一次。公開契約與快照格式保持，驗收見 [ticket 57](docs/tickets/57-recompute-activation-reuse.md)。

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

## 受體位置與一般化學訓練

可選受體位置指名當地化學區域並保留原研究部位、證據及映射版本。沒有個別節點對照的研究只能標示細胞類型推論。實測位置不推論反應係數，純量神經元仍整合各部位的效果，不表示多區室膜模型。

一般 Network／Trainer 用可選 ChemicalEpisode 宣告獨立回合起點，包含化學狀態、來源記憶、資源、已到達回饋與讀出表現調節。每個回合使用同一份起點的私有副本，不推進持續個體。Individual 沿用 EnableChemistry 的唯一持續入口，TrainEpisode 從當下化學狀態、神經時鐘與上一列活動建立獨立副本。兩個入口不得同時宣告互相矛盾的起點。來源時鐘與化學更新次數分開保存。

化學前向依序為上一列原始核心輸出、來源釋放、濃度更新、受體反應、增益／偏移／閾值、核心輸出與讀出。答案只進損失。反向包含濃度的時間關係、神經活動驅動來源的回授，以及讀出表現調節對受體反應的影響。控制器參數在一般核心訓練中固定，原有控制器目標訓練入口保持。一般回合未明示 InitialActivity 時，首步來源讀核心初始輸出，其活動梯度須抵達 Core.Initial。明示初始活動、持續狀態的既有歷史與截斷視窗外活動保持常數，LIF 初始突觸輸出沿用常數零。

CPU 純量連續、LIF 與混合核心沿用自己的動態規則。電流效果先於 bias，LIF 的非零閾值偏移受到 theta_min 下限，觸及下限時相關導數為零。關閉機制保持原始數值與可選欄位缺省相容。GPU、向量及其他未支援組合在運算前明示拒絕，不靜默忽略宣告。

活動控制器的視窗以可選 SourceStates 保存，舊檔缺省表示重新建立來源記憶。新檔保存完整邊界並嚴格核對來源種類、數量、視窗形狀與已填列數。每次操作先複製可變來源，神經、濃度、來源記憶與報告在整次成功後共同提交。一般訓練也先驗證候選參數再提交最佳化器，取消與數值錯誤保留原狀。

四種公開 bundle 載入入口在解碼與正規化前檢查原始必填欄位。個體與模型包沿用單檔的欄位及 null 規則，訓練與任務訓練共用化學專用檢查，資源 map 保留。陣列抽取留下的空陣列先通過欄位檢查，再依 manifest 還原並驗證形狀。選用欄位與合法舊檔省略規則保持，驗收由[ticket 97](docs/tickets/97-chemical-restore-composition.md)管理。

bundle 訓練的化學 resources 與 pending_feedback 必須有鍵，合法空集合可保存為 null。驗證副本僅對這兩個存在且為 null 的值作空集合檢查，解碼後保留原 nil 語意。單檔 episode 與個體快照的既有嚴格 null 規則各自保持。

反向使用全新工作階段，來源多列回授相加且遵守既有固定截斷邊界。連續／LIF 分段重算保存來源視窗、化學狀態與時鐘，重播不提交持續狀態。逐位對照及新程序接續已通過，包含部分累積梯度、最佳化器與化學起點。完整一般與 race 各 58 個套件、格式、build、vet、相依性及 CLI 接續比對通過，見 [最終工程證據](evidence/MOD-02/receptor-chemistry-20261012/verification.json)。原始驗收條件及 RED／GREEN、有限差分、存檔接續與關閉對照由 ticket 65 管理。生物定量正確性與 OCR 品質改善未由本票驗證。

混合核心完整化學路徑的獨立參考使用測試私有前向微分，依方程式傳播每項參數的導數，與正式反向的方向和實作分開。硬事件值採 0／1、導數採宣告的 fast_sigmoid，硬重設停止事件導數，適應保留事件導數。不增加公開 smooth API，不用硬事件普通有限差分判錯。輸入／讀出跨 Insyra float32 邊界須另核對捨入誤差，範圍與驗收由[ticket 88](docs/tickets/88-mixed-chemical-gradient-reference.md)管理。

純 LIF 公開入口及同類受體競爭另由[ticket 121](docs/tickets/121-lif-max-reference.md)驗證。私人參考依種類及細胞混合原始佔用率，再套有正負號的係數與夾限。最大值同值保留第一個正值記錄，零值加總保留局部導數，夾限端點導數為零。純 LIF 以各節點獨立閾值比對完整及分段待更新梯度，既有七個參考案例須在各建置內保持逐位相同。慢速活動穩定與化學並用的梯度、更新及持續狀態，另由 ticket124 與 ticket125 沿用同一私有參考核對。 持續 timeline 與延後回饋的來源鐘使用神經步數，與只在未凍結列前進的化學 Steps 分開。凍結旗標不進保存格式，新程序由呼叫端重套排程。獨立濃度、來源視窗及絕對時點驗收由[ticket126](docs/tickets/126-persistent-chemical-clock.md)管理。

有界閾值的 sigmoid 尾端以私有 `internal/thresholdtail` 共用計算。正參數從 7 起使用小補數，負參數從 -700 起在指數進入次正規數前先縮放跨度。必要時使用半指數乘積，跨度溢位時先合併已縮放的有限端點，再完成最後一次乘法，保留可表示的最後非零位元。一般算式、有限值拒絕與保存格式保持。LIF 一般／分段／耦合、Mixed 與持續模擬使用相同規則，人工閾值範例報告沿用這個轉換。驗收見[ticket 116](docs/tickets/116-threshold-tail.md)。

LIF 與混合核心的 `fast_sigmoid` 保留正常的 `1/(den*den)` 算式。有限 `den` 的平方先溢位而局部斜率仍可表示時，共用私有 helper 以原分母精確平方、取倒數，再一次捨入為 float64。`den >= 2^538` 時局部斜率必定低於最小次正規數的一半，直接為零。這只恢復宣告的局部替代梯度，不改後續反向的浮點儲存政策。數值、一般／重算／化學耦合與舊保存對照見[ticket 115](docs/tickets/115-surrogate-extreme-audit.md)。

## 機制與品質缺陷稽核（2026-10-13）

持續化學的釋放總量也須有限。每列可合法而整段總量溢位時，整次 Advance／AdvanceGated 回錯，保留神經、化學、來源、可塑性及上一份報告。鞏固先在同一把鎖內驗證 slow 與 plastic 候選、三層範數及共同有效權重，再提交。第一掃描按啟用位置查實際核心邊，固定符號的原始 rho 先轉成 exp(rho)，再沿用 `(base + slow) + plastic` 及 w_min 下限。下限可擋住的負向幅度溢位保持合法，不可表示的有效權重回錯且不消耗預算。候選、範數與取消的既有失敗優先序保持。範數使用 Hypot 避免平方中間值溢位，不可表示的實際範數仍回錯。兩次掃描不配置新的邊陣列，Model.Edge 僅回傳實際邊索引與下限。驗收由[ticket 90](docs/tickets/90-slow-memory-composition.md)管理。

ResetOptimizer 保留記憶、預算及當下回合已被禁止重複鞏固的事實，把 LastEpisode 重映射到新 Episodes=0 的時鐘：原 LastEpisode>Episodes 時為 1，其他為 0。舊版可能保存較大的標記，仍允許載入而不改寫，明確重設後下一個成功 TrainEpisode 可再次鞏固。TrainEpisode 與 Consolidate 在各自的回合計數或標記將溢位時，先拒絕而不更新模型或消耗預算。沒有增加快照欄位或版本。

向量核心的非零固定符號在共用 NewNetwork 入口拒絕。單通道 SourceSpec 的 timeline／replay 不得對其他管道非零釋放，Standalone 多管道來源保持原契約。OCR 的所有圖片入口與範例驗證有限 [0,1] 像素；ASR 的 STFT／LogMel 成功只回有限特徵，Floor 必須有限且正。這些修正不變更合法輸入的活化、頻譜或訓練算式，既有合法模型與保存格式保留。範數報告的捨入可能改變，記憶寫入算式不變。

設計仍明示單一神經元整合受體效果，Compartment 是來源部位記錄而不是多區室膜模型。未知受體維持 skipped／assumed 的區別，數值中性不代表實測不反應。ResetNeural 重設神經及來源時鐘，但保留化學濃度、化學更新步數與來源視窗。這些是既有明確假設，不以生物完整性或品質成效宣稱。

本輪 build、完整一般／race 各 58 套件、vet、相依性及最後 11 項治理通過，770 份來源指紋相符。七類公開回歸、合法 OCR／音訊逐位比較與證據限制見 [機制稽核](evidence/MOD-02/mechanism-audit-20261013/README.md)及[最終工程證據](evidence/MOD-02/mechanism-audit-20261013/verification.json)。

plasticity.Model 擁有 Rule 的 GateReceptor／DecayEReceptor 指標值，New 與 Config 各自提供獨立副本。EnablePlasticity、Snapshot 及 RestoreIndividual 沿用此所有權。RestoreIndividual 在所有部件驗證完成後，以 checkReceptorRule 檢查可塑性宣告與化學受體綁定。沒有受體參照的舊快照可缺省 Chemical，有參照但缺少化學層或索引越界則回錯且回傳 nil。合法值、正常算式與保存格式保持，驗收由[ticket 91](docs/tickets/91-receptor-rule-ownership-and-restore.md)管理。

## 公開拒絕與控制器快取契約

Consolidate 的閘門關閉、同回合去重及預算用完報告帶呼叫前的 Used／LastEpisode，成功報告帶提交後計數。報告不改寫記憶。OCR FixtureConfig 及 glyph Render／Generate 入口都要求 Background／Noise 為有限 [0,0.5]，合法空批次仍回空集合，非法空批次設定回錯。

MemoryController.Offset 與 Controller.Release 一樣先計算私有候選。摘要、資源輸入、隱藏層及讀出全部成功後才提交活動視窗和梯度快取。數值溢位與非法輸入保留上一筆成功狀態。tanh 飽和不能掩蓋非有限的中間計算。回錯的 nil 控制器入口不得 panic。

Controller／MemoryController 的參數梯度使用最近成功 forward 的固定參數。成功 Update 保留活動視窗並清除舊快取，下一次 Gradient／BackwardActivity 必須先重新 Release／Offset。失敗 Update 保留參數、視窗與快取。現有正常訓練 caller 原本就按此順序執行。直接改公開 Parameters slice 的 caller 須自行保持 forward 與反向參數一致。本契約不增加保存欄位、版本或參數指紋。驗收由 tickets 74–77 與[本階段計畫](evidence/MOD-06/refusal-input-audit-20261013/plan.md)管理。

## 化學清除、來源寬度與宣告更換

敏感度效果沿用零值預設後，範圍必須有限且滿足 `0 < GammaMin <= 1 <= GammaMax`，`[1,1]` 合法。共用 `checkedEffect` 在前向、反向、化學建構、個體宣告更換及恢復之前驗證，拒絕時不提交狀態。合法設定的計算與保存欄位保持。使用者已核准拒絕不包含 1 的舊設定與存檔；若需要繼續使用，研究者須明確調整原始宣告再建立模型，框架不會自動改寫存檔。公開拒絕與實際舊存檔的驗收由[ticket 89](docs/tickets/89-chemical-composition-neutrality.md)管理。

Kinetics.Step 拒絕有限基礎清除速率與有限 boost 相加後的溢位，Backward 經同一正向驗證入口一致拒絕。有限速率的指數飽和保留既有算法，零 boost 分支的運算順序保持。SourceSpec.Build、NeuralActivity.Validate、InternalResource.Release 及 Controller 要求 Channel+1 能以 int 表示，且 float64 釋放向量的位元組大小可表示。ExternalTimeline 採相同的向量限制。

EnableChemistry 在提交候選宣告前檢查既有 ExpressionGain，與局部學習規則的受體引用檢查一致。不相容更換回錯，完整舊狀態及報告保持，必須由呼叫者明確清除讀出調節才能移除其受體。合法更換依原契約歸零濃度，保留資源及回饋。公開簽名、保存格式、正常來源與動態算式保持，驗收由 tickets 78–80 與[階段計畫](evidence/MOD-02/chemical-boundaries-20261013/plan.md)管理。

控制器活動視窗、濃度區域及有區域運輸時的完整網格，須先確認 float64 資料與外層 slice header 的總位元組大小可表示，再配置或觀察活動。ApplyEffects 也先檢查節點向量。這是宣告算術驗證，不保證可表示的大型配置有足夠實體記憶體。正常算式、公開簽名及保存格式保持，拒絕與相容性驗收見[ticket 87](docs/tickets/87-chemical-capacity-validation.md)。

## 線上回饋與共享訓練狀態一致性

線上 Act 在同一次個體鎖定中先取得參數版本，再推進神經狀態，答案與 ActionRecord.ModelVersion 使用同一份參數。線上損失權重的零神經狀態 episode 預測、target 加權及 TrainEpisode 也共用同一次個體鎖定，外部操作可等候或取消。私人已持鎖 helper 沿用 Individual → Trainer 的鎖順序，公開 Advance／TrainEpisode 保留原有入口。

開啟化學時，預測和訓練共用 chemicalEpisodeNetwork，複製目前濃度、來源視窗、資源、可用回饋、expression、來源時鐘與原始活動，並保留 frozen 設定。不交換共用 Network 指標，也不推進持續狀態。一般路徑保持 Trainer.Predict。score=0 表示新回饋的損失殘差為零，既有 Adam 動量、weight decay、計數及部分累積仍按原算式處理。並行與錯誤控制由[ticket 86](docs/tickets/86-online-individual-boundary.md)管理。

RestoreTrainer 在既有形狀、有限值與累積窗驗證後，要求同一共享群組的 First、Second、Steps 及 partial Sum 相同。singleton 與 -1 保持獨立，錯誤不重寫呼叫者快照。正常框架寫出的保存格式不變，RestoreIndividual 沿用同一拒絕。

Online UpdateReport.GradientSteps 只計 StepResult.Applied 的套用次數，成功但尚在累積的訓練仍消耗已處理回饋。UpdateEvery 只保存有界的間隔位置，梯度失敗不提交位置，重試仍維持該次到期更新。混合梯度／局部可塑性的部分成功行為未在本票變成整體回滾。完整流程、控制及相容性由 tickets 81–84 與[階段計畫](evidence/MOD-02/chemical-boundaries-20261013/online-sharing-plan.md)管理。

混合更新按 action／source 保存梯度階段已完成的事實，涵蓋沒有 target、合法間隔 skip、部分累積及已套用更新。塑性階段失敗後只重試塑性，成功前半保留；梯度失敗不標為完成。整筆回饋成功後移除私人階段記錄，與既有 pending 佇列同生命週期，不新增公開 API 或快照格式。now 倒退時也須保留各筆尚未完成的階段，驗收由[ticket 85](docs/tickets/85-online-mixed-phase-retry.md)管理。

原生可塑性幅度報告保留可靠正常範圍的索引順序位元。中間平方溢位或有實質下溢誤差時才使用縮放與補償加總；接近 float64 上界時，以標準函式庫高精度平方和判定 nearest-even 捨入上限，避免再次雙重捨入。真正不能編碼的幅度仍由 Run 在提交神經與快速狀態前拒絕。合法塑性上限、學習算式、報告結構及模型格式保持。驗收由 [ticket 93](docs/tickets/93-native-plastic-norm-representability.md) 管理。

原生探針的平均輸出在既有加總為有限值時保留索引順序與除法位元。中間加總溢位時才先縮放，再以補償加總計算平均，並維持平均值落在所選有限輸出的上下界。真正不能表示的 sum_output 沿用拒絕。集合量測的 OutputSum 與 MetricResult.Numerator 保留原始總和，Run 在提交神經、快速狀態及最近成功量測前檢查其有限性。模型格式、核心動態及學習算式保持，驗收由 [ticket 94](docs/tickets/94-native-mean-representability.md) 管理。

OCR 人工樣本共用 glyphs.ValidateGenerateDimensions，在 Generate、Render 的衍生影像配置與 FixtureConfig.Validate 的訓練入口確認樣本容器及每列影像的位元組大小可由 int 表示。樣本元素大小依平台型別，影像沿用寬度 9L−1、8 列 float64 像素。零樣本的未使用寬字長範圍保持合法，基本負值／倒置範圍及有限選項規則保持。這是算術可表示性，不保證 Go runtime 的 maxAlloc 或可用記憶體。正常亂數、字形、模型與最佳化器格式保持，驗收由 [ticket 95](docs/tickets/95-ocr-generate-representability.md) 管理。

文字生成的 NewModel 在所有可變大小配置前，以私有長度檢查確認 encoder 與 readout 的 float64 元素及位元組大小可由 int 表示。Save／Load 沿用同一建構入口，不改公開 Vocabulary、正常初始化、PCG 串流、訓練算式或保存格式。算術可表示不保證 runtime 或實體記憶體容量，驗收由 [ticket 109](docs/tickets/109-textgen-vocabulary-dimensions.md) 管理。

## 局部可塑性的資料列與時間定義

`learning.Individual.AdvanceGated` 與 `simulate.Run` 先以目前有效權重推進核心，再把該輸入列的核心輸出交給局部更新，更新後的快速權重從下一列生效。這裡的 `pre`／`post` 指連線兩端，兩者取核心推進後同一列的結果。LIF 使用來源突觸跡與目標當列放電事件，連續核心兩端都使用啟動後輸出。混合個體按每個節點的核心種類選擇目標訊號，來源仍使用該列輸出。

`hebbian_rate` 的 `elig_next = decay_e * elig + pre_j * post_i` 使用上述同列訊號。`stdp_pair` 使用當列兩端的放電事件，先衰退事件跡，eligibility 讀取尚未加上當列事件的跡，再加入各自的當列事件。當列首次同時放電不會互相配對。兩種規則都先得到新的 eligibility，再套閘門更新 plastic。

主規格 10.3 只提供不帶活動時間索引的人工參考式。上述取樣時點是既有執行入口及手算表的工程假設，沒有證明其生物正確性或任務效益。核心連線在步驟開始讀取前一狀態，與核心之後的局部更新取樣是不同操作。文件核對由 [ticket 111](docs/tickets/111-plasticity-time-definition.md) 管理，正常模型、公開介面及保存格式保持。
