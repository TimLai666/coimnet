# 機制與訓練一致性稽核

已確認並修復七類缺陷。基準為 `dd75a62d152e0066ff04c3c917adbd66e4ebb0d8`，不改正常數值公式、公開簽名、保存版本或相依套件。

| Ticket／嚴重度 | 觸發與影響 | 修正位置 | 直接失敗證據 |
| --- | --- | --- | --- |
| 78／P2 CONFIRMED | 有限 Tau、release、boost 合成無限清除速率，靜默產生錯誤零濃度／梯度 | modulation/chemistry.go:232 在運算前拒絕，Backward 經同一入口拒絕 | red-chemistry-source.log.gz |
| 79／P2 CONFIRMED | Channel=MaxInt 的加一溢位讓 Release panic，Build 錯誤接受 | modulation/config.go:44、modulation/sources.go:107,175 在寬度配置前拒絕 | red-chemistry-source.log.gz |
| 80／P1 CONFIRMED | 更換受體宣告移除 expression 引用，推論靜默中性且快照無法恢復 | learning/chemistry.go:306 提交前驗證既有 expression | red-expression.log.gz |
| 81／P1 CONFIRMED | 化學開啟時用普通預測算回饋權重，score 0 也產生錯誤梯度 | learning/individual_chemical_training.go:13、learning/online.go:494 以相同私有化學起點預測及訓練 | red-next-chemical.log.gz |
| 82／P1 CONFIRMED | 共享參數的 optimizer／partial Sum 不同仍可恢復，下次更新違反共享約束 | learning/sharing.go:231、learning/trainer.go:218 提交前驗證群組訓練狀態 | red-next-partial.log.gz |
| 83／P2 CONFIRMED | 累積梯度尚未套用就回報模型已更新，誤導訓練判讀 | learning/online.go:410 只計 StepResult.Applied | red-next-partial.log.gz |
| 84／P1 CONFIRMED | UpdateEvery=2 的到期訓練失敗，重試被當成 skip 並消耗回饋 | learning/online.go:392,413 只在成功後提交有界間隔位置 | red-next-retry-fixed.log.gz |

每類都有修正前的實際失敗與修正後的回歸。共七份測試、22 個新增頂層測試，正常控制組保持通過。84 的第一份草稿有未使用 import，該編譯失敗只作工具紀錄，不作 bug 證據。完整一般與最終 race 各58套件通過。23個race skip中包含既有大型案例，沒有放寬回歸。

22 筆合法公開 API 輸出在基準、前三項修正與最終七項修正間逐位相同。CLI 新程序 80+40 與連續120步保存逐位相同，最後建置與先前合法 CLI 保存也相同。這些結果沒有測量新圖片辨識率。此次 OnlineLearner 修正不能推論改善未呼叫該入口的既有 OCR 範例。

## 設計一致與研究限制

- 受體 Location.Region 決定濃度來源，Compartment 記錄研究部位，沒有多區室膜電位。這符合 docs/model-and-mechanisms.md:53 的明示設計，無法據此宣稱生理完整。
- EnableChemistry 更換宣告並歸零濃度，只驗證受體位置引用仍存在，沒有承諾新舊受體的生物身分相同。
- 尚未 Validate 的 Channels() 沒有錯誤回傳。巨大但可表示的通道數配置沒有新增上限，未執行可能耗盡記憶體的配置反例。
- Controller 大值 softplus 保留既有近似。沒有新的真實模型品質失敗證據支持改掉該算法。

## 待調查，不列為已確認缺陷

- learning/individual_chemical_training.go:36：fresh TrainEpisode 將沒有歷史活動映射為零，fresh Advance 的 NeuralActivity 依既有契約拒絕 nil。須先查明兩種生命週期是否應採相同初始語意。
- learning/online.go:397–406：episodePredict 與 TrainEpisode 各自取得鎖，兩者之間若外部 Advance 改變個體狀態，可能使用不同起點。須確認跨入口並行契約並做可控制的插入重現。
- learning/online.go:417：混合梯度與局部可塑性仍有既有部分成功行為。梯度成功後塑性失敗的重試是否應重做梯度，需要先核對契約與公開重現。

減法審查：共用既有化學起點建構及共享索引，直接修正驗證與提交時機，沒有建立另一套訓練流程。
