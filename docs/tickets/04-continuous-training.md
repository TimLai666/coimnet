# 04 — 研究者可以訓練連續核心的延遲關聯

Epic：核心學習

User Story：研究者可以訓練連續核心的延遲關聯

Blocked by：01、02

Status：done（人工 CPU 連續核心）

## 交付

將輸入映射、同步連續動態、讀出與完整/有限時間回推接成可執行 SDK 與例子。

## 驗收

- [x] 兩種連續輸出函數、多步手算及時間步收斂。
- [x] 損失對輸入映射、核心權重、神經參數與讀出梯度符合有限差分。
- [x] 遮罩對動量/衰減有效，NaN/取消不留下半次更新。
- [x] 固定訓練與保留 seed、門檻與預算，核心非零更新且保留損失改善。
- [x] 提供固定外圍只學核心、凍結核心與破壞回饋對照。
- [x] 公開 API 有錯誤與取消，例子明示人工資料，不宣稱真實圖能力。

## 證據

數值測試與完整建置／race／vet 見 [macOS v3 日誌](../../evidence/cpu-reference-20260913/macos-v3/validation.log) 與 [Ubuntu v3 日誌](../../evidence/cpu-reference-20260913/ubuntu-v3/validation.log)。
三組 seed 及全部對照見 [v2 格式 delayed.json](../../evidence/cpu-reference-20260913/macos-v3/delayed.json)。打亂對照只置換訓練標籤，方法與種子保存在報告中。
每組更新 600 次，保留資料平均 MSE 為 0.0002449057725035022，三組都通過事前設定門檻。
向量神經動態、重算反向、LIF、符號限制及真實圖驗收尚未由本 ticket 完成。
