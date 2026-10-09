# 連續任務的設定隔離驗證

RunContinualMatrix 現在於原有驗證之後、計算識別碼與執行之前，複製完整可變設定。呼叫端、每次執行與各份回傳報告分開持有任務參數、階段、種子、比較設定及化學狀態列。這修正了後續設定修改會改到舊報告，讓宣告與執行結果不一致的問題。

八組公開入口回歸由 Root 親自重跑。修正前六組失敗、兩組空值控制通過，測試凍結後八組全部通過。各欄位的雙向修改、map 插入刪除、預留容量追加、builder 修改原輸入、取消與建模失敗的部分報告都涵蓋在內。

七份完整 SDK 報告與修正前 checkout、原提交的獨立來源及兩個修正後新程序逐位元組一致。四份是正常合法執行，其餘保留空化學列、建模失敗及取消的既有處理。八個真實 CLI 流程涵蓋一般與化學兩種報告、重複路徑拒絕、說明與非法回合數。nil 指標及 [null,[]] 的化學列表示保持。

這是人工連續任務範例的軟體驗證，不新增任務學習或生物機制成果。只有本機 macOS arm64 執行證據。公開介面、格式、依賴、模型、任務設定及原始交接包保持。呼叫端不得在驗證或複製期間並行修改輸入。

## 證據位置

| 項目 | 檔案 |
|---|---|
| 原提交與既有檔案指紋 | baseline.json、entry-documents.json、preservation.json |
| 共用入口、mutable 欄位、分工與範圍 | scope.json、root-review.md |
| 修正前失敗與測試凍結 | ownership-red.log、test-freeze.json |
| 修正後回歸與相關控制 | ownership-green.log、scoped-green.log |
| SDK 完整報告與程序對照 | valid-probe.go.txt、verify-sdk.py、sdk-comparison.json |
| CLI 流程與完整輸出 | verify-cli.py、cli-before.json、cli-after.json |
| 全部 Go 來源凍結 | source-freeze.json |
| 完整一般測試及各項檢查 | full-test.log、build.log、vet.log、format.log、module-verify.log、module-tidy.log |
| 完整 race 測試 | full-race.log，完成結果以同名 JSON 為準 |
| OpenCode 本輪逾時 | opencode.json、opencode.log |
| 另票待辦的公開重現 | related-followup-probe.go.txt、related-followup-summary.json |

check.py 保存每個命令的 argv、工作目錄、返回值、時間及日誌指紋。environment.json 保存本輪執行環境。完整驗收結論須等全部命令與文件檢查完成，不能只依上面的專項通過判斷。

第一次 SDK 探測誤以 .go.txt 檔名執行，Go 拒絕這個副檔名；sdk-before.json 保存該失敗。正式基準使用相同內容的暫存 .go 檔，sdk-baseline.json 記錄成功執行，獨立原提交與兩個新程序再驗證相同結果。失敗嘗試不計入通過數。

## 重跑

從 repo 根目錄，選擇尚未使用的紀錄名稱執行：

```sh
python3 evidence/LRN-08/protocol-ownership-20261009/check.py CHECK_NAME go test -count=1 -run ContinualOwnership -v ./experiment
```

先將 valid-probe.go.txt 複製到 repo 外的暫存 .go 路徑，即可使用 go run 取得七份報告。verify-sdk.py 與 verify-cli.py 是本輪留存的驗收工具，其固定輸出名稱拒絕覆寫既有證據。需重跑整套比較時，另存證據目錄及紀錄名稱。
