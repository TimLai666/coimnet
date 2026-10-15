# LIF build-quality analysis review

## Problem

本輪要確認固定 LIF KMNIST 聚合器是否會拒絕不完整的 12 組程序、錯誤的來源與樣本指紋，並正確核對 6 組參數陣列、3 組 optimizer 陣列、參數總數、更新計數、ThetaRaw 及 frozen tau counter。也要確認 cross-build 可保留實際數值差異，ON／OFF 統計逐筆使用固定標籤，極端浮點與 uint64 比較不會產生假結果。

## Scope

唯讀審查 `runs/.tmp/lif-build-quality-20261015/analysis.py`、`analysis_test.py`、`analyze_completed.py`、`evidence/TSK-11/lif-build-quality-20261015/driver/`、protocol、driver freeze、run-training 回條、四份 smoke log 及 learning／checkpoint 公開 snapshot schema。未執行主 `analyze_completed.py`、Go、build、test、race、訓練或下載。

## CONFIRMED

### [P1] SDK／工程回條沒有被最終聚合器驗證，存在 completion gate 假陽性

- **觸發：** `run-training.py.txt:127-137` 只等待十個 receipt 存在，並只斷言 `receipt['exit_code'] == 0`。其中 `sdk-ordinary-v2`、`sdk-race-v2` 的回條沒有 log SHA 或 protocol SHA 約束，也沒有解析 SDK JSON event。
- **實際程式證據：** `analyze_completed.py:53-58` 的 `events()` 只讀每個正式 run 的 train／restore receipt 與 log；`analyze_completed.py:85-133` 也沒有讀 SDK receipt 或 `SDK-equivalence` event。`protocol.json:52-57` 卻把 public SDK equivalence、neutral／active checks 列為完成條件。
- **影響：** 只要存在一份 exit code 0、但內容不是 SDK probe 或缺少 `SDK-equivalence` 欄位的 receipt，training runner 仍會進入正式 12 組程序，聚合器仍可能輸出 quality result。現有 `sdk-ordinary-v2.log:2`、`sdk-race-v2.log:2` 的 event 目前包含 `network_trainer_exact`、`neutral_outputs_gradients_updates_exact`、`external_loss_contract` 及 active flags，但聚合器沒有消費這些欄位，因此目前回條不能證明 gate 具備拒絕能力。
- **建議：** 在最終分析中核對 SDK command、log SHA、protocol SHA 及恰好一筆 `SDK-equivalence` event，至少檢查三個 contract flags；active flags 是否必須為 true 另依 protocol 明確化。

### [P2] 最終聚合器未重核正式 sample index fingerprints

- **觸發：** `run-training.py.txt:85-88` 會在執行當下核對四個 selection index SHA，但 `analyze_completed.py:102-108` 只核對 `source_files`、metadata SHA、樣本數和 epochs，沒有核對 `selection['train_indices_sha256']`、`test_indices_sha256`、`development_indices_sha256` 或 `confirmation_indices_sha256`。
- **實際程式證據：** 聚合器只在 `analyze_completed.py:49-51` 由複製後的 labels 重算 protocol split hash，沒有把那些 hash 與每一個正式 run 的 selection event 綁定。`training-complete.json` 的 accepted record 也不保存四個 selection hash。
- **影響：** 若 selection event 的 index hash 被改成錯值，或正式 driver 使用同類別內不同圖片而仍輸出結構正確的 score，聚合器仍可通過。`run-training.py` 的前置檢查是補償控制，但其原始碼／回條沒有被 `analyze_completed.py` 綁定，故聚合器本身不具備這個拒絕能力。
- **建議：** 在 `analyze_completed.py` 對 12 組 selection event 逐欄比對 protocol 的四個 index SHA，並把 selection record 或其 digest 納入 accepted／completion evidence。

### [P1] data-copy manifest 沒有不可變的預期指紋錨點

- **觸發：** `analyze_completed.py:34-37` 以當下讀到的 `data-copy.json:4-35` 作為檔案 bytes／SHA 的唯一預期值。`driver-freeze-v2.json:2-26` 沒有 data-copy manifest 的 SHA，`run-training.py.txt:26-35` 也沒有核對它。
- **實際程式證據：** 若同時替換 copied image gzip 與 `data-copy.json` 對應的 bytes／SHA，保留兩個 label gzip，`analyze_completed.py:34-37` 會通過，`analyze_completed.py:49-51` 的固定 label split hash 仍會通過，`analyze_completed.py:102-106` 的 selection source fingerprint 也會跟著同一份被替換的 manifest 通過。
- **影響：** altered image pixels 可以進入 12 組訓練而不被聚合器辨識，品質數字仍可能被報成正式資料結果。這是來源完整性 gate 的程式缺口，不代表目前複製資料已被改動。當前六份檔案與 `data-copy.json` 的 bytes／SHA 逐一相符。
- **建議：** 把 data-copy manifest 或六份固定檔案的預期 SHA 納入 freeze／protocol，分析時同時核對獨立的既有資料摘要；不要只把同一份 manifest 同時當作輸入與驗證答案。

## Numeric checks with no confirmed defect

- `analyze_completed.py:24-27` 會要求 `all_complete == true`、`count == 12`，並將 `(seed, group, build)` 集合與 protocol 的 3×2×2 組合完全比對。缺少、重複或額外 tuple 會被拒絕。
- `analyze_completed.py:60-83` 對 6 組 Parameters 逐組要求 `1248, 52, 52, 52, 21952, 264`，對 optimizer 的 `first`、`second`、`steps` 各要求 `23620`。`steps` 的 `23568` 個 `120000`、52 個 `0` 及 `steps[1300:1352]` 的 frozen log-tau 位置核對正確，符合 learning 公開 flat order（`learning/trainer.go:72-79,661-665`）。
- `analyze_completed.py:140-149` 沒有要求 ordinary／race numeric arrays 或 snapshot 相等，只保存 `compare_float_vectors`／`compare_step_vectors` 結果；cross-build 的實際差異不會被當成失敗。
- `verify_score` 先核對 prediction 長度、0..9 類別、10×10 confusion、correct、samples、accuracy 及有限非負 CE（`analysis.py:38-63`），`score_pair` 再逐筆計算差異和三種正誤轉換（`analysis.py:65-77`），沒有 zip 截斷問題。用固定 copied labels 讀取四份現有 smoke final 時，16 個 split scores 全數通過相同檢查。
- `compare_float_vectors` 以 float64 bits 區分 signed zero，對最小 subnormal `5e-324` 保留非零距離，對 opposite-sign 最大有限 float 轉用有限 Decimal max／L2（`analysis.py:80-101`）。`compare_step_vectors` 以 Python exact int 並拒絕 bool、負值及 `>=2**64`（`analysis.py:104-109`）。標準庫人工計算確認 `2**64-1` 與 max-distance、signed-zero、最小 subnormal 及 Decimal fallback 結果。
- `analysis-green.log` 顯示 10 個 helper contract tests 全數通過，`analysis-red.log` 的 10 個測試先因 stub 失敗；`analysis-controls.json` 中 helper、test、aggregation 三個 SHA 與目前檔案相符。

## NEEDS INVESTIGATION

- 目前沒有 `training-started.json`、`training-complete.json` 或 `quality-and-state-comparison.json`。`driver-freeze-v2.json:24-26` 仍記錄 full training 尚未開始，故未驗證 12 組實際訓練、288 個正式 scores、12 次新程序 restore 或任何長期品質結論。
- 聚合器只要求 `state['config'] == selection['network_config']`（`analyze_completed.py:127-132`），未把 protocol 的完整 LIF 欄位（`protocol.json:147-162`）及 `task_config` 逐值核對；`run-training.py.txt:88` 只核對 `lif.nodes == 52`。目前四份 smoke selection 的 dt、tau、theta range、reset、refractory、surrogate、52 nodes 與 1,248 edges 都和 protocol 相符，尚無實際錯值證據。需要決定 source freeze 是否足以取代聚合器的逐欄 protocol assertion。
- `ThetaRaw` 的長度 52 被核對，但 `analyze_completed.py:60-71` 沒有獨立核對 `theta_raw_initial == -2` 或保存前的 52 個初值，只依賴 frozen driver source 和事件 hash。需要一份固定 initial snapshot／ThetaRaw digest，才能把這項 protocol 條件與正式結果直接綁定。
- `mean_training_cross_entropy` 只在 `analyze_completed.py:148-149` 做 ordinary／race 差值，沒有由 logits 或另一份 objective evidence 獨立重算。現有 driver 以 `comparisonLoss` 產生該欄位，故目前不是已確認的數值錯誤，但需要明確接受「driver objective 自報、score objective 由 predictions／CE metadata 驗證」的證據邊界。

## Problem / Done / Changed / Actions / Verified / Notes / Next

### Done

已完成指定聚合器、helper、測試、driver JSON 欄位、snapshot schema、protocol、freeze、run-training 及現有 smoke 回條的唯讀核對。數值比較 helper 未發現確定錯誤，確認三項可重現的驗收完整性缺口，並把尚無正式訓練證據的部分保留為 NEEDS INVESTIGATION。

### Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `evidence/TSK-11/lif-build-quality-20261015/analysis-review.md` | 新增本輪聚合器與數值驗證審查。 |

### Actions

| 動作 | 對象 | 結果 |
| --- | --- | --- |
| 唯讀 Python／原文檢視 | analysis、driver、protocol、freeze、run-training、smoke logs、learning／checkpoint schema | 完成，未執行 Go、build、test、race、訓練、下載、網路或 Git 寫入。 |

### Verified

- 目前資料 copy 與自身 manifest 的 6 份 bytes／SHA 一致。
- 現有四份 smoke final 的 16 個 score records 以 copied labels 逐筆驗證通過。
- helper 的 10 個 red／green contract tests 與三個記錄 SHA 相符。
- LIF 6 parameter arrays、optimizer 3 arrays、總數 23,620、可訓練數 23,568、更新計數 120,000、ThetaRaw shape 52 與 frozen tau counter slice 的靜態檢查邏輯相符。

### Notes

本報告的 CONFIRMED 項目是聚合／完成 gate 的可重現驗收缺口，不是已證實的 LIF forward、backward 或 optimizer 公式錯誤。現有 smoke／SDK 回條不能替代 12 組正式訓練結果，也不能支持任何長期品質或跨建置結論。

### Next

先由 Root 決定是否把 SDK event、selection index SHA、不可變 data-copy digest 及完整 protocol LIF config 納入最終聚合器，再於 `training-complete.json` 與 `quality-and-state-comparison.json` 出現後重做只讀驗收。

Skills：無
