# TSK-11 active numerical reference review

本報告只讀核對目前 source、`plan.md`、`protocol.json`、`driver-source.json` 與既有 evidence；沒有執行 Go、測試、訓練、下載、Git 或外部服務。五份 driver 原件的目前 SHA-256 與 `driver-source.json:3-29` 及 `driver-lock.json:2-7` 相符。結論分開列出可由原件確認的契約，以及仍缺少的 52 節點 driver 數值證據。

## Confirmed

### 目前 driver 的數值邊界

- `plan.md:3-5` 定義本輪是 52 節點、1,248 邊的人工連續核心、兩列 single-character 輸入、每組 6,000 筆、20 輪及 120,000 次更新。`comparison_main.go:53-63` 建立 `Hidden=24`、`InputProfile=single-character`、可訓練 encoder、學習率 `0.002` 與固定 seed；`tasks/ocr/recognizer.go:103-130` 由 `28+24=52` 及 `52*24=1,248` 建立 tanh 核心；`tasks/ocr/input_profile.go:131-140` 明確產生完整影像列與零傳播列。
- ON／OFF／neutral 的宣告可由原件逐欄核對：`chemistry.go:13-77` 的 off 是 `Chemistry=nil`，on／neutral 是一區一通道、`DT=1`、`Tau=[2]`、`SummaryWindow=3`、controller 參數 `[0.6,0.2,0.05,0.8,-0.1]`、`Kd=0.5`、`N=1`、hidden readout nodes 28..51、`GammaScale=0.8`、`BetaScale=0.1` 及 bounds；`chemistry_test.go:213-262` 逐項檢查這些值。`protocol.json:26-48` 與上述宣告一致。
- target 沒有進化學輸入：`chemistry.go:34-45,69-73` 沒有 feedback、resources 或 pending target；`comparison_main.go:142-163` 先完成兩列 forward，再只把 `classID` 轉成最後一列 loss upstream。評分也在 `PredictAll` 後才讀 target，見 `comparison_main.go:166-200`。就目前資料流，沒有 target leakage 的原始碼證據。

### 已存在、實際涵蓋化學數值與梯度的永久測試

| 契約 | 目前原件可確認的內容 | 邊界 |
| --- | --- | --- |
| 同一組 chemistry 參數的 Continuous 前向 | `learning/chemical_episode_test.go:101-123` 建立 `SummaryWindow=3`、controller `[0.6,0.2,0.05,0.8,-0.1]`、`Kd=0.5,N=1`、`GammaScale=0.8,BetaScale=0.1`、`DT=1,Tau=2`；兩節點分支仍是 tanh Continuous，見 `:125-168`。`chemicalEpisodeControllerExpressionHandRows:520-554` 獨立寫出三列 activity mean／variance、tanh／softplus、kinetics、Hill occupancy、`gain=1+0.8*occupancy`、`offset=0.1*occupancy`、核心 tanh 及 readout。`TestChemicalEpisodeControllerUsesRawActivityBeforeExpression:560-588` 以非零輸入和初始 activity 對照這張手算表。 | 這是同參數族的 1／2 節點 fixture，不是 52 節點、1,248 邊的 driver，也沒有 driver 的隨機初始化與兩列 784 維影像。 |
| 同一組 controller／Hill／γβ 的 full-path gradient | `TestChemicalEpisodeControllerGradientMatchesFullPathFiniteDifference` (`learning/chemical_episode_test.go:683-827`) 用兩節點 chemistry fixture、非零輸入及 expression，呼叫公共 `LossGradient`，再對 core weights、bias、logTau、encoder、readout、每列 input 做中央有限差分；容許式見 `:713-827`。這是完整受化學路徑的梯度契約，不是單純 off/on 差異。 | 有限差分的 objective 仍透過公共 `Predict` 計算，且 fixture 只有兩節點；沒有直接給 52 節點 driver 的固定 expected gradient、`StepFrom` 結果或逐欄數值表。 |
| Continuous chemistry 的獨立 hand forward | `learning/chemistry_test.go:16-64,74-93,135-207` 以兩節點 Continuous、脈衝、Hill `Kd=.5,N=1` 逐列手算 concentration、occupancy、gamma 與 voltage，並保存數值，例如 `gamma=1+occupancy` 與最後 voltage `1.379357325078631`。`learning/chemical_episode_test.go:350-420` 另以兩節點突觸、bias、`GammaScale=.8`、`BetaScale=.15` 寫出獨立 hand rows。 | 前者是 external timeline 且 beta 為零，後者不是 activity-summary controller；兩者都不是 52 節點 driver 的完整組合。 |
| Controller primitive | `modulation/controller_test.go:12-69` 的 `softplusByHand`、mean／population variance 與 window 行為是獨立手算，沒有呼叫 production helper；production denominator 與 variance clamp 見 `modulation/controller.go:286-339`。 | hand fixture 是 2 nodes、window 2、參數 `[1,0,0,1,0]`，不是 driver 的 52 nodes、window 3、`[0.6,0.2,0.05,0.8,-0.1]`。 |
| Hill receptor 與 Sensitivity γβ forward | `modulation/effects_test.go:77-154` 以 exact binary occupancy `0.75/0.5/0.25` 手算 sum／max 下的 gamma、beta 與 clamp；production 公式 `1+GammaScale*total`、`BetaScale*total` 見 `modulation/effects.go:149-184`。 | 這是 effect API 的 occupancy record fixture，沒有經過 controller、kinetics 或 Continuous core。 |
| Hill 與 γβ reverse | `modulation/receptor_gradient_test.go:114-255` 以 `N*o*(1-o)/c` 手算 location／累加、N=1 零濃度邊界及中央差分；`:314-496` 以手算與中央差分檢查 Sensitivity／Threshold、duplicate、sum／max 與 clamp 零斜率。`modulation/receptor_gradient_extreme_test.go:8-45` 另以獨立極限值檢查極小濃度及飽和斜率。production reverse 的相同公式與 log-domain 補救見 `modulation/receptor_gradient.go:8-87`。 | 這些是 chemistry primitive 的梯度契約，沒有證明 52 節點、兩列 driver 的整條 forward／reverse composition。 |

### ticket 65／96 的獨立參考能證明什麼

- ticket 65 的驗收原則明列 hand reaction、smooth full-path finite difference、pulse reference、core／encoder update 及新程序比對，見 `docs/tickets/65-receptor-location-and-chemical-training.md:18-24`。其完成文件與 evidence 記錄的是 CPU scalar continuous／LIF／mixed 一般訓練、continuous／LIF recompute 及 fresh-process continuous／LIF，見 `docs/tickets/65-receptor-location-and-chemical-training.md:28-34`、`evidence/MOD-02/receptor-chemistry-20261012/verification.json:52-64,157-178`。這確認 broader engineering scope，沒有確認本輪 52 節點 OCR 數值，也明確記錄 biology quantitative fidelity 與 OCR quality 未評估 (`verification.json:140-145`)。
- ticket 96 的 independent-reference evidence 是目前最強的整條 production numeric oracle：`learning/chemical_mixed_reference_test.go:14-18` 明確不呼叫 production dynamics／modulation forward/backward helper，只用公共 `Network` forward 與 `LossGradientFrom`；fixture 在 `:95-119,134-225` 是 4 rows、2 nodes，其中一個 Continuous、一個 LIF，含 activity-summary controller、Hill、非零 Sensitivity gamma／beta、transport 及 initial activity；`runMixedReference` 逐欄比對 prediction／gradient，見 `:256-315,384-450`，獨立公式在 `:882-918,993-1160`。
- ticket 96 的保存 evidence 記錄 normal／race 各 160 independent-reference cases、80 個不同 regional occupancy，最大 gradient absolute error `1.11022302e-16`，以及四個刻意錯誤 reference 均在數值欄位失敗，見 `evidence/MOD-06/expression-gradient-audit-20261014/root-review.md:3-13`、`comparison-summary.json:2-50`。這支持 controller／Hill／γβ composition 的一般正確性，但仍不是本輪純 Continuous、52 nodes、兩列 input 的 exact oracle。
- `learning/chemical_initial_gradient_reference_test.go:16-37,47-98,100-179` 另外固定 initial activity boundary 的 hand reference；`learning/chemical_recompute_test.go:16-20,148-249` 固定 source／kinetics clock、controller window、nonzero chemistry 與 Step／StepFrom recompute equality。兩者都是邊界或恢復契約，不能代替本輪 exact active numerical reference。

### ordinary／race 比較欄位

| 類別 | 同一 seed／group 的 ordinary 與 race 應如何處理 |
| --- | --- |
| 必須相同 | data copy 的 bytes／SHA、metadata／class map 指紋、train／development／primary／confirmation selection hash 與順序；seed、group、model／network／chemistry config、initial parameter／snapshot digest、train/test counts、20 epochs、`updates_expected=120000`、`trainable_parameters=23516`。`preflight.py.txt:85-102` 明確要求兩個 selection 除 `program` 外相同且 `program.source_sha256` 相同；`run-pairs.py.txt:54-65` 對每一組重驗 seed、group、split、epoch、預算與 binary。 |
| 必須具備但不必跨 build 數值相等 | 每個 run 都要有 23 個 JSON events、每輪 `updates=epoch*6000`、finite nonnegative loss、development score shape、final `updates=120000` 及四組完整 score；`run-pairs.py.txt:53-69` 是這些契約。這些是完整性和形狀要求，不是 ordinary／race 的 output equality。 |
| 同一 build 的 save／restore 必須逐位相同 | restore selection、after／confirmation／train score、updates、snapshot／parameter／optimizer hash、next-update hash、frozen flag 必須等於 train final，且 restore before 等於 train after，見 `run-pairs.py.txt:70-76`。 |
| 天然不同或只作觀察 | ordinary／race binary SHA、每次 model path、`train_seconds`／總耗時；`comparison_main.go:100-117` 對應這些欄位。跨 build 的 parameters、optimizer、loss、logits、predictions、accuracy、cross-entropy 與 snapshot hash 是本輪要量測的結果，hash 不同本身不是 bug，但必須在同一資料／順序／預算下報告差異。 |

### `checkSDK` 的實際涵蓋

`comparison_main.go:203-250` 同時建立 off、neutral、on。它對 off raw `Trainer` 與 `Recognizer.TrainLine` 的 logits、objective bits、snapshot 及 neutral optimizer／update state 做 exact 比較 (`:213-245`)；neutral 與 off 的輸出也要求 exact。active on 則只在 `:219-221,247-250` 設定 `active_outputs_differ`，沒有 active expected logits、loss、gradient、`StepResult` 或 update 的數值 oracle。driver 永久測試同樣是 active output／gradient／update「不同且 finite」(`chemistry_test.go:308-370`)，neutral 才是 bit-exact (`:372-438`)。因此 SDK check 確實涵蓋中性與 active 分支，但 active 數值契約尚未被涵蓋。

## Needs investigation

- **52 節點 driver 的 exact active numerical oracle 缺失。** 現有 `chemical_episode_test.go` 已有同參數的兩節點手算與 full-path finite difference，`chemical_mixed_reference_test.go` 已有獨立 2-node mixed dual reference，但沒有同時固定 driver 的 52 Continuous nodes、1,248 edges、兩列 784 維 input、readout cells 28..51、seeded initialization 及一區一通道設定。最小驗證是以 driver 的 exact initial snapshot／兩列 input 建一份不呼叫 production reverse helper 的純 Continuous 52-node reference，逐欄比較 chemistry release、concentration、Hill occupancy、gamma／beta、logits、loss、gradient、`StepFrom` result 與後續 snapshot，並保存 tolerance／bits 契約。
- **active SDK 對齊不足。** `checkSDK` 的 `exact_logits_objective_optimizer_updates=true` 主要由 off exact 與 neutral exact 支撐，active 只要求任一 output difference。最小驗證是為 active path 保存 canonical expected numeric values，至少涵蓋兩列 logits、loss／upstream、完整 gradient、StepResult 與一次更新；若要宣稱公開 OCR SDK 等值，還要先明定 active chemistry 是否有可呼叫的高階 OCR API，因目前 `TrainLine` 對照只走 off path。
- **preflight 已補資料與 stale-source 的大部分機制，但仍要看完成 evidence。** `preflight.py.txt:16-24` 逐一檢查 `source-before.json`、`driver-lock.json`、protected models 與六份 data copy 的 bytes／SHA；`:25-64` 檢查 gzip CRC、IDX magic／shape／payload length、class counts、split hash 與 held-out disjoint；`run-command.py.txt:7-15` 在每個 command 前後重驗 formal source、五份 driver 及 data copy。可是 `preflight.py.txt:97-102` 只把 selection 的五份 `source_files` 對回 data copy，沒有把 `selection.metadata_sha256` 另行對回 metadata copy 的 hash，也沒有在 selection event 內綁定五份 driver 的完整 hash，只能靠 command 前後 lock 防止 stale source。最小驗證是逐個 run 檢查 metadata hash、source manifest、driver-lock、binary lock 與 selection event 的一致性。
- **120,000 與 full count 目前只有腳本契約，沒有本輪完成證據。** `run-pairs.py.txt:53-76` 會要求每個 train 23 events、20 epochs、每輪 6,000、final 120,000、四組 score 及 restore exact；`:83-88` 才會在 12 個 `*-accepted.json` 存在後寫 `training-complete.json`。目前 evidence 目錄未見任何 `*-accepted.json` 或 `training-complete.json`，所以不能確認 12 組已完成。另 `23516` 在 `preflight.py.txt:94` 與 `run-pairs.py.txt:57` 是 hard-coded emitted value，尚未由每次 snapshot shape 動態重算。
- **新程序 restore 的設計可成立，實際結果仍未知。** `run-pairs.py.txt:70` 在 train command 結束後另呼叫 restore command；`run-command.py.txt:12` 以獨立 `subprocess.run` 執行並寫獨立 log，靜態上形成新的程序邊界。只有 12 組 restore receipts 與 `training-complete.json` 出現並逐欄通過 `run-pairs.py.txt:71-76` 後，才能把 `new_process_restore` 當成已確認。
- **ticket 65／96 不能替代本輪 exact driver oracle。** 它們的 evidence 以既有 scalar／mixed fixture 或 160-case composition matrix 證明框架邊界，且明確保留 task quality／biology 限制；不可把「不同」控制、歷史 full-suite receipt 或文件中的 pass 聲明當成目前 52-node active 數值已驗證。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `runs/.tmp/build-quality-audit-20261015/active-reference-review.md` | 唯一新增的本輪 active reference／ordinary-race／preflight review。 |

## 未執行項目

本輪未執行 Go build、Go test、`-race`、vet、訓練、下載、模型產生／刪除、Git、外部服務或任何測試命令；沒有 runtime pass，也沒有 ordinary／race 品質結論。
