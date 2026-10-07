# 導航有效設定驗收

本目錄保存 ticket 47 的修正前後報告、命令、來源指紋與原始失敗。軟體紀錄驗收與人工任務成績分開判讀。

在儲存庫根目錄可以重跑：

~~~sh
go test -count=1 -run EffectiveConfig -v ./experiment/nav2d ./experiment ./internal/cli
python3 evidence/OPS-03/nav2d-effective-config-20261007/verify-cli.py --help
python3 evidence/OPS-03/nav2d-effective-config-20261007/verify-cli.py
~~~

CLI 驗收預設建立新的暫存目錄，輸出其路徑。指定 --output-dir 時該目錄必須不存在，會建立執行檔、報告與日誌。原始八個 CLI 流程使用的腳本保存在 cli-proof-recorded-script.py.txt，既有報告不覆寫。

report-probe.go.txt 是修正前後公開 API 的人工固定配置。複製為儲存庫外的 .go 檔，再執行 go run 該檔案，可以產生單任務、suite、歸因及自訂設定的報告。before-after-comparison.json 記錄八份 task report 共 24 筆 run，只有 Config／ConfigHash 有預期差異。
