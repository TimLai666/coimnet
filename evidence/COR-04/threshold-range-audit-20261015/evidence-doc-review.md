# Evidence document review

## Scope

以 Python 標準庫唯讀核對 `README.md`、`root-review.md`，以及兩份文件引用的 JSON、`final-validation/compatibility-archive.json`、`final-validation/compatibility-artifacts.tar.gz` 與來源指紋。未執行 Go、build、test、訓練、下載、網路或 Git 寫入。

## CONFIRMED

No confirmed documentation errors.

實際核對結果：

- `source-final.json` 有 812 筆來源指紋；`final-validation/source-fixed.json` 也有 812 筆，與 `README.md:3`、`root-review.md:3` 的最終來源數字一致。
- `final-validation/normal-compatibility.json` 有兩組建置（normal、race）。每組均記錄 16 組 LIF、8 組獨立 Mixed 設定；訓練情境共 32 組，16 組 LIF，32 組的完整結果與保存接續欄位皆為 `true`。這與 `README.md:21-23` 一致。
- `final-validation/compatibility-archive.json` 記錄 `successful_receipts=206`、`root_independently_rechecked_complete_pairs=32`、`root_independently_rechecked_training_resume_processes=64`。封存檔實際有 412 個成員，206 個 JSON 回條與 206 個 log；206 個 JSON 回條的 `exit_code` 全為 0，封存 SHA-256、每個成員的大小與 SHA-256 均與索引相符。這與 `README.md:23`、`root-review.md:15` 一致。
- 64 次接續的實際拆分是 32 個 `compat-trainer-*-new-resume.json` 加 32 個 `compat-trainer-*-old-resume.json`。因此文件中的「新程序」可成立為 64 次新啟動程序的總數，不能解讀成 64 次都使用最終來源。
- `final-target-summary.json` 記錄一般永久回歸 `pass=40`；`root-final-green.json` 與 `final-validation/root-green-race.json` 的目標命令均以 exit code 0 結束。這支持 `README.md:14` 所述的目標回歸成功；完整 race 仍由 `README.md:16` 明確列為未通過證據。
- `final-validation/full-unit-summary.json` 記錄 58 個 package pass、5,818 個 test pass、22 個 skip；`final-validation/full-unit.json` 的 exit code 為 0。這與 `README.md:15` 的數字一致。
- `final-validation/format.json` 的 `checked_files=808` 且 `unformatted_files=[]`；build、vet、mod verify、module list、CLI 與 dependency JSON 回條均為 exit code 0，並支持 `README.md:17` 的數字與來源路徑。
- `final-validation/wide-recovery.json` 記錄 raw `0.021880151981676275`、持續狀態下一步參考輸出 `0.5`，以及保存物恢復布林值為 `true`，支持 `README.md:25`。`modulation-floor-follow-up.json` 也明確記錄原版與最終版結果 byte-equal，支持 `README.md:45`、`root-review.md:21`。
- `README.md` 與 `root-review.md` 引用的 `source-final.json`、`final-validation/` 回條、封存索引、封存檔及審查檔均存在，沒有發現指向不存在檔案的路徑。

## NEEDS INVESTIGATION

1. `README.md:14` 的「一般與競態各 40 個測試事件」中，一般的 `40` 有 `final-target-summary.json` 直接記錄；競態回條 `final-validation/root-green-race.json` 只記錄命令、exit code 0 和 log 路徑，沒有在 JSON 內記錄事件數。`final-target-summary.json` 同時保留 `race_status: "pending"`，這應是完整 race 狀態而非目標 race，但文件沒有明確標籤。缺少競態 40 事件的獨立 JSON 計數或明確狀態欄位。建議補一份與一般相同的 race 計數回條，或在 summary 明確拆成「目標 race 已完成／完整 race pending」。目前不能判定為數字錯誤。

2. `README.md:15` 將 5,818、22、58 的來源連到 `final-validation/full-unit.json`，該回條本身只記錄命令、exit code 與 log 路徑；精確事件與 package 計數實際在同目錄的 `full-unit-summary.json`。若要求引用路徑本身即可核對數字，應補上 summary 連結。這是可追溯性缺口，不是已證實的數字矛盾。

3. `README.md:39` 聲稱實際改用兩個原生 `gpt-5.6-luna`、`max` subagent，但 `delegation-tool-failure.json` 只記錄 OpenCode 失敗、`external_review_not_invoked=true` 與 `fallback_requested`，沒有兩次實際派工的回條或模型設定。需要 Root 的派工紀錄才能確認這段敘述；目前不把它判為錯誤。

## 未驗證範圍

- 完整 `go test -race ./...` 尚在執行，沒有任何文件或本次審查把它列為通過證據。
- 競態目標回歸的 exit code 已核對，但在允許的 JSON／壓縮檔範圍內沒有重新計數其 40 個事件。
- Root 是否親自逐一讀回 206 份回條是流程敘述；本審查核對了索引、封存內容、成功 exit code 與全部 hash，未重演 Root 的讀回動作。

## Next

完整 race 結束後，更新 `final-target-summary.json` 的 race 狀態；若保留 `README.md:14-15` 的精確數字，補上競態事件計數與 `full-unit-summary.json` 的直接連結。

## Skills

Skills：無
