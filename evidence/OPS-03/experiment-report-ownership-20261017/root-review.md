# Root 實驗報告所有權審查

本輪先以唯讀來源找出四個 report producer，再以公開正常執行重現。RunMultiTask 的 Schedule.Tasks、multimodaleval.Run 的 Holdout／Seeds、studenteval.Run 的 Seeds 共用 caller 的切片，舊 ConfigHash 與實際結果不會隨 caller 後續修改更新。RunAdaptiveEvaluation 固定／適應兩模式的 FeedbackAvailable 也共用 caller 宣告。後者沒有 ConfigHash，仍違反完成報告獨立保存已用宣告的既有 export 所有權契約，不能因缺 hash 欄位就排除。

Root 已親自讀取這四個 producer、輸入型別與合法設定。代理的其他 inventory 只作靜態篩選線索，沒有宣稱整個 experiment 已無其他 bug。OCR／Textgen 與 ASR caller 的獨立範圍另由 related-task-ownership-review.json 保存於 ticket139 證據。

代理只寫指定的測試原文。Root 保留 worker-original.go.txt，移除重複斷言及多餘 JSON 複製，補上正常執行的設定／hash 前置確認，修正失敗訊息與空結果時的測試索引，凍結 regression.go.txt。使用真正正常的共享核心、多模態、student-only 與兩種評估模式，沒有取消或故意失敗替代；student teacher=nil 且教師呼叫為零。Adaptive 保留 score 權限，只改未用的 metadata 類別，caller 的新宣告仍合法。反向修改 returned report 也有檢查。

Root 的原版一般／競態各 7 fail，32 則失敗訊息都來自 caller 修改後的報告變動／hash 不符或反向共用，沒有編譯錯誤、panic 或資料競態。候選各 7 pass，五份完整初始報告的 SHA-256 在各建置內與原版相同。實際 runner37859 terminal0，非代理宣稱。候選只加五行複製，Adaptive 使用既有 slices import 的 Clone 保留 nil／空集合形式，不改學習或評分。

65,120 bytes 探測暫存已清除，必要七份工作輸入與 evidence 原文逐檔一致後保留，父快取工作當時保留供最後治理。來源仍為 ticket138 的857份，尚未採用任何正式修正。與 ticket139 一起採用後，需重新凍結最終來源並完成整套工程驗收。進度2／5，整體目標 active。
