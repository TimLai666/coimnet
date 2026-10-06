# Diff Inspector

Scope: CLEAN。Root 讀過完整實作、測試差異與受影響文件。修改只涉及
`internal/cli` 的能力資料、新增 JSON details 與 help，沒有剩餘確認缺陷。

## 契約與來源

- GPU 六項字串表示有受限實作。條件已對照 `dynamics/gpu_adapter.go` 與
  `learning/gpu_trainer.go`：純量、零延遲連續核心、獨立 episode，神經狀態、
  活化及 AdamW 在 CPU，沒有重算、裝置常駐狀態或裝置更新，完整圖未驗證。
- 每份報告擁有獨立 details。CPU、Doctor 簽名、runtime／作業系統探測、
  Insyra 數值及錯誤處理保持。GPU 字串新增受限支援值，v1 JSON 只新增
  `core.gpu.details`，原欄位型別保持。
- 已追蹤 Doctor 的 `Run`、命令入口及驗證腳本。現有腳本沒有要求舊 GPU
  狀態值。歷史報告逐檔保留，不充當目前產生的探測結果。
- 作業系統探測有時間限制。本機 M3 或可選工具缺失，都不能將核心
  `execution=not_probed` 改成執行通過。Insyra 矩陣能力另外報告。
- Root 退回未執行入口的硬體測試並還原無關重新命名。獨立 CLI 檢查另重現
  help 漏列 WebGPU／continuous／recompute 的問題，已補完並重跑，
  凍結的 Go 測試沒有改動。

## 驗證與適用範圍

Root 的實作前測試有四項新行為失敗，修正後 13 個頂層專項通過。真實 binary
流程涵蓋正常／空 PATH 探測、help／總覽與非法參數，產生的 binary 在計算
指紋後已清理。來源與原件檢查通過，最終全套結果由 `verification.json` 記錄。

這次是單一、低影響的診斷元件。沒有模型更新、資料遷移、驗證身分、對外發布或
新增 GPU 資源操作，依 diff-inspector 小型修改規則採 Root 輕量審查。
唯讀 agent 在實作前獨立盤點支援條件。此修正不提供完整圖 GPU、其他平台、
方向學習或生物機制的新證據。
