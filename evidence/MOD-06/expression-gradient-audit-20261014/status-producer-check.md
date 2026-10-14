# Status producer reachability check

## Problem

先前 `expression-review.md` 的 `NEEDS INVESTIGATION` 第 2 項假設：同一個被 `ExpressionGain.Receptor` 選到的受體索引，可能同時產生一筆可計算的 known occupancy 和一筆 `unknown_skipped` occupancy，因而稀釋 expression 平均的分母。本次只核對 production producer、expression consumer 與受體反向路徑，不修改原審查報告。

## Conclusion

**先前的 mixed known/`unknown_skipped` 分母疑慮收回：依公開 production producer 不可達，因此沒有確認的 P1/P2。**

`ExpressionGain.Receptor` 是 `Receptors.Records` 的單一索引，見 `learning/memory.go:15-20,62-76`。`Status` 是 `Receptor` record 的欄位，和同一 record 的 `Cells` 並列，見 `modulation/receptors.go:31-42`。`Receptors.Validate` 以 record 為單位驗證 status，`Cells` 只在同一 record 內逐一驗證，見 `modulation/receptors.go:117-165`。

`Receptors.Occupancies` 先逐一走訪 `r.Records`，再逐一走訪該 record 的 `Cells`，每個輸出都寫入相同的 `Receptor: i` 和相同的 `Status: record.Status`，見 `modulation/receptors.go:219-225`。接著 status switch 仍讀同一個 record 的 status：`StatusUnknown` 在全域 `AllowAssumedCoefficients` 和同一組 record coefficients 通過時，該 record 的每個 cell 都標記 `Assumed` 並計算 occupancy；否則每個 cell 都標記 `Skipped` 並計入 `UnknownSkipped`，見 `modulation/receptors.go:226-241`。這些判斷沒有 cell-specific status 分支，且任一 cell 產生非有限 occupancy 時整個 call 直接失敗，見 `modulation/receptors.go:242-248`。

因此，同一個 `ExpressionGain.Receptor` 索引的 cells 不能一部分是 known、另一部分是 `unknown_skipped`。不同 `Receptors.Records` record 可以有不同 status，但它們會有不同的 `Receptor` 索引，不會被該 expression 的 equality filter 混在一起。

## Producer and consumer evidence

- Production row 建構在 `learning/chemistry.go:235-262`：`finishRow` 直接呼叫 `receptors.Occupancies`，再依 `rec.Receptor` 索引累加與計數。`learning/individual.go:637-641,697-709` 保存每一列的同一批 occupancy records，供 readout 路徑使用。
- Expression 平均在 `learning/memory.go:102-118` 只接受 `record.Receptor == receptor` 的記錄。它不會把另一個 record index 的 status 帶進來。
- Forward expression 的 gain 與分母在 `learning/chemical_sequence.go:610-642` 同樣以 exact receptor index 計算；公開 readout 路徑在 `learning/individual.go:563-590` 也對同一列 occupancy records 呼叫 `meanOccupancy`。
- Reverse readout 在 `learning/chemical_sequence.go:386-420` 只對相同 `r.expression.Receptor` 的 records 播種，分母由相同 equality filter 的 `chemicalExpressionRecordCount` 提供。之後 `learning/chemical_sequence.go:448-468` 將每筆 seed 合併到 occupancy gradient，再交給 `Receptors.BackwardOccupancies`。
- `BackwardOccupancies` 在 `modulation/receptor_gradient.go:13-38` 重新產生同一 declaration order 的 occupancy records，並對 `Skipped`、unresponsive、未 assumed 的 unknown 記錄跳過導數。它不會把不同 receptor index 合併，也不會產生 mixed status record。

## Findings

- `CONFIRMED P1/P2`：無。
- `NEEDS INVESTIGATION`：就「同一 selected receptor index 混合 known 與 `unknown_skipped`」這個可達性問題，無。先前 `expression-review.md` 的該項疑慮正式收回。
- `StatusUnresponsive` 讓同一 record 的所有 cells 都是宣告的零 occupancy，與 mixed-status 問題不同；本次沒有證據把它判成缺陷。
- 是否應在「整個 selected record 都是 unknown-skipped」時，把這些 production 產出的零 occupancy 排除於 expression 平均，屬另一個公開語意決策。本次來源沒有證明它是 bug，未列為確認缺陷，也沒有猜測預期政策。

## Baseline and fingerprints

- 程式基準：`git rev-parse HEAD` = `318b7518eddef07c9925f7a8b97a3513123786c4`。
- `modulation/receptors.go` SHA-256：`de4b3b2926bdeaa53c1805cab7898d815b79d81015d809542f16fdd12fb8120d`。
- `learning/memory.go` SHA-256：`e43731d6ccecbbfc6c597be869bc9e6f499fa67d7a8332d1bae01eeec02e87d7`。
- `learning/chemical_sequence.go` SHA-256：`c73b9c655e451ac5dd882547a260c69e48ea5379b4b08c53c4dd2cf3b5ac9305`。
- `modulation/receptor_gradient.go` SHA-256：`c87110f8a628c171a908c38695acd06a522634c7600a98f945618482bfb5c953`。
- `modulation/config.go` SHA-256：`888441fa39463afe1afe0fb28c8e008d23c55f5e6b8e6aab171e4fd434928a01`。
- 先以 `codebase-memory-mcp` 的 `coimnet-chemistry-ocr` 索引定位 `Occupancies`、`meanOccupancy`、`BackwardOccupancies`，再核對上述 working-tree 原文與行號。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `runs/.tmp/expression-gradient-audit-20261014/review/status-producer-check.md` | 新增本次 status producer 可達性核對；原 `expression-review.md` 未修改。 |

## Actions

| 動作 | 對象 | 結果 |
| --- | --- | --- |
| 讀取 `git status --short`、`git diff -- learning modulation dynamics`、基準與 SHA-256 | working tree 與指定 production source | 完成；learning、modulation、dynamics diff 為空。 |
| 讀取 codebase-memory 索引 | `coimnet-chemistry-ocr` | 完成；索引定義與 working-tree 原文行號一致。 |
| 執行 Go、測試或 Git 寫入 | 本專案 | 未執行；本輪沒有 Go、測試、commit、stash、reset 或其他 Git 寫入。 |

## Verified

- `Occupancies` 的 record-level status 與 cell loop 證明同一 `Receptor` index 不可產生 mixed known/`unknown_skipped`。
- `meanOccupancy`、`chemicalExpressionRecordCount`、`BackwardReadout` 均以 exact `Receptor` index 對齊，沒有跨 record status 混算。
- `BackwardOccupancies` 只跳過 producer 已標記的 skipped/unresponsive record，不會製造 mixed status。
- Root 提供的 160 組獨立公開 `PredictAll`／`LossGradientFrom` 參考結果本輪不重跑，也不把它們列為未知矩陣；本文件只報告本輪的來源核對證據。

## Notes

- 本文件是對先前疑慮的來源契約更正，沒有修改原審查報告或測試、診斷資料。
- 本輪未做 runtime 數值驗證；結論限於 production producer 的可達性與 consumer index 對齊。

## Next

無。若後續要決定全 unknown-skipped selected record 的平均政策，需先由公開契約明確指定「零值納入」或「記錄排除」，再另案驗證。

Skills：investigate、diff-inspector
