# 化學配置容量修正的 Root 審查

比較基準為 `57cc92d2864bd19f2424a4daa57edad60ad2763d`。候選包含七個正式 modulation 檔案與兩個先凍結的回歸測試。完整指紋見 `source-final.json` 與 `frozen-tests.json`。

## 已確認並修正的四類問題

| 類別 | 原本可見行為 | 修正及直接驗證 |
|---|---|---|
| 來源釋放向量 | Channel+1 可表示，但 float64 位元組大小不可表示，Build 接受後 Release panic | SourceSpec.Build 與每個具有純量寬度宣告的 standalone 入口先回錯 |
| 控制器活動視窗 | Nodes 或視窗總配置不可表示，Validate 接受，運算 panic 或錯誤輸出不符合契約 | Controller／MemoryController 在候選複製與 observe 前檢查整個網格；暖機後拒絕、還原宣告後的輸出與梯度相同 |
| 化學濃度區域 | Regions 宣告被接受，NewState 或 EnableChemistry panic | NewChemistry 在配置前檢查濃度網格；EnableChemistry 失敗後完整快照、宣告與最後報告保持 |
| 效果節點向量 | 即使 effects 為空，ApplyEffects 也會為不可表示的 nodes 配置而 panic | 節點向量在任何節點陣列配置前拒絕，三個輸出均為 nil |

Root 以修正前相同的公開診斷重現 13 個 panic，另有一個化學設定接受錯誤。15 個非法診斷案例中，另一個原本就會因 Tau 形狀錯誤被拒絕。修正後 15 個案例全部回錯，沒有 panic 或逾時。六個正常來源控制逐位相同。正式 RED 有 31 個失敗測試事件，候選 GREEN 有 34 個通過事件、五個頂層測試。

## 算術、狀態與相容性

`checkFloat64Vector` 先比較 count 與 MaxInt/8。`checkFloat64Grid` 先比較 columns 與 (MaxInt-header)/8，才計算 rowBytes，再比較 rows 與 MaxInt/rowBytes。header 為三個 machine word。正值入口已確認維度，檢查本身不配置陣列，也不需要乘法溢位後再判斷。

Controller.Release 與 MemoryController.Offset 都先 Validate，再處理私有活動候選。新增檢查早於 observe，錯誤不替換視窗及最近成功的梯度快取。化學建構先回錯，持續個體的候選宣告不提交。BackwardEffects 經由 ApplyEffects 的同一驗證入口拒絕。

來源、清除、受體占有、效應及梯度的正常算式沒有變更。32 組合法個體的輸出與完整快照，八組活動生命週期對照，以及 CLI 三個新程序的 80+40 與 120 步保存結果均逐位相同。這些是指定案例的相容性證據，沒有量測真實任務品質。

## 獨立審查的採用判斷

獨立對抗式審查未找到候選的已確認缺陷。其 Replay 容量疑點不列為待修 bug：Replay 寬度來自已經存在的合法 float64 slice，並非使用者提供的純量配置宣告。一般 Go 程式無法先持有位元組大小在 int 中不可表示的該 slice。可表示但耗盡實體記憶體的複製，屬本票明示排除的記憶體容量問題。

Root 移除了候選中 Replay 的這項檢查、已由網格檢查涵蓋的單列檢查，以及 checkedTransport 的重複檢查。NewChemistry 在有 Transport 時的早期完整網格檢查保留。正式測試 455 筆凍結指紋相同。

## 正常機制設計稽核的 Root 判定

[獨立正常機制稽核](mechanism-design-review.md)逐一核對來源、濃度與區域運輸、受體位置／Hill 曲線、效果、步驟順序與學習梯度。Root 核對原文及目前程式，沒有採用報告中的三個疑點作為已確認缺陷：

- N1：`learning/memory.go:102-118` 已明示平均同一受體產生的所有記錄。零值的未知／不反應記錄留在分母符合現行工程定義。改成只平均可計算記錄會改變模型算式，沒有證據支持本票這樣改。生物解釋仍須另行校準。
- N2：混合 continuous／LIF 的完整化學反向組合是有效的驗證缺口，已記錄於 `AGENTS.md` 的 Follow-ups。`dynamics/mixed.go:272-285` 明示硬脈衝採替代梯度，不能用硬 0/1 輸出的普通中央差分判斷它錯誤。後續使用獨立同規則反向參考或 test-only smooth 模式。32 組個體相容性中的 mixed 是 gradient＋plasticity 學習模式，並非這項核心組合驗證。
- N3：RewardMapper 與接受 boost 的 Kinetics.Step 是可獨立使用的明示 API。一般 ChemicalConfig 沒有宣告自動獎懲清除接線，`learning/chemistry.go:215-223` 的 nil boost 沒有違反既有一般 episode 契約。不從一個可選 API 的存在推論它必須自動接入，也不在本票新增設定或改變濃度公式。

本票的全套測試能驗證既有小型動態測試的執行結果，不能補上 N2 缺少的獨立完整組合參考。原始 worker 報告保持原文，其裁決建議以本節的 Root 判定為準。

## 限制

驗證主機為 darwin/arm64。32-bit 算術按 strconv.IntSize 靜態核對，沒有 32-bit 實機執行。檢查不保證一般大型合法設定有足夠實體記憶體。初始神經來源的無活動拒絕是既定設計，八組控制排除該疑點。本票沒有全腦訓練、外部生物定量驗證、真實辨識率提升或整個框架零 bug 的結論。
