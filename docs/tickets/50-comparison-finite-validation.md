# Ticket 50：研究者可以在比較開始前發現非法數值設定

**Epic:** 訓練框架的可保存設定
**User Story:** 研究者可以在開始比較前取得設定錯誤，不等訓練完成才發現報告無法保存
**Blocked by:** 無；49 是保存的來源基準
**Status:** in_progress

## Root 決策｜2026-10-09：問題與共用契約

歸因與連續任務的 Comparison.Interval 以小於等於零或大於等於一排除非法值，NaN 的兩個比較都為 false，因此通過 Validate 並進入執行，回傳報告卻不能編碼成 JSON。兩個入口必須只接受嚴格介於 0 與 1 的數值，NaN 與正負無限大在訓練及 builder 前回傳 interval 錯誤與零報告。

沿用現有 Validate → Run → 報告流程與錯誤文字，Root 只改兩處判斷。公開簽名、格式、合法設定、取消與 nil context 次序、連續任務 nil Comparison、訓練參數及歷史原件保持。

| 條件／操作 | 預期結果／處理 | 本票驗收 |
|---|---|---|
| NaN、正負無限大、0、1、負數、超過1 | Validate 與公開 Run 回 interval 錯誤，零報告；連續任務不呼叫 builder | 先失敗回歸與邊界 |
| 合法值，含兩側最鄰近有限值 | Validate 成功 | 合法控制 |
| 合法比較與 nil Comparison | 完整報告及訓練結果與修改前逐位元組相同，JSON 可保存 | 兩個新程序對照 |
| nil context、取消、nil builder、其他非法設定 | 沿用既有較早錯誤 | 次序回歸與相關測試 |
| CLI 非法 --interval | 使用錯誤，沒有產生輸出檔 | 真實 CLI |
| CLI 合法輸出／重複路徑 | 可保存完整報告，拒絕覆寫 | 真實 CLI 與雜湊 |

## 分工

Root 負責骨架、實作、測試審查與凍結、完整驗證及交付。worker 只寫新增的 comparison_finite_test.go。OpenCode 只取得手寫的公開 API 契約，在指定暫存目錄產出測試，不讀取 repo。免費呼叫不可用時依專案規則改 Luna max，僅新增 experiment/comparison_finite_test.go。worker 不改實作、既有測試及文件，不提交、不 stash、不 reset。兩個判斷短於派工成本，由 Root 修改。

## 固定完成條件

- [x] Root 審查先失敗回歸、合法控制並凍結測試。
- [x] 兩個共用比較入口拒絕非有限值，公開 Run 不開始訓練。
- [x] 合法完整報告、CLI 保存與歷史原件通過。
- [x] gofmt、build、完整一般與 race、vet、相依性、治理通過。
- [ ] 完整 diff、需求證據、提交推送與遠端核對完成。

## 減法審查

Won't List：不調參、不修改模型、不處理 OCR 或其他不同欄位。
建議簡化：兩處改成正向範圍判斷，不新增 helper 或保存後補救流程。
沒有格式或資料遷移，也沒有新的外部服務、依賴或並行輸入契約。

## 來源

[工程設計](../../ENG.md)、[原重現](../../evidence/OPS-03/nav2d-finite-config-20261007/related-comparison.log)、[本票證據](../../evidence/OPS-03/comparison-finite-20261009/)。
相關需求 OPS-03、LRN-08、TSK-12，需求狀態不變。

Root 驗收：三個新增頂層測試、14 組非法配置與 10 個合法控制、37 個相關頂層測試、7 份原提交與雙程序合法報告、22 個既有錯誤、13 個 CLI 流程通過。完整一般／race 各 57 套件及格式、build、vet、相依性通過。另確認 report.Protocol.Comparison 共用設定的既有 P2，見 AGENTS.md 與 ownership-followup.json，實作另票處理。工程驗收 4／5，待提交推送與遠端核對。
