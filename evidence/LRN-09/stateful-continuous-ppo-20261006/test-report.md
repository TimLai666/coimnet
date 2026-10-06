# Test Report：保存狀態的連續核心 PPO

最終來源指紋：`c433afec16e04bd78290127850a5336a03b5e247e3b6f41b8de9fdbcc0ce7949`。

## Summary

372 個相關頂層測試通過，包含 22 個專項，兩者不重複加總。完整一般與 race 各 57 個有測試的套件通過。build、vet、gofmt、依賴、原件指紋及 CLI 恢復比較也通過。

## Changes tested

- 保存電位與歷史的前向結果等於 Advance。參數與輸入梯度符合獨立有限差分，包含延遲跨段及截斷。
- Insyra 編碼器與讀出、固定符號、共享權重、遮罩及 AdamW 符合獨立參考。
- PPO 每輪計分與梯度使用同一初始狀態，原個體與現行記憶保持。
- 新程序恢復後，參數、最佳化器、未完成累積及神經歷史的完整 checkpoint 位元組相同。
- 非法狀態、不支援設定、取消、溢位與候選更新失敗都回錯誤，不發布部分更新。原零狀態及重算控制通過。

## Bugs found & fixed

1. [P1] 新參數原本從零狀態檢查，合法的大電位會讓無法接續執行的更新被提交。`learning/trainer.go:537` 改為從同一保存狀態驗證。`TestStepFromStateRejectsCandidateThatOnlyRunsFromZero` 覆蓋立即更新、未完成累積、完整訓練器保持與獨立零狀態控制。

測試 helper 曾複製已清空的歷史列。Root 先重現，再只修正一行複製來源。證據腳本曾讓迴圈變數覆蓋 CLI 結果選項，Root 只修正該變數並重跑比較。原始失敗紀錄均保留。

## Issues found (not yet fixed)

本票沒有尚未解決的確認缺陷。非零 LIF／mixed／Recompute、可塑性／化學 PPO 梯度與 vector／GPU 個體保存，不在本票支援範圍。沒有新增其他平台、全圖、導航學習或生物機制驗收。

## Regression tests added

十份最終測試檔及指紋列在 `tests-frozen-v3.json`。原八檔與修正後九檔清單保留，`root-test-review.md` 說明唯一一行 helper 修正。

## Recommendation

SHIP 這份 CPU 純量連續核心的保存狀態訓練能力。完整命令、環境、來源指紋與日誌見 `verification.json`。
