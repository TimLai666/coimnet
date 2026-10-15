# 本輪檔案變更

本表逐檔列出 ticket 113 交付內容。既有 AGENTS.md 與兩組其他工作證據不在交付範圍。

| 檔案 | 變更摘要 |
|---|---|
| [ENG.md](/Users/timlai/Developer/coimnet/ENG.md) | 記錄膜時間導數的正常算式與例外恢復契約。 |
| [delivery-status.md](/Users/timlai/Developer/coimnet/delivery-status.md) | 記錄本票驗收、清理與交付狀態。 |
| [docs/requirements-status.json](/Users/timlai/Developer/coimnet/docs/requirements-status.json) | 連結受影響需求的實際重現與驗收證據。 |
| [docs/tickets/113-membrane-tau-derivative.md](/Users/timlai/Developer/coimnet/docs/tickets/113-membrane-tau-derivative.md) | 記錄根因、驗收條件及五項完成狀態。 |
| [dynamics/continuous.go](/Users/timlai/Developer/coimnet/dynamics/continuous.go) | 連續核心保留原累加式，只恢復非有限的膜時間導數。 |
| [dynamics/coupled_reverse.go](/Users/timlai/Developer/coimnet/dynamics/coupled_reverse.go) | 連續與 LIF 的化學耦合反向保留正常值，另算例外恢復值。 |
| [dynamics/gpu_adapter.go](/Users/timlai/Developer/coimnet/dynamics/gpu_adapter.go) | GPU 主機端反向補上相同恢復，裝置限制保持。 |
| [dynamics/lif.go](/Users/timlai/Developer/coimnet/dynamics/lif.go) | LIF 核心恢復不應期內原本應為零的膜時間導數。 |
| [dynamics/lif_recompute.go](/Users/timlai/Developer/coimnet/dynamics/lif_recompute.go) | LIF 分段重算補上相同恢復。 |
| [dynamics/mixed.go](/Users/timlai/Developer/coimnet/dynamics/mixed.go) | 混合核心沿用相同的例外恢復與既有梯度檢查。 |
| [dynamics/mixed_coupled.go](/Users/timlai/Developer/coimnet/dynamics/mixed_coupled.go) | 混合化學耦合反向保留正常捨入與累加有限值檢查。 |
| [dynamics/recompute.go](/Users/timlai/Developer/coimnet/dynamics/recompute.go) | 連續核心的分段重算補上相同恢復。 |
| [dynamics/tau_gradient.go](/Users/timlai/Developer/coimnet/dynamics/tau_gradient.go) | 新增有界的精確差乘積恢復，原始非有限值保持拒絕。 |
| [dynamics/vector_core.go](/Users/timlai/Developer/coimnet/dynamics/vector_core.go) | 逐向量分量保留原累加式並恢復可表示導數。 |
| [dynamics/zero_tau_adjoint_test.go](/Users/timlai/Developer/coimnet/dynamics/zero_tau_adjoint_test.go) | 凍結公開回歸，涵蓋核心、重算、化學耦合、向量與 Metal。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/acceptance.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/acceptance.py.txt) | 保存 acceptance.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/baseline.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/baseline.json) | 記錄 baseline.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/candidate-initial.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/candidate-initial.json) | 記錄 candidate-initial.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/candidate-second.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/candidate-second.json) | 記錄 candidate-second.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/candidate-verification.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/candidate-verification.json) | 記錄 candidate-verification.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/changed-files.md](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/changed-files.md) | 逐檔列出本輪交付範圍。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/changed-files.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/changed-files.py.txt) | 保存 changed-files.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-compat.go.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-compat.go.txt) | 保存 chemical-compat.go.txt 的公開重現、凍結測試或相容性程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-accepted-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-accepted-race.json) | 記錄 chemical-resume-accepted-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-accepted-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-accepted-race.log) | 保留 chemical-resume-accepted-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-accepted.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-accepted.json) | 記錄 chemical-resume-accepted.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-accepted.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-accepted.log) | 保留 chemical-resume-accepted.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-before-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-before-race.json) | 記錄 chemical-resume-before-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-before-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-before-race.log) | 保留 chemical-resume-before-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-before.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-before.json) | 記錄 chemical-resume-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-before.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-resume-before.log) | 保留 chemical-resume-before.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-save-before-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-save-before-race.json) | 記錄 chemical-save-before-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-save-before-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-save-before-race.log) | 保留 chemical-save-before-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-save-before.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-save-before.json) | 記錄 chemical-save-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/chemical-save-before.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/chemical-save-before.log) | 保留 chemical-save-before.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/cleanup-inventory.json.gz](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/cleanup-inventory.json.gz) | 保存刪除前逐項內容與指紋。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/cleanup-owned.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/cleanup-owned.py.txt) | 保存 cleanup-owned.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/cleanup.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/cleanup.json) | 記錄 cleanup.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/closure-binary.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/closure-binary.json) | 記錄 closure-binary.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/command-environment.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/command-environment.json) | 記錄 command-environment.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-accepted-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-accepted-race.json) | 記錄 continuous-accepted-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-accepted-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-accepted-race.log) | 保留 continuous-accepted-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-accepted.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-accepted.json) | 記錄 continuous-accepted.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-accepted.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-accepted.log) | 保留 continuous-accepted.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-after-initial-differences.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-after-initial-differences.json) | 記錄 continuous-after-initial-differences.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-after.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-after.json) | 記錄 continuous-after.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-after.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-after.log) | 保留 continuous-after.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-before-corrected.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-before-corrected.json) | 記錄 continuous-before-corrected.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-before-corrected.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-before-corrected.log) | 保留 continuous-before-corrected.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-before-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-before-race.json) | 記錄 continuous-before-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-before-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-before-race.log) | 保留 continuous-before-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-before.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-before.json) | 記錄 continuous-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-before.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-before.log) | 保留 continuous-before.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-compat-initial.go.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-compat-initial.go.txt) | 保存 continuous-compat-initial.go.txt 的公開重現、凍結測試或相容性程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-compat.go.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-compat.go.txt) | 保存 continuous-compat.go.txt 的公開重現、凍結測試或相容性程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-final-initial-differences.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-final-initial-differences.json) | 記錄 continuous-final-initial-differences.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-final-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-final-race.json) | 記錄 continuous-final-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-final-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-final-race.log) | 保留 continuous-final-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-final.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-final.json) | 記錄 continuous-final.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/continuous-final.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/continuous-final.log) | 保留 continuous-final.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-accepted-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-accepted-race.json) | 記錄 core-accepted-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-accepted-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-accepted-race.log) | 保留 core-accepted-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-accepted.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-accepted.json) | 記錄 core-accepted.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-accepted.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-accepted.log) | 保留 core-accepted.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-after-initial-differences.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-after-initial-differences.json) | 記錄 core-after-initial-differences.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-after.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-after.json) | 記錄 core-after.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-after.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-after.log) | 保留 core-after.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-before-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-before-race.json) | 記錄 core-before-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-before-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-before-race.log) | 保留 core-before-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-before.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-before.json) | 記錄 core-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-before.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-before.log) | 保留 core-before.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-compat.go.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-compat.go.txt) | 保存 core-compat.go.txt 的公開重現、凍結測試或相容性程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-final-initial-differences.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-final-initial-differences.json) | 記錄 core-final-initial-differences.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-final-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-final-race.json) | 記錄 core-final-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-final-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-final-race.log) | 保留 core-final-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-final.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-final.json) | 記錄 core-final.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-final.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-final.log) | 保留 core-final.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-old-state-resume-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-old-state-resume-race.json) | 記錄 core-old-state-resume-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-old-state-resume-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-old-state-resume-race.log) | 保留 core-old-state-resume-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-old-state-resume.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-old-state-resume.json) | 記錄 core-old-state-resume.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/core-old-state-resume.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/core-old-state-resume.log) | 保留 core-old-state-resume.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-build.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-build.json) | 記錄 engineering-build.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-build.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-build.log) | 保留 engineering-build.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-closure-binary.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-closure-binary.json) | 記錄 engineering-closure-binary.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-closure-binary.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-closure-binary.log) | 保留 engineering-closure-binary.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-format.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-format.json) | 記錄 engineering-format.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-format.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-format.log) | 保留 engineering-format.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-governance.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-governance.json) | 記錄 engineering-governance.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-governance.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-governance.log) | 保留 engineering-governance.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-module-selection.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-module-selection.json) | 記錄 engineering-module-selection.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-module-selection.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-module-selection.log) | 保留 engineering-module-selection.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-module-verify.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-module-verify.json) | 記錄 engineering-module-verify.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-module-verify.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-module-verify.log) | 保留 engineering-module-verify.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-race.json) | 記錄 engineering-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-race.log.gz](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-race.log.gz) | 保留 engineering-race.log.gz 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-unit.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-unit.json) | 記錄 engineering-unit.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-unit.log.gz](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-unit.log.gz) | 保留 engineering-unit.log.gz 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-verification.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-verification.json) | 記錄 engineering-verification.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-vet.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-vet.json) | 記錄 engineering-vet.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering-vet.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering-vet.log) | 保留 engineering-vet.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/engineering.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/engineering.py.txt) | 保存 engineering.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/final-governance.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/final-governance.json) | 記錄 final-governance.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/final-governance.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/final-governance.log) | 保留 final-governance.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/final-governance.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/final-governance.py.txt) | 保存 final-governance.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/finish-engineering.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/finish-engineering.py.txt) | 保存 finish-engineering.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/foreign-before.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/foreign-before.json) | 記錄 foreign-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/formal-source.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/formal-source.json) | 記錄 formal-source.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/frozen-test.go.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/frozen-test.go.txt) | 保存 frozen-test.go.txt 的公開重現、凍結測試或相容性程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/frozen-test.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/frozen-test.json) | 記錄 frozen-test.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/implementation-before.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/implementation-before.json) | 記錄 implementation-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/independent-reference.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/independent-reference.json) | 記錄 independent-reference.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/opencode-models.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/opencode-models.json) | 記錄 opencode-models.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/opencode-models.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/opencode-models.log) | 保留 opencode-models.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/plan.md](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/plan.md) | 記錄 plan.md 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/preserved-work.tar.gz](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/preserved-work.tar.gz) | 保存人工模型、來源版本、驅動程式及原始模組指紋，供清理後核對。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/probe-nonzero.go.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/probe-nonzero.go.txt) | 保存 probe-nonzero.go.txt 的公開重現、凍結測試或相容性程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/probe.go.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/probe.go.txt) | 保存 probe.go.txt 的公開重現、凍結測試或相容性程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-accepted-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-accepted-race.json) | 記錄 regression-accepted-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-accepted-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-accepted-race.log) | 保留 regression-accepted-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-accepted.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-accepted.json) | 記錄 regression-accepted.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-accepted.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-accepted.log) | 保留 regression-accepted.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-final-green.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-final-green.json) | 記錄 regression-final-green.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-final-green.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-final-green.log) | 保留 regression-final-green.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-final-red-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-final-red-race.json) | 記錄 regression-final-red-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-final-red-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-final-red-race.log) | 保留 regression-final-red-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-final-red.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-final-red.json) | 記錄 regression-final-red.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-final-red.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-final-red.log) | 保留 regression-final-red.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-green.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-green.json) | 記錄 regression-green.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-green.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-green.log) | 保留 regression-green.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-red-race.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-red-race.json) | 記錄 regression-red-race.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-red-race.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-red-race.log) | 保留 regression-red-race.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-red.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-red.json) | 記錄 regression-red.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/regression-red.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/regression-red.log) | 保留 regression-red.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/remote-before.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/remote-before.json) | 記錄 remote-before.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/remote-before.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/remote-before.log) | 保留 remote-before.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/reproduction-accepted.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/reproduction-accepted.json) | 記錄 reproduction-accepted.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/reproduction-accepted.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/reproduction-accepted.log) | 保留 reproduction-accepted.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/reproduction-nonzero-accepted.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/reproduction-nonzero-accepted.json) | 記錄 reproduction-nonzero-accepted.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/reproduction-nonzero-accepted.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/reproduction-nonzero-accepted.log) | 保留 reproduction-nonzero-accepted.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/reproduction-nonzero.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/reproduction-nonzero.json) | 記錄 reproduction-nonzero.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/reproduction-nonzero.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/reproduction-nonzero.log) | 保留 reproduction-nonzero.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/reproduction-verification.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/reproduction-verification.json) | 記錄 reproduction-verification.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/reproduction.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/reproduction.json) | 記錄 reproduction.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/reproduction.log](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/reproduction.log) | 保留 reproduction.log 的原始命令輸出。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/root-review.md](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/root-review.md) | 記錄 root-review.md 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/run-command.py.txt](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/run-command.py.txt) | 保存 run-command.py.txt 的驗收、證據或清理程式。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/verification.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/verification.json) | 記錄 verification.json 的計畫、審查、指紋或驗證結果。 |
| [evidence/COR-04/zero-tau-adjoint-20261015/work-preservation.json](/Users/timlai/Developer/coimnet/evidence/COR-04/zero-tau-adjoint-20261015/work-preservation.json) | 記錄 work-preservation.json 的計畫、審查、指紋或驗證結果。 |
