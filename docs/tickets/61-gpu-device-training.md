# 61 — 研究者可以在 GPU 更新訓練模型並精確接續

Epic：裝置訓練與量測

User Story：研究者可以透過既有 Trainer 執行稀疏前向、反向與裝置 AdamW 更新，保存完整訓練狀態後由新程序接續。

Blocked by：60 的裝置更新與恢復實作。

Status：功能、完整驗證與暫存清理通過，提交交付待完成。

原始依據：主規格 §17.4／18.2 與 OPS-05。常駐神經狀態是「可行時」的效能方向，真實全圖 GPU 與其他平台保留未驗證，不另加為本票必要門檻。

## 使用流程與共同決策

明示 opt-in 建立或恢復 → 設定、float32、容量檢查 → 稀疏前向／反向 → 既有累積、裁切、排程 → GPU 候選 AdamW → 完整候選讀回與前向驗證 → 所有區塊同時提交 → 主機鏡像快照 → bundle → 新程序繼續。

編碼器與讀出保留 Insyra 路徑；參數、動量及計數留在裝置，成功更新讀回鏡像以維持 Snapshot 契約及下次稀疏前向。神經狀態與活化仍在主機，不宣稱全模型常駐或加速。既有 CPU 與 NewGPUTrainer 行為保持。

候選拒絕、取消、非有限參數、不可用 tau 或讀回失敗都不得變更裝置 bank、主機參數、最佳化器、更新次數或累積窗。提交前最後一次取消檢查後，不再執行可失敗操作。禁止先提交再驗證，或用重開裝置當正常 rollback。

## 驗收

- [x] CPU、舊 GPUTrainer 與裝置更新模式的前向、反向、四次更新及更新後推論對照。AdamW 對獨立 float32 參考；遮罩、衰減、累積、裁切與零學習率排程都有實機證據。
- [x] 候選 callback 拒絕、取消與跨區塊非有限結果保留完整狀態，下一次合法更新等於未執行失敗更新的參考。Close 冪等、關閉後不訓練、快照鏡像可讀。
- [x] 啟動前拒絕不支援的 LIF／mixed／vector／delay／recompute／persistent-state／projection／sharing profile，不回退 CPU。建構明示 float32 量化，恢復拒絕不可精確表示的狀態、kernel options 與 uint32 計數溢位。
- [x] 部分累積窗經完整 training bundle 由新程序恢復，後續參數、動量、次數、累積窗與輸出逐位一致。真正子程序不得略過硬體。
- [x] 實測裝置、初始化、編譯、傳輸、暖機、穩態與完整候選讀回量，所有宣告參數均參與。主機與裝置成本分開，執行前容量可核對。
- [ ] 完整格式、build、一般、race、vet、相依性、索引與治理檢查通過，保存來源與日誌，清除不再必要的本票暫存後交付。

## 邊界與檢查責任

nil／取消 context、nil callback、錯形狀、負或非有限 rate 返回具識別錯誤。零 rate 仍更新未凍結參數的動量與個別計數。callback 取得完整獨立快照，不可遞迴呼叫同一 optimizer 的鎖定方法。frozen gradient 在 float32 轉換前歸零，無關的大值不得誤拒絕。構造部分成功後失敗要釋放已開裝置。

公共 Trainer、StepChecked 及既有 bundle 是測試入口。Root 寫骨架與預期、測試 worker 先寫 RED，另一 worker 實作且不得改測試，Root 審查與實機驗證。

## 驗收證據

2026-10-12：evidence/OPS-05/device-training-20261012/verification.json。最終整合命令、日誌與來源指紋見 [91 項驗收](../../evidence/goal-91-20261012/verification.json)。[清理回條](../../evidence/goal-91-20261012/cleanup.json)已保存，提交交付完成後才勾選最後一項。
