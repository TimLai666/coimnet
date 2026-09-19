# 模型定位與可選機制

CoImNet 是建立、訓練及保存接線約束神經網路的框架。接線圖提供神經元之間的連接限制，框架提供狀態如何更新、哪些參數可以學習，以及如何評估結果。使用者的任務與訓練模型另行管理，repo 中的任務程式與小型模型只作為明示範例。

## 與既有神經網路的關係

目前的 `dynamics.Continuous` 最接近**稀疏連續時間循環神經網路**。循環表示目前輸出會受到先前神經狀態影響，稀疏表示只計算設定中的連線。程式以離散時間步近似連續動態，延遲連線可以讀取更早的狀態。這個分類依 [Forward 的實作](../dynamics/continuous.go) 與[主規格第 8 章](handoff/CoImNet_Implementation_Plan.zh-TW.md#chapter-08)判斷。

| 類型 | CoImNet 與它的關係 |
| --- | --- |
| 循環神經網路（RNN） | 目前核心所屬的主要類型。連線權重、偏置與狀態消退時間可訓練，沿時間回推誤差。 |
| 接線約束、任務最佳化模型 | 最接近的研究方向。Lappalainen 等人的 DMN 用果蠅視覺接線限制網路，透過任務訓練求得未知參數。CoImNet 並未重現該論文所有機制或成果。 |
| 圖神經網路（GNN） | 沿有向邊收集鄰居活動的運算方式相近，可從圖上傳訊理解。但目前核心沒有一般 GNN 常見的逐層特徵轉換，不能因此視為某套既有 GNN 架構的完整實作。 |
| 脈衝神經網路（SNN） | `dynamics.NewLIF` 已實作這一類：膜電位累積輸入，超過閾值就產生離散事件並重設。訓練用的是宣告的替代梯度，不是真實放電函數的導數。慢速穩定、個體持續狀態與按類型混合（每個節點各自跟隨連續或 LIF 規則）都已有 CPU 實作。 |
| 固定循環核心、只訓練讀出的 reservoir computing | 可作比較條件。CoImNet 已實作核心參數的學習，預設不是只能訓練讀出端。 |

DMN 的作者將其模型描述為連續時間神經微分方程及卷積循環網路，研究對象是果蠅視覺系統。CoImNet 借鏡接線限制與可訓練動態的做法，目前並沒有因此取得該視覺架構或一般語言能力。[Lappalainen 等，Nature 2024](https://www.nature.com/articles/s41586-024-07939-3)

CoImNet 的新意需要透過方法、實作及對照實驗逐步建立。專案名稱與生物接線來源本身不足以證明新的學術分類或較高的任務能力。

## 官方資料提供到哪裡

Google 與 HHMI Janelia 等合作團隊公布的 MaleCNS 是果蠅腦與腹神經索的接線資源。它讓研究者取得結構與註記，再據此建立運作模型。[Google Research 專案介紹](https://research.google/blog/a-connectomics-milestone-mapping-the-complete-male-fruit-fly-brain/)

目前 CoImNet 已下載並完整讀取七份原件：細胞註記、每個 body 的傳導物質預測、segment 之間的連線強度，以及推導邊參數用的每個 body 統計、逐突觸前位置傳導物質機率、逐突觸配對與 `Neuprint_Meta.csv` 的 ROI 階層。官方另有細部突觸座標與骨架等資料，可在所選機制需要時取得。[MaleCNS 官方下載說明](https://male-cns.janelia.org/download/)

這些資料沒有包含每個細胞所有受體、離子通道、電生理參數與當下化學狀態的完整量測。資料能否支持某項機制，必須逐欄與研究證據核對。目前已讀取每個 body 的傳導物質表，另外也已取得逐突觸傳導物質機率、突觸配對、body 統計與 ROI 階層四份檔案，並依明示規則把它們轉成每條邊的作用符號與強度（見下表「由發布資料推導的邊參數」），但符號是規則推導的結果，不是量測到的作用；化學調節仍未接上。註記中名為 `receptorType` 的欄位不能在沒有定義依據時當成神經傳導物質受體。實際取得欄位與限制見[來源稽核](malecns-source-audit.md)。

Shiu 等人的全腦 LIF 模型，已能在味覺與理毛等研究情境產生可驗證預測，但作者明確省略了神經形態、不同受體動態、電突觸、非脈衝細胞、內在狀態與長距離神經胜肽等因素。這提供簡化模型仍可能有研究價值的例子，也說明「全腦」描述的是涵蓋範圍，並不等於全部生物機制。[Shiu 等，Nature 2024](https://www.nature.com/articles/s41586-024-07763-9)

## 更多機制是否會更強

可能改善某些能力，但目前沒有 CoImNet 的實驗證據能保證改善。應分別檢查任務成績、學習所需資料、長期記憶、對干擾的穩定性，以及是否更符合生物量測。

工程上可檢驗的假設包括：適應機制有助於表達不同時間尺度，局部可塑性有助於利用近期經驗，調節機制有助於依狀態改變反應或學習。這些都是待驗證假設。額外狀態也會增加記憶體與計算量，錯誤的參數或作用規則可能降低成績。生物上的相似程度和任務表現必須分開報告。

判斷貢獻時，以相同接線、資料切分、輸入與讀出容量比較關閉及開啟的版本，保留多組 seed 與失敗結果。除了相同更新次數，也比較相同時間／資源預算；多出來的參數須搭配容量相近的簡單模型，避免把容量增加誤認成某個生物機制的效果。

## 可選模式與目前狀態

沿用原規格第 8、10、11 章，將動態規則、可塑性與調節分開設定。以下是實作狀態與規劃原則，不是已可輸入的 CLI 設定清單。

| 機制 | 目前狀態 | 啟用條件與驗證 |
| --- | --- | --- |
| 連續動態、延遲、權重／偏置／時間常數學習 | 已有 CPU 實作 | 使用合法拓撲與參數，通過手算、時間步收斂與梯度驗證。 |
| LIF 放電核心、可訓練基礎閾值、短期適應 | 已有 CPU 實作 | 產品使用的硬放電模式每步最多產生一次事件，達到閾值就把膜電位重設為 `v_reset`，接下來的不應期保持重設值並忽略當步輸入，對外輸出是會衰減的突觸跡。基礎閾值經 `theta_min + (theta_max - theta_min) * sigmoid(theta_raw)` 限制在設定範圍內，可以單獨訓練。短期適應是可以開關的設定，關閉後前向與反向和未啟用逐位相同。反向使用宣告的 `fast_sigmoid` 替代梯度 `psi(u) = 1/(1 + scale*|u|)^2`，重設分支不傳梯度，硬放電事件不做有限差分。每步一次事件是離散時間步的表示上限，不是量測到的生理放電頻率。 |
| 原生模擬 runner（固定注入、具名探針、不經訓練） | 已有 CPU 實作 | `simulate` 套件把 GraphStore 的節點與邊直接接上連續或 LIF 核心，刺激經固定線性注入進入指定神經元，活動經探針的宣告 reduce 讀出，沒有 encoder、readout 或最佳化器。參數必須明示來源，目前有 `engineering_uniform_positive`（`gain × 原始 weight`、全興奮、統一 bias／log_tau／theta_raw、延遲零）與 `derived_release/v1`（見下一列）兩種，報告與文件都寫明兩者都是明示假設而非生物參數；全圖 300 步實測見 ticket 12（uniform）與 ticket 13（derived）。空模型歸因見下面的「空模型對照與判讀協定」一列。 |
| 由發布資料推導的邊參數（正負號與強度） | 已有 CPU 實作 | `params` 依規則檔 `coimnet-derivation-rules/v1` 讀四份官方發布檔，產生每條邊的正負號、信心度、傳導物質與正規化強度，`simulate run --params` 以 `weight_scale × sign × 推導強度` 執行同一張圖。**正負號是規則推導的，不是量測值**：發布資料只提供每個突觸前位置的傳導物質預測機率，規則取配對突觸的平均機率選出勝出的傳導物質，再換成 `+1`／`-1`／unknown；果蠅 glutamate 多為抑制屬工程假設，規則檔的 basis 必須寫明。未知保持未知：沒配到突觸、低於機率門檻、低於配對比例門檻或規則本身標為 unknown 的邊都不補預設值，由 protocol 的 `unknown_sign`（`exclude`／`excitatory`／`inhibitory`）明示處理並分開計數。bias、log_tau、theta_raw 與零延遲仍是統一工程值，不是細胞類型差異。全圖實測（配對比例 1.000、sign 53.3%／35.9%／10.8%）見 [ticket 13](tickets/13-parameter-adapter.md) 與 `evidence/NAT-02/`。 |
| 空模型對照與判讀協定（歸因用，非機制） | 已有 CPU 實作 | `simulate compare` 用同一份刺激跑原圖與三種空模型（`degree_preserving_rewire`、`sign_shuffle`、`weight_shuffle`）的每個 seed，報告每格的指標、與原圖的差、事前寫定的門檻結果，以及原圖在多 seed 分布中的分位數與百分位。空模型只是記憶體裡的衍生物，不改接線圖與參數集檔，靠 seed 加 PCG 重算後比對 hash。**這一列不是生物機制，是判讀規則**：集合名稱由使用者給，框架不預設任何集合等於某種行為；門檻必須在跑之前寫進 protocol 並進 hash，通過只能表述為「在此規則下該集合的指標達到宣告值」，沒有值的指標記未定義而不是 0。全圖實測（LIF 與連續核心各十格、三個 seed）顯示同一份接線在不同核心下，百分位的方向可以相反，所以歸因結論必須連同核心、參數來源與空模型種類一起陳述。三個 seed 的百分位是位置，不是檢定。見 [ticket 14](tickets/14-null-models-and-behavior.md) 與 `evidence/NAT-03/`、`NAT-04/`、`NAT-05/`。 |
| 慢速穩定（homeostasis） | 已有 CPU 實作 | 每顆神經元多一個活動估計 `r` 與一個閾值偏移 `h`：`r(t+1) = r(t) + (dt/tau_rate) * (spike(t+1) - r(t))`、`h(t+1) = clamp(h(t) + eta*dt*(r(t+1) - target_rate), 0, h_max)`，放電判定用 `theta_eff = theta_base + 適應 + h`，比對的是上一步結束時的 `h`。偏移只會抬高閾值或回落到零，是受限調整而不是無上限的增益；`h >= 0` 加上既有的 `v_reset < theta_min`，保證 `v_reset < theta_eff` 恆成立。`r` 與 `h` 是狀態不是可訓練參數，反向與適應一樣視為常數。這是可以單獨開關的設定：關閉時兩個陣列不存在，前向與反向和從未宣告該機制的模型逐位相同，沒有宣告的設定其編碼與既有指紋也完全不變。`tau_rate`、`target_rate`、`eta`、`h_max` 是固定設定，不參與訓練。手算時序、關閉逐位相同與收斂證據見 [ticket 16](tickets/16-lif-individual-and-model-package.md) 與 `evidence/COR-04/`。 |
| LIF 個體持續狀態 | 已有 CPU 實作 | `learning.NewIndividual` 對連續與 LIF 兩種核心走同一條路徑，LIF 個體有自己的 profile。持續保存的是電位、延遲所需的突觸跡歷史、適應值、不應期計數，以及慢速穩定開啟時的活動估計與閾值偏移；`Advance` 的結果與同輸入的 `dynamics.LIF.Forward` 逐位相同，跨程序恢復後續跑與不中斷執行逐項相等。訓練仍是獨立 episode，梯度不跨 `Advance`。見 [ticket 16](tickets/16-lif-individual-and-model-package.md) 與 `evidence/STA-01/`。 |
| 化學濃度與區域傳輸 | 已有 CPU 實作 | `modulation` 依宣告的區域數、通道數、每通道正時間常數與步長，逐步更新每個區域每個通道的非負濃度：`c' = lambda*c + tau*(1-lambda)*q`，`lambda = exp(-dt/tau)`，`q` 由宣告的來源提供。宣告的傳輸矩陣讓濃度在區域間流動，每列非負且總和不超過 1，所以傳輸不會增加總量，清除只靠 `tau`。**傳輸是化學資料，由研究者宣告，不從接線推導**；單位字串只是宣告，框架不做任何換算。`Individual.EnableChemistry` 把這一層接到持續個體上，每步順序固定為「來源釋放 → 濃度 → 佔用率 → 效果陣列 → 調節後的核心一步 →（可塑性若啟用）」。本階段的來源對所有區域是全域的：一個通道的釋放率會進入每個區域。手算與證據見 [ticket 21](tickets/21-chemical-state-receptors-and-effects.md) 與 `evidence/MOD-02/`、`evidence/MOD-04/`。 |
| 受體與佔用率（含未知與不反應的分離） | 已有 CPU 實作 | 每筆受體紀錄指名帶有該受體的細胞、通道、訊號名稱、證據來源、量測種類與工程映射版本，缺任一項不接受。佔用率是 `c^n/(Kd^n + c^n)`，以對數域計算，極端濃度不會出現 NaN 或無限大。**三種狀態分開表示，不合併**：`unresponsive` 的佔用率是宣告的 0 並標為 unresponsive，不是量測到的 0；`unknown` 只有在紀錄帶有係數且該次執行明示允許時才計算，且一律標為 assumed，否則計入 `unknown_skipped`；`hypothesized` 正常計算。**`Kd` 與 `N` 是工程係數不是量測值**，JSON 欄位名就叫 `engineering_kd`、`engineering_n`，任何報告都不得把它們描述成量測。同一細胞多個受體依宣告的 `sum` 或 `max` 混合。見 [ticket 21](tickets/21-chemical-state-receptors-and-effects.md) 與 `evidence/MOD-03/`。 |
| 當下敏感度與有效閾值（暫時效果） | 已有 CPU 實作 | 佔用率經有界映射變成每步的暫時陣列：敏感度是 FiLM 形式 `gamma = clamp(1 + GammaScale*occ, GammaMin, GammaMax)`、`beta = clamp(BetaScale*occ, ±BetaAbsMax)`，核心的輸入電流變成 `gamma*I + beta`，其中 `I` 是外部輸入加上含延遲的突觸貢獻，bias 是神經元參數不在增益之內；有效閾值只對 LIF，`m = clamp(ThetaScale*occ, ±ThetaAbsMax)`，`theta_eff = max(theta_base + 適應 + 慢速穩定 + m, theta_min)`。**暫時效果永遠不寫回基礎參數**：陣列是單次呼叫的引數，跑 100 步後 `Parameters`（含 `theta_raw`）逐位不變。`occ = 0` 時 `gamma = 1`、`beta = 0`、`m = 0`，連續與 LIF 兩個核心都測過「宣告了效果但濃度為 0」與「未宣告」逐位相同。前向 episode 與反向路徑目前不接調節，可微調節路徑是 ticket 22（MOD-07）。見 [ticket 21](tickets/21-chemical-state-receptors-and-effects.md) 與 `evidence/MOD-03/`、`evidence/MOD-04/`。 |
| 學習閘門與時間窗（受體驅動） | 已有 CPU 實作 | `plasticity.Rule` 的 `GateReceptor`／`DecayEReceptor` 讓每步閘門 `gate = GateScale*occ`、時間窗 `decay_e = clamp(DecayEBase + DecayESpan*occ, DecayEMin, DecayEMax)`（`decay_e` 有效範圍在建構時驗證為 `(0, 1)`）。閘門關時快速變化只衰退不新增，前向與化學-only 執行逐位相同；`StepWith` 的 `DecayE` 覆寫當步替代原衰減。受體閘門與呼叫端直接給的閘門序列不得同時宣告；規則指向受體時不可關閉化學層，無化學時受體規則被拒。見 [ticket 22](tickets/22-modulated-learning-memory-controller-and-interventions.md) 與 `evidence/MOD-05/`。 |
| 記憶表現（讀出增益，不是遺忘） | 已有 CPU 實作 | `SetExpressionGain` 對指定讀出節點乘 `clamp(1 + Scale*occ, Min, Max)`，只動讀出路徑，核心、可塑性、化學狀態與參數逐位不變；增益回到 1 後輸出逐位恢復（抑制表達而非遺忘）。報告欄位 `expression_gain_applied`。快照往返保留宣告。見 [ticket 22](tickets/22-modulated-learning-memory-controller-and-interventions.md) 與 `evidence/MOD-06/`。 |
| 穩定化（寫入較慢狀態） | 已有 CPU 實作 | `Consolidate(ctx, trigger)` 只在 `occ ≥ Threshold`、本 episode 未寫入且 `Used < Budget` 時執行 `slow += Rate*plastic` 再 `plastic *= Retain`；同 episode 第二次與預算用盡分別回 `ErrAlreadyConsolidated`、`ErrBudgetExhausted` 且狀態不變。有效權重為 `(base + slow) + plastic`。三層 L2（`base_l2`／`slow_l2`／`plastic_l2`）分開報告，梯度路徑不動 `slow`。快照接續後的去重判斷與連續執行相同。見 [ticket 22](tickets/22-modulated-learning-memory-controller-and-interventions.md) 與 `evidence/MOD-06/`。 |
| 生物干預（細胞與通道覆寫） | 已有 CPU 實作（fixture） | **干預是授權的實驗模式，不是推論 API**：`Advance`／`AdvanceGated`／`simulate run` 沒有任何參數能覆寫電位，`Intervene` 與協定的 `Interventions` 區塊只有在 `Authorized = true` 且 `Reason` 非空時執行，未授權、空白原因、未知 kind、越界或重疊物件一律拒絕且狀態不變（個體端回 `ErrInterventionNotAuthorized`，未宣告干預的協定 hash 不變）。八種 kind 中 `clamp_voltage`、`force_spike`、`silence` 在個體與原生 runner 都支援；`block_channel`、`fix_concentration`、`remove_channel`、`swap_regions`、`shuffle_delays` 只在個體端（runner 沒有化學層）。每個項目記錄 `Start`／`End` 窗與已套用列數，並量測對同一份輸入的無干預孿生差：`ActivityDelta`／`PlasticDelta`／`BaseParameterDelta` 三種差值每次干預都算（基礎參數逐位不變），通道類另有 `ConcentrationDelta`。`fix_concentration` 的固定值是單次呼叫的宣告，不是儲存參數。見 [ticket 22](tickets/22-modulated-learning-memory-controller-and-interventions.md) 與 `evidence/COR-11/`。 |
| 按類型混合 | 已有 CPU 實作 | `dynamics.NewMixed` 讓每個節點依 `NodeRule` 只跟隨連續或 LIF 其中一條規則（不是同一顆神經元同時輸出兩種訊號），分群依據由研究者在設定裡明示。**輸出契約**：每個節點對外只有一個輸出序列 `out_j(t)`，連續節點是 `phi(v_j)`、LIF 節點是衰減突觸跡 `x_j`，兩種目標收到的突觸輸入都是 `I_i(t) = external_i(t) + Σ_j w_ji * out_j(t − delay_ji)`，延遲 0 讀本步開始時的 `out`；因此一個 LIF 節點的事件只透過 `x` 進入下游一次，不會再有第二條 spike 路徑重複計入。全 0 的指派與 `Continuous`、全 1 的指派與 `LIF` 在同一份參數下結果相同（一般建置逐位相同，`-race` 建置的差異見下）。梯度上連續節點走 8.1 的反向、LIF 節點走 11 宣告的替代梯度，跨類型的邊只是一般的 `w*out` 乘積。`learning.Config.Mixed` 與 `Dynamics`、`LIF` 三選一，個體 profile 是 `mixed-f64-…/v1`，`Parameters.ThetaRaw` 只有 LIF 節點那幾項，索引對照寫在 `Config().LIFIndex`。**全腦一律混合未經實驗支持，不是預設**（主規格 2.1 D11）：這是研究者按類型明示選用的可選機制，本 repo 只有 fixture 規模的證據。目前限制：向量節點與共享參數是第二階段，模型包（`checkpoint.ModelPackage`）還不收混合核心，`stdp_pair` 在混合核心上被拒絕（連續節點沒有事件）。見 [ticket 25](tickets/25-mixed-types-vector-nodes-shared-parameters-and-recompute.md) 與 `evidence/COR-05/`。 |
| 適應性評估 | 已有 CPU 實作 | 基礎參數凍結、適應分割允許宣告的局部更新、評分分割只推論、題間依政策重設、三項污染檢查，證據 `evidence/LRN-10/`。 |
| 暫時連線狀態、局部學習、近期參與紀錄 | 待實作 | 分開保存基礎參數及暫時狀態，驗證更新次序、關閉效果與恢復。 |
| 人工調節與有證據支持的生物調節 | 待實作 | 人工設定使用功能名稱。生物設定附來源、單位、接收端與未知欄位，不能只改名冒充荷爾蒙。 |
| 更細的形態、電突觸、離子通道等模型 | 未納入已實作能力，未承諾全部納入 | 先確認研究用途與可用資料，再決定是否值得新增獨立實驗機制。 |

使用者選取規則與可選機制後，框架應先驗證資料、參數、裝置能力與資源，再建立可訓練狀態。執行報告和快照保存同一組機制版本。關閉機制應還原參考行為，而缺少生物資料須保持未知，或由使用者明確選用有記錄的工程假設。

不支援的模式、未知版本、缺少必要欄位與超出容量都應在開始更新前返回錯誤。中途取消或數值錯誤不能留下半次更新，重新載入時也必須核對機制狀態與版本。這些行為依原有核心、可塑性、調節及保存需求驗收，尚未完成的項目不因寫入文件而判定通過。

減法審查：保留容易比較的簡單參考模式，按用途增加已驗證機制，不提供無法定義或驗收的「全部生物機制」開關。
