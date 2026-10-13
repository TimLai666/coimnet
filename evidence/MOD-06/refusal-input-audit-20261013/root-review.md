# Diff Inspector

Scope: CLEAN
Reviewed: 基準 `2f7a79c` 至本階段五個 production Go 與五個新增回歸測試；來源由 `source-manifest.json` 凍結。Root 已閱讀完整差異、審查測試觸發條件，親自執行 RED／GREEN 與正常多步相容性比對。新 tickets、ENG 與證據文件是本階段必要交付。
Adversarial review: RUN — 多模組、控制器的狀態提交及梯度契約。

## Findings

修正後沒有確認的 P1／P2 遺留於本階段程式差異。已重現及修正的四類缺陷見 `findings.md`，不能由此推論整個框架沒有 bug。

## Needs investigation

`modulation/memory_controller.go:104`、`modulation/controller.go:199`：在已有活動歷史後改動公開 Inputs／Nodes 宣告，MemoryController 可能重建視窗。現有規格未指定這種變更應回錯、重設或保留歷史，既有正常 caller 也不改宣告（`experiment/ablation_controllers.go:226`）。缺少明確支援契約，不列為已確認缺陷。後續先固定公開語意，再測試。

## 審查整合

- 機制表原先使用不存在的 `Controller.Train` 名稱。完整來源查詢與正常呼叫端確認實際是 `Release` → `Gradient` → `Update`，文件已改為實際流程，並補成功更新及失敗 Offset 的狀態契約。沒有新增訓練 API。
- 獨立審查員的 OCR 相容性疑問已用 Root 實跑的 18 render／9 fixture before-after 相同指紋解決，詳見 `compatibility-result.json`。
- 記憶失敗測試最初有兩列活動相同，無法分辨污染；Root 退回更改成不同活動列，`red-memory-atomic-distinct-row.json` 確認三種失敗都能暴露後續污染。
- 測試與實作由不同執行者負責。MemoryController 實作者只改 Validate／Offset，快取失效修正由 Root 處理；沒有實作者修改其驗收測試。
- 正常呼叫端的 fresh-forward 次序已核對，成功 Update 清除快取不改數值算式。直接修改公開 Parameters 的有效性由 caller 負責，Gradient 文件已明示固定參數前提。
- 沒有新增 API、資料格式、依賴、全腦或生物假設。沒有新增外部服務操作。

## 派工記錄

宿主：Codex（CODEX_THREAD_ID `01a099a7-7772-7363-9935-5fe62074aa74`）。本階段 `opencode models` exit 1，錯誤 `Unknown: FileSystem.open (/Users/timlai/.local/share/opencode/log/opencode.log)`，沒有免費模型呼叫。依 AGENTS.md 的指定第二選擇使用原生 collaboration agents，模型 `gpt-5.6-luna`、reasoning effort `max`。

- audit_neural_learning：鞏固回歸測試、74／75 唯讀審查，以及 76 的 MemoryController Validate／Offset 實作。
- audit_task_quality：OCR 與記憶原子性回歸測試、最後唯讀差異及契約審查。
- audit_chemistry_plasticity：控制器快取契約唯讀調查及獨立回歸測試。

Root 負責骨架、收件審查、實際運行、其他 production 修改、證據與交付。工具與模型限制沒有改寫為成功使用 OpenCode。

減法審查：沿用現有私有候選與快取清除機制，沒有加參數指紋或額外狀態型別。
