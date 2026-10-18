# 147 — 極小分類機率的訓練梯度

Epic：訓練與評分的數值正確性

User Story：分類模型應收到可表示的正確更新訊號，不能因類別排列而改變導數。

Status：completed

相關需求：TSK-01 OCR、TSK-03 文字生成、TSK-07 外部工具請求頭。

ticket146 的 877 份來源已完成固定工程驗證與交付。本票接續建立隔離候選，原驗收來源與證據保持。

## Root 決策（2026-10-18）：根因、範圍與驗證骨架

`tasks/media/external_tool.go:239-270` 與 `tasks/textgen/model.go:373-419` 的梯度分母先累加 `1 + tiny`，再從目標類別的機率減去 1。兩次運算都可能捨去仍可由 float64 表示的負導數。ticket146 修正回報誤差，保留了這段既有梯度，因此不是本票的修正。

兩組三分類的 `[0,-40,-40]`、目標為第一類時，媒體平均的目標導數應約為 `-4.248354255291589e-18`，Go 實際回傳 0。文字生成預設 259 類的目標導數應約為 `-1.096075397865229e-15`，winner 放在第 0、27、258 欄時，實際分別為 0、`-2.220446049250313e-16`、`-1.110223024625157e-15`。

獨立 Decimal160 參考與兩份凍結測試已保存。Root 的 Go 一般／競態均重現六個子案例及兩個父測試失敗，沒有編譯錯誤或競態警告，見[重現驗收](../../evidence/TSK-03/categorical-gradient-audit-20261018/reproduction-verification.json)。正式來源保持。

這是局部的數值正確性缺陷。公開更新雙模型對照也已重現：相同合法初始快照中，公開 Step 的目標 Readout 保持 0，獨立解析 upstream 的 StepFrom 產生 16 個媒體及 8 個文字的非零目標參數。一般／競態各兩個公開測試均僅因目標參數差異失敗，其他控制通過，見[公開重現](../../evidence/TSK-03/categorical-gradient-audit-20261018/public-reproduction-verification.json)。這是人工受控狀態，不證明真實任務準確率或最終效果必然下降。

先完成公開更新對照，再修正這兩處導數及相同算式的相關入口。保留模型格式、載入與公開簽名，更新差異依獨立導數驗收，不能以舊版錯誤梯度作為標準。

## 固定完成條件

- [x] 獨立高精度參考、凍結回歸及 Root 的 Go 一般／競態失敗證明根因。
- [x] 相同合法起始狀態的公開更新與獨立參考 upstream 對照，確認可觀察影響。
- [x] 隔離修正通過凍結回歸、正常控制及相關既有測試。
- [x] 正式採用後，固定最終來源通過完整工程、模型載入與最後治理。
- [x] 保存必要證據、清除自有暫存、提交推送並核對遠端。

## 減法審查

修正既有導數，不新增訓練模式、模型版本或數值依賴。任務品質比較須與導數正確性分別驗收。

## 同型態入口盤點

多模態、模仿、走廊與導覽政策、OCR 單字元及蒸餾的標籤分支也有相同算式，尚未逐一動態驗證。須保留各入口的尺度、類別權重及 blank 排除規則，不能只改兩處就宣稱同型態問題全部完成。PPO、教師 KL 與 CTC 使用不同目標，須各自建立參考，見[盤點](../../evidence/TSK-03/categorical-gradient-audit-20261018/related-entry-inventory.json)。

## Root 決策（2026-10-18）：其他入口重現與縮放反例

OCR 單字元入口也已在一般／競態各重現三個子案例及一個父測試失敗，只有目標導數被算成 0，blank 排除、輸入及有限值控制通過，見[OCR 重現](../../evidence/TSK-03/categorical-gradient-audit-20261018/ocr-reproduction-verification.json)。尚未透過公開 TrainLine 做更新對照，也沒有正式修正。現行來源另找到同型態算式的 11 處、分布於九份檔案，這只是指定語法的盤點，不能據此推論已涵蓋所有分類目標，見[來源核對](../../evidence/TSK-03/categorical-gradient-audit-20261018/related-source-review-v2.json)。

兩個主要入口的第一版隔離候選在原有回歸與邊界檢查中得到 34 pass、2 fail，另有三個縮放反例產生 4 fail 終端事件。根因是極小聚合先轉回 float64，之後才除以平均；單一弱類別即使與兩個同分最大類別並存，也須保留縮放直到正規化完成。第一版已拒絕，測試與候選保持，見[Root 核對](../../evidence/TSK-03/categorical-gradient-audit-20261018/candidate-v1-root-review.json)及[第二版骨架](../../evidence/TSK-03/categorical-gradient-audit-20261018/root-candidate-plan-v2.json)。本票仍為 2／5。

兩個入口各自實作的第二版候選，其 40 個一般／競態專項終端事件均通過。兩個完整套件的一般／競態各 345 pass／5 skip，原 310 個結果保持，各新增 40 pass。完整競態 session45683 已結束且 exit0，見[專項與一般核對](../../evidence/TSK-03/categorical-gradient-audit-20261018/candidate-v2-focused-root-review.json)及[完整競態核對](../../evidence/TSK-03/categorical-gradient-audit-20261018/candidate-v2-package-race-root-review.json)。877 份正式來源保持，其他相關入口及正式採用尚未完成。暫存改供共用函式與其他入口驗證使用，尚不能清除。

公開 DistributionDistiller.Loss 的純標籤分支也在一般／競態各重現三個子案例及父測試失敗，只有目標導數被算成 0，見[蒸餾重現](../../evidence/TSK-03/categorical-gradient-audit-20261018/distill-reproduction-verification.json)。教師 KL、混合目標及模型品質不包含在這項結論。

## Root 決策（2026-10-18）：共用導數與呼叫端尺度

九份檔案共用同一個內部分類導數函式，避免重複維護相同的微小值修正。各入口保留原有平均、權重、blank 排除與更新前錯誤處理，蒸餾的標籤導數須加回既有 KL 導數，見[共用骨架](../../evidence/TSK-03/categorical-gradient-audit-20261018/root-shared-gradient-plan.json)。模型格式及公開簽名保持。

Root 已用 Decimal1000 獨立重算 23 組案例的 835 個梯度位元，與參考完全相同。共用函式骨架在 Go 實跑中有 23 個合法子案例及父測試失敗，17 個非法輸入拒絕控制通過，見[參考核對](../../evidence/TSK-03/categorical-gradient-audit-20261018/categorical-reference-root-review.json)及[骨架失敗核對](../../evidence/TSK-03/categorical-gradient-audit-20261018/categorical-skeleton-root-review.json)。最初缺少實體套件資料夾的編譯失敗另行保留，不列為導數失敗證明。實作候選與其他入口更新對照尚待驗收，本票保持 2／5。

第一版共用候選的基本控制得到 41 pass，但另外三個合法權重反例產生 4 fail 終端事件。固定 `2^512` 的中間縮放仍會先捨去極小值，再乘上大權重時已無法恢復可表示的梯度。Root 已拒絕這版，原候選與測試保持，見[共用候選核對](../../evidence/TSK-03/categorical-gradient-audit-20261018/categorical-candidate-v1-root-review.json)。接續修正共用函式的縮放表示，尚未正式採用。

第二版共用函式把尾數與指數分開保留，Root 一般／競態各 45 pass，包含原有 23 組合法、17 組非法控制、新增三個權重反例及兩個父測試。媒體與文字兩個入口改用共用函式後，原 40 個專項在一般／競態也全數通過，見[共用版專項驗收](../../evidence/TSK-03/categorical-gradient-audit-20261018/shared-gradient-verification.json)。兩個入口同時移除舊導數專用的重複正規化迴圈。共用版的完整套件、其他入口、正式來源與舊模型驗收尚待完成，本票保持 2／5。

多工作排程會先相乘兩個有限權重，乘積可能非有限。是否存在最終導數可表示卻被錯誤拒絕的實際入口案例，尚待驗證，不列為已修正，見[呼叫端核對](../../evidence/TSK-03/categorical-gradient-audit-20261018/related-caller-root-review.json)。

## 九份入口整合驗收

OCR 的公開 TrainLine，以及蒸餾的 StepLabel／StepDistribution 都已用相同合法起始快照與獨立導數更新重現。模仿、走廊、排程、三種導覽政策及多模態兩個分類頭也已在一般／競態重現，正常控制保持，見[OCR 公開重現](../../evidence/TSK-03/categorical-gradient-audit-20261018/ocr-public-reproduction-verification.json)及[其他入口重現](../../evidence/TSK-03/categorical-gradient-audit-20261018/related-public-update-reproduction-verification.json)。這些是人工受控狀態的數值證據，不是任務品質成績。

九份入口已整合共用第三版候選，原有凍結檢查的一般／競態各 140 pass，七個套件的案例相同，見[Root 專項核對](../../evidence/TSK-03/categorical-gradient-audit-20261018/shared-all-focused-root-verification.json)。蒸餾保留教師 KL，另將標籤導數加回。各入口的平均、權重、blank 排除與時序保持。

七個套件的一般驗收合計 1,316 pass／14 skip，原 1,190 個結果保持，只新增 140 pass。首輪只有蒸餾的既有 HTTP 測試因本機監聽權限而中止，該完整套件以相同固定版本補跑通過，原失敗紀錄保持，見[一般驗收](../../evidence/TSK-03/categorical-gradient-audit-20261018/shared-all-packages-unit-root-verification.json)。完整競態已結束，合併相同固定版本的蒸餾完整套件補跑後，也是 1,316 pass／14 skip，原 1,190 個結果保持，只新增 140 pass，見[競態驗收](../../evidence/TSK-03/categorical-gradient-audit-20261018/shared-all-packages-race-root-verification.json)。原失敗紀錄保持，沒有將 exit1 當作單輪全部通過。本票隔離修正條件已驗收，進度 3／5。

第三版也移除共用函式重複配置的結果陣列。259 類的單次導數微型量測由 2,304 B／1 次 heap 配置降為 0 B／0 次，三分類原先已無 heap 配置，見[配置量測](../../evidence/TSK-03/categorical-gradient-audit-20261018/helper-allocation-root-verification.json)。這不是整體訓練加速比。

## 後續缺陷

[P2] [CONFIRMED] `tasks/ocr/recognizer.go:363` 與 `distill/distribution.go:256` 的分類回報誤差在合法 `[0,-40,-40]` 且目標為最大分數時變為零，獨立參考約為 `8.4967e-18`。固定 895 份正式來源的一般／競態各重現三個排列子案例及父測試失敗，兩個正常控制通過，見 [OCR 實證](../../evidence/TSK-01/reported-loss-audit-20261018/reproduction-verification.json)及[公開蒸餾實證](../../evidence/TSK-01/reported-loss-audit-20261018/distill-reproduction-verification.json)。兩次探測的 4,000 bytes 自有暫存已清除，必要測試、獨立參考及日誌保留。這會使該狀態的訓練回報不準確，本票導數修正沒有處理它，須另案修正。

## 正式採用與完整工程驗收

九份呼叫端、一份共用函式與 17 份回歸測試已正式採用，固定來源共 895 份，見[採用紀錄](../../evidence/TSK-03/categorical-gradient-adoption-20261018/adoption.json)。公開介面及模型格式保持。首次完整一般只有格式索引缺少測試詞彙列及模仿來源行號移動而失敗，文件修正後的完整一般已通過 8,917 pass／23 skip，原 8,800 個結果保持，只新增 140 pass，見[Root 核對](../../evidence/TSK-03/categorical-gradient-adoption-20261018/full-unit-root-verification.json)。完整工程由 session96150 接續競態與其餘步驟，尚未宣告全部通過。

正式來源的一般／競態均已在新程序載入兩種文字模型設定及三種 OCR 模式，再執行一次訓練。完整輸出與修改前、隔離候選逐位元組相同，見[舊模型驗收](../../evidence/TSK-03/categorical-gradient-adoption-20261018/old-models-root-verification.json)。這些固定人工模型的接續結果不等於真實任務品質改善。完成完整工程、最後治理與證據保存後才可清除自有暫存及交付。

session96150 已實際結束且 exit0，895 份正式來源通過九項完整工程，一般 8,917 pass／23 skip、競態 8,916 pass／24 skip，各 59 套件通過。Root 已重新解析原始及壓縮日誌，確認原 8,800 個結果保持、各新增 140 pass，並核對 12 次舊模型程序的完整輸出，見[完整驗收](../../evidence/TSK-03/categorical-gradient-adoption-20261018/full-engineering-root-verification.json)。新驗收回條起初缺少四個必要欄位，已補齊實際證據並通過 31 項治理，原失敗與原因保持。149 個必要非快取輸入已封存，1,880,270,157 bytes 自有暫存已清除，895 份來源、18 份原模型、6 份既有資料、32 份其他工作與 15,046 份原始模組保持，見[清理核對](../../evidence/TSK-03/categorical-gradient-adoption-20261018/cleanup-root-verification.json)。修正 `ff7868e` 與主要驗收 `83853c2` 已正常推送，遠端讀回一致，見[交付](../../evidence/TSK-03/categorical-gradient-adoption-20261018/delivery.json)。本票完成 5／5，整體零 bug 目標持續。
