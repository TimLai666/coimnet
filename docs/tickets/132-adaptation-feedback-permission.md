# 132 — 使用者可以限制適應評估可使用的回饋

**Epic:** 評估與機制一致性
**User Story:** 使用者可以事先宣告哪些回饋可用，未允許的答案資訊不會改變適應模型。
**Blocked by:** 無，與 ticket131 分開驗收
**Status:** in_progress

## Root 決策｜2026-10-16

Root 在公開 SDK、正常有限數值及合法宣告下確認 P1 缺陷。完全沒有可用回饋、僅宣告可用但未允許該題使用，以及只允許 correctness 的三組，都與明確允許 score 的組使用相同答案衍生分數。將目標由 0.5 換成 100，快速權重由 2.4606582210702888 變成 0，後續評分輸出亦改變。一般與競態結果須逐位核對，證據見[公開探測](../../evidence/LRN-10/policy-composition-20261016/next-audit-ordinary.json)。

根因是適應迴圈忽略 FeedbackAvailable 與各題 AllowedFeedback，無條件計算 MSE、建立 score 回饋並更新。主規格 10.6、11.6 及 LRN-10 要求事前宣告並遵守可用回饋。ticket131 只修評分保持快速狀態，不能修正這項適應污染，因此 LRN-10 保持 specified。

## 驗收

- [x] 固定獨立候選失敗回歸，覆蓋未提供、未允許、已允許及其他種類的回饋。
- [x] 最小修正只使用明確可用且該題允許的回饋，沒有回饋時不由答案產生更新。
- [x] 已允許的 score、固定評分、後續原生學習與舊模型保持對照通過。
- [ ] 完整工程、證據、暫存清理與交付完成。

不自行替 correctness 或其他名稱發明分數語意。若需要改變已公布的種類支援或拒絕行為，先依實際契約與相容性影響決定。既有 scoring 回饋計數的宣告拒絕／實際 Receive 檢查差異另列調查，不把未觸發的風險當成已確認故障。

Root 已在正式採用前保存候選驗證，本輪重新以未修改的正式程式執行一般／競態回歸，各6 fail、1 pass。最小權限檢查已正式採用，一般／競態各39項相關測試及35項固定評分控制通過。10列公開探測證明三組未允許目標互不影響，兩組已允許score與原程式逐位相同，另外兩列重播控制保持。12份舊化學快照的新程序接續與修改前逐位相同。848份正式來源已完成完整工程與清理，本票3／4，待遠端交付，LRN-10保持specified。見[本輪驗收](../../evidence/LRN-10/feedback-permission-20261016/verification.json)與[原始失敗](../../evidence/LRN-10/feedback-permission-20261016/red-verification.json)。

另以公開SDK與精確參考確認極端有限目標的MSE中間加總溢位，另由[ticket134](134-evaluation-mse-representability.md)接續，本票完整工程通過也不會單獨恢復LRN-10。

848份來源的完整一般／競態各58套件通過，8,443／8,442 pass、23／24 skip，原有各8,459個終端事件保持，只新增各7 pass。格式、build、vet、固定Insyra v0.3.4、模組與31項治理通過。最後31項治理通過，33份必要輸入已封存，1,786,439,615 bytes本輪暫存已清除。本票3／4，待遠端交付，工程追蹤維持89／91。見[完整工程](../../evidence/LRN-10/feedback-permission-20261016/engineering-verification.json)。
