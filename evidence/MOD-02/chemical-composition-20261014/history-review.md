# 化學歷史與重算契約稽核

- 稽核日期：2026-10-14
- 基準提交：`c43ff4051ed3e80b3988ee37335353579f0c82d5`
- 範圍：普通 `ChemicalEpisode`、`Individual.TrainEpisode`、`Controller.SourceStates`，以及 scalar continuous/LIF chemistry recompute。檢查 persisted history、`SourceStep`、`SummaryWindow`、truncated window、segment restart、`Step=-1` 與 `initialActivityExplicit`。
- 限制：本輪只讀 source、既有測試與 evidence，未執行 Go、未下載、未使用網路，也未修改 Go、測試、文件或資料。

## 1. CONFIRMED findings

### P0

目前沒有能由 live source、原始契約與現有證據閉合的 P0 項目。

### P1

目前沒有能由 live source、原始契約與現有證據閉合的 P1 項目。

### P2

目前沒有能由 live source、原始契約與現有證據閉合的 P2 項目。

以下表面疑點經 source data-flow 追完後不列為 finding：

- `SourceStep` 的局部／全域步數沒有錯接。原序列在 forward 以 `sourceStep + localStep` 呼叫來源（`learning/chemical_sequence.go:144-167`）；forward recompute 對原序列加 offset（`dynamics/coupled_recompute_continuous.go:147-180`），checkpoint replay 則建立新的 local sequence，並在 `Checkpoint` 將來源時鐘移到 `sourceStep + nextStep`（`learning/chemical_recompute.go:10-48`）。generic recompute 測試也把原序列的 global step 與 replay 的 local step 分開驗證（`dynamics/coupled_recompute_test.go:50-147,294-338`）。
- `SummaryWindow` 與 persisted controller window 的梯度列序沒有發現漏算。`Controller.BackwardActivity` 只對 `Filled` 尾端列產生梯度，並以 oldest-to-newest 回傳（`modulation/controller_state.go:64-220`）；化學 reverse 以 `step - (windowLength - 1 - row) - 1` 映射到 episode activity（`learning/chemical_sequence.go:495-559`）。截斷窗外列與 leading unfilled rows 會落在小於 `-1` 或窗外並被視為常數，符合契約。
- ticket 88 的 `Step=-1` 不會在明示 initial activity 或 persisted history 邊界錯接。只有原始 episode 的首個 truncation window、且 `initialActivityExplicit` 為 false 時，化學 reverse 才輸出 `Step=-1`（`learning/chemical_sequence.go:500-507,597-607`）；coupled reverse 只有 fresh history 才把它接到 `gy[0]`，持續 history 則跳過（`dynamics/coupled_reverse.go:124-149`）。
- `InitialActivity` 非 nil 空 slice 沒有可達的公開錯誤入口。`NewNetwork` 先在 `learning/network.go:150-153` 呼叫 `prepareChemicalEpisode`，而 `learning/chemical_episode.go:60-68` 對非 nil 值要求長度等於正數節點數；因此合法 public `ChemicalEpisode` 進入 `copyChemicalEpisode`（`learning/chemical_episode.go:107-128`）時，不會是非 nil 空 slice。checkpoint 載入最後仍經 `RestoreTrainer`／`NewNetwork`（`learning/trainer.go:194-201`），不能以只檢查 JSON 元素型別的步驟繞過寬度驗證。
- mixed chemistry recompute 被 `learning/chemical_episode.go:155-164,191-194` 明確拒絕；這是既定契約，不是本稽核 finding。持續 neural history 被視為常數，以及 stateful recompute 拒絕，也符合 `ENG.md` 化學契約。

## 2. NEEDS INVESTIGATION

### NI-1：`InitialActivity=nil` 與 chemistry recompute 的組合沒有目前執行證據

- 目前 source 的預期路徑是：`InitialActivity=nil` 時首列來源讀 core 的初始輸出（`learning/chemical_sequence.go:161-167`），reverse 以 `initialActivityExplicit=false` 保留 `Step=-1`，再由 fresh coupled history 接到 core initial（`dynamics/coupled_reverse.go:132-149`）。checkpoint 從 `nextStep>0` 重啟時不攜帶原始 initial activity，只攜帶 checkpoint raw/source boundary（`learning/chemical_recompute.go:39-47`），這與契約相符。
- 缺少的證據是目前可重跑、帶獨立期望值的 public fixture，同時覆蓋 continuous 與 LIF、`SourceStep=7`、`State.Steps=3`、controller `SummaryWindow=4`、`SourceStates.Filled=4`、`InitialActivity=nil`、timeline／neural source、segment restart，以及 truncation `{0,3,5}`。現有 `learning/chemical_recompute_test.go` 的主要 fixture 使用 explicit initial activity（`:16-103,364-467`），`learning/chemical_initial_gradient_reference_test.go` 覆蓋 nil initial 但不是 segmented recompute。
- 最小驗證入口：同一 public `Config`／`Parameters` 先以 `Options.Recompute=nil` 執行 `Trainer.StepFrom`，再以 `Recompute{SegmentSteps:1}` 執行，逐一比較每個 truncation 的輸出、core initial/activity gradient、化學參數梯度與更新結果。獨立期望為：原始首列的來源活動梯度在 truncation 含第 0 列時抵達 `Core.Initial`；segment restart 的 local step 0 讀 checkpoint raw output，不重新消費原始 initial activity；`SourceStep=7` 只影響來源絕對時鐘。

### NI-2：`Replay` 與非零 `SourceStep` 的 trace 基準尚缺公開契約 fixture

- 現行 code 將 `SourceStep` 定義為 timelines/replay/feedback 使用的 model clock（`learning/chemical_episode.go:14-20`），來源呼叫使用 absolute step（`learning/chemical_sequence.go:158-167`），而 `Replay.Release` 直接索引 `Trace[step]`（`modulation/sources.go:208-233`）。因此若 Replay trace 是 global recording，現在的 `Trace[SourceStep+localStep]` 行為一致；若 caller 把 trace 視為 episode-relative，則 `SourceStep>0` 會越界或讀錯列。
- 缺少的證據是正式契約沒有在 chemistry public path 明示 Replay trace 的 global／relative 起點，也沒有 `Replay + SourceStep>0 + segment restart` 的 current fixture。這不足以把目前行為列成 confirmed bug。
- 最小驗證入口：建立每列值可辨識的 Replay trace，`SourceStep=7`、至少兩個 segment，分別以 full 與 recompute 執行。先固定獨立期望：global contract 應逐列讀 `Trace[7+t]`，episode-relative contract 應讀 `Trace[t]`；再以該裁決補測試或另列相容性需求。

### NI-3：`Individual.TrainEpisode` 的持續 SourceStates 與 recompute 組合尚未有本輪執行紀錄

- `chemicalEpisodeNetwork` 會複製 persistent concentration/resources/pending feedback/source window，將 `SourceStep` 設為 persistent neural steps，並把最新 raw activity 作為明示且固定的 `InitialActivity`（`learning/individual_chemical_training.go:20-53`）。`TrainEpisode` 只提交參數／optimizer，保留 persistent neural/chemical/controller state（`learning/individual_chemical_training.go:64-110`、`learning/individual.go:827-859`）。靜態資料流沒有看到來源活動被重複消費或把持續 history 當成可微邊界。
- 缺少的證據是同一個 persistent snapshot 在 continuous 與 LIF、controller `Filled<W`／`Filled=W`、`ResetNeural` 後，再以 recompute 及不同 truncation 完成 `TrainEpisode` 的 current run/readback。既有 `learning/individual_chemical_training_test.go`、`learning/individual_chemical_initial_test.go` 與 `learning/chemical_stateful_test.go` 已提供相鄰契約，但本輪禁止執行測試，不能把 source review 當成行為證明。
- 最小驗證入口：先 `Advance` 形成 persistent neural history 與 `SourceStates`，保存 snapshot；分別建立 full 與 scalar recompute trainer，使用相同 episode input/target，重建 private episode 並比較 parameters、optimizer、chemical snapshot 與 controller window。獨立期望是 persistent neural/source window 只作固定 boundary，private episode 的明示 initial activity 不產生 `Step=-1` 對 core initial 的梯度。

## 3. 已核對契約、檔案與 SHA-256

原始契約：

| 檔案 | 核對位置 | SHA-256 |
|---|---|---|
| `AGENTS.md` | 專案規則與本輪唯讀限制 | `758f8a3c7df1493297e51fe19b41fb117b77296704b1aca0be6038770ae15ba0` |
| `ENG.md` | chemistry/recompute 契約，約 `199-229` | `3c2e550fb165d3d17fe97ee555c64e009cefc7945454dc73172e5820f80b45e5` |
| `docs/handoff/CoImNet_Implementation_Plan.zh-TW.md` | §8、§10.2、§11 | `b92126883a10cdb9fb852276796e0521111520709bb7ca82d06f98dbab8935ea` |
| `evidence/MOD-02/chemical-composition-20261014/plan.md` | 本輪稽核邊界 | `632577097066c95647cbb039a6ed1d135dce581bd34cd900d5db684312a24fb3` |
| `evidence/MOD-02/chemical-composition-20261014/baseline.json` | head 與 source manifest | `4fa4847815affbfd46a762976517ebda5f13c0080ed39121cc71f1fae454ec47` |

主要 live source：

| 檔案 | 核對位置 | SHA-256 |
|---|---|---|
| `learning/chemical_episode.go` | episode boundary、public recompute dispatch | `928fb5dfea73a2ace4be49debf6084aad77173c5b6ff390b6f0df40cb4d941b7` |
| `learning/chemical_sequence.go` | source clock、forward/reverse、Step -1、window mapping | `c73b9c655e451ac5dd882547a260c69e48ea5379b4b08c53c4dd2cf3b5ac9305` |
| `learning/chemical_recompute.go` | checkpoint／restart | `eb0d92c63a41127c19fd1e15236e2ebe4fdd10a64557f71fa0b8911bc523a953` |
| `learning/chemical_source_state.go` | SourceStates restore／legacy boundary | `881a24b1d7be6884d787d6ce05db8019a2bac18b13581ef275b295766924b59e` |
| `learning/chemistry.go` | persistent state、previous activity、source clock helper | `4e6164cbafd843c56c357558e9b8a580598b607daeccffbfc74a714052a52032` |
| `learning/individual_chemical_training.go` | Individual private episode／commit | `dcf14d76ede60f16e36f48f63fff48124d63c645dbde55355e43b617972f399e` |
| `learning/individual.go` | TrainEpisode、ResetNeural、RestoreIndividual | `f5e626c5532df1a95d1abdae3bf836784030fedba8382d63b92f2f1c32bf80e2` |
| `learning/network.go` | NewNetwork public validation | `56fe942df2304d60d13fe46221b43b407d8f343e6e7792258d8a7be751d2f2eb` |
| `learning/trainer.go` | RestoreTrainer／NewNetwork path | `0a2f2cce3a72e00df4198767553f4a5c58785b3d3eaaae2af1b3731c95928943` |
| `modulation/controller.go` | SummaryWindow observe/release | `1974c542a20f123cb3640d8ca6453d861f57afc4ff2a54c5552cbbe4011e1bab` |
| `modulation/controller_state.go` | persisted window reverse | `c568341acf0662a81ebc8a0ddc09eae04c80edbf001fda4a446e670b6aa2b58f` |
| `modulation/sources.go` | timeline／Replay absolute step | `5280342db13372aff9ec3f3f540961135e762d97f0af4821a47e6986b85883af` |
| `dynamics/coupled_recompute_continuous.go` | segmented forward/backward | `cfd31afac8ddf4922739e52771f0f4169dda585695353e194efd8b70c95fb43f` |
| `dynamics/coupled_recompute_lif.go` | LIF segmented replay | `841372c946c2113c13b02f32421705d61ed88432d33fda522371a25af6b645a9` |
| `dynamics/coupled_reverse.go` | global activity／history boundary | `3a7a810e7c3dfda625d3b86f643bd33f9864aa76af679013e127244ffd78d529` |
| `checkpoint/chemical_episode.go` | checkpoint normalization | `0555a9a3573d019a95dc5f5aef3e17e25c759cc32191f0eb088fb2981523b3f8` |

已讀既有測試／證據來源：

| 檔案 | SHA-256 |
|---|---|
| `learning/chemical_recompute_test.go` | `2086215c6c242503cdd49566394fdd8ec53ee5b59dadf420a2f9719d50ed8947` |
| `learning/chemical_initial_gradient_reference_test.go` | `36f1889f9cc38933ebfdf009bc00a4f31d1aedc7b51b27a8cd3304c2de4d044f` |
| `learning/individual_chemical_training_test.go` | `41f040964fcf3c8f31517aa1519215cce741b29cc45bb0f80e96b7d34cd9d459` |
| `learning/individual_chemical_initial_test.go` | `0b9f32dfa1f9529615249e614da59863d7b66082bca87f167799acddb8890af6` |
| `learning/chemical_stateful_test.go` | `bc2aa6726593a6a434137310616ecd1c2e847c930bd6d49b6b283a7cc4f1472c` |
| `checkpoint/chemical_recompute_resume_test.go` | `ab44f9f73865b9ae78a448d0786cf40afae31c2e7498e2541457e5f901e00f07` |
| `checkpoint/chemical_episode.go` | `0555a9a3573d019a95dc5f5aef3e17e25c759cc32191f0eb088fb2981523b3f8` |
| `checkpoint/chemical_episode_validation_test.go` | `874edd82e437124f0df43b22669c546a5a3a238f0a3ea6308c52d76b0d8a213b` |

## 4. 未驗證範圍

- 沒有執行任何 Go test、race、build、vet 或 benchmark，因此沒有測試通過聲明，也沒有把既有 test source 視為本輪行為證據。
- 沒有執行 fresh nil initial 加 segmented recompute 的 continuous/LIF 組合、Replay 非零 `SourceStep` 組合，或 Individual persistent SourceStates 的 full/recompute readback。
- 沒有做 mixed recompute，因為規格明示不支援；沒有重做多受體 clamp 稽核。
- 沒有推論全腦執行、辨識率、訓練改善或 production 行為。
