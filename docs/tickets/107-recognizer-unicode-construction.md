# 107 — 使用者建立的文字辨識模型符合存檔字碼契約

**Epic:** 機制與模型品質稽核
**User Story:** 使用者以 OCR／ASR SDK 建立模型時，無效 Unicode 字碼立即回錯，避免建立後才發現不能存檔。
**Blocked by:** 無，104 固定分析及 106 工程交付已完成
**Status:** in progress

ASR 另有 [108](108-asr-constructor-dimensions.md) 的已確認尺寸缺陷。107 完成後只能恢復 TSK-01，TSK-02 必須同時等待 108 驗收，不得以 Unicode 修正代替尺寸修正。

- [x] 公開 API 重現建構／保存差異，並固定合法新程序恢復控制。
- [x] 凍結失敗回歸，完成兩個模組的隔離候選與 streaming 防護。
- [x] 正式採用共用驗證，核對正常模型與保存相容性。
- [x] 完成適用工程檢查與需求證據。
- [ ] 保存證據、清除自有暫存、提交並核對遠端。

## Root 決策｜2026-10-15

目前 4／5。十個非法字碼案例全部能建構而無法保存，八個合法案例保存後的新程序設定／訓練狀態指紋相同，見[公開重現](../../evidence/TSK-01/recognizer-unicode-20261015/public-reproduction-verification.json)。現有保存層的 Unicode scalar 規則已提前到公開驗證，四份候選來源與四份凍結測試已逐位採用。兩個完整辨識套件一般／競態各 353 個事件通過，與候選逐項一致，見[正式套件驗收](../../evidence/TSK-01/recognizer-unicode-20261015/formal-package-verification.json)。合法設定、正常訓練與保存格式保持。完整根因、檔案責任與對照見[計畫](../../evidence/TSK-01/recognizer-unicode-20261015/plan.md)。接續 108 後的 820 份來源完整一般／競態、build、vet、格式與相依性檢查通過，見[驗收](../../evidence/TSK-01/recognizer-unicode-20261015/verification.json)。自有暫存已清除，完整需求狀態待交付完成後恢復。損壞 UTF-8 字串的正規化不屬本票已確認修正。
