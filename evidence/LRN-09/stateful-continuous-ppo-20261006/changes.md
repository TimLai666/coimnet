# 本輪檔案變更

範圍：基準 `16718541d6adce7cbc15b74308ccfe37c25c461f` 至本票實際變更。共 146 份檔案，每列對應一份檔案。

| 檔案 | 變更摘要 |
| --- | --- |
| [ENG.md](/Users/timlai/Developer/coimnet/ENG.md) | 修改保存狀態的共用訓練契約與候選驗證規則 |
| [README.md](/Users/timlai/Developer/coimnet/README.md) | 新增保存狀態梯度入口與 API 指引 |
| [delivery-status.md](/Users/timlai/Developer/coimnet/delivery-status.md) | 更新本票工程驗收、交付及後續成果 |
| [docs/requirements-status.json](/Users/timlai/Developer/coimnet/docs/requirements-status.json) | 只追加 COR-08、LRN-01、LRN-09 證據，保留全部需求狀態 |
| [docs/tickets/26-continual-matrix-imitation-ppo-and-bio-inspired-protocols.md](/Users/timlai/Developer/coimnet/docs/tickets/26-continual-matrix-imitation-ppo-and-bio-inspired-protocols.md) | 更新 CPU 連續核心的非零狀態限制與承接票 |
| [docs/tickets/43-stateful-continuous-ppo.md](/Users/timlai/Developer/coimnet/docs/tickets/43-stateful-continuous-ppo.md) | 新增完整契約、失敗驗收及五項交付條件 |
| [dynamics/continuous.go](/Users/timlai/Developer/coimnet/dynamics/continuous.go) | 共用保存歷史讀取與前向路徑，反向只回推目前段 |
| [dynamics/state_forward.go](/Users/timlai/Developer/coimnet/dynamics/state_forward.go) | 新增合法保存狀態的 CPU 連續核心入口 |
| [dynamics/state_forward_limits_test.go](/Users/timlai/Developer/coimnet/dynamics/state_forward_limits_test.go) | 新增公開入口、獨立數值或恢復回歸測試 |
| [dynamics/state_forward_test.go](/Users/timlai/Developer/coimnet/dynamics/state_forward_test.go) | 新增公開入口、獨立數值或恢復回歸測試 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/adversarial-review.md](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/adversarial-review.md) | 新增分析、測試或獨立審查報告 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/analysis.md](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/analysis.md) | 新增分析、測試或獨立審查報告 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/api-docs.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/api-docs.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/api-docs.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/api-docs.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/base-source.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/base-source.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/baseline-learning.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/baseline-learning.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/baseline-learning.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/baseline-learning.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/build-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/build-final.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/build-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/build-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/build.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/build.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/build.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/build.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/candidate-probe-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/candidate-probe-final.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/candidate-probe-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/candidate-probe-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/candidate-probe.go.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/candidate-probe.go.txt) | 保存公開入口重現或失敗測試原文 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/candidate-red.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/candidate-red.json) | 保存實際命令、環境、耗時及退出碼 1 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/candidate-red.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/candidate-red.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/changes.md](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/changes.md) | 新增逐檔變更清單 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/checkpoint-red.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/checkpoint-red.json) | 保存實際命令、環境、耗時及退出碼 1 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/checkpoint-red.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/checkpoint-red.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/cli-final-green.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/cli-final-green.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/cli-final-green.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/cli-final-green.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/cli-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/cli-final.json) | 保存實際命令、環境、耗時及退出碼 1 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/cli-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/cli-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/cli-validation-v2.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/cli-validation-v2.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/cli-validation.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/cli-validation.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/cli-workflow.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/cli-workflow.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/cli-workflow.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/cli-workflow.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/cli-workflow.py](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/cli-workflow.py) | 新增可重跑且有 --help 的來源或 CLI 驗證工具 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/core-review-green.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/core-review-green.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/core-review-green.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/core-review-green.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/delegation.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/delegation.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/diff-check.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/diff-check.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/diff-check.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/diff-check.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/docs-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/docs-final.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/docs-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/docs-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics--state_forward_limits_test.go.frozen.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics--state_forward_limits_test.go.frozen.txt) | 保存測試原文與凍結版本 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics--state_forward_test.go.frozen.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics--state_forward_test.go.frozen.txt) | 保存測試原文與凍結版本 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-limits-final-red.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-limits-final-red.json) | 保存實際命令、環境、耗時及退出碼 1 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-limits-final-red.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-limits-final-red.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-limits-red.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-limits-red.json) | 保存實際命令、環境、耗時及退出碼 1 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-limits-red.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-limits-red.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-red.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-red.json) | 保存實際命令、環境、耗時及退出碼 1 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-red.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-red.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-test-frozen.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-test-frozen.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-tests-red.go.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/dynamics-tests-red.go.txt) | 保存公開入口重現或失敗測試原文 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/environment.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/environment.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/evidence-help.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/evidence-help.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/evidence-help.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/evidence-help.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/feature-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/feature-final.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/feature-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/feature-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/feature-green.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/feature-green.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/feature-green.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/feature-green.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/full-race-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/full-race-final.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/full-race-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/full-race-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/full-race-interruption.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/full-race-interruption.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/full-race.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/full-race.json) | 保存實際命令、環境、耗時及退出碼 -15 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/full-race.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/full-race.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/full-tests-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/full-tests-final.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/full-tests-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/full-tests-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/full-tests.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/full-tests.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/full-tests.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/full-tests.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/gofmt-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/gofmt-final.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/gofmt-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/gofmt-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/gofmt.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/gofmt.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/gofmt.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/gofmt.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/handoff-integrity.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/handoff-integrity.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/handoff-integrity.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/handoff-integrity.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/interface-help-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/interface-help-final.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/interface-help-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/interface-help-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--stateful_checkpoint_test.go.frozen.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--stateful_checkpoint_test.go.frozen.txt) | 保存測試原文與凍結版本 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--stateful_clone_test.go.frozen-v2.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--stateful_clone_test.go.frozen-v2.txt) | 保存測試原文與凍結版本 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--stateful_delayed_test.go.frozen.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--stateful_delayed_test.go.frozen.txt) | 保存測試原文與凍結版本 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--stateful_test.go.frozen-v2.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--stateful_test.go.frozen-v2.txt) | 保存測試原文與凍結版本 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--stateful_test.go.frozen.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--stateful_test.go.frozen.txt) | 保存測試原文與凍結版本 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--update_validation_test.go.frozen.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/learning--rl--update_validation_test.go.frozen.txt) | 保存測試原文與凍結版本 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/learning--stateful_candidate_test.go.frozen.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/learning--stateful_candidate_test.go.frozen.txt) | 保存測試原文與凍結版本 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/learning--stateful_options_test.go.frozen.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/learning--stateful_options_test.go.frozen.txt) | 保存測試原文與凍結版本 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/learning--stateful_test.go.frozen.txt](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/learning--stateful_test.go.frozen.txt) | 保存測試原文與凍結版本 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/learning-rl-red.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/learning-rl-red.json) | 保存實際命令、環境、耗時及退出碼 1 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/learning-rl-red.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/learning-rl-red.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/mod-tidy.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/mod-tidy.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/mod-tidy.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/mod-tidy.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/mod-verify.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/mod-verify.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/mod-verify.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/mod-verify.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/ppo-delays-red.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/ppo-delays-red.json) | 保存實際命令、環境、耗時及退出碼 1 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/ppo-delays-red.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/ppo-delays-red.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/protected-manifest.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/protected-manifest.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/related-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/related-final.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/related-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/related-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/related-tests.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/related-tests.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/related-tests.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/related-tests.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/remote-before-delivery.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/remote-before-delivery.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/remote-before-delivery.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/remote-before-delivery.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/root-review.md](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/root-review.md) | 新增分析、測試或獨立審查報告 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/root-test-review.md](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/root-test-review.md) | 新增分析、測試或獨立審查報告 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-final.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-precommit.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-precommit.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-precommit.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-precommit.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-v2.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-v2.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-v2.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity-v2.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity.py](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/source-integrity.py) | 新增可重跑且有 --help 的來源或 CLI 驗證工具 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/source-manifest-v2.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/source-manifest-v2.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/source-manifest.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/source-manifest.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/staged-diff-check.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/staged-diff-check.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/staged-diff-check.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/staged-diff-check.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/staged-scan.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/staged-scan.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/staged-scan.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/staged-scan.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/target-flow.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/target-flow.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/target-flow.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/target-flow.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/test-fixture-red.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/test-fixture-red.json) | 保存實際命令、環境、耗時及退出碼 1 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/test-fixture-red.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/test-fixture-red.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/test-report.md](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/test-report.md) | 新增分析、測試或獨立審查報告 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/tests-frozen-v2.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/tests-frozen-v2.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/tests-frozen-v3.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/tests-frozen-v3.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/tests-frozen.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/tests-frozen.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/verification.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/verification.json) | 新增來源指紋、凍結、驗證或交付回條 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/vet-final.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/vet-final.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/vet-final.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/vet-final.log) | 保存對應命令的原始日誌 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/vet.json](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/vet.json) | 保存實際命令、環境、耗時及退出碼 0 |
| [evidence/LRN-09/stateful-continuous-ppo-20261006/vet.log](/Users/timlai/Developer/coimnet/evidence/LRN-09/stateful-continuous-ppo-20261006/vet.log) | 保存對應命令的原始日誌 |
| [learning/network.go](/Users/timlai/Developer/coimnet/learning/network.go) | 沿用 Insyra 梯度橋接並接上保存狀態 |
| [learning/rl/README.md](/Users/timlai/Developer/coimnet/learning/rl/README.md) | 更新 API、保存歷史及未支援設定的使用說明 |
| [learning/rl/stateful_checkpoint_test.go](/Users/timlai/Developer/coimnet/learning/rl/stateful_checkpoint_test.go) | 新增公開入口、獨立數值或恢復回歸測試 |
| [learning/rl/stateful_clone_test.go](/Users/timlai/Developer/coimnet/learning/rl/stateful_clone_test.go) | 新增公開入口、獨立數值或恢復回歸測試 |
| [learning/rl/stateful_delayed_test.go](/Users/timlai/Developer/coimnet/learning/rl/stateful_delayed_test.go) | 新增公開入口、獨立數值或恢復回歸測試 |
| [learning/rl/stateful_test.go](/Users/timlai/Developer/coimnet/learning/rl/stateful_test.go) | 新增公開入口、獨立數值或恢復回歸測試 |
| [learning/rl/update.go](/Users/timlai/Developer/coimnet/learning/rl/update.go) | 每輪從同一 rollout 初始狀態算分與更新 |
| [learning/rl/update_validation_test.go](/Users/timlai/Developer/coimnet/learning/rl/update_validation_test.go) | 改為驗收合法非零狀態，保留原個體與現行記憶 |
| [learning/stateful.go](/Users/timlai/Developer/coimnet/learning/stateful.go) | 新增保存狀態的梯度及訓練更新公開方法 |
| [learning/stateful_candidate_test.go](/Users/timlai/Developer/coimnet/learning/stateful_candidate_test.go) | 新增公開入口、獨立數值或恢復回歸測試 |
| [learning/stateful_options_test.go](/Users/timlai/Developer/coimnet/learning/stateful_options_test.go) | 新增公開入口、獨立數值或恢復回歸測試 |
| [learning/stateful_test.go](/Users/timlai/Developer/coimnet/learning/stateful_test.go) | 新增公開入口、獨立數值或恢復回歸測試 |
| [learning/trainer.go](/Users/timlai/Developer/coimnet/learning/trainer.go) | 候選參數從同一保存狀態驗證，沿用既有提交與最佳化器 |
