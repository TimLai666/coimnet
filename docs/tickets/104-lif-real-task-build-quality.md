# 104 — 使用者可以核對 LIF 真實辨識品質及跨建置結果

**Epic:** 機制與模型品質稽核
**User Story:** 使用者能以固定的真實單字元資料，核對 LIF 訓練、化學開關與一般／競態建置的結果。
**Blocked by:** 101、103（已完成）
**Status:** in progress

- [x] 固定原始資料、切分、LIF 假設、三個種子與更新預算。
- [x] 公開 LIF API、零化學作用、梯度與新程序保存接續驗證通過。
- [x] 12 組完整訓練及接續完成，保留全部曲線與結果。
- [x] 核對品質、逐筆預測及狀態差異，處理確認缺陷並完成適當工程檢查。
- [ ] 保存證據、清除本輪暫存、提交及核對遠端。

## 範圍與驗收

沿用 ticket 102 的 6,000 張訓練圖片、固定三個種子、20 epochs、每組 120,000 次更新與兩個 2,000 張測試分割。使用先前觀察過的資料，不宣稱全新盲測。LIF 採既有 threshold 範例的工程假設，初始 `theta_raw=-2`，啟用閾值訓練。正式協定見 [protocol.json](../../evidence/TSK-11/lif-build-quality-20261015/protocol.json)。

OCR Recognizer 只提供相同的初始參數與輸入轉換，實際模型由公開 `learning.NewTrainer` 的 LIF 核心持有。這不增加公開 API，不修改原有模型。完整流程、錯誤處理及測試責任見 [plan.md](../../evidence/TSK-11/lif-build-quality-20261015/plan.md)。

結果限於人工接線的 KMNIST 單字元辨識，不外推到全腦、生物機制定量或完整文字辨識。沒有品質改善也須保存，不改參數或挑選最佳 epoch。

## Root 決策｜2026-10-15

目前 4／5。12 組固定訓練與新程序接續已完成，Root 獨立核對 288 份評分與六組各九份數值陣列，原資料、來源、driver、binary、協定與原模型指紋保持，見[驗收](../../evidence/TSK-11/lif-build-quality-20261015/root-final-audit.json)。主要辨識率為 68.30%～71.00%，化學 ON／OFF 沒有穩定品質提升證據。六組跨建置最終狀態都有差異，各自建置內的 12 次保存接續相同，見[結果與限制](../../evidence/TSK-11/lif-build-quality-20261015/README.md)。固定分析已驗收，106 也已完成交付，107／108 可接續正式採用。固定來源的工程檢查與最終 820 份來源的完整一般／競態驗收皆通過。必要資料、16 份恢復封存及工作檔已保存，本票自有暫存已清除，見[驗收](../../evidence/TSK-11/lif-build-quality-20261015/verification.json)與[清理](../../evidence/TSK-11/lif-build-quality-20261015/cleanup.json)。最後治理及交付待完成。
