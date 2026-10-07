# Ticket 49：研究者可以保留不受外部設定修改影響的導航報告

**Epic:** 訓練框架的可重現設定
**User Story:** 研究者可以在取得報告後調整下一次實驗的設定，不改到既有報告
**Blocked by:** 47 共用的有效設定報告入口，已完成；48 有限值驗證是保留的來源基準
**Status:** completed

## 問題與範圍

RunNav2D 複製 Seeds，卻讓 Policies 與呼叫端共用底層切片。呼叫端改策略名稱會改到 report.Config，已保存的 config_hash 不再符合設定。RunNav2DSuite 的四份任務報告也共用這份策略清單。

本票只補足 RunNav2D 的切片複製。單任務報告、呼叫端與四任務報告彼此分開持有 Seeds、Policies。公開簽名、JSON、格式版本、策略及種子順序、預設、驗證與取消次序、訓練與歷史原件保持。

## Root 決策｜2026-10-08：骨架與共用契約

輸入 Nav2DConfig → 既有設定驗證與環境解析 → 複製 Seeds 及 Policies → 計算 config_hash 並執行 → 回傳獨立持有兩份切片的 Nav2DReport。RunNav2DSuite 沿用共同 producer，各次呼叫產生自己的切片。沿用現有 append 複製方式，不新增 helper、介面或報告版本。

| 入口／條件 | 預期結果／錯誤處理 | 驗收 |
|---|---|---|
| 單任務回傳後修改呼叫端 Seeds、Policies | 原報告完整 JSON 及 config_hash 保持 | 公開入口、雙欄修改 |
| 修改單任務報告的 Seeds、Policies | 呼叫端設定保持 | 反向修改、新配置 |
| suite 回傳後修改呼叫端雙欄 | 四份任務報告及各自雜湊保持 | 全四任務 |
| 逐一修改任務報告雙欄 | 呼叫端與其餘三份報告保持 | 各任務獨立反向控制 |
| 切片有額外容量 | 元素修改及追加不互相影響 | 實際修改與 JSON 對照，不比記憶體位址 |
| 合法設定、不修改切片 | 策略順序、種子順序、完整報告、雜湊及訓練結果不變 | 修正前基準、兩個新程序、CLI |
| 空／重複／未知策略、取消 | 原錯誤及部分報告行為保持 | 既有公開入口回歸 |

Root 負責骨架、測試審查及凍結、一處最小實作、完整驗證與文件交付。測試 worker 只能新增 experiment/nav2d_ownership_test.go，不可修改實作、既有測試或其他檔案。一個檔案只有一個寫入者。

## 失敗處理與互動條件

| 操作／條件 | 錯誤回傳與報告 | 擁有者／驗證 |
|---|---|---|
| nil context、執行前取消 | 單任務零報告；suite 沿用既有初始狀態 | 本票保留次序，既有取消回歸及原文比對 |
| 環境解析、空／重複種子、空／重複／未知策略不合法 | 在清單複製及訓練前回錯 | 本票保留既有 Validate 與 Resolve，完整測試 |
| 單次建模、訓練或評估回錯 | 該筆 Failed 與 Error 保留，其餘筆繼續；context 取消回到呼叫端 | 本票保留既有錯誤分支，原文比對 |
| suite 任務回錯／中途取消 | 保留已完成的任務，回原錯誤 | 本票保留 RunNav2DSuite，取消回歸及完整測試 |
| 回傳後修改／追加種子與策略 | 呼叫端及其他報告內容不變 | 本票三個公開入口回歸 |
| 執行期間並行修改輸入 | 不在本票契約內，沒有新增同步能力 | 本票只驗證回傳後修改 |

驗證沿用公開的 RunNav2D／RunNav2DSuite，不替換訓練器或新增測試介面。固定的小配置保留實際執行，初始報告每份四筆 Runs 均不得 Failed。沒有資料庫、格式或保存物遷移，既有報告原件不改寫。

## 固定完成條件

- [x] 先失敗回歸、反向控制經 Root 審查及凍結。
- [x] 單任務及 suite 的雙欄切片互不影響，共用入口修正。
- [x] 合法完整報告逐位元組相同，CLI 與原件保存通過。
- [x] gofmt、build、完整一般／race、vet、相依性及治理通過。
- [x] 完整 diff、需求證據、提交推送與遠端核對完成。

## 減法審查

Won't List：不處理其他比較設定、不調參、不建立不可變報告 API，均另有範圍。
建議簡化：在已有 Seeds 複製處補一行 Policies 複製，suite 不重複實作。
需要留意：report.Config 仍是可自行修改的公開值，修改報告本身不會自動重算雜湊。

## 來源與證據

[工程設計](../../ENG.md)、[有效設定票](47-nav2d-effective-config-reports.md)、[有限值票](48-nav2d-finite-config-validation.md)、[原公開入口重現](../../evidence/OPS-03/nav2d-effective-config-20261007/ownership-probe.json)、[本票證據](../../evidence/OPS-03/nav2d-policy-ownership-20261008/)。
相關需求 OPS-03、TSK-08，需求狀態不變。

實作 1b088e42151941c132a38d870ff64e9ac8517cb7 已推送 origin/main 並核對遠端相同，見[交付紀錄](../../evidence/OPS-03/nav2d-policy-ownership-20261008/delivery.json)。固定進度 5／5。
