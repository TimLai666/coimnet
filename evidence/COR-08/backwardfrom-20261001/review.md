# Diff Inspector

Scope: CLEAN

Reviewed: 相對 `58c97b6a9f96bba0d07cf8acc0a9cdb8e00e3f9a` 的兩處梯度橋接、人工數值參考測試、編譯模式選取與相關文件。

Adversarial review: RUN — Trigger: 梯度橋接影響核心訓練流程。`gpt-5.3-codex-spark` 的工具呼叫回覆 Unknown model，改用 `gpt-5.6-luna`／`max` 唯讀審查。Root 親自檢查 diff、來源與數值結果，另重跑審查涉及的測試。

## Findings

No confirmed findings.

- `learning/network.go:351` 驗證每列上游梯度，最後一步與每步模式的種子沿用 prediction 的形狀。輸出由同一個 tape 的 MatMul 產生，符合 BackwardFrom 的前置條件。
- `learning/network.go:440` 依原 InputNodes 與向量分量順序取得核心輸入梯度，編碼器種子沿用 encoded 的形狀。
- 核心反向、float32 轉換、取消檢查、最佳化器與保存格式沒有變更。
- 文件、ticket 31 與需求證據使用同一契約。需求通過數維持 89／91。

# Test Report — BackwardFrom — 2026-10-02

## Summary

完整一般與 race 檢查各 56 個套件通過，最終命令 exit code 均為 0。Race 的 21 個套件使用符合 Go 快取條件的既有成功結果，另外 35 個套件實際執行。Root 另獨立驗證舊版與新版的一般／race 梯度參考，並保留逐位比對。

## Changes tested

- 連續與 LIF、最後一步與每步、有限時間回推、選擇性輸入與向量狀態：每個編譯模式的 9 組完整梯度與該模式的原版參考逐位相同。
- 既有有限差分、重算與更新測試通過。
- 隔離副本故意清零讀出種子，9 組回歸測試全部失敗。這是測試敏感度驗證，不是既有程式缺陷的重現。
- 原版 80 步後由新版接續 40 步、新版連跑 120 步、停用 Insyra WebGPU 矩陣加速的接續，均與原版連跑 120 步快照逐位元組相同。
- Apple M3 Metal 稀疏運算、訓練器及新程序恢復專項通過。Linux／Windows amd64 交叉編譯通過，未在兩平台實際執行。

## Bugs found and fixed

新增測試最初把一般編譯的參考值用於 race 編譯，四組 LIF 出現微小差值。原版與新版都重現同一錯誤，因此由原版各自產生兩種編譯模式的參考值，維持同模式的逐位比對。沒有更動數值核心。

磁碟不足與沙箱本機連線限制的失敗紀錄另保留在 verification.json。釋放本輪暫存產物後，於允許本機測試連線的環境完成全套驗證。

## Regression tests added

`learning/upstream_baseline_test.go` 的 `TestUpstreamBaseline`，以 `upstream_norace_test.go` 與 `upstream_race_test.go` 選取原版對應的參考值。

## Recommendation

SHIP。驗收僅涵蓋軟體相容性及記錄的人工案例，不增加全圖效能、任務學習或生物機制證據。完整命令、來源指紋、日誌與限制見 verification.json。
