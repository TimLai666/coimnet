# Diff Inspector

Scope: CLEAN
Reviewed: `5b014a8` 之後的 realnav 推論分割修正、回歸、使用說明與追蹤紀錄。

## Findings

No confirmed findings.

## 主 agent 核對

- 共用 `validateRolloutSplit` 檢查全部過濾後試次。`runInfer` 在來源指紋及前處理身分核對後呼叫 `rebuildInferenceSplit`，guard 位於試次選取、樣本建立、模型恢復及輸出目錄建立之前。
- 相同驗證器原先已由 `runRolloutEngine` 呼叫。沒有新增驗證器、保存格式、匯入注入介面或依賴。
- 非法分割的回歸涵蓋跨組重複、遺漏、組內重複、未知與空 ID，合法分割保留兩組 metadata。CLI 真實資料回歸明示需外部資料；主 agent 已設定輸入後獨立實跑，並另跑重疊／遺漏兩個新程序案例。
- 舊驗證 binary 的 SHA-256 與上輪交付相符。它實際接受重疊／遺漏，修正後 binary 對相同模型變體 exit 1 且沒有建立輸出目錄。合法真實報告只排除 runtime 數值後完全相同；資料與原模型前後指紋一致。
- help、範例 README、ENG、ticket 33、狀態與需求證據同步，已刪除完成的推論 Follow-up。ticket 34 只提出方法，尚未改模型或參數；TSK-11 與 OPS-05 不升級。

3 行驗證 guard 與 1 行 help 是局部修正，未觸及認證、付費、並行核心或保存流程，依 diff-inspector 採主 agent 完整差異審查，未另派 adversarial agent。

## 限制

只驗證 Mac CPU 的來源隔離與相容性。軟體驗證不增加導航學習或生物機制證據。整體導航仍需獨立方法決策與驗收。正式完整檢查結果見 verification.json。
