# 本輪檔案變更

本表逐檔列出 ticket 114 交付內容。既有 AGENTS.md 與兩組其他工作證據不在交付範圍。

| 檔案 | 變更摘要 |
|---|---|
| [ENG.md](/Users/timlai/Developer/coimnet/ENG.md) | 記錄適應梯度及例外數值恢復契約。 |
| [delivery-status.md](/Users/timlai/Developer/coimnet/delivery-status.md) | 記錄本票驗收、清理與交付狀態。 |
| [docs/requirements-status.json](/Users/timlai/Developer/coimnet/docs/requirements-status.json) | 連結 COR-04／09 的實際重現與驗收證據。 |
| [docs/tickets/11-lif-core.md](/Users/timlai/Developer/coimnet/docs/tickets/11-lif-core.md) | 修正適應狀態反向被誤稱為常數的矛盾文字。 |
| [docs/tickets/114-lif-event-adjoint.md](/Users/timlai/Developer/coimnet/docs/tickets/114-lif-event-adjoint.md) | 記錄根因、驗收條件及五項完成狀態。 |
| [docs/tickets/115-surrogate-extreme-audit.md](/Users/timlai/Developer/coimnet/docs/tickets/115-surrogate-extreme-audit.md) | 排定下一輪公開重現與公開註解核對，尚未實作。 |
| [docs/tickets/16-lif-individual-and-model-package.md](/Users/timlai/Developer/coimnet/docs/tickets/16-lif-individual-and-model-package.md) | 區分適應狀態跨時間傳遞與慢速穩定常數。 |
| [dynamics/coupled_reverse.go](/Users/timlai/Developer/coimnet/dynamics/coupled_reverse.go) | LIF 化學耦合反向檢查最終事件梯度，恢復可表示結果。 |
| [dynamics/event_adjoint.go](/Users/timlai/Developer/coimnet/dynamics/event_adjoint.go) | 新增事件乘加梯度的私有精確恢復。 |
| [dynamics/event_adjoint_test.go](/Users/timlai/Developer/coimnet/dynamics/event_adjoint_test.go) | 凍結八個失敗案例及停用、截斷與真正溢位控制。 |
| [dynamics/lif.go](/Users/timlai/Developer/coimnet/dynamics/lif.go) | 保留正常 du 與閾值累加，只在非有限事件梯度時恢復。 |
| [dynamics/lif_recompute.go](/Users/timlai/Developer/coimnet/dynamics/lif_recompute.go) | 分段重算沿用相同的事件恢復及正常捨入。 |
| [dynamics/mixed.go](/Users/timlai/Developer/coimnet/dynamics/mixed.go) | 混合核心的 LIF 節點採相同例外恢復。 |
| [dynamics/mixed_coupled.go](/Users/timlai/Developer/coimnet/dynamics/mixed_coupled.go) | 混合化學耦合反向保留有限值檢查並恢復事件梯度。 |
| [evidence/COR-09/event-adjoint-20261016/README.md](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/README.md) | 記錄 README.md 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/baseline-commands.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/baseline-commands.py.txt) | 保存 baseline-commands.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/baseline.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/baseline.json) | 記錄 baseline.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/before-source.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/before-source.json) | 記錄 before-source.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/candidate-commands.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/candidate-commands.py.txt) | 保存 candidate-commands.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/candidate-final-commands.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/candidate-final-commands.py.txt) | 保存 candidate-final-commands.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/candidate-initial-rounding.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/candidate-initial-rounding.json) | 記錄 candidate-initial-rounding.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/candidate-initial-source.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/candidate-initial-source.json) | 記錄 candidate-initial-source.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/candidate-verification.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/candidate-verification.json) | 記錄 candidate-verification.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/changed-files.md](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/changed-files.md) | 逐檔列出本輪交付範圍。 |
| [evidence/COR-09/event-adjoint-20261016/changed-files.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/changed-files.py.txt) | 保存 changed-files.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-before-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-before-race.json) | 記錄 chemical-resume-before-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-before-race.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-before-race.log) | 保留 chemical-resume-before-race.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-before.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-before.json) | 記錄 chemical-resume-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-before.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-before.log) | 保留 chemical-resume-before.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-final-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-final-race.json) | 記錄 chemical-resume-candidate-final-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-final-race.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-final-race.log) | 保留 chemical-resume-candidate-final-race.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-final.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-final.json) | 記錄 chemical-resume-candidate-final.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-final.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-final.log) | 保留 chemical-resume-candidate-final.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-race.json) | 記錄 chemical-resume-candidate-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-race.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate-race.log) | 保留 chemical-resume-candidate-race.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate.json) | 記錄 chemical-resume-candidate.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/chemical-resume-candidate.log) | 保留 chemical-resume-candidate.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/cleanup-inventory.json.gz](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/cleanup-inventory.json.gz) | 保存刪除前逐項內容與指紋。 |
| [evidence/COR-09/event-adjoint-20261016/cleanup-owned.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/cleanup-owned.py.txt) | 保存 cleanup-owned.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/cleanup.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/cleanup.json) | 記錄 cleanup.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/closure-binary.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/closure-binary.json) | 記錄 closure-binary.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/command-environment.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/command-environment.json) | 記錄 command-environment.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-before-corrected.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-before-corrected.json) | 記錄 core-before-corrected.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-before-corrected.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-before-corrected.log) | 保留 core-before-corrected.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/core-before-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-before-race.json) | 記錄 core-before-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-before-race.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-before-race.log) | 保留 core-before-race.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/core-before.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-before.json) | 記錄 core-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-before.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-before.log) | 保留 core-before.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/core-candidate-final-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-candidate-final-race.json) | 記錄 core-candidate-final-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-candidate-final-race.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-candidate-final-race.log) | 保留 core-candidate-final-race.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/core-candidate-final.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-candidate-final.json) | 記錄 core-candidate-final.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-candidate-final.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-candidate-final.log) | 保留 core-candidate-final.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/core-candidate-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-candidate-race.json) | 記錄 core-candidate-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-candidate-race.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-candidate-race.log) | 保留 core-candidate-race.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/core-candidate.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-candidate.json) | 記錄 core-candidate.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-candidate.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-candidate.log) | 保留 core-candidate.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/core-old-state-resume-final-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-old-state-resume-final-race.json) | 記錄 core-old-state-resume-final-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-old-state-resume-final-race.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-old-state-resume-final-race.log) | 保留 core-old-state-resume-final-race.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/core-old-state-resume-final.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-old-state-resume-final.json) | 記錄 core-old-state-resume-final.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-old-state-resume-final.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-old-state-resume-final.log) | 保留 core-old-state-resume-final.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/core-old-state-resume-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-old-state-resume-race.json) | 記錄 core-old-state-resume-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-old-state-resume-race.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-old-state-resume-race.log) | 保留 core-old-state-resume-race.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/core-old-state-resume.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-old-state-resume.json) | 記錄 core-old-state-resume.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/core-old-state-resume.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/core-old-state-resume.log) | 保留 core-old-state-resume.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/delivery.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/delivery.json) | 記錄 delivery.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-build.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-build.json) | 記錄 engineering-build.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-build.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-build.log) | 保留 engineering-build.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-closure-binary.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-closure-binary.json) | 記錄 engineering-closure-binary.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-closure-binary.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-closure-binary.log) | 保留 engineering-closure-binary.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-format.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-format.json) | 記錄 engineering-format.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-format.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-format.log) | 保留 engineering-format.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-governance.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-governance.json) | 記錄 engineering-governance.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-governance.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-governance.log) | 保留 engineering-governance.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-module-selection.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-module-selection.json) | 記錄 engineering-module-selection.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-module-selection.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-module-selection.log) | 保留 engineering-module-selection.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-module-verify.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-module-verify.json) | 記錄 engineering-module-verify.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-module-verify.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-module-verify.log) | 保留 engineering-module-verify.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-race.json) | 記錄 engineering-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-race.log.gz](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-race.log.gz) | 保留 engineering-race.log.gz 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-resume.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-resume.py.txt) | 保存 engineering-resume.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-unit-retry.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-unit-retry.json) | 記錄 engineering-unit-retry.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-unit-retry.log.gz](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-unit-retry.log.gz) | 保留 engineering-unit-retry.log.gz 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-unit.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-unit.json) | 記錄 engineering-unit.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-unit.log.gz](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-unit.log.gz) | 保留 engineering-unit.log.gz 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-verification.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-verification.json) | 記錄 engineering-verification.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-vet.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-vet.json) | 記錄 engineering-vet.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/engineering-vet.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering-vet.log) | 保留 engineering-vet.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/engineering.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/engineering.py.txt) | 保存 engineering.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/environment.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/environment.json) | 記錄 environment.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/exact-reference.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/exact-reference.json) | 記錄 exact-reference.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/final-governance-retry.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/final-governance-retry.json) | 記錄 final-governance-retry.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/final-governance-retry.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/final-governance-retry.log) | 保留 final-governance-retry.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/final-governance-retry.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/final-governance-retry.py.txt) | 保存 final-governance-retry.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/final-governance.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/final-governance.json) | 記錄 final-governance.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/final-governance.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/final-governance.log) | 保留 final-governance.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/final-governance.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/final-governance.py.txt) | 保存 final-governance.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/finish-engineering.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/finish-engineering.py.txt) | 保存 finish-engineering.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/foreign-before.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/foreign-before.json) | 記錄 foreign-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/formal-source.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/formal-source.json) | 記錄 formal-source.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/frozen-test-initial.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/frozen-test-initial.json) | 記錄 frozen-test-initial.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/frozen-test.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/frozen-test.json) | 記錄 frozen-test.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/go-version.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/go-version.json) | 記錄 go-version.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/go-version.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/go-version.log) | 保留 go-version.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/opencode-models.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/opencode-models.json) | 記錄 opencode-models.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/opencode-models.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/opencode-models.log) | 保留 opencode-models.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/plan.md](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/plan.md) | 記錄 plan.md 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/prepare-final-tools.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/prepare-final-tools.py.txt) | 保存 prepare-final-tools.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/preservation-during-tests.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/preservation-during-tests.json) | 記錄 preservation-during-tests.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/preserve-work.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/preserve-work.py.txt) | 保存 preserve-work.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/preserved-work.tar.gz](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/preserved-work.tar.gz) | 保存人工模型、來源版本、驅動程式及原始模組指紋，供清理後核對。 |
| [evidence/COR-09/event-adjoint-20261016/probe.go.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/probe.go.txt) | 保存 probe.go.txt 的公開重現、凍結測試或相容性程式。 |
| [evidence/COR-09/event-adjoint-20261016/push-preflight.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/push-preflight.json) | 記錄 push-preflight.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/red-verification.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/red-verification.json) | 記錄 red-verification.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/regression-before-corrected.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-before-corrected.json) | 記錄 regression-before-corrected.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/regression-before-corrected.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-before-corrected.log) | 保留 regression-before-corrected.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/regression-before-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-before-race.json) | 記錄 regression-before-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/regression-before-race.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-before-race.log) | 保留 regression-before-race.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/regression-before.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-before.json) | 記錄 regression-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/regression-before.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-before.log) | 保留 regression-before.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/regression-candidate-final-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-candidate-final-race.json) | 記錄 regression-candidate-final-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/regression-candidate-final-race.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-candidate-final-race.log) | 保留 regression-candidate-final-race.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/regression-candidate-final.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-candidate-final.json) | 記錄 regression-candidate-final.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/regression-candidate-final.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-candidate-final.log) | 保留 regression-candidate-final.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/regression-candidate-race.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-candidate-race.json) | 記錄 regression-candidate-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/regression-candidate-race.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-candidate-race.log) | 保留 regression-candidate-race.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/regression-candidate.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-candidate.json) | 記錄 regression-candidate.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/regression-candidate.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/regression-candidate.log) | 保留 regression-candidate.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/remote-before.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/remote-before.json) | 記錄 remote-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/reproduction-candidate-final.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/reproduction-candidate-final.json) | 記錄 reproduction-candidate-final.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/reproduction-candidate-final.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/reproduction-candidate-final.log) | 保留 reproduction-candidate-final.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/reproduction-candidate.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/reproduction-candidate.json) | 記錄 reproduction-candidate.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/reproduction-candidate.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/reproduction-candidate.log) | 保留 reproduction-candidate.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/reproduction-verification.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/reproduction-verification.json) | 記錄 reproduction-verification.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/reproduction.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/reproduction.json) | 記錄 reproduction.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/reproduction.log](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/reproduction.log) | 保留 reproduction.log 的原始命令輸出。 |
| [evidence/COR-09/event-adjoint-20261016/root-review.md](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/root-review.md) | 記錄 root-review.md 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/run-command.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/run-command.py.txt) | 保存 run-command.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-09/event-adjoint-20261016/verification.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/verification.json) | 記錄 verification.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-09/event-adjoint-20261016/work-preservation.json](/Users/timlai/Developer/coimnet/evidence/COR-09/event-adjoint-20261016/work-preservation.json) | 記錄 work-preservation.json 的計畫、審查、指紋或驗證結果。 |
