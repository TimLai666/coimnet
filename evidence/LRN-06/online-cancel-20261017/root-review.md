# 線上取消與回饋識別核對

結論：這次沒有確認產品缺陷，不修改正式學習流程。

## 取消、部分成功與重試

`learning/online.go:334-356` 與 ticket23 的第 12 項裁決允許部分成功。無 target 或合法間隔略過可以完成該筆回饋，回饋處理成功不等於有模型訓練。真正到期的訓練在取消時回錯，未成功的回饋及間隔位置保留。

Root 實際執行一般及競態，各 24 個 pass 事件，沒有 fail 或 skip。每種執行包含 10 個新增情境、一個新增父測試，以及 13 個既有控制事件。直接核對兩份原始 JSON 日誌的雜湊、測試名稱及實際 terminal 0，不採信代理的測試聲明。

10 個情境交叉比較取得鎖前／後取消，以及無目標、間隔略過、到期更新、先略過再到期、先無目標再到期。取消觀察後的完整 `IndividualSnapshot` 保持，接續重試後與普通孿生的完整模型快照、累計報告、pending、eligible 相同，再更新不會重複處理已完成回饋。到期更新另有確實改變參數的正向控制。

隔離 overlay 只在 Update 取得鎖後加入一次 callback 並定義私人 hook，Root 已比對完整生成內容。callback 只呼叫標準 `context.WithCancel` 的取消函式，不偽造 Context.Err 的結果。

## 證據適用範圍

新 probe 的 policy 只有梯度訓練，沒有 plastic。日誌中的 `full_state_equal=true` 指完整 IndividualSnapshot 以及明列的 queue／interval／log 比較，不宣稱所有 OnlineLearner 私人欄位或外部 RewardGate 狀態皆已比較。`gradientDone` 的零長度檢查只表示這些純梯度情境沒有殘留標記，不當成混合階段的動態證據。混合階段另由這次重跑的既有控制驗證。

日誌的 `canceled=` 表示是否預期回傳 context.Canceled，不表示 context 是否曾被取消。所有 10 個 context 都確實取消，5 個取得鎖後的 hook 也逐例斷言只執行一次。

## 回饋識別

Root 核對 `Receive`（online.go:290-329）拒絕同 action／source 再計入，`AvailableFeedback`（signal/feedback_available.go:5-31）只依時間過濾並保留順序，混合階段的 `[2]string` key 與成功後才刪 pending 的路徑一致。`Act` 固定產生不含 NUL 的 `act-N`，因此未發現公開合法 action 能造成 remainingFeedback 的分隔符碰撞。

唯讀代理提出的「第一筆 plastic 失敗卻 Consumed=1」預期不符合實作與契約，Root 拒絕該句。`applied++` 只在整筆 apply 成功後發生，此時正確值是 Consumed=0，已成功的梯度階段仍可保留。

## 反向審查與清理

同一個 gpt-5.6-luna／max 代理已唯讀審查診斷腳本與原始輸出。Root 採納上述比較範圍說明，沒有把證據命名限制列為產品 bug。

代理另指出惡意或並行程序替換整個暫存目錄與 ownership 記錄的假設。這輪沒有觀察到此種替換，所有工作者都是唯讀，只有 Root 寫入自建暫存。清理使用固定路徑、inode、輸入雜湊、兩次內容清單、無開啟 handle 檢查及抗 symlink 的 rmtree，不能宣稱對任意惡意真實目錄替換具原子防護。實際清理 terminal 0，目錄已移除，856 份來源、18 份模型、6 份資料、32 份其他工作及兩個原有暫存目錄保持。

## 工程與交付

這輪沒有新增正式 Go。856 份來源與 ticket137 完整 build／一般／競態／vet／模組驗收來源逐檔相同，原完整驗收的日誌雜湊也重新核對。這次只新跑明列的隔離診斷，沒有宣稱再次執行整套測試或真實任務訓練。

一般 session40468、競態 session38151、清理 session20146 都已有實際 terminal 0。完整證據見 verification.json、terminal.json 與 cleanup.json。工程追蹤保持 91／91，整體排查目標保持 active。
