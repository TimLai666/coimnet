# 媒體報告設定所有權：Root 審查

已確認 P2 報告紀錄缺陷。原始設定在 RunMedia 返回後合法重用，會改到舊報告的 Seeds、Holdout，卻不改 ConfigHash 及 Runs。Root 已實際讀取三個 producer、三個 CLI consumer、GeneratorConfig 與 VideoConfig 的欄位，確認只有這兩個輸入切片需要複製。共同依據為 ENG.md:18、主規格 16.2／16.3 與既有 ticket49 的報告隔離契約。

Root 的合法預設尺寸公開探測 session76735 實際退出 0，normal_run_succeeded 與後續 caller Validate 都成功。seed 由 1 變成 2，report.Runs[0].Seed 保留 1，保留條件由 blue diagonal 變為 red square，ConfigHash 不再符合 report.Config。首次 red horizontal 不合法，該次探測失敗完整保留，不算產品驗證。

Luna 代理只寫入指定的 ownership-test.go.txt，沒有跑 Go 或改實作。Root 審查全部 231 行後，移除純欄位的存取 closure，凍結成 120 行 regression.go.txt。四個子案例使用真正的正常訓練／評估，沒有替換訓練器；在模型已正常返回後才修改 caller 設定，另檢查反向修改報告不影響 caller。Root 的原版一般／競態各 5 fail，每個子案例均在報告變動、指紋不符與反向共用三個行為斷言失敗。不是編譯錯誤、panic 或資料競態。

候選只在三個 producer 的既有驗證後、JSON 與雜湊前複製兩個切片，共六行，不增加 API、helper、設定欄位、版本或模型格式。Root 已執行一般／競態各 5 pass，四份修改前完整報告的 SHA-256 各自在原版與候選一致。沒有比較不同建置當作數值相容性的必要條件，也沒有調整訓練目標、生成器或解碼器。證據以實際 receipt、JSON log 與 runner terminal 為準。

另一個唯讀代理核對音訊多區塊索引，Root 也讀取 samples.go 與 run.go，沒有確認對齊缺陷。現有文件已說明後續區塊是時間一致性診斷，不需要另向使用者要求改為多區塊目標訓練。來源 hash 的一處代理抄錄錯誤已由 Root 重新計算，採用目前 formal-source.json 的實際值。

候選驗證 session74636 實際 terminal 0（3c0a2e）。必要的八份工作輸入各自與 evidence 原文比對後保留，額外 xcrun_db 僅為執行快取。清理 session15756 實際 terminal 0（ca7ec5），移除 9 檔 68,295 bytes。18 份模型、6 份必要資料、32 份其他工作與正式 857 份來源保持，使用中的 ticket138 工作及快取保留。

本票 2／5，尚無正式採用或完整工程驗收。必須先完成 ticket138 原 session61304、最後治理、清理與交付，再依 tests-frozen.json 及 candidate-sources.json 採用。下一輪不能把 candidate_verification 的 all_passed 當成正式修正完成。

減法審查：保留各入口兩行複製，移除測試的多餘 accessor closure；不建立報告不可變介面，不重做模型，不擴張成其他套件的設定重構。
