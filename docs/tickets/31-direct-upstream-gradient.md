# 31 — 以 Insyra BackwardFrom 傳遞外部梯度

Epic：核心學習維護

Blocked by：Insyra v0.3.4 相容性驗收

Status：done（2026-10-02，軟體相容性驗收）

## Root 決策 — 2026-10-01

使用 Insyra v0.3.4 的 `BackwardFrom` 接收外部梯度，保留兩個 tape 與 float64 核心。用修改前的完整梯度與快照接續結果驗收，參考值依一般與 race 編譯模式分開保存。

## 交付

`learning.Network.lossGradientReverse` 的讀出與編碼器兩處橋接改用 `Tape.BackwardFrom`。上游梯度沿用輸出張量的形狀，移除為建立純量種子而加入的 `Reshape`、`MatMul` 與 `Backward`。

## 共用契約

- 保留所有上游列的寬度與有限值檢查，最後一步模式只使用最後一列。
- 每步模式的種子為 `[steps, outputs]`。編碼器種子為 `[steps, inputWidth]`，保留 `InputNodes` 與向量狀態的選取順序。
- 核心 float64、Insyra float32 轉換、核心反向、固定符號鏈鎖梯度、重算、取消及錯誤返回方式不變。
- 原版數值參考依一般與 race 編譯分開保存，同一編譯模式、Go 版本、作業系統與架構才要求逐位相同。
- 不改公開 API、最佳化器、保存格式、MSE 損失與 GPU 裝置流程。
- 實作責任限 `learning/network.go`。新增的回歸測試及小型人工參考值放在 `learning/`，由原內積橋接產生並記錄來源提交。

## 驗收

- [x] 修改前建立完整梯度參考，涵蓋連續／LIF、最後一步／每步、選擇性輸入與完整／有限時間回推。
- [x] 在隔離副本切斷讀出梯度，9 組回歸測試全部失敗，再保留原始實作。
- [x] 新橋接的全部梯度與原參考逐位相同，既有有限差分與重算測試通過。
- [x] 修改前快照在修改後新程序接續，與原版連跑快照逐位元組相同。
- [x] 完整 build、test、race、vet、依賴檢查通過，保存命令、環境、輸入指紋與日誌。

## 證據

[完整證據](../../evidence/COR-08/backwardfrom-20261001/verification.json) 記錄來源提交、人工參考值的來源與授權、一般／race 各 9 組逐位比對、刻意清零種子的失敗驗證，以及舊快照的新程序接續。Mac 完整一般與 race 測試各 56 個套件通過；兩個編譯模式分別使用原版產生的參考值。Linux／Windows 僅完成交叉編譯，沒有全圖效能、任務學習或生物機制的新證據。
