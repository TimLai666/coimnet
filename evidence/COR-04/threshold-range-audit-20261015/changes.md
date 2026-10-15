# Ticket 103 完整交付檔案清單

本表列出本輪相對於原始提交的全部新增或修改檔案。模型、原始訓練資料與既有暫存目錄沒有納入變更。

比較基準：`2f34aec4822059857fae3a5748a0eaeff6eaee9c`
檔案數：191

| 檔案 | 變更摘要 |
|---|---|
| [AGENTS.md](../../../AGENTS.md) | 記錄原版已有的負向位移取下界契約差異。 |
| [ENG.md](../../../ENG.md) | 記錄極端跨度計算與最終閾值拒絕契約。 |
| [delivery-status.md](../../../delivery-status.md) | 更新 ticket 103 的驗收、清理、交付與下一個成果。 |
| [docs/requirements-status.json](../../../docs/requirements-status.json) | 為 COR-04 保留失敗及修復證據，恢復 91 項 passed。 |
| [docs/tickets/103-lif-threshold-range.md](../../../docs/tickets/103-lif-threshold-range.md) | 記錄五項驗收、Root 決策與實際交付結果。 |
| [dynamics/lif.go](../../../dynamics/lif.go) | 修正大範圍閾值與斜率，拒絕最終閾值溢位。 |
| [dynamics/lif_coupled.go](../../../dynamics/lif_coupled.go) | 修正化學耦合前向的大範圍閾值與斜率。 |
| [dynamics/lif_effective_threshold_range_test.go](../../../dynamics/lif_effective_threshold_range_test.go) | 新增適應／穩態閾值溢位與失敗狀態保留回歸。 |
| [dynamics/lif_segment.go](../../../dynamics/lif_segment.go) | 修正分段重算的閾值與斜率，拒絕最終閾值溢位。 |
| [dynamics/lif_state.go](../../../dynamics/lif_state.go) | 修正持續狀態的基礎閾值，拒絕最終閾值溢位。 |
| [dynamics/lif_threshold_range_test.go](../../../dynamics/lif_threshold_range_test.go) | 新增跨度溢位、正常範圍、飽和與非法輸入回歸。 |
| [dynamics/mixed.go](../../../dynamics/mixed.go) | 修正混合核心的閾值與斜率，拒絕最終閾值溢位。 |
| [dynamics/mixed_state.go](../../../dynamics/mixed_state.go) | 混合核心持續狀態拒絕最終閾值溢位。 |
| [evidence/COR-04/threshold-range-audit-20261015/README.md](../../../evidence/COR-04/threshold-range-audit-20261015/README.md) | 兩個缺陷、驗收、相容性、限制與交付報告。 |
| [evidence/COR-04/threshold-range-audit-20261015/adversarial-review-final.md](../../../evidence/COR-04/threshold-range-audit-20261015/adversarial-review-final.md) | 保存測試、程式或證據的審查結果與未驗證事項。 |
| [evidence/COR-04/threshold-range-audit-20261015/adversarial-review.md](../../../evidence/COR-04/threshold-range-audit-20261015/adversarial-review.md) | 保存測試、程式或證據的審查結果與未驗證事項。 |
| [evidence/COR-04/threshold-range-audit-20261015/agent-follow-up-change.json](../../../evidence/COR-04/threshold-range-audit-20261015/agent-follow-up-change.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/archive-logs.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/archive-logs.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/baseline.json](../../../evidence/COR-04/threshold-range-audit-20261015/baseline.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/before/lif.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/before/lif.go.txt) | 保存修正前的正式核心原始內容。 |
| [evidence/COR-04/threshold-range-audit-20261015/before/lif_coupled.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/before/lif_coupled.go.txt) | 保存修正前的正式核心原始內容。 |
| [evidence/COR-04/threshold-range-audit-20261015/before/lif_segment.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/before/lif_segment.go.txt) | 保存修正前的正式核心原始內容。 |
| [evidence/COR-04/threshold-range-audit-20261015/before/lif_state.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/before/lif_state.go.txt) | 保存修正前的正式核心原始內容。 |
| [evidence/COR-04/threshold-range-audit-20261015/before/mixed.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/before/mixed.go.txt) | 保存修正前的正式核心原始內容。 |
| [evidence/COR-04/threshold-range-audit-20261015/before/mixed_state.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/before/mixed_state.go.txt) | 保存修正前的正式核心原始內容。 |
| [evidence/COR-04/threshold-range-audit-20261015/build.json](../../../evidence/COR-04/threshold-range-audit-20261015/build.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/build.log](../../../evidence/COR-04/threshold-range-audit-20261015/build.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/cache-preparation.json](../../../evidence/COR-04/threshold-range-audit-20261015/cache-preparation.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/cache-prepare-origin.json](../../../evidence/COR-04/threshold-range-audit-20261015/cache-prepare-origin.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/cache-prepare.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/cache-prepare.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/candidate-v1-reconstruction.json](../../../evidence/COR-04/threshold-range-audit-20261015/candidate-v1-reconstruction.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/candidate-v1.patch.gz](../../../evidence/COR-04/threshold-range-audit-20261015/candidate-v1.patch.gz) | 逐位元組保留第一版候選的原始 patch。 |
| [evidence/COR-04/threshold-range-audit-20261015/changes.md](../../../evidence/COR-04/threshold-range-audit-20261015/changes.md) | 本輪全部交付檔案與變更摘要。 |
| [evidence/COR-04/threshold-range-audit-20261015/cleanup-inventory.json.gz](../../../evidence/COR-04/threshold-range-audit-20261015/cleanup-inventory.json.gz) | 清理前逐檔內容、連結與來源狀態的完整清單。 |
| [evidence/COR-04/threshold-range-audit-20261015/cleanup.json](../../../evidence/COR-04/threshold-range-audit-20261015/cleanup.json) | 保存清理前後核對、刪除範圍與保留物證據。 |
| [evidence/COR-04/threshold-range-audit-20261015/cleanup.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/cleanup.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/committed-source-check.json](../../../evidence/COR-04/threshold-range-audit-20261015/committed-source-check.json) | 固定該驗證階段的完整來源 SHA-256 清單。 |
| [evidence/COR-04/threshold-range-audit-20261015/compat-driver-correction.json](../../../evidence/COR-04/threshold-range-audit-20261015/compat-driver-correction.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/compat-driver-lock-final.json](../../../evidence/COR-04/threshold-range-audit-20261015/compat-driver-lock-final.json) | 固定測試、檢查程式或執行檔的來源指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/compat-driver-lock-reviewed.json](../../../evidence/COR-04/threshold-range-audit-20261015/compat-driver-lock-reviewed.json) | 固定測試、檢查程式或執行檔的來源指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/compat-driver-lock.json](../../../evidence/COR-04/threshold-range-audit-20261015/compat-driver-lock.json) | 固定測試、檢查程式或執行檔的來源指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/compatibility-archive.json](../../../evidence/COR-04/threshold-range-audit-20261015/compatibility-archive.json) | 保存封存成員、大小與逐位元組指紋核對。 |
| [evidence/COR-04/threshold-range-audit-20261015/compatibility-artifacts.tar.gz](../../../evidence/COR-04/threshold-range-audit-20261015/compatibility-artifacts.tar.gz) | 封存逐項相容性回條、日誌或小型人工恢復樣本。 |
| [evidence/COR-04/threshold-range-audit-20261015/compatibility.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/compatibility.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/core-compat-before-api-correction.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/core-compat-before-api-correction.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/core-compat-final.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/core-compat-final.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/core-compat-reviewed.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/core-compat-reviewed.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/core-compat.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/core-compat.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/delegation-dispatch.json](../../../evidence/COR-04/threshold-range-audit-20261015/delegation-dispatch.json) | 保存兩個原生派工的實際模型、effort 與接受回覆。 |
| [evidence/COR-04/threshold-range-audit-20261015/delegation-tool-failure.json](../../../evidence/COR-04/threshold-range-audit-20261015/delegation-tool-failure.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/delivery.json](../../../evidence/COR-04/threshold-range-audit-20261015/delivery.json) | 保存提交、正常推送、遠端讀回與完成條件。 |
| [evidence/COR-04/threshold-range-audit-20261015/effective-tests-frozen.json](../../../evidence/COR-04/threshold-range-audit-20261015/effective-tests-frozen.json) | 固定測試、檢查程式或執行檔的來源指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/effective-threshold-finding.json](../../../evidence/COR-04/threshold-range-audit-20261015/effective-threshold-finding.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/effective-threshold-probe.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/effective-threshold-probe.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/effective-threshold-probe.json](../../../evidence/COR-04/threshold-range-audit-20261015/effective-threshold-probe.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/effective-threshold-probe.log](../../../evidence/COR-04/threshold-range-audit-20261015/effective-threshold-probe.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/effective-threshold-test-worker-review.md](../../../evidence/COR-04/threshold-range-audit-20261015/effective-threshold-test-worker-review.md) | 保存測試、程式或證據的審查結果與未驗證事項。 |
| [evidence/COR-04/threshold-range-audit-20261015/engineering-log-archives.json](../../../evidence/COR-04/threshold-range-audit-20261015/engineering-log-archives.json) | 保存封存成員、大小與逐位元組指紋核對。 |
| [evidence/COR-04/threshold-range-audit-20261015/engineering.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/engineering.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/evidence-doc-review.md](../../../evidence/COR-04/threshold-range-audit-20261015/evidence-doc-review.md) | 保存測試、程式或證據的審查結果與未驗證事項。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-target-summary.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-target-summary.json) | 保存測試事件、套件或數值對照的計數。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/build.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/build.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/build.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/build.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/cli-build.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/cli-build.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/cli-build.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/cli-build.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/cli-lif-threshold.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/cli-lif-threshold.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/cli-lif-threshold.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/cli-lif-threshold.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/compat-driver-lock-reviewed.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/compat-driver-lock-reviewed.json) | 固定測試、檢查程式或執行檔的來源指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/compatibility-archive.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/compatibility-archive.json) | 保存封存成員、大小與逐位元組指紋核對。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/compatibility-artifacts.tar.gz](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/compatibility-artifacts.tar.gz) | 封存逐項相容性回條、日誌或小型人工恢復樣本。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/compatibility.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/compatibility.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/dependency-check.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/dependency-check.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/engineering.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/engineering.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/format.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/format.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/full-race-summary.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/full-race-summary.json) | 保存測試事件、套件或數值對照的計數。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/full-race.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/full-race.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/full-race.log.gz](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/full-race.log.gz) | 逐位元組保留完整工程命令的壓縮原始日誌。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/full-unit-summary.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/full-unit-summary.json) | 保存測試事件、套件或數值對照的計數。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/full-unit.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/full-unit.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/full-unit.log.gz](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/full-unit.log.gz) | 逐位元組保留完整工程命令的壓縮原始日誌。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-binary-build.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-binary-build.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-binary-build.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-binary-build.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-final-validated.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-final-validated.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-final-validated.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-final-validated.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-post-cleanup.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-post-cleanup.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-post-cleanup.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/governance-post-cleanup.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/mod-verify.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/mod-verify.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/mod-verify.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/mod-verify.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/modulation-floor-current.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/modulation-floor-current.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/modulation-floor-current.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/modulation-floor-current.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/modulation-floor-original.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/modulation-floor-original.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/modulation-floor-original.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/modulation-floor-original.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/module-list.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/module-list.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/module-list.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/module-list.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/normal-compatibility.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/normal-compatibility.json) | 保存新舊來源的完整數值及跨程序相容性核對。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/overlay.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/overlay.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/root-green-race.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/root-green-race.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/root-green-race.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/root-green-race.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/run-command.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/run-command.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/source-before.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/source-before.json) | 固定該驗證階段的完整來源 SHA-256 清單。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/source-fixed.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/source-fixed.json) | 固定該驗證階段的完整來源 SHA-256 清單。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/vet.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/vet.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/vet.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/vet.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-begin.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-begin.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-begin.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-begin.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-build.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-build.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-build.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-build.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-continue-previous-candidate.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-continue-previous-candidate.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-continue-previous-candidate.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-continue-previous-candidate.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-continue.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-continue.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-continue.log](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-continue.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-fixtures.tar.gz](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-fixtures.tar.gz) | 封存逐項相容性回條、日誌或小型人工恢復樣本。 |
| [evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-recovery.json](../../../evidence/COR-04/threshold-range-audit-20261015/final-validation/wide-recovery.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/fix-commit-checks.json](../../../evidence/COR-04/threshold-range-audit-20261015/fix-commit-checks.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/format.json](../../../evidence/COR-04/threshold-range-audit-20261015/format.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/full-race-interrupted.json](../../../evidence/COR-04/threshold-range-audit-20261015/full-race-interrupted.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/full-race.log.gz](../../../evidence/COR-04/threshold-range-audit-20261015/full-race.log.gz) | 逐位元組保留完整工程命令的壓縮原始日誌。 |
| [evidence/COR-04/threshold-range-audit-20261015/full-unit.json](../../../evidence/COR-04/threshold-range-audit-20261015/full-unit.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/full-unit.log.gz](../../../evidence/COR-04/threshold-range-audit-20261015/full-unit.log.gz) | 逐位元組保留完整工程命令的壓縮原始日誌。 |
| [evidence/COR-04/threshold-range-audit-20261015/go-env.json](../../../evidence/COR-04/threshold-range-audit-20261015/go-env.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/go-env.log](../../../evidence/COR-04/threshold-range-audit-20261015/go-env.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/governance-binary-lock.json](../../../evidence/COR-04/threshold-range-audit-20261015/governance-binary-lock.json) | 固定測試、檢查程式或執行檔的來源指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/modulation-floor-follow-up.json](../../../evidence/COR-04/threshold-range-audit-20261015/modulation-floor-follow-up.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/modulation-floor-probe.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/modulation-floor-probe.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/module-selection.log](../../../evidence/COR-04/threshold-range-audit-20261015/module-selection.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/normal-compatibility.json](../../../evidence/COR-04/threshold-range-audit-20261015/normal-compatibility.json) | 保存新舊來源的完整數值及跨程序相容性核對。 |
| [evidence/COR-04/threshold-range-audit-20261015/plan.md](../../../evidence/COR-04/threshold-range-audit-20261015/plan.md) | 保存測試、程式或證據的審查結果與未驗證事項。 |
| [evidence/COR-04/threshold-range-audit-20261015/post-cleanup-governance.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/post-cleanup-governance.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/pre-delivery-check.json](../../../evidence/COR-04/threshold-range-audit-20261015/pre-delivery-check.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/probe-before-input-shape-fix.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/probe-before-input-shape-fix.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/probe-input-shape-error.json](../../../evidence/COR-04/threshold-range-audit-20261015/probe-input-shape-error.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/probe-red.json](../../../evidence/COR-04/threshold-range-audit-20261015/probe-red.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/probe.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/probe.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/protected-models.json](../../../evidence/COR-04/threshold-range-audit-20261015/protected-models.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/red-verification.json](../../../evidence/COR-04/threshold-range-audit-20261015/red-verification.json) | 保存需求重現、環境與實際觀察結果。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-controls.json](../../../evidence/COR-04/threshold-range-audit-20261015/root-controls.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-controls.log](../../../evidence/COR-04/threshold-range-audit-20261015/root-controls.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-effective-red.json](../../../evidence/COR-04/threshold-range-audit-20261015/root-effective-red.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-effective-red.log](../../../evidence/COR-04/threshold-range-audit-20261015/root-effective-red.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-effective-test-review.json](../../../evidence/COR-04/threshold-range-audit-20261015/root-effective-test-review.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-final-green.json](../../../evidence/COR-04/threshold-range-audit-20261015/root-final-green.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-final-green.log](../../../evidence/COR-04/threshold-range-audit-20261015/root-final-green.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-green-race.json](../../../evidence/COR-04/threshold-range-audit-20261015/root-green-race.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-green-race.log](../../../evidence/COR-04/threshold-range-audit-20261015/root-green-race.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-green.json](../../../evidence/COR-04/threshold-range-audit-20261015/root-green.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-green.log](../../../evidence/COR-04/threshold-range-audit-20261015/root-green.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-independent-compatibility-review.json](../../../evidence/COR-04/threshold-range-audit-20261015/root-independent-compatibility-review.json) | 保存新舊來源的完整數值及跨程序相容性核對。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-red-final.json](../../../evidence/COR-04/threshold-range-audit-20261015/root-red-final.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-red-final.log](../../../evidence/COR-04/threshold-range-audit-20261015/root-red-final.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-red-summary.json](../../../evidence/COR-04/threshold-range-audit-20261015/root-red-summary.json) | 保存測試事件、套件或數值對照的計數。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-red.json](../../../evidence/COR-04/threshold-range-audit-20261015/root-red.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-red.log](../../../evidence/COR-04/threshold-range-audit-20261015/root-red.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-review.md](../../../evidence/COR-04/threshold-range-audit-20261015/root-review.md) | 保存測試、程式或證據的審查結果與未驗證事項。 |
| [evidence/COR-04/threshold-range-audit-20261015/root-test-review.json](../../../evidence/COR-04/threshold-range-audit-20261015/root-test-review.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/run-command.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/run-command.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/smoke-baseline.json](../../../evidence/COR-04/threshold-range-audit-20261015/smoke-baseline.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/smoke-cleanup.json](../../../evidence/COR-04/threshold-range-audit-20261015/smoke-cleanup.json) | 保存清理前後核對、刪除範圍與保留物證據。 |
| [evidence/COR-04/threshold-range-audit-20261015/smoke-cleanup.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/smoke-cleanup.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/source-before.json](../../../evidence/COR-04/threshold-range-audit-20261015/source-before.json) | 固定該驗證階段的完整來源 SHA-256 清單。 |
| [evidence/COR-04/threshold-range-audit-20261015/source-effective-red.json](../../../evidence/COR-04/threshold-range-audit-20261015/source-effective-red.json) | 固定該驗證階段的完整來源 SHA-256 清單。 |
| [evidence/COR-04/threshold-range-audit-20261015/source-final.json](../../../evidence/COR-04/threshold-range-audit-20261015/source-final.json) | 固定該驗證階段的完整來源 SHA-256 清單。 |
| [evidence/COR-04/threshold-range-audit-20261015/source-fixed.json](../../../evidence/COR-04/threshold-range-audit-20261015/source-fixed.json) | 固定該驗證階段的完整來源 SHA-256 清單。 |
| [evidence/COR-04/threshold-range-audit-20261015/source-red-final.json](../../../evidence/COR-04/threshold-range-audit-20261015/source-red-final.json) | 固定該驗證階段的完整來源 SHA-256 清單。 |
| [evidence/COR-04/threshold-range-audit-20261015/source-red.json](../../../evidence/COR-04/threshold-range-audit-20261015/source-red.json) | 固定該驗證階段的完整來源 SHA-256 清單。 |
| [evidence/COR-04/threshold-range-audit-20261015/staged-format-correction.json](../../../evidence/COR-04/threshold-range-audit-20261015/staged-format-correction.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/summarize.py.txt](../../../evidence/COR-04/threshold-range-audit-20261015/summarize.py.txt) | 保存本輪驗證、證據整理或受保護清理的腳本。 |
| [evidence/COR-04/threshold-range-audit-20261015/test-worker-review.md](../../../evidence/COR-04/threshold-range-audit-20261015/test-worker-review.md) | 保存測試、程式或證據的審查結果與未驗證事項。 |
| [evidence/COR-04/threshold-range-audit-20261015/tests-frozen-final.json](../../../evidence/COR-04/threshold-range-audit-20261015/tests-frozen-final.json) | 固定測試、檢查程式或執行檔的來源指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/tests-frozen.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/tests-frozen.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/tests-frozen.json](../../../evidence/COR-04/threshold-range-audit-20261015/tests-frozen.json) | 固定測試、檢查程式或執行檔的來源指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/threshold-transform-inventory.json](../../../evidence/COR-04/threshold-range-audit-20261015/threshold-transform-inventory.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/trainer-compat.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/trainer-compat.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/verification.json](../../../evidence/COR-04/threshold-range-audit-20261015/verification.json) | 保存需求重現、環境與實際觀察結果。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-begin-final.json](../../../evidence/COR-04/threshold-range-audit-20261015/wide-begin-final.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-begin-final.log](../../../evidence/COR-04/threshold-range-audit-20261015/wide-begin-final.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-begin.json](../../../evidence/COR-04/threshold-range-audit-20261015/wide-begin.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-begin.log](../../../evidence/COR-04/threshold-range-audit-20261015/wide-begin.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-build.json](../../../evidence/COR-04/threshold-range-audit-20261015/wide-build.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-build.log](../../../evidence/COR-04/threshold-range-audit-20261015/wide-build.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-continue-repeat.json](../../../evidence/COR-04/threshold-range-audit-20261015/wide-continue-repeat.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-continue-repeat.log](../../../evidence/COR-04/threshold-range-audit-20261015/wide-continue-repeat.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-continue.json](../../../evidence/COR-04/threshold-range-audit-20261015/wide-continue.json) | 保存實際命令、環境、結束狀態與輸出指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-continue.log](../../../evidence/COR-04/threshold-range-audit-20261015/wide-continue.log) | 保存命令的原始輸出。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-driver-lock.json](../../../evidence/COR-04/threshold-range-audit-20261015/wide-driver-lock.json) | 固定測試、檢查程式或執行檔的來源指紋。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-fixtures.tar.gz](../../../evidence/COR-04/threshold-range-audit-20261015/wide-fixtures.tar.gz) | 封存逐項相容性回條、日誌或小型人工恢復樣本。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-probe-setup-correction.json](../../../evidence/COR-04/threshold-range-audit-20261015/wide-probe-setup-correction.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-restore-verification.json](../../../evidence/COR-04/threshold-range-audit-20261015/wide-restore-verification.json) | 保存需求重現、環境與實際觀察結果。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-restore.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/wide-restore.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/wide-saved-before-repeat.json](../../../evidence/COR-04/threshold-range-audit-20261015/wide-saved-before-repeat.json) | 保存此項重現、校正、範圍或數值檢查的實際紀錄。 |
| [evidence/COR-04/threshold-range-audit-20261015/worker-effective-tests-before-review.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/worker-effective-tests-before-review.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
| [evidence/COR-04/threshold-range-audit-20261015/worker-test-before-review.go.txt](../../../evidence/COR-04/threshold-range-audit-20261015/worker-test-before-review.go.txt) | 保存本輪重現、參考計算或相容性檢查的 Go 程式。 |
