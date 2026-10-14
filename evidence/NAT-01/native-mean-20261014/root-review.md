# 原生平均與集合量測修正審查

範圍：CLEAN。正式程式只修改 `simulate/runner.go`，永久回歸新增 `simulate/mean_range_test.go`。基準為 abcd2f2bf73d8fba679cc8de03643a5757c1d41a。模型格式、核心動態與學習算式保持。

## 已確認並修正的缺陷

- [P2][CONFIRMED] `simulate/runner.go:824`：多個有限 softplus 輸出先加總成無限值，有限平均因而被錯誤拒絕。原公開案例三個輸出各為 1.1363587890114286e308。現在只在中間加總溢位時縮放並補償加總，平均維持在輸出上下界。正常有限路徑沿用原順序。上界案例的獨立高精度參考與結果都是 MaxFloat64。
- [P2][CONFIRMED] `simulate/runner.go:753`：單節點探針成功時，具名集合的原始總和仍可溢位，舊 Run 成功提交且 Measurements 無法 JSON 編碼。現在先建立並驗證候選量測，再一起提交神經、快速狀態及最近成功量測。原始 OutputSum 與 `simulate/metrics.go:347` 的 Numerator 語意保持。

## Root 實際核對

Root 完整讀取正式 diff、新測試、相關 producer／consumer 與 worker 回報，並親自執行公開回歸與相容性驗證。

- 原來源永久回歸有 8 個失敗事件，修正後 13 個事件通過。初版兩個 fixture 違反公開快照及遞增節點契約，依來源證據修正後重新凍結。沒有改變期待結果，原件保留於 `fixture-adjudication.json` 與 v1 測試。
- 正常 16 組涵蓋 continuous tanh／softplus、LIF 四種探針、量測開關。每組完整 Go JSON 字串逐位元組相同，包含兩次執行的 report、神經狀態與量測，沒有排除欄位。
- 實際新程序讀入舊 JSON 狀態，再 RestoreState 與 Run，16 組接續均與原來源第二次結果相同。這是序列化恢復，不是獨立 seeded 重跑。
- LIF 公開補驗在原來源有 3 個失敗事件，修正後一般與 race 各 3 個事件通過。kappa 捨入為 1 的既有合法設定容許有限大突觸跡，MaxFloat64 平均成功，真正 raw sum 溢位時回錯且完整舊狀態／量測保持。此案例是結構合法的恢復狀態，不宣稱其來自一般從零訓練。
- 802 份來源及 466 份凍結測試保持，ticket 89 的正式增益政策未改。

## 分工結果裁決

OpenCode 免費 `opencode/big-pickle --agent plan` 只取得與公開 Git 提交一致的片段，沒有本機工具事件。第一次建議逐項先除再加，仍會在三個 MaxFloat64 輸出溢位，未採用。第二次確認縮放方向。第二次 F4 將 Run 成功後的獨立 EvaluateMetrics 失敗混成失敗 Run 已提交，沒有跨 API 原子性依據，未採納該分類。實際缺陷是成功發布無法編碼的量測，已用公開流程重現。

未提交候選使用 native `gpt-5.6-luna / max` 唯讀審查，報告保留於 `native-fix-review.md`。未發現候選的新確認缺陷。它提出的 LIF 覆蓋疑點已由 Root 公開補驗處理。正常位元已有 16 組外部相容性證據，但永久 ordinary 測試只檢查有限與空集合，這是後續回歸保護的強度限制，不是目前輸出偏離。

## 限制與待決策

這次驗證證明數值與成功發布契約的修正，沒有證明真實辨識率提升、生物定量正確、完整圖的新量測、GPU 或絕對零 bug。ticket 89 的中性增益相容性決策 pending，沒有擅自拒絕舊設定。

減法審查：共用原生入口即可修復，沒有新增精度設定、報告表示法或依賴。保留 normal 路徑，避免為極端範圍重寫所有加總。

完整工程、最後治理、清理與交付結果由 `verification.json`、`cleanup.json`、`delivery.json` 的實際回條判定，本段不提前宣稱通過。

Skills：software-engineering-guidelines、agent-delegation、investigate、eng-architect、test-and-fix、diff-inspector、subtraction-thinking、human-writing、project-memory（唯讀）
