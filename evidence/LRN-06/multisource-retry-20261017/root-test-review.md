# 同一行動多來源階段回歸審查

這次補的是覆蓋缺口，尚未確認正式產品缺陷。只新增 `learning/online_multisource_state_test.go`，原有 856 份 Go／模組來源逐檔保持。

既有 `online_mixed_retry_extra_test.go:189-249` 在第一個來源已完整成功後才讓第二個來源失敗，只有一筆未完成階段。`online_mixed_retry_lifecycle_test.go:153-211` 讓時間倒退、保留兩筆未完成階段，但兩筆來自不同 Act。新案例用同一 Act 的 late／early 來源各自失敗，讓兩個來源標記同時存在，再獨立接續並保留另外兩個延後來源。

Root 定義並親自撰寫公開 API 參考。零目標、score=1 令 weightedTarget 的有限 p+(0-p) 精確為 0，公開 TrainEpisode 可以獨立產生同一梯度階段。fixture 沒有 chemistry 或 GateReceptor，individual.go:818-829 的受體分支不會進入，公開 AdvanceGated 的顯式全 1 閘門對應局部階段。參考按實際成功階段交錯，不假設梯度與局部學習可以交換順序。

四個 UpdateEvery 1／2 與 AccumulateSteps 1／2 組合都在每次 Update 後比較完整 IndividualSnapshot，核對部分報告、時間倒退、重複來源拒絕、延後回饋仍存在、歷史答案不改寫，最後也有參數確實更新的正向控制。新測試的 SHA-256 為 `f37bfc5667071b3fbf39f6f3251f98ed5f620b1ed8dfa14a4807af6ae3697f85`。

Root 的一般 session3915 及競態 session19773 均實際 terminal 0，各 5 pass（一個父測試及四個子案例）。兩種刻意錯誤分別將階段標記或 pending 刪除身份縮成只有 ActionID，各自的一般／競態都實際得到 5 個預期失敗事件，沒有編譯錯誤或 data race；控制 runner session37132 terminal 0。失敗發生於報告或完整參考狀態斷言，沒有修改正式原始碼。

gpt-5.6-luna／max 代理只做唯讀覆蓋與反向審查，不執行 Go。Root 已親自核對其依據、報告預期與來源指紋。OpenCode 這輪在 models 探測即因日誌開啟失敗，沒有宣稱使用它完成分工，紀錄見 delegation.json。

完整工程原 session61304 已實際 terminal 0（dd0fbe），九個步驟均通過。一般 8,633 pass／23 skip、競態 8,632 pass／24 skip，各 58 套件通過，原有各 8,651 個終端事件保持，只新增各 5 pass。Root 已重新解析壓縮／原始雜湊與前版逐筆事件。清理回條的 856 舊常數在任何清理之前已補正為 857，精確派生及 AST 檢查見 cleanup-recipe-review.json。最後治理、清理與交付另記，整體排查目標保持 active。
