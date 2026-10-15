# 110 — 化學門檻下限投影先於最終有限值檢查

**Epic:** 化學與動態組合稽核
**User Story:** 有限化學偏移經既定下限投影後可表示時，一般模擬與耦合訓練採相同公式。
**Blocked by:** 無
**Status:** in progress

- [x] 公開重現差異，核對既有契約與精確有理數參考。
- [x] 凍結 LIF、混合核心與重算的失敗回歸。
- [x] 最小候選通過回歸與正常相容性對照。
- [x] 正式採用並完成適用工程驗收與需求證據。
- [ ] 保存證據、清除自有暫存、拆分提交並核對遠端。

## Root 決策｜2026-10-15

採用 ticket 21 已明示的 `max(theta_base + adaptation + homeostasis + shift, theta_min)` 公式，並依 ENG 檢查供放電使用的最終門檻。有限輸入的精確相加結果若低於有限下限，投影結果就是該下限。這次修正補足既有投影公式，沒有新增模型設定或另一種數值政策。

`ThetaMin=-2^1023`、`ThetaMax=0`、`raw=0` 與 `shift=-MaxFloat64` 的相加結果低於下限。精確有理數計算確認投影結果有限，原始公開 `AdvanceModulated` 輸出 `[[1]]`，但 `ForwardCoupled` 在投影前回 `non-finite threshold at step 0 neuron 0`，見[重現](../../evidence/MOD-04/lif-threshold-floor-20261015/reproduction.json)。同樣的提前檢查出現在混合核心的 LIF 節點，兩處一起處理。

只調整下限投影與有限值檢查的次序。明確非有限的調節輸入、正向溢位及其他不可表示狀態繼續拒絕，不發布部分 trace。下限上的基礎門檻、適應與偏移導數維持零。正常公式、公開介面、保存格式及依賴保持。範圍、驗收與分工見[計畫](../../evidence/MOD-04/lif-threshold-floor-20261015/plan.md)。修正前一般與競態各 4 個失敗事件、10 個控制通過，三條失敗路徑都符合預期，見[失敗回歸](../../evidence/MOD-04/lif-threshold-floor-20261015/regression-red-verification.json)。

候選一般／競態各 14 個回歸事件通過。32 組正常核心對照與 12 次舊化學模型的新程序接續完整 JSON 逐位相同，12 個模型皆有實際訓練更新，見[候選驗收](../../evidence/MOD-04/lif-threshold-floor-20261015/candidate-verification.json)。823 份正式 Go／module 來源已完成完整工程驗收，一般／競態各 58 套件通過、5,929／5,928 個事件成功、22／23 個略過、零失敗。格式、build、vet、固定 Insyra v0.3.4、相依性與 31 項治理通過，見[完整驗收](../../evidence/MOD-04/lif-threshold-floor-20261015/verification.json)。MOD-04 恢復 passed，目前 91／91 工程需求 passed。最後 31 項治理在不使用 Go 快取及 module cache 的既有測試二進位檔下通過。自有暫存 1,789,486,258 bytes 已清除，原模型、必要資料及舊目錄保持，見[清理](../../evidence/MOD-04/lif-threshold-floor-20261015/cleanup.json)。本票 4／5，尚待提交與遠端交付。
