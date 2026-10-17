# OCR 與蒸餾分類回報誤差重現

現行 895 份正式來源的單字元 OCR，在非 blank 分數 `[0,-40,-40]` 且正確類別為最大分數時，回報誤差是負零。獨立 Decimal300 參考約為 `8.496708510583178e-18`，Root 再以 Decimal1000 重算五組預期位元相同。

[Go 一般／競態實證](reproduction-verification.json)各有三個子案例及父測試失敗、兩個正常控制通過。[Root 核對](root-review.json)確認失敗只有可表示正值被算成零，沒有編譯錯誤、blank 排除錯誤或競態警告。正式程式尚未修正，本項是數值回報缺陷，沒有新增真實任務品質結論。

命令為 `python3 evidence/TSK-01/reported-loss-audit-20261018/run.py.txt --run`，其回條保存當次固定來源、環境與隔離映射。該工作目錄已[清除](cleanup.json)，重現前須重新準備隔離目錄與指紋。必要測試、參考、映射及日誌已保留，沒有刪除仍由 ticket147 使用的快取。

同型態盤點後，公開 `DistributionDistiller.Loss` 的純標籤分支也已在一般／競態各重現三個子案例與父測試失敗、兩個正常控制通過，見[公開蒸餾實證](distill-reproduction-verification.json)及[Root 核對](distill-root-review.json)。三個合法輸入的回報誤差是正零，獨立參考同為約 `8.4967e-18`，目標導數仍為負且輸入及回報設定控制保持。此測試使用 Mix0，沒有驗證教師 KL 或實際任務品質。該次 2 份自有暫存也已[清除](distill-cleanup.json)，必要輸入及日誌保留。
