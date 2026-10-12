# 59 — 使用者可以保存 OCR、語音與文字模型，重新啟動後推論或接續訓練

Epic：五類任務的完整流程

User Story：使用者可以保留訓練成果與任務設定，從資料夾模型重新啟動，不必重新訓練。

Blocked by：29 的三類辨識／文字流程、55 的網路磁碟 bundle 保存已完成。

Status：completed，功能、完整驗證、暫存清理與主要提交交付完成。

原始依據：[主規格 13.1、15.1–15.3](../handoff/CoImNet_Implementation_Plan.zh-TW.md)、[ticket 29](29-real-tasks-ocr-asr-text-and-media-generation.md)。本票補齊保存恢復，不能單獨將 TSK-11 標為通過，也不修改既有負面品質結果。

## 使用流程

任務訓練器 → 任務設定與完整最佳化器 → 排他建立的新 bundle 目錄 → 校驗與完成清單 → 重新載入任務 → 相同推論與下一次更新。

詞表、影像列高、音訊前處理及讀出設定隨模型保存。既有 Snapshot 方法與未指定保存旗標的 CLI 行為保持。

## 驗收

- [x] OCR、整檔語音與位元組文字模型均可保存及載入。輸出、詞表與完整最佳化器狀態一致，下一次更新與不中斷參考相同。
- [x] 新程序從保存物獨立推論；真實資料範例可明確選擇保存或讀取模型，讀取模式不重新訓練，重建資料與分割。
- [x] nil／取消、空路徑、既有目標、錯誤或缺失設定、校驗／版本／拓撲不相容都明確失敗。既有成果與呼叫端狀態保持。
- [x] 沿用 v2 bundle 的網路磁碟發布與完成清單，實際 SMB 小模型保存恢復通過，不把本機單檔測試代替。
- [x] 格式、build、完整一般與 race、vet、相依性、格式索引與治理檢查通過；證據保存後清除本輪暫存。

## 檢查對象

Save／Load 公開入口直接測試，保存使用既有 checkpoint bundle，沒有額外訓練引擎。測試 agent 與實作 agent 分開；root 審查失敗測試後才填入實作。

取消或寫入失敗由原保存函式回錯；尚未完成的 bundle 不可讀取。未讀到完整設定不補零，也不重新隨機初始化。模型不含原始語料或音訊。

## 驗收證據

2026-10-12：evidence/TSK-11/task-checkpoint-20261012/native-verification.json、smb-verification.json。最終整合命令、日誌與來源指紋見 [91 項驗收](../../evidence/goal-91-20261012/verification.json)。[清理回條](../../evidence/goal-91-20261012/cleanup.json)與[交付回條](../../evidence/goal-91-20261012/delivery.json)已保存，主要提交 `a28ce23` 已推送核對並同步原專案。
