# LIF KMNIST driver API review

## Scope

這份審查只涵蓋 ticket 101 尚未量測的 LIF 長期 KMNIST 品質所需的公開 API 接法。對照來源是 `evidence/TSK-11/chemical-quality-20261013/sources/` 的五份 driver、`learning/lif_test.go`、`learning/network.go`、`learning/core.go`、`dynamics/lif.go`、`checkpoint/bundle_kinds.go`、`ENG.md`、ticket 11、ticket 101，以及既有 LIF threshold 範例回條。沒有執行 Go、build、test、訓練或下載。

ticket 101 明確記錄「長期訓練品質沒有量測」（`docs/tickets/101-lif-build-numerics.md:19-23`）。現有 KMNIST comparison 是連續核心，不能把它的品質結果外推到 LIF（`evidence/TSK-11/chemical-quality-20261013/protocol.json:3-4`、`evidence/TSK-11/build-quality-audit-20261015/protocol.json:69-70`）。

## 可替換契約

### `tasks/ocr.Recognizer` 的邊界

`ocr.NewRecognizer` 固定建立 `learning.Config.Dynamics`，節點、邊、`DT: 1`、輸入節點、讀出節點及 `ReadoutEveryStep: true` 都在 `tasks/ocr/recognizer.go:89-135` 建立；`NewRecognizerWith` 也只是在相同連續配置上替換參數（`tasks/ocr/recognizer.go:183-201`）。因此沒有可由 `RecognizerConfig` 選成 LIF 的公開路徑。`TrainLine` 亦直接使用這個內部連續 `Trainer` 的 `StepFrom`（`tasks/ocr/recognizer.go:371-435`）。這是範例層的固定範圍，不是框架缺陷。

切換 LIF 時應移除 driver 對 `ocr.Recognizer` 作為模型持有者的依賴，直接建立公開的 `learning.Config`、`learning.Parameters`、`learning.NewNetwork` 和 `learning.NewTrainer`。`learning.Config` 要只宣告一個核心，LIF 設定放在 `LIF`，`Dynamics` 保持零值（`learning/network.go:14-25`、`learning/core.go:381-423`）。`LIFIndex` 只能給 mixed core，這條 LIF 路徑應保持 nil（`learning/network.go:26-33`、`learning/core.go:411-414`）。

### 保留的拓樸、時序與輸入

舊 driver 的單字元模型可從來源逐項重建：

| 欄位 | 應保留的值 | 數字依據 |
| --- | --- | --- |
| `Nodes` | `52` | `Height 28 + Hidden 24`，見 `tasks/ocr/recognizer.go:103-126`。 |
| `Sources`／`Targets` | 舊 driver 的 1,248 條 dense edge 順序逐項保留 | 來源對每個 `s=0..51`、每個 `h=0..23` 建立 `s -> 28+h`，見 `tasks/ocr/recognizer.go:107-118`，所以 `52×24=1,248`。 |
| `Delays` | 所有 edge 的語意都是 `0` | 舊連續配置只宣告 `Sources`、`Targets`、`DT:1`，沒有延遲（`tasks/ocr/recognizer.go:128-135`）。LIF 可傳 nil，但 `dynamics.NewLIF` 會把空延遲陣列正規化成每條 edge 的零值（`dynamics/lif.go:206-218`）。正式 protocol 建議記錄 canonicalized 的 1,248 個零，不能帶入三神經元 fixture 的 `[0,1,0]`。 |
| `DT` | `1` | 舊連續配置明確寫 `DT: 1`，見 `tasks/ocr/recognizer.go:128-130`。 |
| `InputNodes` | `0..27`，共 `28` 個 | `tasks/ocr/recognizer.go:119-122`。 |
| `ReadoutNodes` | `28..51`，共 `24` 個 | `tasks/ocr/recognizer.go:123-126`。這也正是既有化學 protocol 的 hidden-node selection（`evidence/TSK-11/chemical-quality-20261013/protocol.json:39`）。 |
| `InputSize` | `784` | 單字元輸入是 `28×28`，見 `tasks/ocr/input_profile.go:51-56` 及 driver 將 `CharacterWidth` 設成 `28`（`evidence/TSK-11/chemical-quality-20261013/sources/main.go.txt:895-906`）。 |
| 輸入列 | `[copy(image.Pixels), zero(784)]`，共 `2` 步 | `tasks/ocr/input_profile.go:112-140`。第一列是整張圖片，第二列是零傳遞步。應在 LIF driver 內建立獨立副本，不為了取得 `Inputs` 再建立一個連續 `Recognizer`。 |
| `OutputSize` | `11` | 10 個官方字元再加 blank，見 `tasks/ocr/recognizer.go:127-135` 及 `main.go.txt:884-906`。 |
| `ReadoutEveryStep` | `true` | 舊模型在 `tasks/ocr/recognizer.go:128-135` 開啟；`Network.PredictAll` 在關閉時明確拒絕（`learning/network.go:230-245`）。 |

LIF 對外讀出的是每步衰減突觸跡 `x`，不是 0／1 spike。`learning` 的 LIF adapter 將 `trace.Outputs()` 接到 edge 與 readout（`learning/core.go:183-195`），`Trainer.Spikes` 才是獨立的事件 API（`learning/trainer.go:287-299`）。因此舊 driver 的最後一列分類、cross-entropy 和 readout upstream 都應繼續作用在 `PredictAll` 的第二列 `x`，放電率若要記錄則另用 `Spikes`，不可拿 spike 取代 logits。

### `Parameters`、閾值與 trainable groups

LIF 的參數欄位與形狀應明確寫入 protocol：

| 欄位 | 長度／初始化來源 |
| --- | --- |
| `Core.Weights` | `1,248`，沿用 `NewRecognizer` 的 `uniform(PCG(seed,0), edges)` 初始化（`tasks/ocr/recognizer.go:107-118`、`160-168`）。 |
| `Core.Bias` | `52`，舊 driver 初始化為全零（`tasks/ocr/recognizer.go:155-165`）。 |
| `Core.LogTau` | `52`，舊 driver 初始化每項為 `log(2)`（`tasks/ocr/recognizer.go:155-159`）。`TauSyn` 是 LIF 設定，不是這個可訓練陣列。 |
| `ThetaRaw` | **必須是 `52`**，每個 LIF neuron 一項。`learning.lifCore` 在 forward 直接拒絕其他長度（`learning/core.go:183-195`）；測試也明確拒絕 nil、空陣列、2 項及 4 項，且只允許 LIF 的 `Trainable.Theta`（`learning/lif_test.go:114-145`）。 |
| `Encoder` | `784×28=21,952`，沿用單字元 profile 的 seeded whole-image encoder（`tasks/ocr/recognizer.go:136-154`）。 |
| `Readout` | `24×11=264`，沿用 `uniform(PCG(seed,1), hidden*classes)`（`tasks/ocr/recognizer.go:160-168`）。 |

以上合計 LIF 儲存參數 `1,248+52+52+52+21,952+264=23,620`。舊 driver 的 `Trainable` literal 只開 `Encoder`、`Weights`、`Bias`、`Readout`，省略 `Tau` 和 `Theta`，所以若為了保持原 paired baseline 明確寫成 `Trainable{Encoder:true, Weights:true, Bias:true, Tau:false, Theta:false, Readout:true}`，可訓練數仍是 `1,248+52+21,952+264=23,516`，與舊 driver 的固定回報一致（`evidence/TSK-11/chemical-quality-20261013/sources/comparison_main.go.txt:53-64`、`tasks/ocr/recognizer.go:169-172`）。若另開事前宣告的 threshold-training arm，`Trainable.Theta:true` 會多出 `52` 項，變成 `23,568`；不可在分數出來後選擇其中一種。

LIF core 需要額外的設定欄位。現有已封存的預註冊 threshold fixture 是可核對、非分數挑選的來源：`tau_syn=1`、`theta_min=0.05`、`theta_max=1`、`v_reset=-0.5`、`refractory_steps=1`、adaptation 關閉、`fast_sigmoid` scale `2`（`evidence/TSK-11/chemical-quality-20261013/full-verification/lif-threshold.json:18-49`）。ticket 11 另記錄該範例採 `theta_raw=-2` 並以固定值建構（`docs/tickets/11-lif-core.md:166-169`）。若採用這組設定，應把它逐值寫入新的 KMNIST protocol，並把 `ThetaRaw` 以固定 `-2` 初始化 52 項；不要搬入該 fixture 的三節點拓樸、延遲邊、tonic readout bias、300 updates 或 `0.02` learning rate，因為那些是 synthetic fixture 的協定。`learning/lif_test.go:13-35` 的 `theta_raw=-1` 是單元測試 fixture，不應與真實任務的參數來源混用。

Threshold 轉換仍須遵守 `theta_base = theta_min + (theta_max-theta_min)*sigmoid(theta_raw)` 及 `v_reset < theta_min < theta_max`（`docs/tickets/11-lif-core.md:26-29`、`dynamics/lif.go:146-163`）。LIF 的正常輸出、狀態與 gradient 契約由公開 `Network`／`LIF` 路徑提供，driver 不應自行重寫核心公式。

## 必要 driver 修改

1. **模型建立。** 以顯式 `dynamics.LIFConfig` 加 `learning.Config{LIF:&core, InputSize:784, OutputSize:11, InputNodes:0..27, ReadoutNodes:28..51, ReadoutEveryStep:true}` 建立網路和 trainer。不要將舊 `Dynamics` 一起帶入，因為核心選擇要求恰好一種（`learning/core.go:381-423`）。

2. **輸入與 objective。** `sampleLogits`、`score`、`trainSample` 不再接受 `*ocr.Recognizer`，改用兩列 input helper 和 `Trainer.PredictAll`。保留 `comparisonLoss` 的 non-blank softmax cross-entropy（`comparison_main.go.txt:253-290`）。不要改成 `Network.LossGradient` 或 `Trainer.Step`，它們的公開 objective 是 MSE（`learning/network.go:258-328`、`learning/trainer.go:301-338`）。應把 `comparisonLoss` 的第二列 gradient 放入第一列全零、第二列 gradient 的 upstream，交給 `Network.LossGradientFrom` 作獨立梯度檢查，並用 `Trainer.StepFrom` 更新（`learning/network.go:331-355`、`learning/trainer.go:340-371`）。

3. **化學組裝。** `comparisonTrainer` 目前以 `base.Config.Dynamics.Nodes` 建 `nodeRegion` 和 controller（`evidence/TSK-11/chemical-quality-20261013/sources/chemistry.go.txt:13-20,25-46`）。LIF 版本必須改讀 `base.Config.LIF.Nodes`，否則 52 節點 LIF 會把來源／區域向量和 controller 宣告成零節點。`receptorCells` 仍可保留 `base.Config.ReadoutNodes`，也就是 28..51，因為這是原 protocol 的固定 hidden-node selection。

4. **化學常數。** ON／neutral 的 `Regions=1`、`Channels=1`、`DT=1`、`Tau=[2]`、初始濃度 `[[0]]`、controller `SummaryWindow=3`／`Hidden=1`／parameters `[0.6,0.2,0.05,0.8,-0.1]`、`Kd=0.5`、`N=1`、ON 的 gamma/beta `.8/.1`、neutral 的 `0/0`、`GammaMin=.5`、`GammaMax=2`、`BetaAbsMax=1` 依原 protocol 保持（`evidence/TSK-11/chemical-quality-20261013/protocol.json:26-48`、`sources/chemistry.go.txt:25-70`）。這是固定工程對照，不是可調參數。LIF 的來源活動應沿用公開核心輸出 `x` 的語意，不另把 receptor signal 改成 spike。

5. **SDK equivalence。** 舊 `checkSDK` 的 `mapper.Snapshot()`、`mapper.Inputs()`、`mapper.TrainLine()` 及連續 recognizer clone（`comparison_main.go.txt:203-245`）要改成 `Trainer.Snapshot()`、獨立 input helper、`learning.NewNetwork`／`Network.PredictAll`／`LossGradientFrom` 和 LIF `Trainer.StepFrom`。可保留的檢查是：同一 snapshot 的 `Network.PredictAll` 與 `Trainer.PredictAll` 逐位相同；off 與 neutral 的 outputs、`Gradient`、更新後 `Parameters`／`Optimizer`／`Updates` 逐位相同；active 在固定十筆檢查樣本中產生有限且至少一處可觀察差異。`StepFrom` 的 `LossKnown` 必為 false、`Loss` 為 0，不能再要求它與 `Recognizer.TrainLine` 的 loss bits 相等（`learning/trainer.go:340-344`）。

6. **保存與接續。** 保留 `checkpoint.SaveTrainingBundle`／`LoadTrainingBundle` 和 `learning.RestoreTrainer`，不要用 `ocr.Save`／`LoadRecognizer`。checkpoint 公開入口會要求 training schema、重建 trainer、驗證 topology，再回傳 snapshot（`checkpoint/bundle_kinds.go:111-164`）；LIF `ThetaRaw`、LIF config、optimizer layout 和下一次 `StepFrom` 都應納入比較。舊 driver 的 `reflect.DeepEqual(sn.Config, expected.Config)`／`Options` 及 clone-next-update 意圖可保留，模型建構改成直接從 LIF snapshot 做。

7. **報告欄位。** `task_config` 應記錄顯式 LIF config 及 trainable groups，另記 `ThetaRaw` 長度、總參數 `23,620`、可訓練數 `23,516` 或 threshold arm 的 `23,568`。舊 driver `trainable_parameters:23516` 的固定值（`comparison_main.go.txt:63`）只有在 `Theta:false` 且 `Tau:false` 的 baseline 才仍正確。

8. **事件診斷。** 如需放電率，使用 `Trainer.Spikes`，驗證每列寬度 `52` 且值只能是 0／1；這是額外診斷，不改分類讀出（`learning/trainer.go:287-299`、`learning/lif_test.go:585-623`）。

## 驗收建議

- 先以 `learning/lif_test.go` 的公開 Network／LIF 對照模式確認 `ThetaRaw` 形狀、讀出是 synaptic trace、`LossGradientFrom` upstream 及 `Trainer.StepFrom` 更新；這些是 API smoke check，不是 KMNIST 品質結果（`learning/lif_test.go:148-260`、`291-370`）。
- KMNIST paired run 保留既有資料選擇、6,000 筆 train、20 epochs、每組 120,000 updates、learning rate `0.002`、三個 seed `20260924/25/26`、兩個 2,000 筆 held-out split、圖片順序和 loss 定義（`evidence/TSK-11/chemical-quality-20261013/protocol.json:9-24,50-63`）。LIF core 設定、`ThetaRaw` 初始值及 `Trainable.Theta` 必須在第一個分數前固定，不能依測試結果調整。
- 每個 seed 用同一份初始 LIF snapshot 建立 off／on，neutral 只用於 API 對照，不納入正式品質組。每次評估前後都比對 snapshot，確認 `PredictAll`／`Spikes` 沒有推進 trainer state。
- 每個正式模型保存 TrainingBundle，讀回後比較 LIF config、所有 `Parameters`（包含 52 項 `ThetaRaw`）、Adam `First`／`Second`／`Steps`、`Updates`、primary／confirmation score 及下一次 `StepFrom` snapshot。這直接驗證公開 checkpoint 接續，不把人工輸出或單次預測當長期品質證據。
- 報告要把 continuous、LIF、ON／OFF、threshold frozen／trainable 分成明確 protocol，不用只比較最好的 seed、epoch 或條件。三個 seed 仍只是配對觀察，不能宣稱族群統計。

## NEEDS INVESTIGATION

1. **KMNIST 專用 LIF core 設定尚未被既有證據指定。** 目前可核對的 LIF 數值表是 synthetic threshold fixture（`evidence/TSK-11/chemical-quality-20261013/full-verification/lif-threshold.json:18-49`），不是 KMNIST。建議若沿用它就把每個值、`ThetaRaw=-2` 和不啟用 adaptation/homeostasis 寫入新的預註冊 protocol；若不沿用，必須在訓練前另定來源與理由，不能用分數挑選。

2. **`Trainable.Theta` 的正式品質問題尚未定義。** `false` 是保留舊 driver 可訓練群組與 23,516 項可更新參數的直接 baseline；`true` 是另外多 52 項的 threshold-learning arm。ticket 11 的 synthetic all／theta-only／frozen 三組（`docs/tickets/11-lif-core.md:143-148`）不能直接被當成 KMNIST 品質門檻，需先決定是單一 frozen baseline、事前配對的兩組，或另設品質 gate。

3. **LIF 化學 active 差異尚未在 KMNIST 實跑。** 公開 contract 已能指定 neutral／active，且化學 protocol 固定了所有常數，但本輪未執行 Go 或訓練，因此尚不能宣稱 active 一定改變分類、gradient 或長期品質。若十筆 SDK probe 沒有差異，先記為該 protocol 的觀察結果，再查來源活動是否在兩步輸入中足夠，不直接判定核心缺陷。

## 未驗證事項與限制

- 未執行 Go、build、test、race、vet、KMNIST 訓練或 checkpoint 實際讀寫；本文件是來源與公開契約審查。
- 尚無 LIF KMNIST 的訓練前後分數、放電率、化學差值、跨程序接續或普通／race 對照，因此不能宣稱 ticket 101 的長期任務品質已通過。
- 既有 continuous driver 的實際資料、source hashes 和品質數字只作協定基準，不作 LIF 結果或生物效益證據。

目前沒有確認到框架公開 API 需要修改的缺陷。已確認的必要工作是把 continuous-only OCR wrapper 換成公開 LIF `Network`／`Trainer`／`checkpoint` 組合，並修正 driver 化學組裝對 `base.Config.Dynamics.Nodes` 的假設。
