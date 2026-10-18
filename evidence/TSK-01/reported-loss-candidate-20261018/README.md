# OCR／蒸餾誤差與共用分類導數修正

[ticket148](../../../docs/tickets/148-reported-classification-loss.md) 與 [ticket149](../../../docs/tickets/149-categorical-gap-residual.md) 均為 5／5。914 份正式來源的完整工程與舊模型接續已通過，恢復需求狀態後的 31 項治理已通過，99 份必要輸入已封存，2,715,076,500 bytes 自有暫存已清除，主要提交 aa6e366 已正常推送，遠端讀回完全相同，見[交付](delivery.json)。

工程追蹤為 89／91，TCH-03／TCH-04 恢復 passed。OCR 多字與語音共用 CTC 的極小值缺陷另由 [ticket150](../../../docs/tickets/150-ctc-tail-representability.md) 接續，TSK-01／TSK-02 保持 specified，見[獨立查核](../../TSK-02/ctc-tail-audit-20261018/root-review.json)。本次沒有量測真實任務品質或生物機制效益。

## 已驗證

- [完整工程](verification.json)：一般 9,135 pass／23 skip、競態 9,134 pass／24 skip，各 59 套件通過，每種建置保留 8,940 個原有結果，只新增 218 pass。
- [Root 獨立核對](full-engineering-root-verification.json)：九項工程命令、完整日誌與逐筆結果均核對，實際 session20951 已結束且 exit0，見[終端回條](runner-terminal.json)。
- [舊模型接續](old-models-root-verification-joint.json)：三種 OCR 與兩種文字模型的候選／正式、一般／競態共八個新程序，完整輸出與原版逐位元組相同。
- [凍結控制](formal-tests-frozen-joint.json)與[正式來源](formal-source-joint.json)：維持 4 ULP、非零、符號與真正下溢的既定門檻，公開介面、教師 KL、模型格式與依賴保持。
- [隔離候選](precision-candidate-comprehensive-verification.json)及[正式採用](adoption-joint.json)：七套件一般／競態各 1,534 pass、14 skip，13 份固定候選原樣採用。

## 修改範圍

OCR 單字元與蒸餾標籤分支共用分類誤差函式，將最大類別以外的質量分開累加，再取 Log1p。誤差與導數共用保留分數差餘量的指數表示，權重與除數完成後才縮放。半個最小正值附近從原始輸入重算，避免過早變成零；導數每次呼叫最多重算整列一次。

[共同骨架](joint-exponent-plan.json)與[極小值補充](half-minimum-plan.json)保存根因、邊界與驗收條件。這些修正不套用 CTC 或教師 KL。

## 歷史證據

前版 901／903 份來源的失敗、中止與局部通過均保留。原始分類誤差的[失敗驗收](original-red-root-verification.json)、差值餘量的[公開重現](gap-public-red-root-verification.json)、正常分數差的[補充失敗](expanded-loss-root-verification.json)，以及半最小值的[失敗驗收](half-minimum-red-root-verification.json)均可核對。各次中止原因見[首次中止](engineering-interrupted-root-verification.json)、[903 份來源中止](engineering-final-interrupted-root-verification.json)與[舊候選中止](joint-candidate-race-interrupted-root-verification.json)。

## 剩餘交付

本輪交付已完成，狀態見 [progress.json](progress.json)。自有暫存 `runs/.tmp/reported-loss-candidate-20261018` 已清除，原模型、資料、模組及其他暫存保持，見[清理核對](cleanup-root-verification-joint.json)。

減法審查：共用兩個入口的必要算式，移除被替換的正規化工作，不新增訓練設定或依賴。
