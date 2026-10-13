# 本階段逐檔變更

基準：`2f7a79c723a8c33dd63675084b942581da91c547`。本表涵蓋框架、回歸、文件與全部新增證據。

| 檔案 | 變更摘要 |
| --- | --- |
| [AGENTS.md](../../../AGENTS.md) | 移除兩項已修復提醒，保留宣告變更的待釐清契約 |
| [ENG.md](../../../ENG.md) | 記錄拒絕報告、失敗狀態及更新後快取契約 |
| [delivery-status.md](../../../delivery-status.md) | 記錄本階段的驗證、清理與待交付狀態 |
| [docs/model-and-mechanisms.md](../../../docs/model-and-mechanisms.md) | 校正實際控制器訓練入口及狀態契約 |
| [docs/requirements-status.json](../../../docs/requirements-status.json) | 為四項需求追加本階段實證，保留 91 項既有狀態 |
| [docs/tickets/74-consolidation-refusal-counters.md](../../../docs/tickets/74-consolidation-refusal-counters.md) | 固定本階段根因、責任與驗收條件 |
| [docs/tickets/75-ocr-fixture-render-validation.md](../../../docs/tickets/75-ocr-fixture-render-validation.md) | 固定本階段根因、責任與驗收條件 |
| [docs/tickets/76-memory-controller-atomic-offset.md](../../../docs/tickets/76-memory-controller-atomic-offset.md) | 固定本階段根因、責任與驗收條件 |
| [docs/tickets/77-controller-update-cache-validity.md](../../../docs/tickets/77-controller-update-cache-validity.md) | 固定本階段根因、責任與驗收條件 |
| [evidence/MOD-06/refusal-input-audit-20261013/README.md](../../../evidence/MOD-06/refusal-input-audit-20261013/README.md) | 本階段範圍、根因、審查或驗收說明 |
| [evidence/MOD-06/refusal-input-audit-20261013/artifact-manifest.json](../../../evidence/MOD-06/refusal-input-audit-20261013/artifact-manifest.json) | 證據檔案大小與指紋索引，排除索引自身 |
| [evidence/MOD-06/refusal-input-audit-20261013/baseline.json](../../../evidence/MOD-06/refusal-input-audit-20261013/baseline.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/build.json](../../../evidence/MOD-06/refusal-input-audit-20261013/build.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/build.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/build.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/changed-files.md](../../../evidence/MOD-06/refusal-input-audit-20261013/changed-files.md) | 逐檔變更清單 |
| [evidence/MOD-06/refusal-input-audit-20261013/check.py.txt](../../../evidence/MOD-06/refusal-input-audit-20261013/check.py.txt) | 保存執行、證據核對或清理腳本原文 |
| [evidence/MOD-06/refusal-input-audit-20261013/cleanup.json](../../../evidence/MOD-06/refusal-input-audit-20261013/cleanup.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/cleanup.py.txt](../../../evidence/MOD-06/refusal-input-audit-20261013/cleanup.py.txt) | 保存執行、證據核對或清理腳本原文 |
| [evidence/MOD-06/refusal-input-audit-20261013/compatibility-after.json](../../../evidence/MOD-06/refusal-input-audit-20261013/compatibility-after.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/compatibility-after.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/compatibility-after.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/compatibility-before.json](../../../evidence/MOD-06/refusal-input-audit-20261013/compatibility-before.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/compatibility-before.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/compatibility-before.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/compatibility-result.json](../../../evidence/MOD-06/refusal-input-audit-20261013/compatibility-result.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility-after.json](../../../evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility-after.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility-after.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility-after.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility-before.json](../../../evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility-before.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility-before.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility-before.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility-result.json](../../../evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility-result.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility.go.txt](../../../evidence/MOD-06/refusal-input-audit-20261013/controller-compatibility.go.txt) | 保存相容性探針原文 |
| [evidence/MOD-06/refusal-input-audit-20261013/dependencies.json](../../../evidence/MOD-06/refusal-input-audit-20261013/dependencies.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/dependencies.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/dependencies.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/final-build.json](../../../evidence/MOD-06/refusal-input-audit-20261013/final-build.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/final-build.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/final-build.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/final-vet.json](../../../evidence/MOD-06/refusal-input-audit-20261013/final-vet.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/final-vet.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/final-vet.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/finalize.py.txt](../../../evidence/MOD-06/refusal-input-audit-20261013/finalize.py.txt) | 保存執行、證據核對或清理腳本原文 |
| [evidence/MOD-06/refusal-input-audit-20261013/findings.md](../../../evidence/MOD-06/refusal-input-audit-20261013/findings.md) | 本階段範圍、根因、審查或驗收說明 |
| [evidence/MOD-06/refusal-input-audit-20261013/format-final.json](../../../evidence/MOD-06/refusal-input-audit-20261013/format-final.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/format.json](../../../evidence/MOD-06/refusal-input-audit-20261013/format.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/full-race-recovered.json](../../../evidence/MOD-06/refusal-input-audit-20261013/full-race-recovered.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/full-race-recovered.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/full-race-recovered.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/full-race-serial.json](../../../evidence/MOD-06/refusal-input-audit-20261013/full-race-serial.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/full-race-serial.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/full-race-serial.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/full-race.json](../../../evidence/MOD-06/refusal-input-audit-20261013/full-race.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/full-race.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/full-race.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/full-unit.json](../../../evidence/MOD-06/refusal-input-audit-20261013/full-unit.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/full-unit.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/full-unit.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/governance-final.json](../../../evidence/MOD-06/refusal-input-audit-20261013/governance-final.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/governance-final.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/governance-final.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/governance-preclosure.json](../../../evidence/MOD-06/refusal-input-audit-20261013/governance-preclosure.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/governance-preclosure.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/governance-preclosure.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/green-consolidation.json](../../../evidence/MOD-06/refusal-input-audit-20261013/green-consolidation.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/green-consolidation.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/green-consolidation.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/green-controller-cache.json](../../../evidence/MOD-06/refusal-input-audit-20261013/green-controller-cache.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/green-controller-cache.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/green-controller-cache.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/green-memory-atomic.json](../../../evidence/MOD-06/refusal-input-audit-20261013/green-memory-atomic.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/green-memory-atomic.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/green-memory-atomic.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/green-ocr.json](../../../evidence/MOD-06/refusal-input-audit-20261013/green-ocr.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/green-ocr.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/green-ocr.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/helper-review.md](../../../evidence/MOD-06/refusal-input-audit-20261013/helper-review.md) | 本階段範圍、根因、審查或驗收說明 |
| [evidence/MOD-06/refusal-input-audit-20261013/helper-validation.json](../../../evidence/MOD-06/refusal-input-audit-20261013/helper-validation.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/ocr-compatibility.go.txt](../../../evidence/MOD-06/refusal-input-audit-20261013/ocr-compatibility.go.txt) | 保存相容性探針原文 |
| [evidence/MOD-06/refusal-input-audit-20261013/plan.md](../../../evidence/MOD-06/refusal-input-audit-20261013/plan.md) | 本階段範圍、根因、審查或驗收說明 |
| [evidence/MOD-06/refusal-input-audit-20261013/race-environment-failure.json](../../../evidence/MOD-06/refusal-input-audit-20261013/race-environment-failure.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/race-live-processes.json](../../../evidence/MOD-06/refusal-input-audit-20261013/race-live-processes.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/race-reschedule-termination.json](../../../evidence/MOD-06/refusal-input-audit-20261013/race-reschedule-termination.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/race-reschedule.json](../../../evidence/MOD-06/refusal-input-audit-20261013/race-reschedule.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/race-summary.json](../../../evidence/MOD-06/refusal-input-audit-20261013/race-summary.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-consolidation.json](../../../evidence/MOD-06/refusal-input-audit-20261013/red-consolidation.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-consolidation.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/red-consolidation.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-controller-cache.json](../../../evidence/MOD-06/refusal-input-audit-20261013/red-controller-cache.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-controller-cache.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/red-controller-cache.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-empty-batch.json](../../../evidence/MOD-06/refusal-input-audit-20261013/red-empty-batch.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-empty-batch.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/red-empty-batch.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-memory-atomic-distinct-row.json](../../../evidence/MOD-06/refusal-input-audit-20261013/red-memory-atomic-distinct-row.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-memory-atomic-distinct-row.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/red-memory-atomic-distinct-row.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-memory-atomic.json](../../../evidence/MOD-06/refusal-input-audit-20261013/red-memory-atomic.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-memory-atomic.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/red-memory-atomic.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-memory-nil.json](../../../evidence/MOD-06/refusal-input-audit-20261013/red-memory-nil.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-memory-nil.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/red-memory-nil.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-ocr-public-entrypoints.json](../../../evidence/MOD-06/refusal-input-audit-20261013/red-ocr-public-entrypoints.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-ocr-public-entrypoints.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/red-ocr-public-entrypoints.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-ocr.json](../../../evidence/MOD-06/refusal-input-audit-20261013/red-ocr.json) | 命令、環境、exit 與日誌指紋回條 |
| [evidence/MOD-06/refusal-input-audit-20261013/red-ocr.log.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/red-ocr.log.gz) | 保留原始命令日誌與失敗或成功結果 |
| [evidence/MOD-06/refusal-input-audit-20261013/root-review.md](../../../evidence/MOD-06/refusal-input-audit-20261013/root-review.md) | 本階段範圍、根因、審查或驗收說明 |
| [evidence/MOD-06/refusal-input-audit-20261013/source-manifest.json](../../../evidence/MOD-06/refusal-input-audit-20261013/source-manifest.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/targeted-summary.json](../../../evidence/MOD-06/refusal-input-audit-20261013/targeted-summary.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/temporary-inventory.json.gz](../../../evidence/MOD-06/refusal-input-audit-20261013/temporary-inventory.json.gz) | 保留已刪除暫存的逐檔指紋 |
| [evidence/MOD-06/refusal-input-audit-20261013/test-report.md](../../../evidence/MOD-06/refusal-input-audit-20261013/test-report.md) | 本階段範圍、根因、審查或驗收說明 |
| [evidence/MOD-06/refusal-input-audit-20261013/test-summary.json](../../../evidence/MOD-06/refusal-input-audit-20261013/test-summary.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/unit-summary.json](../../../evidence/MOD-06/refusal-input-audit-20261013/unit-summary.json) | 保存驗證結果、來源或清理證據 |
| [evidence/MOD-06/refusal-input-audit-20261013/verification.json](../../../evidence/MOD-06/refusal-input-audit-20261013/verification.json) | 保存驗證結果、來源或清理證據 |
| [learning/consolidation.go](../../../learning/consolidation.go) | 拒絕報告回傳既有預算與回合計數 |
| [learning/consolidation_report_test.go](../../../learning/consolidation_report_test.go) | 新增公開回歸，涵蓋失敗、接續或合法邊界 |
| [modulation/controller_objective.go](../../../modulation/controller_objective.go) | 成功更新清除舊快取，nil 梯度回錯 |
| [modulation/controller_update_cache_test.go](../../../modulation/controller_update_cache_test.go) | 新增公開回歸，涵蓋失敗、接續或合法邊界 |
| [modulation/memory_controller.go](../../../modulation/memory_controller.go) | 失敗前私有候選與有限值檢查，成功更新清除舊快取 |
| [modulation/memory_controller_atomic_test.go](../../../modulation/memory_controller_atomic_test.go) | 新增公開回歸，涵蓋失敗、接續或合法邊界 |
| [tasks/ocr/fixture.go](../../../tasks/ocr/fixture.go) | 入口拒絕非有限或超界背景與雜訊 |
| [tasks/ocr/fixture_validation_test.go](../../../tasks/ocr/fixture_validation_test.go) | 新增公開回歸，涵蓋失敗、接續或合法邊界 |
| [tasks/ocr/glyphs/options_validation_test.go](../../../tasks/ocr/glyphs/options_validation_test.go) | 新增公開回歸，涵蓋失敗、接續或合法邊界 |
| [tasks/ocr/glyphs/render.go](../../../tasks/ocr/glyphs/render.go) | 入口拒絕非有限或超界背景與雜訊 |
