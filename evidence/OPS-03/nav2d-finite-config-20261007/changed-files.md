# Ticket 48 變更與交付清單

Problem：導航環境的非法浮點設定可能進入訓練，或延後到報告匯出才失敗。

Done：四欄共用有限值驗證與完整回歸完成。合法設定的 24 筆結果逐位元組保持，完整一般與 race 各 57 套件通過。

## Changed

相對於實作前提交 739d3944487ccab8bf476867b9fdfbf7b1a536dc，本票新增或修改 82 份檔案。沒有移動或刪除檔案。

| 檔案 | 變更摘要 |
|---|---|
| [AGENTS.md](/Users/timlai/Developer/coimnet/AGENTS.md) | 修改：移除已解決的環境數值提醒，保留並登錄另兩項 P2 後續問題。 |
| [ENG.md](/Users/timlai/Developer/coimnet/ENG.md) | 修改：記錄既有共用入口的四欄有限值契約與相容性條件。 |
| [README.md](/Users/timlai/Developer/coimnet/README.md) | 修改：說明 SDK 環境設定須為有限值。 |
| [delivery-status.md](/Users/timlai/Developer/coimnet/delivery-status.md) | 修改：更新 ticket 48 的 5／5 驗收、交付與後續成果。 |
| [docs/INDEX.md](/Users/timlai/Developer/coimnet/docs/INDEX.md) | 修改：票號範圍更新至 48。 |
| [docs/requirements-status.json](/Users/timlai/Developer/coimnet/docs/requirements-status.json) | 修改：OPS-03、TSK-08、TSK-12 加入本票證據，需求狀態保持。 |
| [docs/tickets/48-nav2d-finite-config-validation.md](/Users/timlai/Developer/coimnet/docs/tickets/48-nav2d-finite-config-validation.md) | 新增：驗證契約、角色責任、固定驗收條件與完成證據。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/api-doc.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/api-doc.json) | 新增：公開 Config 文件的精確命令輸出。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/baseline.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/baseline.json) | 新增：修正前 3,253 份已追蹤檔案的指紋及七份允許修改檔案。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/build.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/build.json) | 新增：完整建置的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/build.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/build.log) | 新增：完整建置的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/changed-files.md](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/changed-files.md) | 新增：本票 82 份實際新增／修改檔案的完整清單與操作紀錄。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/cli-workflow.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/cli-workflow.json) | 新增：當次來源的八個 CLI 流程、報告、輸出保護與本機暫存位置。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/cli.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/cli.json) | 新增：八個 CLI 完整流程的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/cli.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/cli.log) | 新增：八個 CLI 完整流程的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/comparison-probe.go.txt](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/comparison-probe.go.txt) | 新增：Comparison.Interval=NaN 的公開入口重現程式。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/delegation.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/delegation.json) | 新增：OpenCode 實際 403、Luna max 任務邊界與 Root 驗證。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/delivery.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/delivery.json) | 新增：實作提交、推送成功、遠端 main 一致與來源驗證連結。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/diff-check.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/diff-check.json) | 新增：Git diff 空白與格式的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/diff-check.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/diff-check.log) | 新增：Git diff 空白與格式的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/doc-read.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/doc-read.json) | 新增：工程規則及文件的讀取紀錄。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/entry-documents.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/entry-documents.json) | 新增：工作入口文件的來源指紋。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/environment.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/environment.json) | 新增：實際 Go、作業系統與固定 Insyra 版本。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/format.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/format.json) | 新增：gofmt 的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/format.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/format.log) | 新增：gofmt 的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/governance-before-delivery.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/governance-before-delivery.json) | 新增：交付前治理及格式索引的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/governance-before-delivery.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/governance-before-delivery.log) | 新增：交付前治理及格式索引的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/governance-completion.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/governance-completion.json) | 新增：完成交付紀錄後的八個治理及格式索引的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/governance-completion.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/governance-completion.log) | 新增：完成交付紀錄後的八個治理及格式索引的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/governance-final.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/governance-final.json) | 新增：原始治理證據缺鍵失敗的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/governance-final.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/governance-final.log) | 新增：原始治理證據缺鍵失敗的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/governance-ready.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/governance-ready.json) | 新增：修正證據後的治理及格式索引的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/governance-ready.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/governance-ready.log) | 新增：修正證據後的治理及格式索引的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/green-scoped.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/green-scoped.json) | 新增：修正後 11 個相關頂層測試的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/green-scoped.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/green-scoped.log) | 新增：修正後 11 個相關頂層測試的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/isolated-dispatch.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/isolated-dispatch.json) | 新增：隔離人工契約的 OpenCode 命令、耗時與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/isolated-output.jsonl](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/isolated-output.jsonl) | 新增：OpenCode 免費服務 403 的原始輸出。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/isolated-prompt.txt](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/isolated-prompt.txt) | 新增：不包含儲存庫來源的人工驗證派工契約。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/isolated-stderr.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/isolated-stderr.log) | 新增：隔離 OpenCode 呼叫的標準錯誤紀錄。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/json-check.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/json-check.json) | 新增：嚴格 JSON 解析的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/json-check.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/json-check.log) | 新增：嚴格 JSON 解析的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/mod-verify.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/mod-verify.json) | 新增：Go 模組完整性的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/mod-verify.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/mod-verify.log) | 新增：Go 模組完整性的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/opencode-models.txt](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/opencode-models.txt) | 新增：當次查得的免費 OpenCode 模型清單。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/plan.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/plan.json) | 新增：固定目標、範圍與五項完成條件。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/preservation.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/preservation.json) | 新增：原始交接包、既有檔案與已凍結來源的保存查核。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/race-all.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/race-all.json) | 新增：完整 race 測試 57 套件的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/race-all.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/race-all.log) | 新增：完整 race 測試 57 套件的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/red-all.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/red-all.json) | 新增：修正前四個新增回歸的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/red-all.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/red-all.log) | 新增：修正前四個新增回歸的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/red-env.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/red-env.json) | 新增：修正前環境非法值回歸的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/red-env.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/red-env.log) | 新增：修正前環境非法值回歸的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/related-comparison.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/related-comparison.json) | 新增：Comparison.Interval 非有限值的另項重現的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/related-comparison.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/related-comparison.log) | 新增：Comparison.Interval 非有限值的另項重現的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/root-review.md](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/root-review.md) | 新增：Root 完整程式審查、測試結果、P2 後續問題與 SHIP 結論。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/run-check.py](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/run-check.py) | 新增：僅用於本票的命令、日誌及退出結果蒐集程式。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/scope-review.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/scope-review.json) | 新增：四欄共用驗證的 producer／consumer、來源與修改範圍審查。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/source-freeze.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/source-freeze.json) | 新增：實作及兩份回歸測試的凍結 SHA-256。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/stage-review.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/stage-review.json) | 新增：實作提交 77 檔的範圍、暫存內容一致及提交掃描結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/test-all.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/test-all.json) | 新增：完整一般測試 57 套件的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/test-all.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/test-all.log) | 新增：完整一般測試 57 套件的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/test-freeze.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/test-freeze.json) | 新增：實作前已重現失敗並經 Root 審查的兩份測試指紋。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/tidy.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/tidy.json) | 新增：離線模組整理差異的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/tidy.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/tidy.log) | 新增：離線模組整理差異的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/valid-after-report.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/valid-after-report.json) | 新增：修正後新程序的完整合法報告。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/valid-after-report.stderr.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/valid-after-report.stderr.log) | 新增：valid-after-report 的標準錯誤紀錄。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/valid-before-command.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/valid-before-command.json) | 新增：修正前合法報告的執行命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/valid-before.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/valid-before.json) | 新增：修正前的完整合法報告。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/valid-before.stderr.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/valid-before.stderr.log) | 新增：valid-before 的標準錯誤紀錄。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/valid-comparison.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/valid-comparison.json) | 新增：24 筆合法結果及整份 JSON／位元組逐筆一致查核。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/valid-flow.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/valid-flow.json) | 新增：合法報告完整流程的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/valid-flow.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/valid-flow.log) | 新增：合法報告完整流程的實際執行日誌。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/valid-probe.go.txt](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/valid-probe.go.txt) | 新增：八份合法報告的公開 SDK 完整流程程式。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/valid-repeat-report.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/valid-repeat-report.json) | 新增：第二個新程序的完整合法報告。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/valid-repeat-report.stderr.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/valid-repeat-report.stderr.log) | 新增：valid-repeat-report 的標準錯誤紀錄。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/verification.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/verification.json) | 新增：需求、命令、環境、輸入指紋、結果、日誌與修正過的治理缺口。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/verify-valid.py](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/verify-valid.py) | 新增：僅用於本票的合法報告與新程序逐位元組比較程式。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/vet.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/vet.json) | 新增：Go vet 的精確命令與退出結果。 |
| [evidence/OPS-03/nav2d-finite-config-20261007/vet.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/nav2d-finite-config-20261007/vet.log) | 新增：Go vet 的實際執行日誌。 |
| [experiment/nav2d/env.go](/Users/timlai/Developer/coimnet/experiment/nav2d/env.go) | 修改：四個浮點欄位拒絕 NaN 與正負無限大，保留合法值與錯誤順序。 |
| [experiment/nav2d/env_nonfinite_test.go](/Users/timlai/Developer/coimnet/experiment/nav2d/env_nonfinite_test.go) | 新增：環境公開入口的 12 組非法值、合法控制與既有錯誤次序回歸。 |
| [experiment/nav2d_nonfinite_test.go](/Users/timlai/Developer/coimnet/experiment/nav2d_nonfinite_test.go) | 新增：設定 Validate 與三個 Run 入口的非法值及合法完整流程回歸。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|
| 提交並推送實作 | origin/main | 6f2aa38172132398e1c910adbd353b2a55e4a286，遠端讀回一致。 |
| 執行 CLI 驗收 | 本機暫存目錄 `/var/folders/1m/qxhd6mg10158sb5ft6bltlwc0000gn/T/coimnet-config-cli-uc4zfhn2` | 八個流程通過。二進位、報告與日誌保留在 Git 外。 |

## Next

建議另票處理報告與呼叫端共用 Policies 切片，以及 Comparison.Interval=NaN 的驗證缺口。兩項 P2 均已記入 AGENTS.md，不影響本票四個環境欄位的完成條件。

💡 減法提醒：所有入口沿用既有共用驗證，只補四組檢查，不新增驗證器或依賴。
