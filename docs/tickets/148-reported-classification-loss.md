# 148 — OCR 與蒸餾的分類誤差回報

Epic：訓練與評分的數值正確性

User Story：使用者應取得仍可表示的分類誤差，不能因正規化或權重套用順序而被回報為零。

Status：done

目前完成：5／5。914 份固定正式來源的完整工程、舊模型接續與 31 項治理已通過，99 份必要輸入已封存，2,715,076,500 bytes 自有暫存已清除，主要提交 aa6e366 已正常推送，遠端讀回完全相同，見[交付](../../evidence/TSK-01/reported-loss-candidate-20261018/delivery.json)。

前置：與 ticket149 共用同一份隔離候選與 914 份最終來源驗收。

相關需求：TSK-01、TCH-03、TCH-04。

## Root 決策（2026-10-18）：根因與驗證骨架

原版的三種 `[0,-40,-40]` 排列將約 `8.4967e-18` 的正誤差回報為零。正常分數差與半最小值補查也已先重現並凍結回歸，見 [原版失敗](../../evidence/TSK-01/reported-loss-candidate-20261018/original-red-root-verification.json)、[正常差值失敗](../../evidence/TSK-01/reported-loss-candidate-20261018/expanded-loss-root-verification.json)及[極小值失敗](../../evidence/TSK-01/reported-loss-candidate-20261018/half-minimum-red-root-verification.json)。

將最大類別以外的質量分開累加並使用 Log1p，保留分數差的捨入餘量，權重套用後才縮放。差值中間溢位從原始輸入重算，半最小值附近以高精度補充分支完成最後捨入。 完整算式、控制與來源見[共同骨架](../../evidence/TSK-01/reported-loss-candidate-20261018/joint-exponent-plan.json)及[極小值補充](../../evidence/TSK-01/reported-loss-candidate-20261018/half-minimum-plan.json)。保留 4 ULP、非零、符號與真正下溢控制，不放寬舊測試，不修改公開介面、教師 KL、模型格式或依賴。

## 固定完成條件

- [x] 獨立參考與公開入口的一般／競態原版失敗，正常控制保持。
- [x] 凍結完整邊界及公開回報／導數控制，完成 Root 失敗驗收。
- [x] 隔離修正通過全部凍結控制、受影響套件與舊模型接續。
- [x] 正式採用後固定最終來源，通過完整工程及最後治理。
- [x] 保存必要證據、清除自有暫存、提交推送並核對遠端。

## 已驗證結果

[完整工程](../../evidence/TSK-01/reported-loss-candidate-20261018/verification.json)的一般測試為 9,135 pass／23 skip，競態為 9,134 pass／24 skip，各 59 套件通過。每種建置的 8,940 個原有結果保持，只新增 218 pass，實際 session20951 已結束且 exit0，九項命令與日誌由 [Root 獨立核對](../../evidence/TSK-01/reported-loss-candidate-20261018/full-engineering-root-verification.json)。恢復需求狀態後的 [31 項治理](../../evidence/TSK-01/reported-loss-candidate-20261018/metadata-governance-preclose.json)已通過。

三種 OCR 與兩種文字舊模型，在候選／正式、一般／競態共八個新程序的完整接續輸出與原版逐位元組相同，見[舊模型驗證](../../evidence/TSK-01/reported-loss-candidate-20261018/old-models-root-verification-joint.json)。

TCH-03／TCH-04 已恢復 passed，工程追蹤為 89／91。TSK-01／TSK-02 因 [ticket150](150-ctc-tail-representability.md) 的共用 CTC 缺陷維持 specified，本票沒有修正 CTC 或量測真實任務品質。

## 減法審查

共用必要的分類算式並移除被替換的重複正規化工作，沿用既有呼叫端、正常模型與保存控制，不新增訓練模式、設定或依賴。歷次候選的失敗與中止證據保留在[驗收索引](../../evidence/TSK-01/reported-loss-candidate-20261018/README.md)。

[清理核對](../../evidence/TSK-01/reported-loss-candidate-20261018/cleanup-root-verification-joint.json)確認原模型、資料、模組與其他工作保持。
