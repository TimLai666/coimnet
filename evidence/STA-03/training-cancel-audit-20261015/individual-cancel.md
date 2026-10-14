# 個體訓練與 OnlineLearner 取消稽核

日期：2026-10-15

## Problem

稽核 `Individual.TrainEpisode`／`trainEpisodeLocked`、化學個體訓練，以及 `OnlineLearner` 與個體共用狀態的取消與提交界線。主規格要求可失敗更新先完成候選計算再提交，取消、NaN、Inf 或形狀錯誤不能留下半次更新；同時線上混合更新明確採用「前半成功、後半失敗可重試」的部分成功契約。本稽核不把取消在提交完成後才抵達視為必須回錯的條件。

## Done

- 以 codebase-memory-mcp discovery 找到 `TrainEpisode`、`trainChemicalEpisode`、`OnlineLearner` 與 `Update` 的索引位置，再以目前工作樹逐行核對。索引行號不作為唯一依據。
- 核對三個指定來源與 `evidence/STA-03/training-cancel-audit-20261015/source-before.json` 的 SHA-256，內容一致；三個來源目前沒有 Git diff。
- 只讀查驗來源與既有測試，沒有執行 Go、沒有修改測試或正式程式。
- 新增本稽核報告；沒有找到明確的「回傳錯誤後個體持久狀態已提交一半」CONFIRMED 缺陷。

## Changed

| 檔案 | 變更摘要 |
|---|---|
| `runs/.tmp/training-cancel-audit-20261015/review/individual-cancel.md` | 新增個體訓練、化學邊界、OnlineLearner 部分成功與取消覆蓋稽核。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| codebase-memory-mcp discovery | CoImNet 程式索引 | 找到指定函式與型別；再以目前來源核對行號。 |
| `git diff -- learning/individual.go learning/individual_chemical_training.go learning/online.go` | 三個指定來源 | 空 diff。 |
| SHA-256 比對 | `individual.go`、`individual_chemical_training.go`、`online.go` 與 `source-before.json` | 三檔均相符。 |

## Verified

### 來源身分與規格

- `learning/individual.go`：`531d3ebba6829853a2b8a3eefaf81f744668a281c4afd926170898b5c88486d9`，對應 `source-before.json:474`。
- `learning/individual_chemical_training.go`：`dcf14d76ede60f16e36f48f63fff48124d63c645dbde55355e43b617972f399e`，對應 `source-before.json:476`。
- `learning/online.go`：`85dd936bef11d61de51535dd32c0c518cb25da8ee14ee5349c3b12718f21c7ef`，對應 `source-before.json:497`。
- `ENG.md:64` 要求所有可失敗更新先驗證並計算候選結果再提交，取消或數值／形狀錯誤不得留下半次更新。
- `docs/tickets/85-online-mixed-phase-retry.md:5,11,20,27` 明確保留混合更新成功前半，重試不得重複梯度、部分累積或間隔位置。
- `docs/tickets/86-online-individual-boundary.md:22,26-28` 明確保留個體鎖邊界、取消與失敗契約，以及混合更新不做整批回滾。

### 已確認契約行為：個體訓練的部分累積是成功提交，不是失敗半更新

- **檔案／行號：** `learning/trainer.go:447-475`、`learning/individual.go:849-864`。
- **觸發條件：** `Options.AccumulateSteps > 1`，本次梯度讓累積窗從 `Count < window` 增加一格；`ctx.Err()` 在 `learning/trainer.go:470-472` 檢查時仍為 nil。
- **提交前：** `Trainer.stepWithGradient` 使用局部 `accumulator` 加總；參數、Adam state、`updates` 尚未改變。
- **提交後：** `learning/trainer.go:473-474` 寫入 `tr.accumulator` 並回傳成功、`Applied=false`；`learning/individual.go:863` 隨後把 `i.episodes` 加一。參數、Adam state、`updates` 保持原值，累積窗成為個體的持久 optimizer 狀態。
- **依據：** 普通個體 `learning/individual_test.go:399-404,443-450` 驗證 `TrainEpisode` 成功留下部分窗；化學個體 `learning/individual_chemical_training_test.go:413-441` 驗證只改 accumulator 與 episode counter；Online 普通／化學路徑 `learning/online_accumulation_report_test.go:43-105` 驗證部分窗不報 `GradientSteps`，完整窗才報一次更新。
- **判定：** 這是主規格允許且已有測試的部分提交，不是取消或錯誤留下半次更新。`TrainEpisode` 只有在 `trainer.Step`／化學候選回傳成功後才到 `learning/individual.go:863` 增加 episode counter；錯誤在 `860-861` 直接返回，不增加 counter。

### 已確認契約行為：化學 `TrainEpisode` 的候選提交保留個體持久邊界

- **檔案／行號：** `learning/individual_chemical_training.go:10-61,64-110`、`learning/individual.go:849-864`。
- **觸發條件：** 個體啟用化學時，`trainEpisodeLocked` 走 `trainChemicalEpisode`。`chemicalEpisodeNetwork` 將化學 state、resources、queued feedback、source controller window、expression 與 source clock 複製到獨立 `ChemicalEpisode`，再建立私有 `Network`。
- **提交前：** `learning/individual_chemical_training.go:91-101` 對私有 `candidate` 執行 `Trainer.Step`。候選失敗直接返回；原始 `Individual` 的 neural/history、plastic fast、slow、chemical/controller/source、report、expression、queued feedback、parameters、optimizer、accumulator 與 episode counter 都尚未寫入。
- **取消界線：** `learning/individual_chemical_training.go:103-105` 在候選 `Step` 成功後、發佈到原 trainer 前再檢查 `ctx.Err()`；此處取消會丟棄候選，不寫入原 trainer。
- **提交後：** 僅在上述檢查通過後，`106-109` 一起替換 parameters、optimizer、updates、accumulator；`trainEpisodeLocked` 再於 `learning/individual.go:863` 增加 episode counter。沒有把候選 chemistry、controller、expression 或 neural state 寫回個體。
- **依據：** `learning/individual_chemical_training_test.go:321-329,362-411` 驗證非法輸入、預先取消、候選數值錯誤及 source clock overflow 都保留完整 snapshot 與 `ChemistryReport`；`263-293` 逐項比對 Neural、Plastic、Chemical、report、parameters、Adam state、updates、accumulator；`413-441` 驗證成功的部分累積只改 accumulator 與 episode counter。
- **判定：** 目前來源與既有測試支持化學候選錯誤／預先取消不會留下部分提交。尚未把「候選計算途中取消」做成可控測試，見 NEEDS INVESTIGATION。

### 已確認契約行為：個體持久神經／可塑性／化學前向在提交前維持候選狀態

- **檔案／行號：** `learning/individual.go:626-642,644-830,611-623`；`learning/chemical_source_state.go:111-125`。
- **觸發條件：** `Advance` 或 `AdvanceGated` 遇到取消、輸入／形狀／數值錯誤，或化學、可塑性、readout 產生錯誤。
- **提交前：** `advanceStepwise` 以局部 `neural`、`fast`、`chemical`、`chemistry`、`chemicalSources` 建立 `stepwiseResult`；controller source 由 `cloneSources` 深拷貝。每列在 `learning/individual.go:691-694` 檢查 context，任何錯誤均回傳空的候選結果。
- **提交後：** 所有 readout 值通過有限值檢查且 `learning/individual.go:611-612` 的 context 檢查通過後，`614-622` 才同時寫入 persistent neural/history、plastic fast、chemical state/report/source states。slow layer 在這條前向路徑只被讀取，沒有中途寫回；expression 只參與 readout 計算，也沒有寫回個體欄位。
- **依據：** `learning/individual.go:626-628` 直接定義結果為未提交候選；`learning/individual_test.go:300-313` 驗證 final readout 錯誤不提交 core steps；`learning/online_individual_boundary_controls_test.go:310-320,363-390` 驗證預先取消的 Act、Update、TrainEpisode 不改 snapshot；`410-506` 驗證在個體鎖等待時取消不提交外部 TrainEpisode。
- **判定：** 來源有明確的提交前最後 context 檢查與整組狀態寫入，沒有看到錯誤返回後只寫入 neural/history 或 plastic/chemical 任一半的路徑。

### 已確認契約行為：Online 混合更新取消後保留成功梯度半段，屬明示部分成功契約

- **檔案／行號：** `learning/online.go:374-386,389-440`；`learning/individual_online.go:30-50`。
- **觸發條件：** `AllowGradient && AllowPlastic`，feedback 有 target 且梯度半段成功；接著 reward gate 或 `AdvanceGated` 因取消／錯誤失敗。
- **前半提交：** `apply` 在 `learning/online.go:395-417` 先呼叫 `trainWeightedEpisode`。成功後才更新 `l.eligible`，並以 `415-417` 寫入 `gradientDone[key]`。個體的 parameters／Adam state／updates／accumulator 及 `i.episodes` 已依 `TrainEpisode` 契約提交。`AccumulateSteps=2` 時，提交的是 accumulator 而非 Adam update。
- **後半失敗：** `learning/online.go:422-434` 的 reward gate 或 `AdvanceGated` 回錯時，plastic state、neural/history、chemical/controller/source/report 保持未提交狀態；`Update` 的 `applied` 仍為 0，所以 `learning/online.go:384-385` 保留該 feedback 於 `l.pending`。
- **重試結果：** `gradientDone[key]` 使重試跳過梯度，成功的 plastic 半段在 `437-439` 清除 marker，下一次 queue rebuild 才消費 feedback。前半不重複。
- **依據：** `learning/online_mixed_retry_lifecycle_test.go:243-273` 的 gate 在第一次正分數呼叫時取消 context；第一次 `Update` 明確得到 `context.Canceled`、`Consumed=0`、`GradientSteps=1`、`PlasticSteps=0`，重試為 `GradientSteps=0`、`PlasticSteps=1`。`learning/online_mixed_retry_extra_test.go:116-179` 另外驗證 `AccumulateSteps=1/2` 的 gate failure 不重複梯度或部分累積；`184-248` 驗證多來源與 deferred queue。
- **判定：** 這是已實際驗證且由 ticket 85／86 明確授權的部分成功，不是需要整批回滾的缺陷。

### 已確認契約行為：Online batch 失敗只回滾未到達的 feedback，不回滾已完成前項

- **檔案／行號：** `learning/online.go:347-350,374-386`、`learning/online.go:395-417,422-439`。
- **觸發條件：** `available` 中多筆 feedback 依序處理，第 N 筆在梯度或 plastic 半段失敗。
- **提交前後：** 每筆成功後 `applied++`；失敗停止迴圈，`report.Consumed` 只計成功前項，`remainingFeedback(available[:applied])` 只移除成功前項。失敗筆仍在 queue，若它已完成梯度半段則 `gradientDone` 仍保留；已消費項的個體更新與 queue 移除不回滾。
- **依據：** `learning/online_mixed_retry_extra_test.go:184-248` 驗證第一 source 成功、第二 source 在梯度後失敗、第三筆 deferred；retry 只消費第二 source 且 `GradientSteps=0`，最後才處理 deferred source。
- **判定：** 這是 `Update` 公開註解與 ticket 85 的既定部分成功契約，不是 cancellation 造成的未宣告半次更新。

### 狀態所有權與共用鎖邊界

- `learning/individual.go:134-153` 的 `Individual` 擁有 trainer parameters／optimizer／accumulator、persistent neural/history、plastic fast、chemical/controller/source/report、expression 與 episode counter。`learning/online.go:145-163` 的 `OnlineLearner` 另外擁有 log、targets、Online `pending`、step／actions／eligible 與 `gradientDone`。
- `learning/online.go:260-287,303-330,358-386` 先持有 Online learner lock，再呼叫需要 Individual lock 的操作；`learning/individual_online.go:15-27,37-50` 與 `learning/individual.go:842-846` 維持 Individual → Trainer 的順序。`online_individual_boundary_controls_test.go:410-506` 的等待取消案例驗證外部 `TrainEpisode` 只在等待個體鎖時退出，釋放後結果與 serial snapshot 一致。
- Online `pending`／`gradientDone` 與化學 `Individual` 的 `ChemicalPart.PendingFeedback` 是兩個不同佇列。化學個體訓練在 `learning/individual_chemical_training.go:29-49` 只複製後者供私有 episode 使用；不會替 Online queue 或個體 chemical queue 消費 feedback。
- 因此混合更新的合法部分提交是「Individual 的梯度／累積窗」加上「Online 的 eligible／gradientDone」，而 plastic half 失敗時不應把兩者誤判成同一個 atomic transaction；ticket 85／86 明確要求保留這個邊界。

### CONFIRMED：無明確的取消失敗後半提交

目前沒有一條可由來源與既有測試同時確認為「回傳取消／錯誤後，個體或 OnlineLearner 已留下未宣告半次更新」的失敗路徑。已確認的部分累積、混合更新前半保留與批次前項消費都屬既定契約，因此不列為 CONFIRMED 缺陷。

### 已確認但不應判為缺陷：取消在提交完成後抵達會被忽略

下列路徑在最後一次可取消檢查通過後，沒有再把 context 當成回錯條件。這符合「提交界線後可成功返回」的本輪判準，不要求取消抵達已提交之後仍回錯：

- `learning/trainer.go:470-475`：累積窗在 context 檢查後寫入 `tr.accumulator`；`learning/trainer.go:568-575`：完整參數／Adam 更新在 context 檢查後整組提交。
- `learning/individual.go:611-623,849-864`：前向狀態提交後沒有第二次 context 檢查；成功的 trainer step 後 episode counter 在 `863` 增加。
- `learning/online.go:267-287`：`Act` 的 `advanceVersioned` 已提交個體狀態後才追加 log；`303-330`：`Receive` 取得 learner lock 後驗證並追加 source／pending；`377-386`：`Update` 在 apply 完成後重建 queue；`433-439`：`AdvanceGated` 成功後清除 mixed marker。

這些是可由目前來源直接確認的提交順序，但本輪沒有在精確指令間插入取消的 deterministic hook；不把缺少此類測試誤報為失敗。

### NEEDS INVESTIGATION：個體 `TrainEpisode` 計算途中取消的完整狀態證據不足

- **涉及檔案／行號：** `learning/individual.go:838-864`、`learning/individual_chemical_training.go:91-110`、`learning/trainer.go:328-337,447-575`。
- **待查觸發條件：** context 在 `lossGradient`、候選 forward、累積窗 `ctx.Err()` 檢查後但尚未完成外層返回的精確時點取消。
- **目前可知提交關係：** ordinary path 由 `Trainer.Step` 自行在 `568-575` 提交；chemical path 由 candidate `Step` 成功後再於 `103-109` 對原 trainer 發布。錯誤返回前的候選資料沒有明確寫回個體欄位。
- **缺少證據：** 沒有可控取消點能分別驗證 ordinary／chemical、`AccumulateSteps=1/>1` 在中途取消時的完整 `IndividualSnapshot`，尤其是 Neural/history、Plastic fast／slow、Chemical controller/source window、expression、queued feedback、parameters、Adam state、accumulator 及 episode counter 的前後相等性。
- **判定：** NEEDS INVESTIGATION。現有來源支持原子提交設計，不能僅因沒有精確時點測試就標 CONFIRMED bug；也不應要求提交完成後的取消必須回錯。

### NEEDS INVESTIGATION：Online pure／chemical 路徑的提交後取消與跨層快照覆蓋不足

- **涉及檔案／行號：** `learning/online.go:248-330,351-440`、`learning/individual_online.go:11-70`。
- **目前測試已覆蓋：** `learning/online_individual_boundary_controls_test.go:310-390` 覆蓋預先取消的 Act、Receive、Update、TrainEpisode；`410-506` 覆蓋等待個體鎖時取消；`learning/online_mixed_retry_lifecycle_test.go:243-273` 覆蓋混合更新在梯度後、plastic 前取消。
- **缺少證據：** 沒有 deterministic case 在 `Act` 的 `advanceVersioned` 提交後、`Receive` 的 source／pending 寫入後、pure gradient `trainWeightedEpisode` 成功後、chemical gradient candidate 發布後、或 `AdvanceGated` 成功後插入取消，再逐項比對 learner log、`pending`、`eligible`、`gradientDone` 與完整個體 snapshot。也沒有把 chemical 個體與 `AccumulateSteps=2` 的混合取消交叉覆蓋。
- **判定：** NEEDS INVESTIGATION。這些位置的來源已清楚顯示提交順序與「後來取消可成功返回」的界線，缺的是可重現的邊界測試，不是目前可確認的錯誤提交。

## Notes

- 本輪沒有執行 `go test`、`go test -race`、`go build` 或任何 Go 程式，符合「初步只看來源、不執行 Go」的限制；因此本報告的測試證據是既有檔案內容，不是本輪重跑結果。
- `source-before.json` 目前在工作樹狀態中是既有未追蹤證據檔；本輪只讀取，沒有修改。
- 沒有把文件中的時點未定義、可能的 goroutine 競態或缺少測試直接升級成 bug。
- 未審查 GPU／device Adam、`simulate` 原生 runner、consolidation、非指定的訓練入口，亦未修改其測試或契約。

## Next

- 若要關閉兩項 NEEDS INVESTIGATION，需新增可控取消點測試，至少覆蓋 ordinary／chemical、`AccumulateSteps=1/>1`、Online pure／mixed／chemical，並在每個取消點比對完整個體 snapshot 及 learner queue／marker 狀態。
- 本輪不修改測試；依目前證據，沒有可安全標成 CONFIRMED 缺陷的項目。

Skills：software-engineering-guidelines、investigate、human-writing、project-memory
