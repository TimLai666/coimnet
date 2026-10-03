# 33 — 使用者可以拒絕污染保留集的真實軌跡推論快照

Epic：TSK-11 真實任務資料

User Story：使用者載入既有軌跡模型時，可以確定訓練集與保留集互斥，且完整涵蓋目前來源的試次。

Blocked by：ticket 32 的整試次分割驗證已完成。

Status：done（2026-10-03）。本票只修正推論隔離，不驗收導航學習。完整驗證及遠端提交核對通過。

## Root 決策（2026-10-03）

沿用既有 `validateRolloutSplit`，在 `rebuildInferenceSplit` 選取試次及建立樣本前檢查同一份經 `after_relocation` 過濾的資料。拒絕跨組重複、組內重複、空識別、未知試次與不完整覆蓋。合法快照的格式、參數、最佳化器與計分維持既有行為。

資料流：來源與快照指紋核對 → 共用完整分割檢查 → 重建樣本 → 凍結推論 → 排他寫入報告。分割錯誤由推論入口回傳，錯誤發生時不建立輸出目錄。

## 驗收

- [x] 回歸先證明既有推論接受重疊及不完整覆蓋，再完成最小修正。
- [x] 重疊、組內重複、空識別、未知試次與遺漏皆拒絕。合法分割保留正確 metadata。
- [x] 實際推論入口對非法快照拒絕且不產生報告，既有新程序合法推論回歸通過。
- [x] 真實來源與原模型前後指紋一致；合法模型的推論結果與修正前相同。
- [x] 主 agent 審查完整差異；Mac 完整 build、test、race、vet、格式及相依性檢查通過，保存來源指紋及日誌後提交推送。

## 限制與減法審查

共用現有檢查，不新增驗證器或保存格式。歷史重現見 [legacy-infer-overlap.json](../../evidence/TSK-11/autonomous-rollout-20261002/legacy-infer-overlap.json)。TSK-11 與 OPS-05 維持指定狀態，不用此修正替代導航或 GPU 驗收。

## 驗證與交付證據

[verification.json](../../evidence/TSK-11/inference-isolation-20261003/verification.json) 保存紅／綠回歸、真實來源與模型指紋、新程序非法拒絕與合法結果比對，以及 Mac 完整檢查。修正提交 `467d53a` 已推送並核對遠端 main 相符，見 [delivery.json](../../evidence/TSK-11/inference-isolation-20261003/delivery.json)。
