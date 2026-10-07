# 導航報告設定隔離的交付清單

Problem：取得導航報告後改呼叫端策略清單，會改到既有報告設定，使保存的設定與雜湊不一致。四任務報告也有相同問題。

Done：共同的 RunNav2D 入口複製策略清單，單任務、呼叫端及四任務報告各自持有種子與策略清單。實作 1b088e42151941c132a38d870ff64e9ac8517cb7 已推送 origin/main，遠端查核相同，見[交付紀錄](delivery.json)。

## Changed

相對基準 974afa3421a8d2d415026d02d3284fbb0aca38c4，本票共 86 個新增或修改檔案。沒有移動或刪除檔案。

| 檔案 | 變更摘要 |
|---|---|
| [AGENTS.md](../../../AGENTS.md) | 移除已修正的策略清單共享提醒，保留其他待辦。 |
| [ENG.md](../../../ENG.md) | 記錄共同入口的種子與策略清單隔離契約。 |
| [README.md](../../../README.md) | 說明取得導航報告後的設定修改隔離行為。 |
| [delivery-status.md](../../../delivery-status.md) | 更新 ticket 49 完成狀態、實際交付與下一個可驗證缺口。 |
| [docs/INDEX.md](../../../docs/INDEX.md) | 票號索引延伸至 49。 |
| [docs/requirements-status.json](../../../docs/requirements-status.json) | 為 OPS-03、TSK-08 加入本票證據，需求狀態不變。 |
| [docs/tickets/49-nav2d-report-config-ownership.md](../../../docs/tickets/49-nav2d-report-config-ownership.md) | 記錄問題、共用入口、失敗處理、分工與五項已驗證驗收。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/api-doc.json](api-doc.json) | 公開說明的命令與結果回條，逐字保留原始輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/api-doc.log](api-doc.log) | 公開說明輸出，移除末尾多餘空行，原文保存在 api-doc.json。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/baseline.json](baseline.json) | 保存基準提交、原件指紋與可改範圍。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/build.json](build.json) | 完整建置的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/build.log](build.log) | 完整建置的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/cli-result.json](cli-result.json) | 核對八個真實 CLI 流程、六份報告與目前來源指紋。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/cli-workflow.json](cli-workflow.json) | 八個真實 CLI 流程的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/cli-workflow.log](cli-workflow.log) | 八個真實 CLI 流程的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/contract-discovery.json](contract-discovery.json) | 保存共同入口與相關消費者的索引盤點及追蹤。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/delegation.json](delegation.json) | 記錄實際模型、旗標、工具限制與備案。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/delivery.json](delivery.json) | 保存實作提交、推送輸出及遠端相同的查核。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/dependency-result.json](dependency-result.json) | 核對固定 Insyra 版本與依賴檔指紋。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/diff-check.json](diff-check.json) | 文字差異空白檢查的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/diff-check.log](diff-check.log) | 文字差異空白檢查的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/entry-documents.json](entry-documents.json) | 核對接手文件的讀取與保留原文。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/environment.json](environment.json) | 記錄本機 Go、作業系統與執行設定。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/evidence-audit.json](evidence-audit.json) | 獨立核對命令、結果數量、雜湊與 CLI 證據。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/format.json](format.json) | 兩份修改 Go 檔案格式檢查的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/format.log](format.log) | 兩份修改 Go 檔案格式檢查的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/general.json](general.json) | 完整一般測試，57 套件的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/general.log](general.log) | 完整一般測試，57 套件的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/governance-completion.json](governance-completion.json) | 完成文件更新後的八個治理測試的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/governance-completion.log](governance-completion.log) | 完成文件更新後的八個治理測試的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/governance-ready.json](governance-ready.json) | 實作交付前的治理的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/governance-ready.log](governance-ready.log) | 實作交付前的治理的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/governance.json](governance.json) | 需求與格式索引治理的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/governance.log](governance.log) | 需求與格式索引治理的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/host-load-observation.json](host-load-observation.json) | 保存測試期間的主機負載觀察，沒有資源設定修改。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/inventory.md](inventory.md) | 提供本票每個實際檔案的變更摘要與操作紀錄。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/isolated-audit.json](isolated-audit.json) | 查核 OpenCode 實際讀取範圍、目錄不符及子程序停止。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/isolated-contract.go.txt](isolated-contract.go.txt) | 保存派工使用的人工契約。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/isolated-dispatch.json](isolated-dispatch.json) | 保存人工契約派工命令與退出結果。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/isolated-output.jsonl](isolated-output.jsonl) | 保存 OpenCode 原始事件與工具輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/isolated-prompt.txt](isolated-prompt.txt) | 保存具有七項要求的原派工內容。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/isolated-stderr.log](isolated-stderr.log) | 保存人工契約派工的標準錯誤輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/mod-verify.json](mod-verify.json) | 全部 Go 模組驗證的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/mod-verify.log](mod-verify.log) | 全部 Go 模組驗證的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/model-discovery.json](model-discovery.json) | 實際可用 OpenCode 免費模型的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/model-discovery.log](model-discovery.log) | 實際可用 OpenCode 免費模型的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/ownership-fixed.json](ownership-fixed.json) | 修正後三個清單隔離回歸的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/ownership-fixed.log](ownership-fixed.log) | 修正後三個清單隔離回歸的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/ownership-green.json](ownership-green.json) | 檔名保留的第二次 RED，退出碼 1，並非通過結果的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/ownership-green.log](ownership-green.log) | 檔名保留的第二次 RED，退出碼 1，並非通過結果的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/plan.json](plan.json) | 固定五項完成條件、檔案責任與排除範圍。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/precommit-review.json](precommit-review.json) | 保存實作提交前的逐檔指紋與 Root 審查。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/preservation.json](preservation.json) | 核對 654 份 Go 來源及 3,321 份保護原件。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/protected-before-delivery.json](protected-before-delivery.json) | 保存交付前保護原件指紋的核對。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/race-process-progress.json](race-process-progress.json) | 保存競態檢查執行時的程序觀察。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/race.json](race.json) | 完整競態檢查，57 套件的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/race.log](race.log) | 完整競態檢查，57 套件的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/red.json](red.json) | Root 重現的三個修正前失敗的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/red.log](red.log) | Root 重現的三個修正前失敗的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/root-review.md](root-review.md) | 記錄 Root 對完整修改、測試、契約與相容性的審查。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/run-check.py](run-check.py) | 以真實退出碼保存驗證命令與輸出回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/scope-interim.json](scope-interim.json) | 核對階段中途的檔案範圍。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/scoped-result.json](scoped-result.json) | 盤點實際通過的 20 個相關頂層測試。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/scoped.json](scoped.json) | 20 個相關頂層測試的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/scoped.log](scoped.log) | 20 個相關頂層測試的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/source-freeze.json](source-freeze.json) | 凍結 654 份 Go 來源與測試的 SHA-256。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/test-freeze.json](test-freeze.json) | 記錄先失敗回歸與測試凍結，保留重複 RED 的說明。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/tidy.json](tidy.json) | 離線相依性整理差異的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/tidy.log](tidy.log) | 離線相依性整理差異的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/valid-after-report.json](valid-after-report.json) | 保存修正後八份完整人工配置報告。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/valid-after-report.stderr.log](valid-after-report.stderr.log) | 保存修正後報告程序的標準錯誤。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/valid-before-command.json](valid-before-command.json) | 保存基準報告的實際命令與退出碼。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/valid-before.json](valid-before.json) | 保存修正前八份完整人工配置報告。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/valid-before.stderr.log](valid-before.stderr.log) | 保存基準報告程序的標準錯誤。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/valid-comparison.json](valid-comparison.json) | 核對八份報告與 24 筆結果逐位元組相同。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/valid-probe.go.txt](valid-probe.go.txt) | 保存合法報告對照的固定人工配置程式。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/valid-repeat-report.json](valid-repeat-report.json) | 保存第二個新程序產生的完整報告。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/valid-repeat-report.stderr.log](valid-repeat-report.stderr.log) | 保存重現報告程序的標準錯誤。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/valid-workflow.json](valid-workflow.json) | 固定合法配置的前後與跨程序對照的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/valid-workflow.log](valid-workflow.log) | 固定合法配置的前後與跨程序對照的實際輸出。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/verification.json](verification.json) | 整合命令、環境、指紋、實際結果、日誌與交付狀態。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/verify-valid.py](verify-valid.py) | 比較修正前後及兩個新程序的完整報告與雜湊。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/vet.json](vet.json) | 完整靜態檢查的命令與結果回條。 |
| [evidence/OPS-03/nav2d-policy-ownership-20261008/vet.log](vet.log) | 完整靜態檢查的實際輸出。 |
| [experiment/nav2d_ownership_test.go](../../../experiment/nav2d_ownership_test.go) | 新增單任務、四任務及追加容量的雙向修改回歸。 |
| [experiment/nav2d_suite.go](../../../experiment/nav2d_suite.go) | 在已有種子複製處複製策略清單，補公開契約註解。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|
| 提交、推送及遠端查核 | CoImNet origin/main | 實作 1b088e4 的本機與遠端 SHA 相同。完成紀錄隨後單獨提交，另核對遠端。 |
| 真實 CLI 驗證 | 本機暫存目錄 coimnet-config-cli-w7s90bl8 | 八個流程通過。二進位及人工報告保留於 cli-result.json 記錄的完整路徑。 |
| 停止錯誤目錄的派工子程序 | OpenCode big-pickle PID 88792 | 只停止本次子程序，實際退出碼 -15。改用 Luna max 只新增指定測試。 |

## Verified

三個新回歸在修正前失敗，修正後通過。七個種子隔離控制在修正前已通過。20 個相關頂層測試、完整一般及競態檢查各 57 套件通過，沒有採用快取結果。建置、靜態檢查、格式、相依性與完成文件更新後的八個治理測試通過。

八份完整合法報告、24 筆結果在兩個新程序中與基準逐位元組相同。八個真實 CLI 流程通過。654 份凍結 Go 來源及 3,321 份保護原件 SHA-256 相符。命令、環境、指紋及原始日誌見[工程驗證](verification.json)。

## Notes

本票驗證軟體行為，沒有新增任務學習或生物機制證據。整體需求為 89／91，TSK-11、OPS-05 保留原狀。清單隔離適用於回傳後的修改，執行期間並行改輸入不在此契約內。報告本身的設定可修改，修改後不會自動重算雜湊。

OpenCode 使用 --pure --agent build --model opencode/big-pickle --format json，實際錯誤為 File not found: /Users/timlai/Developer/coimnet/contract.go，並誤讀 repo 的 go.mod 及檔名。停止後由 collaboration.spawn_agent 的 gpt-5.6-luna、reasoning_effort=max 完成指定測試。實際事件見[派工查核](isolated-audit.json)與[模型紀錄](delegation.json)。

## Next

建議接續處理 AGENTS.md 已確認的 Comparison.Interval 非有限值驗證（P2），在執行前拒絕無法保存的比較設定。

💡 減法提醒：在已有清單複製處補一行即可，四任務沿用共同入口。
