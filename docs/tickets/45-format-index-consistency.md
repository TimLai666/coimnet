# 45 — 維護者可以查到每個版本宣告及其用途

**Epic:** GOV-01／GOV-04 文件準確性
**User Story:** 維護者可以從文件找到框架、範例、驗證工具與測試反例的版本來源，不把版本識別或錯誤案例誤當可讀取格式。
**Blocked by:** 無
**Status:** completed

## Root 決策（2026-10-07）

根因是 docs/INDEX.md 手動維護的部分清單與原始碼沒有一致性檢查，並混入未知或未來版本測試。沿用文件單一入口，不增加執行時註冊表、CLI、依賴或保存格式。規則與雜湊識別明示用途，僅在測試中使用的驗證報告與錯誤案例分開。

## Root 骨架與責任

- Root：契約、本票、ENG、交付狀態與需求證據，審核測試、完整 diff、最終驗證及 Git 交付。
- 盤點 agent：唯讀核對版本字串的實際用途、產生與拒絕入口，提供檔案及行號。
- 測試 agent：只新增 format_index_test.go，沿用 package coimnet_test。不得改 INDEX 或其他檔案。
- 文件 agent：只修改 docs/INDEX.md。不得修改測試；認為測試錯就回報 Root。

盤點版本限定為 `coimnet-名稱/v數字` 形式。檢查範圍是儲存庫內 Go 字串 literal（以 AST 讀取及解碼，不把註解當宣告）、scripts 的 shell 與 evidence 的 Python 版本引用，以及四份治理 JSON 的 schema_version（requirements-status、requirements-addendum、handoff requirements／sources）。略過隱藏目錄、vendor、data、runs 與 bin，不掃描下載資料、模型、歷史 JSON 輸出或規格中的未實作提案。

每個版本在索引只有一列。列出用途、套件與可核對的來源檔案及行號。來源必須實際含該宣告；正式框架／範例宣告不能只用測試來源充數。原始碼中的別名、合法 Go escape 或 raw string 在解碼後核對。已知未知／未來版本與人造治理資料列為測試資料／反例，驗證工具產生的真實報告另外列出。

## 使用流程與失敗驗收

1. 維護者從 INDEX 版本列找到實際來源與用途，檔案連結及行號可核對。
2. 新增或更動直接版本宣告，go test 的索引檢查拒絕漏列、重複、過期版本、錯誤來源或錯誤分類。
3. 在沒有 .git 的原始碼目錄執行，檢查同樣有效，不依賴 Git、外部索引、網路或執行模型。

| 情況 | 期望 | 負責 |
| --- | --- | --- |
| 原索引漏列或誤分類 | 失敗，指出版本與來源位置 | 本票檢查 |
| 零個來源／空索引、重複列、過期列、缺少來源或無效行號 | 明確失敗，不成功跳過 | 本票檢查 |
| 註解、Go escape、raw string、測試反例、驗證工具輸出 | 正確分辨，避免未實作內容進入正式格式 | 本票檢查及獨立盤點 |
| 來源讀取或 Go 解析失敗 | 檢查失敗並指明檔案 | 本票檢查 |
| 原件、模型及公開契約 | 指紋保持，沒有遷移或模型變更 | Root 原件查核 |

## 驗收

- [x] Root 審核失敗測試，在原 INDEX 保存失敗與來源指紋，見 `evidence/GOV-01/format-index-20261007/red.log`、`frozen-tests.json`。
- [x] 完整盤點與分類通過：124 個版本，正式 87、規則識別 10、驗證工具 14、測試資料與反例 13，見同目錄的 `inventory-delta.json` 及 `source-review.md`。
- [x] INDEX 與來源、用途、連結及新增檢查一致，Root 全文審查、反例測試、173 個本機連結與無 Git 匯出驗證通過，見 `green.log`、`archive.log`、`integrity.log`、`frozen-index.json`。
- [x] 完整格式、建置、一般／race 各 57 個套件、vet、相依性、8 個格式／治理專項及既有檔案指紋檢查通過，見 `verification.json`、`checks.json` 與對應日誌。
- [x] 命令、環境、指紋、結果及日誌保存，GOV-01／GOV-04 追蹤更新。實作 `baf7b91` 已提交、推送並核對遠端，見 `delivery.json`。

減法審查：維護既有單一索引，檢查加入現有 go test，不另建格式註冊表或新指令。
