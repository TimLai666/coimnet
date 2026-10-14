# 89 — 研究者可以核對多通道化學與來源歷史符合宣告機制

**Epic:** 機制與模型品質稽核
**User Story:** 研究者可以確認零化學作用、多通道聚合與來源歷史符合已宣告規則。
**Blocked by:** 88（已完成）；相容性變更已取得使用者授權
**Status:** completed

本票延續化學組合與持續歷史稽核，固定範圍與責任見[計畫](../../evidence/MOD-02/chemical-composition-20261014/plan.md)。只有已重現的差異才判為 bug，人工小圖通過不能證明整體零 bug 或真實品質提升。

- [x] Root 與獨立稽核核對多通道、MixSum／MixMax、零作用及來源歷史契約，區分確認缺陷與待調查。
- [x] 對實際差異先建立公開入口失敗測試，Root 審查凍結後才修正；無差異則保存有非零控制的獨立驗證。
- [x] 修改適用工程檢查通過，保留來源／測試指紋及實測範圍，相容性規則按 AGENTS.md 處理。
- [x] 同步需求證據、清除本輪暫存，完成已授權提交／推送及遠端讀回。

## Root decision 2026-10-14（採用前紀錄）

確認共用增益驗證接受不包含 1 的正數範圍，零佔用率會被截成非中性增益。公開入口的 15 個案例有 9 個偏離、6 個控制保持中性，三種 mix 都會觸發。MOD-04 原始中性驗收不成立，目前標為 failed，歷史通過證據保持。

建議要求範圍包含 1，候選只在 overlay 通過化學模組一般與 race 各 375 個測試事件。此變更可能使舊模型被拒絕載入，已向使用者提出一次相容性決策，尚未收到回答。正式 Go 程式與既有測試沒有更動，候選不算修正完成。

120 個完整／分段歷史組合、手算 Replay 時鐘，以及 96 個初始梯度斷言通過一般與 race。部分 neural filled 案例屬重複控制，沒有推論生物校準或任務辨識率。

見[Root 審查](../../evidence/MOD-02/chemical-composition-20261014/root-review.md)及[實際驗證](../../evidence/MOD-02/chemical-composition-20261014/verification.json)。目前完成 2／4，正式修正與完整工程驗收待決策後接續。

稽核證據 d8b013e 已推送並核對遠端，本輪 872,621,783 bytes 暫存已清除，793 份提交來源指紋相符。正式修正及完整工程驗收待相容性決策。見[交付](../../evidence/MOD-02/chemical-composition-20261014/delivery.json)與[清理](../../evidence/MOD-02/chemical-composition-20261014/cleanup.json)。

## 已核准的正式修正

使用者已於 2026-10-14 核准「採用修正，拒絕不包含 1 的增益範圍」。本輪由[採用計畫](../../evidence/MOD-04/neutral-gain-adoption-20261014/plan.md)及[授權](../../evidence/MOD-04/neutral-gain-adoption-20261014/authorization.json)記錄。原有稽核與候選證據保留，上述 pending 是當時狀態。共用敏感度驗證已正式收緊，零值預設、合法算式與保存欄位保持。20 個原始失敗事件改為 21 個通過事件。15 組合法舊個體及 10 組合法舊訓練保存物的新程序後續結果完全相同，3 組不合法舊個體與 10 組不合法舊訓練保存物拒絕且原檔不變。完整一般／race 各 58 套件通過，5702／5701 pass，22／23 skip，沒有 fail。見[本輪驗收](../../evidence/MOD-04/neutral-gain-adoption-20261014/verification.json)。正式採用已完成 4／4。最後 11 項治理通過，1,304,068,945 bytes 本輪暫存已清除，原有目錄保留。修正 `90914c3` 與回歸／證據 `831158f` 已推送並讀回一致，806 份提交來源相符，見[清理](../../evidence/MOD-04/neutral-gain-adoption-20261014/cleanup.json)與[交付](../../evidence/MOD-04/neutral-gain-adoption-20261014/delivery.json)。
