# 149 — 分類導數的差值餘量

Epic：訓練與評分的數值正確性

User Story：分類導數在最終仍可表示時應保留正負更新訊號，不能因分數相減的捨入先變成零。

Status：in_progress

目前完成：4／5。914 份固定正式來源的完整工程、舊模型接續與 31 項治理已通過，99 份必要輸入已封存，2,715,076,500 bytes 自有暫存已清除，提交推送及遠端讀回待完成。

前置：與 ticket148 共用同一份隔離候選與 914 份最終來源驗收。

相關需求：TSK-01、TCH-03、TCH-04。

## Root 決策（2026-10-18）：根因與驗證骨架

原版的極小差值兩種排列遺失最小正值及負值，公開蒸餾入口也重現。最大權重案例偏離獨立參考 323 ULP，正常分數差與半最小值補查均納入失敗回歸，見[導數實證](../../evidence/TSK-01/reported-loss-candidate-20261018/related-gradient-root-verification.json)、[共同失敗驗收](../../evidence/TSK-01/reported-loss-candidate-20261018/joint-red-root-verification.json)及[極小值失敗](../../evidence/TSK-01/reported-loss-candidate-20261018/half-minimum-red-root-verification.json)。

Loss 與 Gradient 共用保留原始分數差餘量的指數表示。導數將權重、除數與正規化完成後才縮放，半最小值附近從原始輸入重算，每次呼叫最多重算整列一次。 完整算式、控制與來源見[共同骨架](../../evidence/TSK-01/reported-loss-candidate-20261018/joint-exponent-plan.json)及[極小值補充](../../evidence/TSK-01/reported-loss-candidate-20261018/half-minimum-plan.json)。保留 4 ULP、非零、符號與真正下溢控制，不放寬舊測試，不修改公開介面、教師 KL、模型格式或依賴。

## 固定完成條件

- [x] 獨立參考與公開入口的一般／競態原版失敗，正常控制保持。
- [x] 凍結完整邊界及公開回報／導數控制，完成 Root 失敗驗收。
- [x] 隔離修正通過全部凍結控制、受影響套件與舊模型接續。
- [x] 正式採用後固定最終來源，通過完整工程及最後治理。
- [ ] 保存必要證據、清除自有暫存、提交推送並核對遠端。

## 已驗證結果

[完整工程](../../evidence/TSK-01/reported-loss-candidate-20261018/verification.json)的一般測試為 9,135 pass／23 skip，競態為 9,134 pass／24 skip，各 59 套件通過。每種建置的 8,940 個原有結果保持，只新增 218 pass，實際 session20951 已結束且 exit0，九項命令與日誌由 [Root 獨立核對](../../evidence/TSK-01/reported-loss-candidate-20261018/full-engineering-root-verification.json)。恢復需求狀態後的 [31 項治理](../../evidence/TSK-01/reported-loss-candidate-20261018/metadata-governance-preclose.json)已通過。

三種 OCR 與兩種文字舊模型，在候選／正式、一般／競態共八個新程序的完整接續輸出與原版逐位元組相同，見[舊模型驗證](../../evidence/TSK-01/reported-loss-candidate-20261018/old-models-root-verification-joint.json)。

TCH-03／TCH-04 已恢復 passed，工程追蹤為 89／91。TSK-01／TSK-02 因 [ticket150](150-ctc-tail-representability.md) 的共用 CTC 缺陷維持 specified，本票沒有修正 CTC 或量測真實任務品質。

## 減法審查

共用必要的分類算式並移除被替換的重複正規化工作，沿用既有呼叫端、正常模型與保存控制，不新增訓練模式、設定或依賴。歷次候選的失敗與中止證據保留在[驗收索引](../../evidence/TSK-01/reported-loss-candidate-20261018/README.md)。

[清理核對](../../evidence/TSK-01/reported-loss-candidate-20261018/cleanup-root-verification-joint.json)確認原模型、資料、模組與其他工作保持。
