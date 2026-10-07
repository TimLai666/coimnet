# Ticket 48：研究者可以在導航開始前發現無效數值設定

**Epic:** 訓練框架的輸入驗證
**User Story:** 研究者可以在開始導航或歸因實驗前取得明確設定錯誤
**Blocked by:** 47 共用環境設定解析，已完成
**Status:** completed

## 問題與範圍

環境四個浮點欄位的範圍比較不能攔住 NaN；部分單向比較也接受正無限大。這些設定會污染執行或讓 JSON 匯出失敗。修正限於 WallDensity、StepPenalty、CollisionPenalty、GoalReward，所有使用相同環境的入口共用驗證。

合法有限值、零值預設、有限值的原有錯誤文字及檢查順序保持。單任務、suite、歸因的既有取消與部分報告行為保持。沒有模型、訓練參數、格式、依賴或資料遷移。

## Root 決策｜2026-10-07：骨架與共用契約

Config.Resolve() (Config, error) 先沿用預設，再由既有 validate(Config) error 驗證四個欄位；New(Config) (*Env, error) 共用此入口。每個欄位在自己的範圍比較前拒絕 NaN 及正負 Inf，錯誤包含欄位名稱及非法值。保持其他欄位的錯誤次序。

流程：輸入設定 → 共用環境驗證 → 錯誤，或建立環境與執行 → 可序列化報告。Nav2DConfig.Validate、AttributionConfig.Validate 與三個 Run 入口已有共同 producer，不新增驗證框架或介面。

| 條件／入口 | 預期結果／錯誤處理 | 驗收 |
|---|---|---|
| 四欄 × NaN、+Inf、-Inf | Resolve 回零 Config 與欄位錯誤；New 回 nil 與相同錯誤 | 12 組公開入口回歸 |
| 有限範圍邊界、零值預設、極小正數 | 成功，合法值保持、預設相同 | 正向控制、既有回歸 |
| 有限負值與密度超界 | 原錯誤文字保持 | 反向控制 |
| 任一非法欄位搭配其他非法整數欄位 | 既有第一個錯誤保持 | 次序控制 |
| Nav2D／Attribution Validate | 在環境設定處回錯，包含欄位 | 公開設定入口 |
| 單任務／歸因 Run | 回錯與零報告，沒有 run | 公開完整流程 |
| suite Run | 回錯、沒有 Tasks，保留既有 SchemaVersion | 公開完整流程 |
| nil／已取消 context | 原 context 錯誤優先 | 既有回歸 |
| 合法的固定人工配置 | 新程序報告與修正前相同 | 與 ticket 47 的有效設定基準逐筆比較 |

同一個檔案只有一個寫入者。Root 負責 env.go、環境回歸、契約、凍結、文件、證據及交付；測試 worker 只可填指定報告回歸檔，不可修改實作或其他測試。

## 固定完成條件

- [x] 先失敗回歸與正反向控制經 Root 審查、凍結。
- [x] 共用入口拒絕四欄所有非有限值，錯誤與無執行結果符合契約。
- [x] 合法設定、報告、預設與歷史原件保持，公開流程驗收通過。
- [x] gofmt、build、完整一般／race、vet、相依性及治理通過。
- [x] 完整 diff、需求證據、提交推送與遠端核對完成。

## 減法審查

Won't List：不處理策略切片所有權、其他任務設定或調參，均不屬本票環境驗證。
建議簡化：沿用已有共用入口，只補四個數值檢查，不另建通用驗證器。
需要留意：有限值本身合法不保證任意極端值的後續運算不溢位，本票不新增數值上限。

## 來源與證據

[主規格 §18.4](../handoff/CoImNet_Implementation_Plan.zh-TW.md)、[前置工作](47-nav2d-effective-config-reports.md)、[原重現](../../evidence/OPS-03/nav2d-effective-config-20261007/nonfinite-probe.json)、[本票證據](../../evidence/OPS-03/nav2d-finite-config-20261007/)。
相關需求 OPS-03、TSK-08、TSK-12，狀態計數維持 89／91。

## 驗收結果

5／5 完成。四個新增回歸、11 個選取的頂層測試、八個 CLI 流程、24 筆合法結果逐位元組比較，以及完整一般／race 各 57 套件通過。實作 6f2aa38 已推送，遠端 main 核對一致。完整命令、指紋、日誌、Root 審查與交付紀錄見[工程驗證](../../evidence/OPS-03/nav2d-finite-config-20261007/verification.json)及[交付紀錄](../../evidence/OPS-03/nav2d-finite-config-20261007/delivery.json)。
