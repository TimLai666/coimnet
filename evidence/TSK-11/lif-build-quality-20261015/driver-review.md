# LIF build-quality driver review

## Problem

本輪要確認固定 LIF factory 是否保留連續 OCR 初值、只接受 fresh snapshot、建立正確的 optimizer／Theta 版面，且正式 `comparison_main` 的預測、梯度、更新、化學節點與保存接續都走 LIF 公開 API。審查也要找出 SDK equivalence 檢查可能漏驗或產生假陽性的地方。這份文件不把未完成的長期訓練或跨建置結果當成程式缺陷。

## Scope

- 逐項閱讀 `runs/.tmp/lif-build-quality-20261015/driver/` 的 factory、comparison、chemistry、LIF 測試與 SDK probe，並對照 `evidence/TSK-11/chemical-quality-20261013/sources/` 的 archived driver。
- 只讀核對 `evidence/TSK-11/lif-build-quality-20261015/protocol.json`、`driver-red.log`、`driver-green.log`、`sdk-ordinary.log`、`sdk-race.log`，以及 `learning`、`dynamics`、`checkpoint`、`tasks/ocr` 公開來源。
- Protocol 固定為 6,000 張訓練資料、20 epochs、三個 seed `20260924/25/26`、每個正式組 120,000 updates、ordinary/race 兩種 build，以及 12 個完整訓練程序（`protocol.json:9-19,51,65-70`）。

## Resolved findings

### Factory、初值與版面

目前沒有確認到 LIF factory 的數值或狀態錯誤。

- `makeLIFBase` 只接受 `Updates==0`、無 accumulator、無既有 LIF／Mixed／Chemistry 宣告，且 factory optimizer 的 moments／steps 都為零（`driver/lif.go:9-25`）。形狀及 schema 再由 `learning.RestoreTrainer` 驗證（`learning/trainer.go:194-224`）。
- 轉換後清除 continuous `Dynamics`，保留原本 edge、input/readout 選擇與既有 Core／Encoder／Readout 數值，新增 uniform LIF 設定及 52 個 `ThetaRaw=-2`，並只開 `Theta`、關閉 `Tau`（`driver/lif.go:26-45`）。測試逐項檢查 caller snapshot 不變、既有參數 bit-exact、LIF topology、threshold 與 optimizer ownership（`driver/lif_test.go:60-188`）。
- 公開 optimizer 版面是 `weights, bias, log_tau, theta_raw, encoder, readout`（`learning/trainer.go:72-75,661-665`）。在本 protocol 中，`52*24=1,248` weights、52 bias、52 log-tau、52 theta、`784*28=21,952` encoder、`24*11=264` readout，合計 `23,620` stored。Tau 凍結而其餘五組可訓練，故 `1,248+52+52+21,952+264=23,568`，與測試常數及 protocol（`driver/lif_test.go:12-24,159-169`; `protocol.json:147-174`）一致。
- `parameterCounts` 對本固定 uniform scalar LIF 的 nil mask／nil sharing 版面與 `learning.parameterMask` 一致。公開 mask 實作在 scalar core 每個 edge/node span 都是 1（`learning/trainer.go:685-743`），沒有證據顯示本 protocol 的 count 有錯。

### 正式 comparison path 與化學建構

目前沒有確認到正式訓練路徑仍以 continuous Recognizer 更新模型。

- `comparison_main` 建立 `ocr.Recognizer` 後立即呼叫 `makeLIFBase`，之後由 `comparisonTrainer` 建立 `learning.Trainer`；分類、`StepFrom`、評估與 checkpoint 都接受這個 trainer（`driver/comparison_main.go:53-60,71-130,155-213`）。`*ocr.Recognizer` 只提供 `Inputs` 與 alphabet，和 driver 內明示的 input-factory 說明一致（`driver/check_lif.go:12-13`; `driver/comparison_main.go:155-176`）。同 package 內 archived 的 continuous `runWithModel` 沒有被目前 `main` 呼叫，實際入口是 `comparison_main.go:30`。
- `comparisonTrainer` 已從 LIF config 取 52 nodes，建立 52 長度 `NodeRegion` 與 52-node controller；continuous path 才讀 `Dynamics.Nodes`（`driver/chemistry.go:13-23,37-48`）。LIF runtime probe 的 selection JSON 也記錄 `dynamics.nodes=0`、`lif.nodes=52`，並以 LIF core 完成 SDK event（`smoke-ordinary-off.log:1-2`; `sdk-ordinary.log:1-2`）。固定 chemistry 常數與 protocol 相符（`driver/chemistry.go:28-76`; `protocol.json:26-48`）。
- `Trainer.PredictAll` 的輸出接在 LIF synaptic trace，放電率則另以 `Trainer.Spikes` 檢查 52 寬度與二值性（`driver/check_lif.go:77-109`; `learning/network.go:230-245`; `learning/trainer.go:273-299`），沒有看到把 hard spikes 誤當分類 logits 的程式證據。

## CONFIRMED

以下是可由目前程式直接觸發的驗收漏驗或假陽性。它們不是已證實的 LIF 核心公式錯誤。

### [P1] Active chemistry 可在沒有任何差異時仍被報成 SDK-equivalence

- **觸發：** `checkLIFSDK` 只把 `activeOutputs`／`activeGradients` 累積成布林值（`driver/check_lif.go:20,28,53`），迴圈結束後沒有 `if !activeOutputs && !activeGradients` 的拒絕；事件欄位則無條件寫出 `active_outputs_differ`、`active_gradients_differ` 及其他成功旗標（`driver/check_lif.go:74`）。
- **實際證據：** archived continuous probe 曾在相同語意的 active guard 以 `panic("chemical mechanism has no forward effect")` 拒絕無差異情況（`evidence/TSK-11/chemical-quality-20261013/sources/comparison_main.go.txt:211-250`），目前 LIF 版本移除了這個 guard。現有 `sdk-ordinary.log:2` 與 `sdk-race.log:2` 恰好記錄兩個差異旗標為 `true`，所以本次回條沒有證明已觸發失敗；它證明的是檢查器本身不會在旗標為 `false` 時失敗。
- **影響：** 若化學 ON 在另一批合法 probe 上完全無輸出、梯度或更新差異，driver 仍會輸出 `SDK-equivalence`，使 active mechanism 的驗收可假陽性通過。
- **建議證據：** 依 protocol 的「active 診斷」規則（`protocol.json:51-57,174-176`），至少在十筆 probe 中要求一項事前定義的可觀察差異，並在事件前拒絕全數 `false`。目前只能確認這是驗收缺口，不能把本輪已觀察到的 `true` 旗標外推成長期品質。

### [P1] 自己保存的 TrainingBundle 被 comparison guard 以 nil／empty 差異拒絕

- **觸發：** `mode restore` 在 `checkpoint.LoadTrainingBundle` 成功後，以 `reflect.DeepEqual(sn.Config, expected.Config)` 和 `Options` 比對（`driver/comparison_main.go:82-90`）。公開 checkpoint loader 會把空的 `Config.Dynamics.Sources/Targets/Delays` 正規化成非 nil 的空 slice（`checkpoint/bundle_kinds.go:235-246`），但本程序從 fresh LIF factory 建出的 `expected.Config` 保留 nil slice。
- **實際證據：** ordinary/off 與 ordinary/on 都先成功保存 smoke bundle，接著 restore 命令以 exit code 2 結束（`smoke-ordinary-off-restore.json:1-14`; `smoke-ordinary-on-restore.json:1-14`），兩份 log 都在 `comparison_main.go:86` 報 `panic: restored configuration mismatch`（`smoke-ordinary-off-restore.log:2-6`; `smoke-ordinary-on-restore.log:2-6`）。獨立公開 `LoadTrainingBundle` probe 明確列出只有 `Config.Dynamics.Sources/Targets/Delays: []int(nil) != []int{}`，`config_JSON_equal false` 而 `options_JSON_equal true`（`restore-probe.log:1-5`; `restore-probe.json:1-9`）。這是 driver 的過嚴 guard 造成的可重現 restore failure，不是目前證據支持的 framework core bug。
- **影響：** 有效 TrainingBundle 不能進入 `learning.RestoreTrainer` 後的評估／下一步更新，fresh restore 驗收在 guard 就中止；因此目前不能宣稱保存接續通過。
- **建議證據：** 比對前使用與 checkpoint 相同的 canonical representation，或逐欄比對時把 nil／empty 的宣告等價化；保留實際 topology、LIF config、ThetaRaw、optimizer 與 options 的嚴格比對。修正後仍須由獨立程序做下一個 `StepFrom`，不能只依賴同程序 clone。

### [P1] 所謂 fresh-process next-update 其實是同一程序的 in-memory clone

- **觸發：** train 後的接續檢查直接把當前 `sn` 傳給同一程序內的 `learning.RestoreTrainer`，再對 clone 做一次 `trainSample`（`driver/comparison_main.go:122-130`）。程式沒有啟動新程序，也沒有重新讀取剛寫出的 `.state.json`（該檔只在 `driver/comparison_main.go:115-121` 寫出）。即使先修正上一項 guard，`mode restore` 目前也沒有將讀回 snapshot 的下一次更新 fingerprint 與 train 時 `next_update_sha256` 比對。
- **實際證據：** 註解仍寫「exact new-process」但實際只有 `learning.RestoreTrainer(sn)`（`driver/comparison_main.go:123-126`）。可讀的 smoke restore 已在 guard 失敗，smoke final 僅是 `mode=check`、一個 sample update（`smoke-ordinary-off.log:3`; `smoke-ordinary-on.log:3`），`sdk-ordinary.log:2`／`sdk-race.log:2` 只有 SDK check，沒有成功的 fresh-process restore event。
- **影響：** 目前最多可證明 snapshot clone 的 ownership 及原 trainer 不被 clone 改動，不能證明 JSON／TrainingBundle 經另一程序讀回後，LIF config、ThetaRaw、Adam layout 與下一個 `StepFrom` 仍逐位一致。這直接未滿足 protocol 的 fresh-process restore completion item（`protocol.json:51-57`）。
- **建議證據：** 每一個正式模型都要由獨立程序 `LoadTrainingBundle` 或等價 JSON restore，對固定同一筆輸入執行一次 `StepFrom`，再與保存程序的 `next_update_sha256`／完整 snapshot 比對。未取得該回條前，不宣稱接續已驗證。

### [P2] 化學 unit tests 沒有覆蓋 LIF node-count 分支

- **觸發：** 目前 `chemistry_test.go` 與 archived 原文完全相同，所有 `TestComparisonTrainer*` 都從 continuous `publicSnapshot(t)` 建立 base（`driver/chemistry_test.go:23-46,264-320,372-438`）。`requireProtocolChemistry` 以 `base.Config.Dynamics.Nodes` 驗證區域與 controller 節點數（`driver/chemistry_test.go:213-242`）。轉成正式 LIF 後 `Dynamics.Nodes` 是 0，而實際正確期望是 `base.Config.LIF.Nodes==52`。
- **實際證據：** production `comparisonTrainer` 已加入 LIF fallback（`driver/chemistry.go:18-21`），但 unit test 沒有呼叫 `makeLIFBase`。因此目前 green test（`driver-green.log:1-36`、`driver-race.log:1-36`）只證明 continuous chemistry，不能獨立防止 LIF fallback、52-node `NodeRegion` 或 controller 節點數回歸。
- **影響：** 這不是已證實的 runtime node-count bug，因為 LIF SDK smoke 目前能建立並跑過 on／neutral；它是正式目標路徑的 coverage gap。若把 LIF base 接入現有 helper，現有 `Dynamics.Nodes` 斷言會在合法 LIF config 上期待 0，先暴露的是 test helper 的 continuous 假設。
- **建議證據：** 加一個只讀的 LIF chemistry assertion，node count 以 `Config.LIF.Nodes` 或核心有效 node count 取值，直接檢查 52 長度 region/controller、hidden receptor 28..51、neutral/off 的 gradient／update。這輪不修改測試。

### [P2] Network／Trainer 等價只驗 off，neutral／on 的輸出沒有獨立 Network 對照

- **觸發：** `checkLIFSDK` 只用 `off.Snapshot()` 建立 `learning.NewNetwork`，並比較 `network.PredictAll` 與 off trainer output（`driver/check_lif.go:21-35`）。neutral／on 僅走 `Network.LossGradientFrom`（`driver/check_lif.go:42-53`），沒有對應的 `Network.PredictAll` 與 neutral／on `Trainer.PredictAll` 比對。unit tests 雖有 off/on/neutral output 比對，但它們使用 continuous `publicSnapshot`（`driver/chemistry_test.go:308-438`）。
- **影響：** 若 chemistry-specific 的 Network／Trainer forward 路徑只在 neutral 或 on 分歧，現有 SDK check 可能仍通過。當前 source 沒有顯示該分歧已發生，這是等價契約漏驗，不是已確認的核心錯誤。
- **建議證據：** 對同一個 `inputs`、同一個 snapshot，分別為 off／neutral／on 建立 Network，逐位比較三組 `PredictAll` 與 Trainer 結果；保留現有 off／neutral gradient equality 與 active gradient difference。

### [P2] `external_loss_contract` 只驗 `StepFrom` 回傳欄位，未驗梯度到更新的關係

- **觸發：** probe 以 off Network 的 `LossGradientFrom` 計算 gradient，接著只檢查 off／neutral／on `StepFrom` 的 `Applied==true`、`LossKnown==false`、`Loss==0`（`driver/check_lif.go:37-59`）。它沒有把實際更新後的 Parameters／Optimizer／Updates 與該 external gradient 的獨立預期比對，也沒有比較 active `StepFrom` 更新是否反映 active gradient。事件 `external_loss_contract:true` 在這些 metadata 檢查後無條件輸出（`driver/check_lif.go:54-74`）。
- **影響：** `StepFrom` 即使錯誤忽略 upstream 或使用錯誤的 optimizer layout，仍可能滿足三個回傳欄位；off／neutral 狀態相等只證明兩條實際更新路徑相等，不能證明它們都正確套用 `LossGradientFrom`。現有來源沒有足夠證據宣稱該錯誤真的發生。
- **建議證據：** 用固定 external upstream、獨立計算的 gradient 與公開 AdamW 版面，核對一次更新後的完整 Parameters／Adam moments／Updates；至少增加 StepResult 的完整 bitwise equality 對 off／neutral 檢查。這輪不執行更新或測試。

## NEEDS INVESTIGATION

- `protocol.json:51-57,69` 要求六個 ordinary/race pair、12 個完整 20-epoch runs、各自完整 snapshot／prediction／objective／next-update fingerprints。可讀的 `sdk-ordinary`／`sdk-race` 是 `-mode check` probe，smoke final 也只顯示 `updates:1`；本文件沒有讀到完整 12 組訓練及獨立 restore 的最終回條。Root 的進行中 fullchecks 完成前，不判定品質或跨建置結果。
- `comparison_main.go:53-57` 仍以 `ocr.NewRecognizer` 取得初值與 `Inputs`。現有 protocol 明確把它定義成「input/initial-parameter factory」而非模型持有者（`protocol.json:161-175`），因此沒有證據把這當成 continuous-model runtime bug；若要完全移除 wrapper，仍需另定公開 LIF input factory 的契約。
- `parameterCounts` 對本 protocol 的 uniform scalar、nil masks、nil sharing 已有算術與測試依據；沒有本任務需要支援 vector core、mixed `LIFIndex` 或 parameter sharing 的 protocol 證據，故不把泛化限制列為 bug。
- `.state.json` 的讀寫權限與排他建立行為有 source evidence，但其 JSON canonicalization 與 checkpoint manifest 是否在獨立程序完全相同，尚無本輪 fresh-restore 回條。

## Done

已完成指定 driver／archived source／protocol／公開 API 的唯讀審查，並確認 factory 的初值、fresh guard、LIF topology、ThetaRaw 與 optimizer 數值沒有可核對的 runtime 缺陷；另列出一項可重現的 driver restore failure、五項驗收漏驗，以及成功 fresh-process next-update 尚未具備的證據。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `evidence/TSK-11/lif-build-quality-20261015/driver-review.md` | 新增本輪 driver、公開 API、SDK probe、化學節點、保存接續與未驗證範圍的證據審查。 |

## Actions

| 動作 | 對象 | 結果 |
| --- | --- | --- |
| 唯讀 Python／原文檢視 | driver、archived sources、protocol、logs、公開 Go API | 完成；沒有執行 Go、build、test、訓練、下載、網路或 Git 寫入。 |

## Verified

- Factory 測試與 source arithmetic 對應 `stored=23,620`、`trainable=23,568`、`ThetaRaw` 52 項、optimizer 三列各 23,620 項且 fresh 為零。
- 正式入口的 actual model path 是 LIF `Trainer`，continuous Recognizer 僅作初值／輸入工廠。
- 化學 LIF fallback 目前取 52 nodes，ON／neutral 常數與 protocol 一致；可讀 SDK probe 的 active output／gradient flags 均為 `true`，但檢查器沒有把它設為必要條件。
- Unit green／race logs 及 SDK ordinary／race check logs 均只作既有回條核對，未被當作長期品質或 fresh-process restore 通過證據；新增的 smoke restore 回條實際以 `restored configuration mismatch` 失敗。

## Notes

本報告的 CONFIRMED 項目包含一項 driver restore guard 的實際失敗，以及 driver／驗收契約的漏驗或假陽性路徑；restore failure 的證據指向 nil／empty canonicalization，不是對 LIF forward／backward 核心公式的臆測，也不是目前證據支持的 framework bug。現有回條中沒有可核對的 active runtime failure；也沒有完整訓練、成功獨立程序接續或跨建置品質證據。

## Next

Root 應先修正 nil／empty canonicalization guard，再補齊 active guard、LIF chemistry 獨立 assertion、neutral/on Network／Trainer 對照與 external-gradient update evidence，最後以獨立程序對每個正式 TrainingBundle 做下一步 bitwise continuation；在這些回條與 12 組完整訓練完成前，不宣稱 ticket 101 的 LIF 長期品質已通過。

Skills：無
