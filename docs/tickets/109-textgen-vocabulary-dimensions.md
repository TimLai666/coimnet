# 109 — 文字模型在配置前拒絕無法表示的詞表尺寸

**Epic:** 模型建構與公開拒絕稽核
**User Story:** 使用者提供自訂 Vocabulary 時，無法表示的 encoder／readout 尺寸在配置前回錯，不中斷程序。
**Blocked by:** 無
**Status:** done

- [x] 公開介面安全重現並核對完整詞表契約與正常控制。
- [x] 凍結失敗回歸，核對兩項陣列的元素數與位元組界線。
- [x] 完成最小候選，核對正常模型及舊保存的新程序接續。
- [x] 正式採用並完成適用工程驗收與需求證據。
- [x] 保存證據、清除自有暫存、拆分提交並核對遠端。

## Root 決策｜2026-10-15

目前 5／5。NewModel 在配置參數前檢查兩個 float64 陣列的元素與位元組大小，無法以 int 表示時回傳錯誤。共用建構路徑讓 Save／Load 使用同一檢查，公開介面、正常初始化、亂數串流、訓練算式、保存格式與依賴保持。

修正前的合法 lazy byte／special Vocabulary 在一般／競態各三個公開案例觸發 panic，兩個正常控制通過。極大值只在舊 encoder 乘積嚴格為負時進入公開建構器，其餘界線以不配置參數陣列的長度函式及 BigInt 參考核對。凍結測試一般／競態各 18 個失敗、6 個通過，修正後各 24 個事件通過，見[失敗驗收](../../evidence/TSK-03/textgen-dimensions-20261015/regression-red-verification.json)與[修正回歸](../../evidence/TSK-03/textgen-dimensions-20261015/regression-green-verification.json)。

8 組新舊模型對照的初值、三次訓練、logits、NLL、完整 snapshot 與下一次更新位元組相同，16 次舊保存的新程序接續相同。全部模型都有實際參數更新，見[相容性](../../evidence/TSK-03/textgen-dimensions-20261015/candidate-compatibility-verification.json)及[訓練控制](../../evidence/TSK-03/textgen-dimensions-20261015/normal-training-controls.json)。

822 份 Go／module 來源的一般／競態各 58 套件通過，分別 5,915／5,914 個成功事件、22／23 個略過、零失敗。原有事件保持，只新增本票 24 個通過事件。格式、build、vet、固定 Insyra v0.3.4 與相依性校驗通過，最後 31 項治理通過，TSK-03／OPS-01 恢復 passed，目前 91／91 工程需求 passed，見[完整驗收](../../evidence/TSK-03/textgen-dimensions-20261015/verification.json)。

模型與工作原檔逐位保存後，1,848,615,536 bytes 的本輪暫存已清除。18 個原模型檔、6 個必要資料檔與兩個舊暫存目錄保持，見[清理](../../evidence/TSK-03/textgen-dimensions-20261015/cleanup.json)。修正 `71ad3af` 與回歸／證據 `bdb1fea` 已正常推送，遠端讀回相同，822 份已提交來源指紋一致，見[交付](../../evidence/TSK-03/textgen-dimensions-20261015/delivery.json)。

算術可表示不保證實體記憶體或 Go runtime 配置容量。本票在 macOS arm64 實跑，未新增 32 位平台執行、自訂詞表保存、真實模型品質或生物機制定量結論。完整根因、期望結果與分工見[計畫](../../evidence/TSK-03/textgen-dimensions-20261015/plan.md)。整體零 bug 目標保持進行中。
