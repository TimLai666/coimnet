# Ticket 45 審查

Scope: CLEAN。本次修正既有格式索引，新增一份只使用標準函式庫的 Go 測試。框架 API、相依性、原始交接文件及模型／訓練設定保持。

## Findings

最終測試與索引沒有尚未處理的確定缺陷。主 agent 親自審查完整 diff、124 個來源位置及實際專項測試輸出。文件 agent 只改 INDEX，凍結測試指紋保持。

主 agent 修正四份報告的來源連結、LIF v0 與隱私人造資料的說明，以及 ASR 套件範圍。分類區分 12 個拒絕版本、一個隱私測試識別及六份測試產生的真實報告。

盤點限定 `coimnet-名稱/v數字` 形式，涵蓋 Go 直接字串、shell／Python 引用及四份治理 JSON。檢查能指出漏列、重複、過期、不完整的版本列、錯誤來源位置、已知誤分類、空資料、解析／讀取失敗，以及四分類以外的版本列。實際匯出的原始碼目錄沒有 Git 資料，也能執行同一份檢查。

## 驗證範圍

本輪證據驗證 macOS 上的軟體與文件行為。沒有新增任務學習、生物機制、遠端 GPU 或全腦訓練證據。OPS-05 與 TSK-11 保持 specified，需求通過數維持 89／91。

## 減法審查

沿用單一 INDEX 與既有 go test，不新增執行時格式註冊表、CLI、相依性或資料遷移。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| [AGENTS.md](../../../AGENTS.md) | 移除已解決的索引待辦，其餘規則及待辦保留。 |
| [ENG.md](../../../ENG.md) | 記錄版本命名範圍、四分類及現有 go test 的一致性檢查。 |
| [delivery-status.md](../../../delivery-status.md) | 追蹤 ticket 45 的驗收、交付狀態及下一個成果。 |
| [docs/INDEX.md](../../../docs/INDEX.md) | 補齊 124 個版本及來源行號，分開正式格式、規則識別、工具及反例。 |
| [docs/requirements-status.json](../../../docs/requirements-status.json) | 僅追加 GOV-01／GOV-04 的本輪證據，狀態維持 89／91。 |
| [docs/tickets/45-format-index-consistency.md](../../../docs/tickets/45-format-index-consistency.md) | 固定範圍、責任、失敗驗收與五項完成條件。 |
| [evidence/GOV-01/format-index-20261007/archive-command.txt](archive-command.txt) | 可重現的無 Git 匯出驗證命令。 |
| [evidence/GOV-01/format-index-20261007/archive.log](archive.log) | 最終匯出原始碼的實際格式測試輸出。 |
| [evidence/GOV-01/format-index-20261007/baseline.json](baseline.json) | 基準提交、五個目標及 3,022 份既有檔案指紋。 |
| [evidence/GOV-01/format-index-20261007/build-allowed.log](build-allowed.log) | 授權快取後的成功建置輸出。 |
| [evidence/GOV-01/format-index-20261007/build.log](build.log) | 保存最初 Go 快取遭 sandbox 拒絕的輸出。 |
| [evidence/GOV-01/format-index-20261007/checks.json](checks.json) | 實際命令、exit code、結果及日誌指紋。 |
| [evidence/GOV-01/format-index-20261007/delegation.json](delegation.json) | Spark 拒絕、Luna/max 分工責任及外部索引拒絕紀錄。 |
| [evidence/GOV-01/format-index-20261007/final-metadata.log](final-metadata.log) | 最終證據與狀態的八個專項輸出。 |
| [evidence/GOV-01/format-index-20261007/frozen-index.json](frozen-index.json) | 最終索引、文件 worker 及凍結測試指紋。 |
| [evidence/GOV-01/format-index-20261007/frozen-tests.json](frozen-tests.json) | 原索引失敗、分類數量及凍結測試指紋。 |
| [evidence/GOV-01/format-index-20261007/full-race.log](full-race.log) | 完整 race 的 57 個套件成功輸出。 |
| [evidence/GOV-01/format-index-20261007/full-tests.log](full-tests.log) | 完整一般測試的 57 個套件成功輸出。 |
| [evidence/GOV-01/format-index-20261007/gofmt-reproduction.log](gofmt-reproduction.log) | 依來源清單重現格式檢查的輸出。 |
| [evidence/GOV-01/format-index-20261007/gofmt.log](gofmt.log) | 初次 647 份 Go 來源的格式檢查輸出。 |
| [evidence/GOV-01/format-index-20261007/governance.log](governance.log) | 追加證據與釐清範圍後的八個專項輸出。 |
| [evidence/GOV-01/format-index-20261007/green.log](green.log) | 修正後格式檢查與控制案例輸出。 |
| [evidence/GOV-01/format-index-20261007/integrity-command.txt](integrity-command.txt) | 可重現的既有檔案、來源及連結檢查命令。 |
| [evidence/GOV-01/format-index-20261007/integrity.log](integrity.log) | 完整測試後的指紋、124 列及 173 個本機連結結果。 |
| [evidence/GOV-01/format-index-20261007/inventory-delta.json](inventory-delta.json) | 原 75 個版本與補齊的 49 個版本差異。 |
| [evidence/GOV-01/format-index-20261007/mod-tidy.log](mod-tidy.log) | 離線相依性整理差異檢查輸出。 |
| [evidence/GOV-01/format-index-20261007/mod-verify.log](mod-verify.log) | 模組校驗成功輸出。 |
| [evidence/GOV-01/format-index-20261007/red.log](red.log) | 原索引失敗、控制案例成功的實際輸出。 |
| [evidence/GOV-01/format-index-20261007/remote-before.log](remote-before.log) | 交付前遠端 main 與基準提交相符的證據。 |
| [evidence/GOV-01/format-index-20261007/review.md](review.md) | 完整來源／diff 審查、驗證範圍與本檔案清單。 |
| [evidence/GOV-01/format-index-20261007/source-manifest.json](source-manifest.json) | 647 份凍結 Go 原始碼的完整指紋。 |
| [evidence/GOV-01/format-index-20261007/source-review.md](source-review.md) | 用途分類的來源與拒絕行為依據。 |
| [evidence/GOV-01/format-index-20261007/verification.json](verification.json) | 本輪工程驗證的命令、環境、輸入指紋、結果及日誌。 |
| [evidence/GOV-01/format-index-20261007/vet.log](vet.log) | 完整靜態檢查成功輸出。 |
| [format_index_test.go](../../../format_index_test.go) | 新增來源盤點、索引驗證及正常／失敗控制。 |
| [evidence/GOV-01/format-index-20261007/commit-scan.log](commit-scan.log) | 提交前範圍、敏感內容與檔案大小掃描通過。 |
