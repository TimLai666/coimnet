# 144 — 重新取樣先檢查輸出緩衝的位元組尺寸

Epic：音訊尺寸與錯誤處理

User Story：使用者的重新取樣要求超出原生緩衝可表示範圍時，Resample 回傳錯誤，不會觸發配置 panic。

Blocked by：142／143 的凍結正式驗收與交付；隔離重現與回歸可先進行。

Status：in_progress

相關需求：TSK-02 音訊前處理與失敗處理。

## Root 決策（2026-10-18）：根因與修正骨架

公開 Resample 的一個零樣本、來源率 1、目標率 MaxInt，在 darwin/arm64 Go1.26.5 重現 makeslice len panic。精確長度可由 int 表示，但 float64 位元組乘積超過 uintptr。Root 先核對本機 runtime 在乘積溢位時於 mallocgc 前拒絕，再執行受控 probe，沒有嘗試配置巨量緩衝。正常四樣本輸出與只查長度的 MaxInt 控制通過。

修正只在 Resample 的既有有限輸入檢查之後、make 之前檢查 n×8 是否可由原生 int 表示，錯誤回傳 nil、零報告與 error。保留精確長度公式、插值與 ResampleOutputLength 的大整數查詢契約。不保證可表示的緩衝有足夠實體記憶體，也不增加固定容量上限。正式來源在 142／143 完成前保持凍結，先以隔離候選驗證。

## 固定完成條件

- [x] 公開重現配置 panic，核對獨立乘積、正常插值與只查長度控制。
- [x] Root 凍結先失敗的公開回歸，完成隔離候選一般／競態對照。
- [ ] 正式採用最小修正，專項一般／競態與既有控制通過。
- [ ] 格式、build、完整一般／競態、vet、固定模組與治理通過。
- [ ] 保存證據、清除自有暫存、提交推送並核對遠端。

## 減法審查

在既有配置入口補位元組檢查，不收緊只查長度的 helper，也不新增資料層或任意容量限制。

## 證據

[公開重現與 Root 核對](../../evidence/TSK-02/resample-capacity-audit-20261018/verification.json)。原版一般／競態各 1 fail／3 pass，凍結回歸重現 panic，正常與只查長度控制保持，見[失敗驗收](../../evidence/TSK-02/resample-capacity-audit-20261018/red-verification.json)。隔離候選各 4 pass，整個音訊套件一般／競態各 81 pass，原有 77 個事件保持，只新增 4 pass，見[候選驗收](../../evidence/TSK-02/resample-capacity-audit-20261018/candidate-verification.json)。10,459 bytes 自有探測／回歸暫存已清除，必要來源保留，見[清理](../../evidence/TSK-02/resample-capacity-audit-20261018/regression-cleanup.json)。目前 2／5，863 份正式來源保持，正式採用待 142／143 交付後接續。
