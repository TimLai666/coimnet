# Ticket 47：研究者可以從導航報告取得完整有效設定

**Epic:** 訓練框架的可重現設定
**User Story:** 研究者可以用報告中的設定重跑同一個導航範例
**Blocked by:** 28 導航與歸因入口、45 格式索引一致性，均已完成
**Status:** completed

## 問題與範圍

RunNav2D 與 RunAttribution 的環境執行會補入預設值，但 config.env 與 config_hash 使用呼叫端尚未展開的設定。四任務 suite 沿用前者，也受影響。設定全為零時實際環境為 9×9、牆密度 0.2、視野 3、時限 60、步數懲罰 0.01、碰撞懲罰 0.05、抵達獎勵 1。

本票修正新報告的紀錄與雜湊，不改預設值、訓練機制、模型、歷史證據或格式版本。省略預設值與明填相同預設值的新報告應相同。舊報告的雜湊仍代表原來的輸入設定。

前置：ticket 28 的導航與歸因入口及 ticket 45 的索引檢查已存在。相關需求：OPS-03、TSK-08、TSK-12。狀態計數保持 89／91。

## Root 決策｜2026-10-07：骨架與共用契約

- Config.Resolve() (Config, error)：純值輸入與輸出，沿用 withDefaults、validate 及 resolveTask；有效設定回傳完整九欄，非法設定回零值與原有錯誤。解析可重複，呼叫端保持。
- nav2d.New(Config) (*Env, error)：改用相同 Resolve；環境、亂數串流、驗證順序與錯誤保持。
- RunNav2D(context.Context, Nav2DConfig) (Nav2DReport, error)：既有驗證後解析 Env，再建立報告與計算 SHA-256。suite 由此共用。
- RunAttribution(context.Context, AttributionConfig) (AttributionReport, error)：保留必填 Task 的原有驗證，再解析 Env；所有組別共用同一設定。
- CLI 的 JSON、檔案與 --help，以及 README，說明新報告已展開環境設定。

## 想要的結果與失敗案例

| 入口或條件 | 結果 | 驗收 |
|---|---|---|
| Resolve 全零、部分零、完整明填 | 九欄實際預設、明填保持、重複解析相同、輸入保持 | 獨立常數與 New 的觀察／規則 |
| Resolve 非法寬高、密度、視野、時限、獎懲、任務 | 原有錯誤，不回部分設定 | 回歸測試 |
| 單任務與四任務 suite | report.Config.Env 完整，Task 相符 | 公開 API 與實際 CLI |
| 歸因 | 新報告完整，原本空 Task 仍拒絕 | 公開 API 與實際 CLI |
| 明填與省略預設 | Config、獨立重算的 config_hash 與 Runs 相同 | 回歸與新程序重現 |
| 自訂設定 | 九欄明填保持，改有效值則改雜湊 | 回歸測試 |
| 原有失敗、取消與目的地存在 | 錯誤、輸出與拒絕覆寫保持 | 既有測試與 CLI |
| 修正前後同一固定人工配置 | 除 Config／ConfigHash 之外的報告逐筆一致 | 前後基準資料 |

## 分工與檔案責任

Root 負責契約、測試審查與凍結、三個回歸檔、三個實作檔、文件、證據、完整 diff、全套驗證與提交推送。OpenCode 以隔離的人工契約試派工，實際 403；Claude／Antigravity 額度受限，Luna 首次測試派工滿載，後續 max 唯讀審查通過。主 agent 完成可自行處理的小範圍實作，外部模型沒有取得既有原始碼。
同一檔案只由一個寫入者負責，修改須先讀取並以原文相符為條件。

## 固定完成條件

- [x] 修正前基準、失敗回歸與 Root 審查凍結完成。
- [x] 共用解析及三種報告入口完整記錄有效設定與雜湊。
- [x] 訓練結果、明填設定、錯誤及歷史原件保持，實際 CLI 流程通過。
- [x] gofmt、build、完整一般／race、vet、相依性及治理檢查通過。
- [x] 完整 diff、需求證據、提交推送與遠端核對完成。

## 減法審查

共用既有預設值與驗證，移除報告層重複推定預設任務的邏輯。不新增設定框架、報告版本或額外調參工作。非有限值驗證與效能改進另行處理。

## 來源與證據

[主規格 §16.3](../handoff/CoImNet_Implementation_Plan.zh-TW.md#chapter-16)、[原導航工作票](28-navigation-multimodal-shared-core-and-attribution.md)、[工程決策](../../ENG.md)、[證據目錄](../../evidence/OPS-03/nav2d-effective-config-20261007/)。

## 已驗證結果

六個新頂層回歸均先在空骨架失敗，再於修正後通過。三個報告入口、九欄有效設定、獨立 SHA-256、明填／省略一致、普通非法設定、取消與拒絕覆寫均有覆蓋。八個真實 CLI 流程通過，24 筆修正前後記錄逐筆相同，兩個新程序的完整新報告相同。明填自訂設定的原雜湊保持。完整一般及 race 各 57 個套件通過，其他必要軟體檢查通過。Root 與 Luna max 唯讀審查完成，無本票新增問題。實作 685a090 已提交推送，遠端 main 核對一致，見 [交付紀錄](../../evidence/OPS-03/nav2d-effective-config-20261007/delivery.json)。
