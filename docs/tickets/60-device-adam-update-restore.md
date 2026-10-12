# 60 — 研究者可以在 GPU 更新參數並保存、恢復最佳化器

Epic：裝置訓練與量測

User Story：研究者可以把參數、AdamW 動量與更新次數留在 GPU，執行有遮罩的更新，保存後精確接續。

Blocked by：30 的 WebGPU 稀疏前向／反向已完成。

Status：completed，功能、完整驗證、暫存清理與主要提交交付完成。

原始依據：[主規格 17.4、18.2 與 OPS-05](../handoff/CoImNet_Implementation_Plan.zh-TW.md)、[ticket 30](30-platforms-full-graph-training-governance-and-flywire.md)。本票驗證裝置更新，稀疏訓練器整合仍需接續驗收。主規格 §17.4 的完整神經狀態常駐是可行時的效能方向，GPU 全圖與其他平台保留未驗證，不能追加為本票的無條件門檻。

## 使用流程

啟動前驗證參數與總緩衝預算 → 硬體裝置與編譯 → 一次上傳初始參數／動量 → 梯度／遮罩 → 裝置候選更新 → 裝置有限值旗標 → 所有區塊同時提交。Snapshot 明確讀回完整狀態，Restore 驗證後重新上傳。

獨立 float32 格式保留既有 CPU float64 AdamW 與 GPUTrainer 行為。內部分塊覆蓋所有宣告參數，不縮減圖或變更學習遮罩。梯度累積、裁切及排程由訓練器處理。

## 驗收

- [x] 真正硬體 AdamW 與獨立 float32 參考比對通過，包括不同參數更新次數、遮罩、權重衰減與工作群組邊界。
- [x] 不足預算、形狀、nil／取消、非有限值、更新次數溢位及錯誤快照均明確拒絕。任一區塊失敗，所有參數、動量及計數不變。
- [x] 裝置參數與動量只在建立／恢復時上傳，Step 僅上傳梯度、遮罩與設定，讀回錯誤旗標；實際傳輸量與配置量可核對。
- [x] 保存恢復後的後續更新及輸出一致，量測初始化、編譯、一次傳輸、暖機及穩態。沒有實機的 profile 保留未驗證。
- [x] 格式、build、完整一般與 race、vet、相依性、格式索引與治理檢查通過；證據保存後清除本輪暫存。

## 檢查對象

DeviceAdamW 公開入口作為驗證入口。GPU 數值與 CPU 參考分開，軟體渲染器不算通過。跨平台與完整圖的證據不得以本票小模型替代。

## 驗收證據

2026-10-12：evidence/OPS-05/device-adam-20261012/verification.json。最終整合命令、日誌與來源指紋見 [91 項驗收](../../evidence/goal-91-20261012/verification.json)。[清理回條](../../evidence/goal-91-20261012/cleanup.json)與[交付回條](../../evidence/goal-91-20261012/delivery.json)已保存，主要提交 `a28ce23` 已推送核對並同步原專案。
