# 連續核心分段活化共用驗收

同一段落內，每個神經元的活化輸出供所有連線及權重梯度共用。連續相同目標的邊以本機變數依序累加，保持數值順序。公開 API、訓練設定、LIF 及快照格式保持。

固定人工密集圖的八組案例耗時降低 58.88–91.41%，事前門檻為每組至少降低 15%。量測為同一台 Apple M3 的七次交錯配對，每次用獨立程序，完整 24 案例與暖機均保存。這不是完整果蠅圖、GPU 或其他處理器的速度保證。額外配置量、RSS 及稀疏控制見 [資源說明](../../../docs/resources.md#分段活化共用量測2026-10-11)。

34 個重算專項、完整一般與 race 各 57 套件、格式、build、vet、相依性及 11 個治理專項通過。CLI 新程序接續與連續執行快照逐位元組相同。一般頂層測試 1,774 個通過、20 個跳過，race 為 1,773 個通過、21 個跳過。race 額外跳過既有的大型 bundle fixture，測試原註解指出競爭檢查的記憶體成本過高；一般測試已通過該案例。完整日誌保留各跳過原因。

## 證據索引

| 檔案 | 用途 |
| --- | --- |
| [verification.json](verification.json) | 最終驗收命令、環境、來源指紋及結論 |
| [source-final.json](source-final.json) | 667 份 Go 與兩份模組檔的最終 SHA256 |
| [frozen-tests-final.json](frozen-tests-final.json) | 374 份凍結測試來源 |
| [baseline-fingerprints.json](baseline-fingerprints.json) | 基準 HEAD 90c6b93 的 667 份來源及模組指紋 |
| [benchmark-builds.json](benchmark-builds.json) | 相同測試、原始兩份 production overlay 及候選執行檔指紋 |
| [performance/comparison.json](performance/comparison.json) | 全部案例時間與配置中位數及固定門檻 |
| [performance/runs.json](performance/runs.json) | 每次程序命令、負載、RSS 與原始輸出指紋 |
| [performance-summary.json](performance-summary.json) | 活化暫存估算與 RSS 觀察 |
| [assembly-verification.json](assembly-verification.json) | 本次 arm64 build 參考與改善後的相同 FMADDD 乘加 |
| [root-review.md](root-review.md) | Root 契約、所有權、錯誤與數值審查 |
| [cleanup.json](cleanup.json) | 本輪驗收副本與暫存清理回條 |
| [delivery.json](delivery.json) | 提交、同步、推送、遠端與提交內容核對 |

`candidate-one`、`candidate-three`、`candidate-four` 保留未達門檻的候選量測，`candidate-two` 曾通過 dense 門檻，但之後零邊前向的額外配置被修正。`candidate-two-checks` 的 race 曾中止，不能當最終驗收。最終候選為 candidate5。所有先前資料保留以供稽核。

`logs/` 中超過 1 MB 的 JSON 日誌與 CPU profile 用 gzip 無損保存。 差異檔、組譯輸出及帶有格式空白的 CLI 文件輸出也原封壓縮保存，原始與壓縮指紋見 [raw-archives.json](raw-archives.json)。CLI 的小型人工快照僅作恢復比對證據，不作模型發布。

## 重現

從最終提交建立本機原生磁碟 checkout，使用 Go 1.26.5 與既有依賴。暫存與 Go build cache 放 `runs/.tmp/recompute-activation-20261011/`，模組使用既有離線 cache。不要在 SMB 上執行完整套件、APFS 映像或 Go cache。

每次重現使用新暫存目錄，先建立必要子目錄。腳本不會覆寫既有同名日誌，重跑前應先保存上一輪證據。

```sh
mkdir -p runs/.tmp/recompute-activation-20261011/{logs,gocache,gotmp,tmp,bin}
python3 evidence/LRN-02/activation-reuse-20261011/run_checks.py green
python3 evidence/LRN-02/activation-reuse-20261011/run_checks.py full
python3 evidence/LRN-02/activation-reuse-20261011/run_cli.py
```

CLI 驗證 80+40 步接續與 120 步連續執行的快照逐位元組相同。

效能的 `run_performance.py` 需要先建置兩份 dynamics test binary。基準 overlay 只將 `dynamics/recompute.go`、`dynamics/forward_recompute.go` 替換成基準 HEAD 90c6b93 原始內容，其餘含新測試相同。完整 `go test -c` 命令在 `benchmark-builds.json`，binary 名稱為 `dynamics-baseline-candidate5.test`、`dynamics-improved-candidate5.test`，放在上述暫存的 `bin/`。每個獨立程序使用 `-test.benchtime=3x`、`-test.count=1`、`-test.cpu=1` 及 GOMAXPROCS=1。量測完先保存必要證據，再清除暫存。
