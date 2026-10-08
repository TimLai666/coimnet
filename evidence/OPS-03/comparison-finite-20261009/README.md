# 比較設定有限值驗證

歸因與連續任務在開始訓練前只接受嚴格介於 0 與 1 的比較區間。驗證拒絕 NaN、正負無限大與範圍外有限值，保留原錯誤次序、合法報告及訓練設定。工程驗收見 verification.json，交付見 delivery.json；ticket 50 已完成 5／5 驗收與交付，實作 933ba4c 已推送且遠端一致。

核心修改只有兩個判斷。測試先在舊來源重現兩個 NaN 缺口，再凍結新增測試。合法控制含最小正有限值及比 1 小一個可表示值，兩個公開入口都接受。

- red-reviewed.json、fixed.json：Root 的先失敗與修正後回歸。
- ready-before.json、ready-after-a.json、ready-after-b.json：每份報告使用獨立比較設定，原提交與兩個新程序逐位元組相同，宣告與實際區間一致。
- probe-correction.json：初版對照程式共用比較設定，舊報告被後續設定變動影響，原始輸出留作問題證據，不作本票合法報告驗收。
- ownership-followup.json、ownership-ready.json：公開入口證明舊報告宣告會跟著呼叫端修改，雜湊與已實現區間卻保持原值；已記錄另票 P2。
- priority-before.json、priority-ready.json：22 個既有錯誤及取消／nil 次序。
- cli-report-before.json、cli-report-after.json：真實 CLI 的完整報告、命令、輸出與狀態。
- cli-before.json：初版查核腳本把 failed 矩陣當成單一布林值，實際框架結果無 Failed；verify-cli.py 已逐格檢查，重跑通過。此失敗留存。
- baseline.json、source-freeze.json、test-freeze.json、preservation.json：來源與歷史原件指紋。source-freeze 保存本模組 655 份追蹤及新增 Go 檔案，執行用的忽略目錄不納入。
- models.json、opencode-result.json、delegation.json：免費模型核對、90 秒逾時及 Luna max 的派工與 Root 審查。
- valid-isolated-*、valid-probe-before-format.go.txt：修正指標共用後、格式整理前的對照仍留存；最終驗收採 ready-*。
- check.py：保存命令、時間、環境之外的 stdout／stderr 原文及退出碼，文字日誌只移除多餘尾端空行。

## 重現

在 repo 根目錄執行：

```sh
go test -count=1 -run ComparisonFinite -v ./experiment
go test -count=1 -timeout 60m ./...
go test -count=1 -race -timeout 60m ./...
go build ./...
go vet ./...
go mod verify
go mod tidy -diff
```

verify-sdk.py 以原提交的本機暫存來源及兩個現有來源的新程序核對合法報告，接受新的輸出前綴，避免覆寫已存證據。合法 SDK 與錯誤次序的獨立程式是 valid-probe.go.txt 與 priority-probe.go.txt。將各檔案複製為暫存目錄的 main.go，從 repo 根目錄執行 go run 該路徑，與對應 JSON 的 raw_stdout 比較。verify-cli.py 接受任意新的階段名稱，從 repo 根目錄執行後以暫存 binary 核對完整流程並自行清理；輸出名稱採新的階段名稱，避免覆寫本票原件。

這是設定與保存流程的工程驗證，沒有新增任務學習成績或生物機制證據。
