# 正常、合法小型化學機制契約稽核

日期：2026-10-14

## 範圍與判定

本稽核只核對正常、合法的小型化學路徑：來源釋放、濃度清除與區域運輸、受體位置／Hill 親和性／狀態、gain／offset／threshold、神經核心訊號、loss／gradient，以及 Controller 與 MemoryController 的梯度邊界。fresh neural first-step refusal 已依既有契約排除；Inputs／Nodes 宣告後的 mutable reset 仍是未定義契約，也不列為缺陷。

本輪未執行全套 Go／race，也未新增或修改正式測試。下表的「現有直接驗證」是逐一檢視既有測試中的手算、輸出差異與中央差分斷言，不把它們擴大解讀成生物定量或真實 OCR 品質證據。

`runs/.tmp/chemical-entry-audit-20261014/source-final.json` 共 791 個來源項目，寫報告前比對結果為 `missing=0 mismatch=0`。檔案 SHA-256：`601b622639eb179f7f8d259791099050c658863dd8570009628878212331817f`。

## Confirmed findings

**無。** 目前沒有找到能以既有 bounded fixture 的正常輸入直接重現、且可明確指向設計矛盾的輸出或梯度錯誤。ticket 87 的容量拒絕只證明非法宣告會被拒絕，不能證明合法機制提高辨識率，也不能取代以下動態組合驗證。

## Needs investigation

### N1：未知／不反應受體是否應列入平均分母

- **正常觸發：** 同一受體宣告兩個細胞，其中一筆為合法 `StatusHypothesized` 且濃度大於零，另一筆為 `StatusUnknown` 且未允許假設係數，或為 `StatusUnresponsive`。設定 expression gain、受體 gate 或 `DecayE`，讓該平均值確實影響讀出或 plasticity。
- **目前實作：** `Occupancies` 對未知跳過計算、對不反應保留零值但兩者都輸出記錄（`/Users/timlai/Developer/coimnet/modulation/receptors.go:219-245`）；`chemicalRuntime` 以同一受體的所有記錄計數並平均（`/Users/timlai/Developer/coimnet/learning/chemistry.go:251-263`）；expression 也以所有記錄平均（`/Users/timlai/Developer/coimnet/learning/memory.go:102-118`、`/Users/timlai/Developer/coimnet/learning/chemical_sequence.go:596-617`），而反向對 skipped／unresponsive 記錄不傳導數值梯度（`/Users/timlai/Developer/coimnet/modulation/receptor_gradient.go:33-37`）。
- **直接影響：** 同一個已計算 occupancy 會被分母 2 稀釋，而該零記錄沒有對 concentration 的反向導數。這會改變 expression／gate／DecayE 的作用量與 concentration gradient。
- **為何不是 Confirmed：** 主規格只要求區分「不反應／未知／假設反應」，沒有指定受體平均是以所有宣告記錄，還是只以可計算記錄為分母（`/Users/timlai/Developer/coimnet/docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:527-537`）。現有測試只驗證狀態記錄與 summary（`/Users/timlai/Developer/coimnet/modulation/receptor_location_test.go:128-151`），未驗證這個聚合契約。
- **精確驗證方法：** 以既有兩區域 chemistry fixture（`/Users/timlai/Developer/coimnet/learning/chemical_stateful_test.go:41-70`）改成同一受體的兩筆細胞記錄，固定正釋放、正 effect／expression，分別計算「所有記錄」與「只計算記錄」兩種預期 gain／gate，並對 `LossGradientFromState` 做參數與輸入中央差分。先決定分母契約，再判斷現行程式是否錯。

### N2：混合 continuous／LIF 的完整化學反向路徑沒有直接證據

- **目前邊界：** `ChemicalEpisode` 正式接受 continuous、LIF、mixed 三種 CPU scalar core（`/Users/timlai/Developer/coimnet/learning/chemical_episode.go:39-45`），mixed forward 確實接上 chemistry sequence（`/Users/timlai/Developer/coimnet/learning/chemical_episode.go:231-238`），mixed core 也處理 gain、offset、LIF threshold 與 reverse（`/Users/timlai/Developer/coimnet/dynamics/mixed_coupled.go:20-28,128-143,193-203,244-270`）。
- **已有證據的界線：** LIF 有化學 threshold 的直接手算（`/Users/timlai/Developer/coimnet/learning/chemical_episode_test.go:976-1010`）；continuous controller／neural source 有完整 loss 中央差分（`/Users/timlai/Developer/coimnet/learning/chemical_episode_test.go:683-895`）。但現有測試沒有同時包含 mixed core、化學 transport／explicit receptor location、threshold／gain、controller 或 neural source feedback 的完整 loss gradient。mixed recompute 被明確拒絕且保持狀態（`/Users/timlai/Developer/coimnet/learning/chemical_recompute_test.go:331-354`），這是已宣告的能力邊界，不是本項缺陷。
- **精確驗證方法：** 從既有 mixed node fixture 建立一個 continuous node 與一個 LIF node，加入同一合法 chemistry episode、至少一個 explicit-location receptor、continuous gain／offset 與 LIF threshold effect，再各用 controller 與 neural activity source 跑 `LossGradient`。對 core weight、bias、log tau、LIF threshold、readout、encoder、輸入與 source feedback 做中央差分，並檢查 mixed raw activity 與 expression readout 的時間順序。

### N3：`RewardMapper.ClearanceBoost` 是否應自動接入一般 ChemicalEpisode

- **已確認的程式事實：** `RewardMapper.Map` 對合法負向 relu delta 產生非負 `ClearanceBoost`（`/Users/timlai/Developer/coimnet/modulation/reward.go:75-101`）；`Kinetics.Step` 接受該 boost，將 rate 改成 `1/tau + boost` 並降低清除後穩態（`/Users/timlai/Developer/coimnet/modulation/chemistry.go:184-247`）；但一般 `chemicalRuntime` 固定以 `nil` boost 呼叫 kinetics（`/Users/timlai/Developer/coimnet/learning/chemistry.go:215-223`）。
- **判定：** 這表示「negative feedback → 額外清除」目前沒有一般 `ChemicalEpisode` 的端到端路徑，但主規格目前明確要求的是原始獎懲、預期值、轉換值與實際作用值分開保存，且禁止把負值直接當濃度（`/Users/timlai/Developer/coimnet/docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:552-558`）。`RewardMapper` 到 chemistry 的自動接線是否屬本輪正常契約，現有 `ChemicalConfig` 沒有宣告欄位，不能只憑 API 存在就判成 bug。
- **精確驗證方法：** 先在需求中決定 RewardMapper 是呼叫端顯式輸入，還是一般 episode 的必經路徑。若是後者，建立負向 raw reward 的 bounded episode，對比 `ClearanceBoost=0` 與正 boost 的 concentration、occupancy、output 及 loss gradient；若是前者，保留目前一般 runtime 的 `nil` 行為，並只驗證直接 `Kinetics.Step` 路徑。

## 設計 → 實作 → 現有直接驗證 → 未覆蓋範圍

| 核心機制 | 設計 | 實作 | 現有直接驗證 | 尚未覆蓋範圍 |
|---|---|---|---|---|
| 來源與釋放 | 來源可為外部、神經活動、控制器、內在資源或重播，不得讀取答案（`/Users/timlai/Developer/coimnet/docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:498-512`）。 | `advanceOne` 建立 `SourceContext`，逐來源產生非負 channel release，現階段廣播到每個區域（`/Users/timlai/Developer/coimnet/learning/chemistry.go:186-214`）；Controller 從活動摘要／feedback／resource 產生 release（`/Users/timlai/Developer/coimnet/modulation/controller.go:123-271`）。 | Controller source 與 neural activity source 的 full-path output／loss 中央差分：`/Users/timlai/Developer/coimnet/learning/chemical_episode_test.go:683-895`；Controller activity reverse 手算與中央差分：`/Users/timlai/Developer/coimnet/modulation/controller_state_test.go:515-600`。 | 未以 mixed core 驗證同一來源順序；區域來源仍是全區域 broadcast 工程契約，沒有 per-region source 的直接驗收。 |
| 濃度、清除與區域運輸 | `lambda*c + tau*(1-lambda)*q`，濃度／釋放非負；區域 transport 要保留非負、流入、流出與穩定性（`/Users/timlai/Developer/coimnet/docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:514-525`）。 | `Kinetics.Step` 先做 local decay/release，再做 outflow／inflow transport（`/Users/timlai/Developer/coimnet/modulation/chemistry.go:184-276`）；一般 runtime 在 core 前呼叫一次（`/Users/timlai/Developer/coimnet/learning/chemistry.go:215-223`）。 | steady state、clearance、zero、nonnegative、transport、clearance boost：`/Users/timlai/Developer/coimnet/modulation/chemistry_test.go:37-212`；kinetics transpose／中央差分：`/Users/timlai/Developer/coimnet/modulation/kinetics_gradient_test.go:9-129`；transport fixture：`/Users/timlai/Developer/coimnet/learning/chemical_stateful_test.go:41-70`。 | RewardMapper 的 optional boost 是否要接入一般 episode（N3）；mixed + transport + full reverse 尚無直接證據（N2）。 |
| 受體位置、親和性與狀態 | occupancy 使用明示 `Kd`／`n` 的 Hill 曲線；位置與「不反應／未知／假設」分開（`/Users/timlai/Developer/coimnet/docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:527-537`）。 | `Occupancies` 依 explicit `Location.Region` 或 node-region mapping 讀濃度，unknown 可 skip／assume，unresponsive 保留零 occupancy；反向只對可計算記錄傳導（`/Users/timlai/Developer/coimnet/modulation/receptors.go:182-248`、`/Users/timlai/Developer/coimnet/modulation/receptor_gradient.go:8-37`）。 | explicit region occupancy／effect、multi-region mixing、nil-location fallback、status preservation：`/Users/timlai/Developer/coimnet/modulation/receptor_location_test.go:40-151`；Hill gradient 手算／中央差分：`/Users/timlai/Developer/coimnet/modulation/receptor_gradient_test.go:114-257`。 | 同受體混合 status 的平均分母、expression／gate／DecayE 與其 gradient 尚無直接契約測試（N1）。 |
| gain、offset、threshold 與 plasticity effect | 受體效果應在核心步驟前產生有界 gain／offset／threshold；閘門與時間窗只影響局部學習，neutral 時應等同未啟用（`/Users/timlai/Developer/coimnet/docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:539-548`）。 | occupancy → `ApplyEffects` 形成 modulation arrays（`/Users/timlai/Developer/coimnet/learning/chemistry.go:265-275`）；每列順序是 effects → modulated core → local plastic update，gate／DecayE 讀同一列 receptor average（`/Users/timlai/Developer/coimnet/learning/individual.go:639-651,725-815`）。 | effect neutral／mix／clamp：`/Users/timlai/Developer/coimnet/modulation/effects_test.go:45-178`；LIF threshold 化學路徑：`/Users/timlai/Developer/coimnet/learning/chemical_episode_test.go:976-1010`；受體 gate／expression／consolidation 的既有證據在 `docs/tickets/22-modulated-learning-memory-controller-and-interventions.md:50-55`。 | 同一組 mixed core 中 continuous modulation 與 LIF threshold、plasticity gate 的合成反向尚無 bounded FD（N2）。 |
| 時間順序與 episode／persistent lifecycle | 化學狀態應依時間演化，化學與神經接線分離；主規格要求多步與替代梯度驗證（`/Users/timlai/Developer/coimnet/docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:514-525,907-915`）。 | 正常逐步順序固定為 source release → concentration → occupancy → effect arrays → core step → plastic update（`/Users/timlai/Developer/coimnet/learning/individual.go:639-651`）；sequence 保存每列 before／after／release／occupancy，並在 core 後套 expression（`/Users/timlai/Developer/coimnet/learning/chemical_sequence.go:131-207,596-617`）。 | persistent two-region controller／location／transport fixture 的 saved-start loss FD：`/Users/timlai/Developer/coimnet/learning/chemical_stateful_test.go:265-381`；LIF recompute full-history equality：`/Users/timlai/Developer/coimnet/learning/chemical_recompute_test.go:507-695`。 | mixed recompute 明確不支援（既有 refusal 測試），但正常 mixed full-history／reverse composition 仍缺（N2）。 |
| loss、反向與兩種 controller 邊界 | 一般 core loss 需回到 readout、core、input mapping；controller 可固定參數，另以自身 objective 訓練；capacity-matched MemoryController 不得偽裝成 chemistry controller（`/Users/timlai/Developer/coimnet/docs/model-and-mechanisms.md:60-61`、`/Users/timlai/Developer/coimnet/docs/tickets/22-modulated-learning-memory-controller-and-interventions.md:97-124`）。 | reverse 依序反傳 expression seed → effects → receptor → kinetics → source release；Controller source 只回傳 release 對 activity window 的梯度（`/Users/timlai/Developer/coimnet/learning/chemical_sequence.go:427-594`、`/Users/timlai/Developer/coimnet/modulation/controller_state.go:64-69`）。Controller 自身 objective 明確不跨 chemistry／core（`/Users/timlai/Developer/coimnet/modulation/controller_objective.go:28-31,79-138`）；MemoryController 只對自己的 offset target 求梯度（`/Users/timlai/Developer/coimnet/modulation/memory_controller.go:156-206`）。 | controller source、neural source、core／encoder／input／readout 的 full-path FD：`/Users/timlai/Developer/coimnet/learning/chemical_episode_test.go:683-895`；Controller objective FD：`/Users/timlai/Developer/coimnet/modulation/controller_objective_test.go:123-172`；MemoryController FD：`/Users/timlai/Developer/coimnet/modulation/memory_controller_test.go:134-217`。 | 未有 mixed full-path FD；未有測試證明「Controller 參數不穿過一般 core loss」是刻意固定邊界而非漏接，但文件、實作註解與獨立 objective 都一致，暫不列缺陷。 |

## Verified

- `source-final.json` 寫入前完整比對：791／791 項存在，0 mismatch；manifest SHA-256 如上。
- independent 與 persistent lifecycle 有明確邊界：standalone `ChemicalEpisode` 不推進 persistent `Individual`，只提交既有 parameters／optimizer（`/Users/timlai/Developer/coimnet/learning/chemical_episode.go:18-22`）；`TrainEpisode` 則從複製的 chemical/source/neural boundary 建立獨立零狀態 episode，成功只提交訓練更新並保留 persistent neural／chemical state（`/Users/timlai/Developer/coimnet/learning/individual_chemical_training.go:10-70`、`/Users/timlai/Developer/coimnet/learning/individual.go:827-859`）。
- 直接時間順序可由 `/Users/timlai/Developer/coimnet/learning/individual.go:639-651,692-725` 核對：化學先於 core，plasticity 在 core 後；expression reverse 在 `/Users/timlai/Developer/coimnet/learning/chemical_sequence.go:364-424` 先把讀出梯度乘回 raw core 並建立 occupancy seed。
- source controller 的梯度沒有在正常 loss path 被切斷：`/Users/timlai/Developer/coimnet/learning/chemical_sequence.go:521-550` 將 release gradient 送進 `Controller.BackwardActivity`，並由既有 full-path FD（`/Users/timlai/Developer/coimnet/learning/chemical_episode_test.go:683-827`）驗證 core／input 等參數；Controller 參數本身固定是文件明示的設計邊界。
- MemoryController 沒有被接到 chemistry source，符合容量匹配對照的 readout offset 設計；其自己的 gradient 與 update 由 `/Users/timlai/Developer/coimnet/modulation/memory_controller.go:156-238` 實作，既有 FD 在 `/Users/timlai/Developer/coimnet/modulation/memory_controller_test.go:134-217`。
- 既有測試與文件都只能支持工程機制與小型 fixture 的數值契約，不能推出真實果蠅生理正確性、全腦泛化或 OCR 辨識率提升。

## 未驗證範圍

- 本輪沒有執行測試命令，因此沒有新增的 pass／fail runtime receipt；root 的 full Go／race 執行結果應另行核對。
- 沒有 MaleCNS／FlyWire 生物定量校準，也沒有真實資料的受體表現或親和性驗證。
- 沒有 mixed core 的完整化學 loss／gradient bounded fixture；N2 是目前最直接的動態覆蓋缺口。
- N1 的 status 聚合分母需要規格裁決；N3 的 RewardMapper → normal ChemicalEpisode 接線需要契約裁決。兩者在裁決前都不應寫成 confirmed bug。
