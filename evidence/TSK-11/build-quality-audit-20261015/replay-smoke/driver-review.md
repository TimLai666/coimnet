# TSK-11 build-quality driver review

## 範圍與限制

本報告唯讀核對：

- `evidence/TSK-11/build-quality-audit-20261015/plan.md`
- `evidence/TSK-11/build-quality-audit-20261015/protocol.json`
- `evidence/TSK-11/build-quality-audit-20261015/driver-source.json`
- `runs/.tmp/build-quality-audit-20261015/driver/chemistry.go`
- `runs/.tmp/build-quality-audit-20261015/driver/chemistry_test.go`
- `runs/.tmp/build-quality-audit-20261015/driver/comparison_main.go`
- `runs/.tmp/build-quality-audit-20261015/driver/main.go`
- `runs/.tmp/build-quality-audit-20261015/driver/model.go`

另以目前正式的 `tasks/ocr`、`learning`、`modulation` 原件核對公開契約。五份 driver 原件目前與 `driver-source.json:3-29` 宣告的 SHA-256 與 bytes 完全相符。這是原始碼靜態審查，不是執行結果。

## Confirmed

### 原連續核心與固定配置

- `comparison_main.go:53-63` 使用 `Height=28`、`Hidden=24`、`LearningRate=0.002`、`TrainableEncoder=true`、`InputProfile=single-character`、`CharacterWidth=28`，與 `protocol.json:9-13,19-20` 相符。
- `tasks/ocr/recognizer.go:128-172` 將模型建立為 `28+24=52` 個連續節點、`52*24=1,248` 條有向邊、`tanh` 動態、784 維整張影像輸入，讀出每步啟用。`single-character` 的兩列輸入由 `tasks/ocr/input_profile.go:131-140` 建立，第二列是零傳播列。
- `tasks/ocr/recognizer.go:169-172` 明確只開啟 encoder、weights、bias、readout；tau 與 theta 未開啟。因此目前可訓練參數數量是 `1,248+52+21,952+264=23,516`。`comparison_main.go:63-68` 的 23,516 與此手算相符，但 driver 沒有在執行時由 snapshot 計算或斷言，見 Needs investigation。

### ON／OFF／neutral 化學配置

- `chemistry.go:13-77` 的 `off` 將 `Config.Chemistry` 設為 nil；`on` 與 `neutral` 建立同一個一區一通道、`DT=1`、`Tau=[2]` 的 activity-summary controller，controller 為 `SummaryWindow=3`、`Hidden=1`、五個固定參數 `[0.6,0.2,0.05,0.8,-0.1]`，receptor 指向 readout nodes，effect bounds 為 `GammaMin=0.5`、`GammaMax=2`、`BetaAbsMax=1`。
- `chemistry.go:21-25,47-70` 的 on 設定與 `protocol.json:26-48` 相符。`neutral` 只把 gamma／beta scale 設為零，仍保留 declared chemistry，符合 `chemistry_test.go:372-393` 的中性控制意圖。
- `chemistry_test.go:213-262` 逐項檢查來源、region、receptor cells、Hill 係數與 effect；`chemistry_test.go:308-370` 檢查 active 的輸出、完整梯度與 StepFrom 更新確實不同且有限值；`chemistry_test.go:372-438` 檢查 neutral 的輸出、完整梯度、StepResult 及參數／optimizer／update state 與 off 相同。
- controller 公開契約本身沒有 target 欄位，且只讀 activity、已到達 feedback 與宣告 resources，見 `modulation/controller.go:18-24,123-180`。本 driver 的 `UseFeedback` 維持 false、resources 為空、pending feedback 與 task target 未放入 `ChemicalEpisode`，見 `chemistry.go:34-45,69-73`。就原始碼資料流而言，未發現 target 進入化學 controller 的路徑。

### 120,000 更新、loss 與 StepFrom

- `comparison_main.go:87-100` 以 `rand.New(rand.NewPCG(seed,37))` 逐 epoch shuffle 同一個 6,000 筆 order，跑 20 個 epoch；`trainSample` 每張圖呼叫一次 `StepFrom`，並在 `comparison_main.go:153-163` 拒絕 `Applied=false`。在目前 `AccumulateSteps=0` 的配置下，設計上的更新數為 `20*6,000=120,000`。
- `comparison_main.go:253-290` 的 `comparisonLoss` 與正式 `tasks/ocr/recognizer.go:332-368` 的 single-character non-blank softmax cross-entropy 與梯度逐項相同。`comparison_main.go:153-159` 先用 final row 算 loss，再把 row 0 的 upstream 置零、row 1 傳入 `StepFrom`，與正式 `tasks/ocr/recognizer.go:394-410,432-435` 的兩列契約相同。
- `learning/trainer.go:340-371` 定義 `StepFrom` 使用 caller upstream、沿用 mask／scale／accumulation／clip／schedule／AdamW／projection；它不計算 objective，`Loss=0` 且 `LossKnown=false`。driver 目前把 loss 當作外部 objective 回報，沒有把 StepFrom 的 loss 欄位冒充已計算 loss。

### score 凍結與 target 邊界

- `comparison_main.go:166-200` 在 score 前後對完整 `TrainingSnapshot` 做 digest，且每筆只呼叫 `PredictAll`。正式 `learning/trainer.go:273-285` 也明確保證 PredictAll 是 frozen independent episode，因此 score 不應更新參數、optimizer、update counter 或 accumulator。
- `comparison_main.go:142-150,176-182` 先以 image 建立 inputs、完成 forward，再以 sample text 取得 target 計算評分；target 沒有進 `Inputs`。訓練時 target 只在 `comparison_main.go:153-159` 轉成 loss upstream。這符合 `protocol.json:19-20,26-48` 的 no-target／no-feedback chemistry boundary。

### 保存、恢復與下一次更新的程式路徑

- `comparison_main.go:73-84` 的 restore 模式使用 `checkpoint.LoadTrainingBundle`，核對保存的 `Config` 與 `Options` 後交給 `learning.RestoreTrainer`。`checkpoint/bundle_kinds.go:111-167` 顯示 training bundle 寫入與讀回都會先經 `RestoreTrainer`、schema、topology 及 snapshot 正規化驗證。
- `comparison_main.go:103-117` 訓練完成後保存完整 `TrainingSnapshot`，並從該 snapshot 建立 clone、執行一個下一次 `trainSample`，輸出 `next_update_sha256`。restore 模式也會在載入後建立 clone 並輸出下一次更新摘要。
- `model.go:19-43` 的 `realOCRModelArtifact` 只會記錄已驗證 checkpoint manifest digest、path、mode 與 update count；它不是這個 paired chemistry driver 的保存流程。

## ordinary／race 比較契約

### 每個相同 seed／group 必須相同

| 欄位 | 必須相同的內容 |
| --- | --- |
| 資料與切分 | 五份資料檔案的 SHA-256／bytes、metadata／class map 指紋、train/test selection hash、primary／development／confirmation 的索引與順序。driver 產生的 selection event 在 `comparison_main.go:43-68` 提供這些欄位。 |
| 模型初始狀態 | 52 節點、1,248 邊、輸入／讀出 shape、activation、trainable mask、optimizer options、initial parameter／snapshot digest。來源為 `comparison_main.go:53-68` 與 `tasks/ocr/recognizer.go:128-172`。 |
| 化學宣告 | off 必須是 nil；on 必須是 `protocol.json:26-48` 的 controller／receptor／effect；neutral 必須保留 on declaration 且只有 effect scales 為零。 |
| 訓練順序與預算 | 三個固定 seed、同一 PCG stream、每 epoch 同一張 shuffled order、20 epochs、每 epoch 6,000 次 sample，完整 run 的 `Updates` 應為 120,000。來源為 `protocol.json:9-20`、`comparison_main.go:87-100`。 |
| 同建置恢復 | 同一建置內，保存後讀回的完整 Config、Options、Parameters、Optimizer、Accumulator、Updates、score predictions／loss 與下一次 update 必須精確相同。`comparison_main.go:76-117` 只輸出摘要，需由外部證據完成配對。 |

### ordinary／race 天然可以不同

- `ProgramFingerprint.BinarySHA256` 必然可能不同，因為 race build 的 binary 本身不同。`comparison_main.go:63-68` 的 selection event 應以 source／VCS／Go／OS／ARCH 識別相同環境，不能要求 binary digest 相同。
- `model` path 由各 run 的輸出目錄決定，`comparison_main.go:117` 不應要求 ordinary／race 路徑相同。
- `train_seconds`、`training_including_development_seconds` 與外層 elapsed time 是觀察值，見 `comparison_main.go:100-102` 及 `main.go:298`，不應當作品質或數值一致性欄位。
- 跨 ordinary／race 的參數、optimizer、loss、logits、predictions、accuracy、cross-entropy 與 snapshot／parameter／optimizer hash 是本票要量測的結果，不能因 hash 不同就直接判定缺陷，也不能預先免責。應在同一輸入與預算下報告差值、錯誤互換及品質影響。相同建置的 save／restore 與下一次 update 則仍應逐位相同。
- `ProgramFingerprint.SourceSHA256` 理論上應相同，但目前只雜湊 `main.go`，不是五份 driver 原件的整體 digest，見 Needs investigation。

## Needs investigation

- `[NEEDS INVESTIGATION] comparison_main.go:30-68`：driver 接受任意 seed，只在 flag 層限制 group 為 off/on，沒有讀取或驗證 `protocol.json` 的 seed list、training／epoch／learning-rate、預期 source hashes 或 confirmation index hash。需核對實際 12 次命令與 selection event 是否逐項等於固定協定，尤其 `protocol.json:63` 的 confirmation hash。
- `[NEEDS INVESTIGATION] main.go:441-452,984-990`：metadata 的 `SHA256` 與 `CompressedBytes` 都是可省略欄位，source fingerprint 只有在 metadata 有填值時才拒絕不一致。IDX parser 可證明自洽、完整讀取與 gzip CRC，但不能單獨證明是 protocol 指定的官方六份原件。需把 `data-copy.json`／baseline 的六份檔案指紋、大小、IDX totals／CRC 與每個 run 的 selection event 對回；目前 driver 也沒有輸出 metadata bytes。
- `[NEEDS INVESTIGATION] main.go:93-117`：20*6,000 的迴圈與 `Applied` 檢查支持 120,000 的設計，但沒有在保存前明確斷言 `sn.Updates == 120000`、每 epoch 恰增 6,000、`StepResult.Updates` 恰增一，或 `Accumulated==0`。需逐一核對 12 個 final event 與每輪 updates。
- `[NEEDS INVESTIGATION] comparison_main.go:153-163,203-250`：driver 以複製的 `comparisonLoss` 回報 loss，並只檢查 `StepFrom` 的 `Applied`。`learning.StepFrom` 契約的 `Loss=0`、`LossKnown=false`、`GradientHorizonSteps`、learning rate、projection、gradient／update norm 並未在 checkSDK 的 public-vs-driver 比對中完整斷言。需補看每個建置的 StepResult 欄位與正式 OCR TrainLine 的逐筆結果。
- `[NEEDS INVESTIGATION] comparison_main.go:253-290; tasks/ocr/recognizer.go:332-368`：目前兩段 loss 原始碼相同，但 driver 是手動複製 private implementation，沒有共享 resolver 或 drift guard。需確認全量公開 SDK 對齊檢查不會只在目前版本相同，並確認未來 loss／gradient 變更不會只改其中一份。
- `[NEEDS INVESTIGATION] comparison_main.go:203-250; chemistry_test.go:308-438`：checkSDK 確實覆蓋 neutral 的 bit-exact output／gradient／StepFrom state，以及 active 的 finite、output-different、gradient-different、update-different；但 active 只有「不同」判準，沒有獨立 expected numerical reference、公式逐項對照或跨 build tolerance contract。需確認同建置數值契約是否另有獨立證據，否則 active 可能以任意非零偏差通過。
- `[NEEDS INVESTIGATION] comparison_main.go:203-250; tasks/ocr/persistence.go:26-98; model.go:19-43`：checkSDK 的 public alignment 只把 off raw `learning.Trainer.StepFrom` 與 `ocr.Recognizer.TrainLine` 比對，沒有用 active chemistry 的高階 OCR API，也沒有執行 `ocr.Recognizer.Save`／`LoadRecognizer`、`Recognizer.Evaluate` 或 `RunWithModel` 的等值檢查。需確認本票是否明確把 raw learning SDK 視為公開契約的完整範圍；若不是，公開 SDK 對齊不足。
- `[NEEDS INVESTIGATION] comparison_main.go:100-117`：每輪只保存 mean training cross-entropy 與 development score，final 只輸出完整 state 的 digest，沒有輸出可追查的 full parameter／optimizer arrays、每輪 snapshot 或 StepResult。`plan.md:12` 要求比較完整參數／最佳化器及逐輪記錄；需確認其他 evidence 是否保存 canonical full state，否則只能知道 hash 不同，不能定位是哪個欄位造成差異。
- `[NEEDS INVESTIGATION] comparison_main.go:103-117`：train 模式的下一次 update 是從保存前的 in-memory `sn` clone 建立，並未在同一流程中讀回剛寫出的 bundle 再做下一次 update。restore 模式雖然會從 bundle 讀回並輸出 next hash，driver 沒有自動把 train 與 restore 的完整 snapshot、score、下一次 update 做 equality assertion。需核對每個 seed／group／build 的真正新程序 restore evidence，不能只看 in-memory clone。
- `[NEEDS INVESTIGATION] main.go:1005-1024; driver-source.json:3-29`：`programFingerprint` 的 `SourceSHA256` 只雜湊 `main.go`，但行為還來自 chemistry、comparison_main、chemistry_test、model 四份原件；selection event 有 binary hash，final event 沒有 program fingerprint。需以 `driver-source.json` 與實際 binary／VCS 指紋核對每個 run 沒有 stale source 或混用 driver。
- `[NEEDS INVESTIGATION] comparison_main.go:63-68; chemistry_test.go:118-125`：trainable parameter count 23,516 與固定 chemistry parameter count 5 是報告中的 hard-coded 數字；`trainingStateBitsEqual` 為 neutral 控制刻意忽略 Config，且也忽略 Options、Accumulator 與 schema。當前固定配置的 accumulator 應為 nil，但需核對 snapshot 實值，並確認報告／控制沒有掩蓋意外的更新選項或累積狀態。

## 未執行項目

本輪未執行 Go build、Go test、race、vet、訓練、下載、Git、模型產生／刪除或任何外部服務操作，因此沒有 runtime pass、訓練結果或 ordinary／race 實測品質結論。

## 結論

原始碼支持：連續 52 節點／1,248 邊模型、固定 ON／OFF／neutral 宣告、兩列輸入、無 target leakage 的資料邊界、score 不更新狀態，以及 raw training bundle 的保存／讀回路徑。是否真正完成 ticket 102，仍取決於 Needs investigation 所列的固定資料與 12 組完整 run 證據、120,000 終值、active 獨立數值契約、full-state 比對及保存後新程序下一次更新比對。
