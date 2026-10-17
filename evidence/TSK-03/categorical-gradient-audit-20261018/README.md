# 分類梯度的極小值與排列查核

媒體請求頭與文字生成的既有梯度會捨去可表示的負導數。文字生成只改變 winner 所在欄位，也會改變對應導數。這條路徑與 ticket146 的回報誤差修正分開驗證。

[root-gradient-plan.json](root-gradient-plan.json) 保存 Decimal160 解析參考及實作前骨架。[tests-frozen.json](tests-frozen.json) 記錄兩份測試、隔離映射與 877 份正式來源的指紋。Root 已閱讀全部測試，核對參考值並以 Go 一般／競態各重現 8 個失敗終端事件，見[重現驗收](reproduction-verification.json)。事件包含六個子案例與兩個父測試，不能解讀成八個獨立情境。

三分類目標導數應約為 `-4.248354255291589e-18`，實際為 0。259 類目標導數應約為 `-1.096075397865229e-15`，winner 在第 0、27、258 欄的實際結果不同。回歸也檢查其他類別、有限性、符號及未計分列，沒有修改正式來源或降低門檻。

原始測試診斷在異號值間印出的是位元圖樣距離，不能將該數字當作排序後的 ULP 距離。零值缺陷另由明確的負號斷言證明，同號值的四 ULP 比較有效，見[Root 核對](unit-root-review.json)。凍結測試保持原樣。

公開更新雙模型對照已在一般／競態重現，見[公開驗收](public-reproduction-verification.json)。相同起始狀態中，公開 Step 的目標 Readout 都為 0，獨立導數對照則有 16 個媒體及 8 個文字非零參數，其他控制通過。三個實際導數經 Decimal160 重算，與測試參考逐位相同。正式採用、真實任務品質與完整147工程尚未驗證。工作與固定完成條件見 [ticket147](../../../docs/tickets/147-categorical-gradient-stability.md)。

兩次探測只使用專案內暫存，共 30,466 bytes 已清除，見[初次清理](cleanup.json)及[公開探測清理](public-cleanup.json)。測試、映射與必要證據保留，原模型、資料、15,046 份原始模組及其他工作保持。重用的快取由 ticket146 的清理程序負責，不能用這兩份小型清理收據宣稱大型快取已清除。

兩個入口各自實作的第二版候選已通過 40 個一般／競態專項。兩個完整套件的一般／競態各 345 pass／5 skip，原 310 個結果保持，見[專項與一般核對](candidate-v2-focused-root-review.json)及[完整競態核對](candidate-v2-package-race-root-review.json)。第一版因極小值提早捨入被拒絕，其候選、反例及日誌保持。其他相關入口尚未完成，未正式採用；暫存繼續供共用函式與其他入口驗證使用，清理條件見[candidate-progress.json](candidate-progress.json)。

其他同型態入口另有 [OCR 私有導數重現](ocr-reproduction-verification.json)與[公開蒸餾純標籤分支重現](distill-reproduction-verification.json)，這些是原版重現，接續修正狀態見下文，不外推教師 KL、CTC、PPO 或真實任務品質。

[共用函式骨架](root-shared-gradient-plan.json)保留九份檔案各自的尺度及類別處理。23 組新參考的 835 個梯度位元經 Root 用 Decimal1000 重算相同，見[參考核對](categorical-reference-root-review.json)。骨架實跑得到 23 個合法子案例及父測試失敗、17 個非法輸入拒絕控制通過，見[骨架核對](categorical-skeleton-root-review.json)。原先缺少實體套件資料夾的編譯失敗另存，不算數值失敗。正式 Go 來源保持 877 份。

第一版共用候選在基本控制得到 41 pass，但三個大權重反例得到 4 fail 終端事件，見[Root 拒絕理由](categorical-candidate-v1-root-review.json)。固定 `2^512` 中間縮放仍會過早下溢，不能據此接受一般有限權重的完整契約。候選與反例保持，尚未正式採用。

第二版共用函式的一般／競態各 45 pass，兩個主要入口改用它後，原有專項各 40 pass，見[共用版專項驗收](shared-gradient-verification.json)。共用版目前完成專項，完整套件及其他相關入口尚待驗收。877 份正式來源保持，暫存尚未清除。

九份入口共用第三版導數後，一般／競態各 140 個專項全數通過，見[整合專項驗收](shared-all-focused-root-verification.json)。原版 OCR、蒸餾的公開更新，以及模仿、走廊、排程、導覽和多模態的受控更新失敗，分別保存在 [OCR 公開實證](ocr-public-reproduction-verification.json)與[其他入口實證](related-public-update-reproduction-verification.json)。

七個受影響套件的一般驗收合計 1,316 pass／14 skip，原 1,190 個結果保持，見[一般核對](shared-all-packages-unit-root-verification.json)。蒸餾本機 HTTP 權限失敗與原紀錄保持，該完整套件已用相同版本補跑通過。完整競態也已結束，合併相同固定版本的蒸餾完整套件補跑後，各 1,316 pass／14 skip，原有 1,190 個結果及新增 140 pass 均核對通過，見[隔離整體驗收](isolated-comprehensive-verification.json)。正式採用後來源為 895 份，完整工程正在執行，見[採用紀錄](../categorical-gradient-adoption-20261018/adoption.json)。本次工作目錄仍供正式驗收使用，尚未清除。

[配置量測](helper-allocation-root-verification.json)確認 259 類的單次 helper 額外配置由 2,304 B／1 次降為 0 B／0 次，不代表整體訓練加速。另已確認 OCR 與蒸餾純標籤的回報誤差可能被算成零，兩項獨立正值及一般／競態實證見[後續缺陷](../../TSK-01/reported-loss-audit-20261018/README.md)，本票沒有修正它們。

正式 895 份來源已通過九項完整工程，一般 8,917 pass／23 skip、競態 8,916 pass／24 skip，原 8,800 個結果保持，各新增 140 pass，見[完整核對](../categorical-gradient-adoption-20261018/full-engineering-root-verification.json)。12 次舊模型程序的完整輸出保持，31 項治理通過。本票目前 4／5，必要輸入已封存，1,880,270,157 bytes 自有暫存已[清除](../categorical-gradient-adoption-20261018/cleanup.json)，剩下提交交付。
