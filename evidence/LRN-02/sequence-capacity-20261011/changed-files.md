# Ticket 56 檔案變更

本輪修正分段重算訓練的容量限制。下表列出程式、測試、文件與保留的驗收證據。清理移除的驗收副本另見 cleanup.json 及 cleanup-inventory.json.gz。

| 檔案 | 變更摘要 |
| --- | --- |
| [AGENTS.md](/Volumes/SSD/Developer/coimnet/AGENTS.md) | 移除已解決的容量待辦，記錄既有量測暫存路徑問題。 |
| [ENG.md](/Volumes/SSD/Developer/coimnet/ENG.md) | 更新有限序列前向契約與記憶體成本。 |
| [delivery-status.md](/Volumes/SSD/Developer/coimnet/delivery-status.md) | 登錄 ticket 56 的驗收與交付狀態。 |
| [docs/INDEX.md](/Volumes/SSD/Developer/coimnet/docs/INDEX.md) | 登錄新 SDK 方法與工作票。 |
| [docs/requirements-status.json](/Volumes/SSD/Developer/coimnet/docs/requirements-status.json) | 加入 LRN-02 容量驗收證據，原需求狀態保持。 |
| [docs/resources.md](/Volumes/SSD/Developer/coimnet/docs/resources.md) | 加入兩種人工圖的獨立程序記憶體量測。 |
| [docs/tickets/56-recompute-sequence-capacity.md](/Volumes/SSD/Developer/coimnet/docs/tickets/56-recompute-sequence-capacity.md) | 保存共用契約、固定完成條件及實際驗收。 |
| [dynamics/forward_recompute.go](/Volumes/SSD/Developer/coimnet/dynamics/forward_recompute.go) | 新增兩種核心的有限序列分段前向。 |
| [learning/recompute.go](/Volumes/SSD/Developer/coimnet/learning/recompute.go) | 訓練前向呼叫有限序列入口。 |
| [dynamics/forward_recompute_test.go](/Volumes/SSD/Developer/coimnet/dynamics/forward_recompute_test.go) | 新增 SDK 輸出等價與錯誤、極端值回歸。 |
| [learning/recompute_capacity_test.go](/Volumes/SSD/Developer/coimnet/learning/recompute_capacity_test.go) | 新增公開訓練容量、最佳化器等價與持續狀態保護回歸。 |
| [evidence/LRN-02/sequence-capacity-20261011/cleanup-inventory.json.gz](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cleanup-inventory.json.gz) | 刪除前驗收副本的逐檔指紋與大小清單。 |
| [evidence/LRN-02/sequence-capacity-20261011/cleanup.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cleanup.json) | 清理完成及原專案成果再次讀回的紀錄。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/commands.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/commands.json) | 九個 CLI／SDK 文件命令的結果。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/continuous-api.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/continuous-api.log) | 對應 CLI 或 SDK 文件命令的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/continuous.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/continuous.json) | 人工模型連續訓練 120 步的恢復測試快照。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/data-sources.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/data-sources.log) | 對應 CLI 或 SDK 文件命令的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/delayed.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/delayed.log) | 對應 CLI 或 SDK 文件命令的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/doctor.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/doctor.log) | 對應 CLI 或 SDK 文件命令的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/first.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/first.json) | 人工模型訓練 80 步的恢復測試快照。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/lif-api.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/lif-api.log) | 對應 CLI 或 SDK 文件命令的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/lif-threshold.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/lif-threshold.log) | 對應 CLI 或 SDK 文件命令的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/resume-comparison.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/resume-comparison.json) | 續訓與連續訓練的快照位元組比對。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/resumed.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/resumed.json) | 人工模型在新程序續訓 40 步的恢復測試快照。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/train-continuous.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/train-continuous.log) | 對應 CLI 或 SDK 文件命令的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/train-first.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/train-first.log) | 對應 CLI 或 SDK 文件命令的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/cli/train-resumed.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/cli/train-resumed.log) | 對應 CLI 或 SDK 文件命令的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/copy-receipt.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/copy-receipt.json) | 67 份成果回存原專案的逐檔核對紀錄。 |
| [evidence/LRN-02/sequence-capacity-20261011/environment.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/environment.json) | 原生磁碟驗收環境。 |
| [evidence/LRN-02/sequence-capacity-20261011/frozen-tests.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/frozen-tests.json) | 實作前固定的兩份測試指紋。 |
| [evidence/LRN-02/sequence-capacity-20261011/head-baseline.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/head-baseline.json) | 修改前來源與模組指紋。 |
| [evidence/LRN-02/sequence-capacity-20261011/log-manifest.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/log-manifest.json) | 驗收日誌的原始及封存指紋清單。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/baseline.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/baseline.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/build.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/build.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/cli-build.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/cli-build.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/dependencies.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/dependencies.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/format-commands.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/format-commands.json) | 對應驗收命令、結束狀態與執行時間。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/format.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/format.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/full-commands.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/full-commands.json) | 對應驗收命令、結束狀態與執行時間。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/governance-commands.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/governance-commands.json) | 對應驗收命令、結束狀態與執行時間。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/governance.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/governance.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/green-commands.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/green-commands.json) | 對應驗收命令、結束狀態與執行時間。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/green.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/green.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/modules.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/modules.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/race.log.gz](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/race.log.gz) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/red-commands.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/red-commands.json) | 對應驗收命令、結束狀態與執行時間。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/red.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/red.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-build.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-build.log) | 初次受限環境量測的原始輸出及限制。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-continuous-full.stderr](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-continuous-full.stderr) | 初次受限環境量測的原始輸出及限制。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-continuous-full.stdout](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-continuous-full.stdout) | 初次受限環境量測的原始輸出及限制。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-build.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-build.log) | 原生環境獨立程序量測的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-continuous-full.stderr](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-continuous-full.stderr) | 原生環境獨立程序量測的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-continuous-full.stdout](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-continuous-full.stdout) | 原生環境獨立程序量測的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-continuous-recompute.stderr](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-continuous-recompute.stderr) | 原生環境獨立程序量測的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-continuous-recompute.stdout](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-continuous-recompute.stdout) | 原生環境獨立程序量測的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-lif-full.stderr](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-lif-full.stderr) | 原生環境獨立程序量測的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-lif-full.stdout](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-lif-full.stdout) | 原生環境獨立程序量測的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-lif-recompute.stderr](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-lif-recompute.stderr) | 原生環境獨立程序量測的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-lif-recompute.stdout](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-lif-recompute.stdout) | 原生環境獨立程序量測的原始輸出。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-runs.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/rss-native/rss-runs.json) | 四個獨立程序的記憶體量測及參數指紋。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/target-flow.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/target-flow.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/unit.log.gz](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/unit.log.gz) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/vet.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/vet.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/logs/worker-impl.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/logs/worker-impl.log) | 對應驗收命令的原始輸出，壓縮日誌保留原始內容。 |
| [evidence/LRN-02/sequence-capacity-20261011/root-review.md](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/root-review.md) | Root 完整差異審查、獨立審查及限制。 |
| [evidence/LRN-02/sequence-capacity-20261011/run_checks.py](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/run_checks.py) | 封存一般、race 與必要檢查的重現腳本。 |
| [evidence/LRN-02/sequence-capacity-20261011/run_cli.py](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/run_cli.py) | 封存 CLI 與跨程序續訓的重現腳本。 |
| [evidence/LRN-02/sequence-capacity-20261011/run_rss.py](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/run_rss.py) | 封存獨立程序記憶體量測的重現腳本。 |
| [evidence/LRN-02/sequence-capacity-20261011/scan-fix.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/scan-fix.log) | 修正提交的 staged diff 掃描結果。 |
| [evidence/LRN-02/sequence-capacity-20261011/scan-proof.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/scan-proof.log) | 證據及文件提交的 staged diff 掃描結果。 |
| [evidence/LRN-02/sequence-capacity-20261011/scan-tests.log](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/scan-tests.log) | 回歸提交的 staged diff 掃描結果。 |
| [evidence/LRN-02/sequence-capacity-20261011/source-final.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/source-final.json) | 665 份最終 Go 來源與模組指紋。 |
| [evidence/LRN-02/sequence-capacity-20261011/verification.json](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/verification.json) | LRN-02 命令、環境、結果、指紋及原始日誌索引。 |
| [evidence/LRN-02/sequence-capacity-20261011/changed-files.md](/Volumes/SSD/Developer/coimnet/evidence/LRN-02/sequence-capacity-20261011/changed-files.md) | 本輪全部新增與修改檔案的逐檔摘要。 |
