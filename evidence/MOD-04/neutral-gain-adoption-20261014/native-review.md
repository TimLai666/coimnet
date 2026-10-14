# Neutral gain adoption review

## Problem

審查已核准的敏感度增益範圍收緊是否只在共用驗證入口拒絕不合法舊設定，並確認前向、反向、建構、個體更換、恢復與 checkpoint 沒有繞過該入口。

## Done

已完成目前來源、`git diff`、新增測試與 MOD-04 RED／GREEN 收據的唯讀審查；只新增本檔，沒有執行 Go 或修改正式程式與測試。

## 結論

目前沒有確認的 P1/P2 缺陷。`modulation/effects.go` 的最小修正符合已核准的 `0 < GammaMin <= 1 <= GammaMax`，零值仍先套用既有預設值；目前可見的建構、前向、反向、個體啟用／更換、訓練恢復與個體 checkpoint 恢復都經過共用驗證入口。這份審查不宣稱全面零 bug，也不宣稱真實辨識率提升。

## CONFIRMED(P1/P2)

無。未找到可由目前來源或收據重現的 P1/P2 缺陷。

## 通過的來源推論

- `modulation/effects.go:196-210` 先把 `GammaMin`／`GammaMax` 的零值換成 `DefaultGammaMin`／`DefaultGammaMax`，再檢查有限值、順序與是否包含 1；`modulation/effects.go:149-168` 的增益、偏移公式沒有改動。
- `modulation/effects.go:107-123` 的 `ApplyEffects` 逐一通過 `checkedEffect` 後才建立效果陣列。`modulation/config.go:247-256` 的 `ChemistryConfig.Validate` 以空佔用率呼叫同一個入口，因此建構時不會繞過增益範圍檢查。
- `modulation/receptor_gradient.go:96-100,131-136` 的 `BackwardEffects` 先呼叫 `ApplyEffects`，再用同一個 `checkedEffect` 建立反向宣告；反向路徑不能接受不合法範圍。
- `learning/chemical_episode.go:54-58,97-103`、`learning/chemical_sequence.go:67-86`、`learning/chemistry.go:117-132,265-267,291-315`、`learning/trainer.go:145-200` 與 `learning/individual.go:245-281` 都經 `ChemistryConfig.Validate` 或 `newChemicalRuntime`。失敗的 `EnableChemistry`／更換在 `learning/chemistry.go:297-315` 於指派新 runtime 前返回，既有個體狀態不會被替換。
- `checkpoint/individual.go:47-73,76-111,158-169` 的 Encode、Decode、Load 最後都經 `learning.RestoreIndividual`；`checkpoint/individual.go:261-267` 只做 wire 結構檢查，語意規則仍交給 runtime 恢復入口。訓練 checkpoint 也經 `learning.RestoreTrainer`，見 `checkpoint/checkpoint.go:62-67,82-96` 與 `checkpoint/bundle_kinds.go:111-126,140-159`。
- `git diff` 顯示正式程式只有 `modulation/effects.go` 的公開說明與共用條件改動，沒有針對化學前向、反向或保存格式增加個別繞路。`source-fixed.json` 的 806 份目前來源指紋以 Python 全量重算，缺少與不一致皆為 0。

## 收據與覆蓋

- `evidence/MOD-04/neutral-gain-adoption-20261014/red-v2.json:2-23` 的公開 RED 收據預期失敗，`exit_code=1`、20 個失敗事件、1 個通過事件；其 log 與 source manifest SHA 均重算相符。
- `evidence/MOD-04/neutral-gain-adoption-20261014/green.json:2-23` 的 GREEN 收據使用目前 `source-fixed.json`，`exit_code=0`、21 個通過事件、0 失敗、0 skip、兩個套件通過；log 與 source manifest SHA 均重算相符。
- `modulation/neutral_gain_range_test.go:12-42` 覆蓋空／sum／max 三種 mix、nil／零／非零佔用率、一般排除案例與相鄰 `float64` 邊界，並同時檢查前向與反向拒絕。`modulation/neutral_gain_range_test.go:44-54` 覆蓋零值預設、`[1,1]` 與其他合法範圍的中性輸出。
- `learning/chemical_gain_range_test.go:14-97` 覆蓋 `NewNetwork`、`NewTrainer`、`RestoreTrainer`、`EnableChemistry` 更換、`RestoreIndividual` 與 `checkpoint.EncodeIndividual`，並檢查拒絕時呼叫端設定、snapshot 與 live individual 不變。
- `evidence/MOD-04/neutral-gain-adoption-20261014/test-fixture-adjudication.json:2-7` 說明原始測試的 nil 初始電位是永久整合 fixture 用法錯誤，已改為 `[]float64{0}`，且 `production_modified=false`。這是測試 fixture 修正，不是正式程式缺陷；沒有理由判定目前測試錯誤。
- 既有 `modulation/effects_test.go:43-156`、`modulation/receptor_gradient_test.go:314-493` 與 `modulation/config_test.go:164-182` 仍保留正常公式、預設 clamp、反向數值與 JSON round-trip 的覆蓋；本輪未執行這些 Go 測試，僅以來源差異與指紋判斷新增條件沒有改寫公式或 wire 欄位。

## NEEDS INVESTIGATION

- 尚缺實際的合法舊模型跨程序驗收：使用舊來源建立合法範圍（包含 `[1,1]` 與零值預設）的模型，在新程序 `Load` 後執行前向與下一次訓練，並逐項比較輸出、參數與 optimizer state。這是採用計畫的相容性驗收，GREEN 收據只涵蓋 `./modulation` 與 `./learning` 的 `TestNeutralGain`。
- 新增 learning 測試沒有直接呼叫 `checkpoint.DecodeIndividual`／`LoadIndividual`，也沒有用實際序列化 bytes 驗證不合法舊個體 checkpoint 的拒絕與原始 bytes 不變。來源路徑已靜態確認會進 `RestoreIndividual`，但 endpoint 行為仍應由 Root 實跑確認。
- `learning/chemical_gain_range_test.go` 的上層拒絕案例只有三個代表值；相鄰 `Nextafter` 邊界目前在 `modulation` 層覆蓋，尚未在 Network／Trainer／Individual／checkpoint 每個入口各跑一次。`[1,1]` 的建構、恢復與下一次訓練也尚未由本輪收據直接證明。
- OpenCode 當輪替代審查未產生結果：`evidence/MOD-04/neutral-gain-adoption-20261014/opencode-review.log:1` 記錄 `mimo-v2.5-free` HTTP 410 deprecated；本審查因此只採用來源、指紋與既有收據，不把該 agent 的空結果當證據。

## 未執行項目

- 本輪沒有執行任何 Go、race、build、vet、相依性檢查、CLI 或高記憶體測試。
- 只讀了目前來源、`git diff`、測試檔與 MOD-04 收據，並以 Python 重算 SHA、逐行整理 JSON log 與比對 806 份 source manifest。

## Next

1. 以合法舊來源建立 snapshot，分別用零值預設、`[0.25,1.5]`、`[1,1]`，實際 `Save`／新程序 `Load`／前向／下一次訓練，保存輸出、參數、optimizer 與 snapshot 的逐位比較。
2. 以 `1.25,0`、`0,0.75`、`1.5,1.5`、`math.Nextafter(1,+Inf),2`、`.5,math.Nextafter(1,0)` 修改個體與訓練 checkpoint 的 wire payload，確認 `DecodeIndividual`、`LoadIndividual`、訓練 checkpoint／bundle 都明確拒絕，且來源 bytes、呼叫端 snapshot 與 live state 不變。
3. 直接跑合法 `[1,1]` 的 forward、`BackwardEffects`、chemical sequence reverse、`EnableChemistry` replacement 與 restore，確認中性增益仍為 bitwise 1、beta／threshold 與保存 JSON 不變。
4. 完成採用計畫要求的 gofmt、build、一般／race、vet、相依性與 CLI 接續驗證，再更新需求狀態與交付證據。
