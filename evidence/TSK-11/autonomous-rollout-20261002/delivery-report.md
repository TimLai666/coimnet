# 真實軌跡自主工程評估交付紀錄（2026-10-02）

## Problem

原有範例只用記錄位置預測下一步，無法評估模型產生位置後的後續動作。

## Done

新增獨立 rollout 入口。參數與最佳化器凍結，模型動作產生後續位置，目標只供計分。兩次訓練模型相同，兩次自主評估除執行量測外相同。13 個保留試次中，模型與三個控制組各命中 0 次，沒有學會導航的證據。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| [AGENTS.md](/Users/timlai/Developer/coimnet/AGENTS.md) | 記錄既有 infer 分割重疊缺口 |
| [ENG.md](/Users/timlai/Developer/coimnet/ENG.md) | 保存工程評估共用契約 |
| [README.md](/Users/timlai/Developer/coimnet/README.md) | 區分單步預測與自主工程評估 |
| [delivery-status.md](/Users/timlai/Developer/coimnet/delivery-status.md) | 記錄階段與下一個成果 |
| [docs/INDEX.md](/Users/timlai/Developer/coimnet/docs/INDEX.md) | 補 API、協定與 ticket 索引 |
| [docs/real-data-sources.md](/Users/timlai/Developer/coimnet/docs/real-data-sources.md) | 補真實軌跡評估證據 |
| [docs/requirements-status.json](/Users/timlai/Developer/coimnet/docs/requirements-status.json) | 新增證據，需求仍為 89／91 |
| [docs/tickets/29-real-tasks-ocr-asr-text-and-media-generation.md](/Users/timlai/Developer/coimnet/docs/tickets/29-real-tasks-ocr-asr-text-and-media-generation.md) | 驗收條件與研究結果 |
| [docs/tickets/32-real-trajectory-autonomous-rollout.md](/Users/timlai/Developer/coimnet/docs/tickets/32-real-trajectory-autonomous-rollout.md) | 驗收條件與研究結果 |
| [evidence/TSK-11/autonomous-rollout-20261002/build.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/build.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/checks.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/checks.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/concurrent-0-stderr.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/concurrent-0-stderr.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/concurrent-1-stderr.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/concurrent-1-stderr.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/concurrent-output.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/concurrent-output.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/control_reference.py](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/control_reference.py) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/delivery-report.md](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/delivery-report.md) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/environment.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/environment.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/first-checks.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/first-checks.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/first-race.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/first-race.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/first-source-after.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/first-source-after.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/first-source-before.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/first-source-before.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/format.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/format.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/handoff-checksums.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/handoff-checksums.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/independent-persistent-control.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/independent-persistent-control.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/initial-pairs-reference.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/initial-pairs-reference.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/legacy-infer-overlap.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/legacy-infer-overlap.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/mod-tidy.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/mod-tidy.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/mod-verify.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/mod-verify.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/race.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/race.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/real-data-commands.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/real-data-commands.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/real-data-summary.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/real-data-summary.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/real-runs.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/real-runs.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/real_runs.py](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/real_runs.py) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/report-summary.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/report-summary.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/review.md](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/review.md) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/root-green.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/root-green.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/root-path-red.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/root-path-red.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/root-regression-red.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/root-regression-red.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/source-after.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/source-after.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/source-audit.md](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/source-audit.md) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/source-before.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/source-before.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/targeted-race.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/targeted-race.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/targeted.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/targeted.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/test.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/test.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/verification.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/verification.json) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/verify.py](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/verify.py) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/vet.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/vet.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/worker-metadata-red.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/worker-metadata-red.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/autonomous-rollout-20261002/worker-rollout-red.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/worker-rollout-red.log) | 保存驗證、來源指紋或審查證據 |
| [evidence/TSK-11/verification.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/verification.json) | 保留歷史數據，新增子證據 |
| [examples/realnav/README.md](/Users/timlai/Developer/coimnet/examples/realnav/README.md) | 說明用法、判準與結果 |
| [examples/realnav/main.go](/Users/timlai/Developer/coimnet/examples/realnav/main.go) | 新增 rollout 入口與輸出保護 |
| [examples/realnav/rollout.go](/Users/timlai/Developer/coimnet/examples/realnav/rollout.go) | 凍結模型、產生位置、控制組與計分 |
| [examples/realnav/rollout_regression_test.go](/Users/timlai/Developer/coimnet/examples/realnav/rollout_regression_test.go) | 獨立核心對照與錯誤回歸 |
| [examples/realnav/rollout_test.go](/Users/timlai/Developer/coimnet/examples/realnav/rollout_test.go) | 驗證協定、隔離與幾何 |
| [tasks/nav2d/trajectory/return_targets.go](/Users/timlai/Developer/coimnet/tasks/nav2d/trajectory/return_targets.go) | 分開回傳評估目標中心 |
| [tasks/nav2d/trajectory/return_targets_test.go](/Users/timlai/Developer/coimnet/tasks/nav2d/trajectory/return_targets_test.go) | 驗證目標及原匯入相容性 |
| [tasks/nav2d/trajectory/trajectory.go](/Users/timlai/Developer/coimnet/tasks/nav2d/trajectory/trajectory.go) | 共用匯入流程，原 Read 相容 |
| [evidence/TSK-11/autonomous-rollout-20261002/governance.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/governance.log) | 治理檢查或檔案指紋 |
| [evidence/TSK-11/autonomous-rollout-20261002/governance-race.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/governance-race.log) | 治理檢查或檔案指紋 |
| [evidence/TSK-11/autonomous-rollout-20261002/diff-check.log](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/diff-check.log) | 治理檢查或檔案指紋 |
| [evidence/TSK-11/autonomous-rollout-20261002/governance-summary.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/governance-summary.json) | 治理檢查或檔案指紋 |
| [evidence/TSK-11/autonomous-rollout-20261002/file-manifest.json](/Users/timlai/Developer/coimnet/evidence/TSK-11/autonomous-rollout-20261002/file-manifest.json) | 治理檢查或檔案指紋 |

## Actions

| 動作 | 對象 | 結果 |
| --- | --- | --- |
| 真實資料訓練、評估與重跑 | Git 外 autonomous-rollout-20261002 目錄 | 保存完整模型、報告及指紋 |
| 並行輸出驗證 | 暫存目錄 | 一個成功，另一個拒絕已存在目錄 |
| 修正前測試輸出清理 | repo 內空白 forbidden-rollout-output | 檢查為空後移除，正式路徑保護回歸通過 |
| 提交及推送 | 本專案 origin/main | 待交付，遠端核對後另記 delivery.json |

## Verified

Mac CPU 完整一般與 race 測試各 56 個套件通過，建置、vet、格式與相依性檢查通過。13 個試次的起始位置、目標及方向延續控制組皆與獨立計算一致。模型、資料與完整快照未變，原有模型可由新程序載入。命令、環境與指紋見 [驗證紀錄](verification.json)。

## Notes

工程步數不能換算成動物秒數，2 cm 半徑是事前固定的工程判準。原始預印本半徑為 2.8 cm，本輪沒有重現動物實驗。沒有新增 GPU、全腦或 Ubuntu 執行證據。既有 infer 接受被改過且互相重疊的 train/test 試次，已列 P2 後續修正，新 rollout 會拒絕。

宿主：Codex。分工工具 collaboration，實際模型 gpt-5.6-luna，reasoning effort max。Spark 遭工具拒絕：Unknown model gpt-5.3-codex-spark。

減法提醒：沿用既有核心、訓練與保存流程，本輪只新增評估入口。

## Next

TSK-11 下一個成果是有來源依據的觀察與導航訓練契約，再用獨立試次驗收。OPS-05 需補裝置常駐更新、全圖容量／速度及 Ubuntu RTX 4070 執行。需求仍為 89／91。

Skills：software-engineering-guidelines、project-memory、agent-delegation、eng-architect、diff-inspector、test-and-fix、subtraction-thinking、human-writing；唯讀審查另用 investigate。
