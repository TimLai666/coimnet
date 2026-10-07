# Ticket 47 Root 審查

Problem：新導航與歸因報告沒有展開環境預設值，記錄與實際執行不同。四任務 suite 共用同一缺陷。

Done：共用 nav2d.Config.Resolve 的既有預設與驗證，執行與報告都使用解析結果。省略與明填預設得到相同完整報告，明填自訂設定保持。固定配置的 24 筆訓練／評估紀錄在修正前後相同，兩個新程序的完整報告相同。只有設定與其雜湊預期改變。

## Diff Inspector

Scope: CLEAN。Root 已親讀全部 production diff 與三個新增回歸。環境建構、單任務、suite、歸因、CLI JSON／檔案、help、README 與索引皆有對應。既有範圍外程式及模型保持。

No confirmed findings within this fix. 新 Config.Resolve 為純值 API，既有驗證與錯誤次序保持，歸因空任務仍拒絕，沒有狀態共享、依賴變動或資料遷移。兩個 report producers 在 hash(c) 前使用同一解析設定；suite 呼叫 RunNav2D，CLI 直接輸出其結果。已提交歷史報告代表原來輸入設定，不重新產生。

[P2] [CONFIRMED] experiment/nav2d/env.go 的浮點範圍比較會接受非有限值，輸出 JSON 才失敗。四個公開入口重現見 nonfinite-probe.json，AGENTS.md 已記為獨立後續工作，本票保持既有輸入語意。

[P2] [CONFIRMED] experiment/nav2d_suite.go 的 Policies 沒有複製，呼叫端在回傳後改動 Policies 會修改 report.Config，已存的 hash 不再相符。Seeds 的相同修改不影響報告。公開入口重現見 ownership-probe.json，屬既有切片所有權問題，已記於 AGENTS.md，建議後續補複製與雙向修改回歸。

## Verified

- 六個新增頂層回歸先在骨架失敗，填入修正後通過，最終測試指紋保持。初次新測試誤用了 Step 回傳數量，編譯錯誤保留；修正測試後取得真正行為失敗，再凍結。
- 八個真實 CLI 流程包含三個正向、兩份 help、兩個非法任務與拒絕既有目的地；六份正向報告共十六筆 run，均無 failed，Python 獨立重算 hash 相同。
- 修正前後八份 task report 共二十四筆 run，所有非 Config／ConfigHash 欄位相同。明填自訂設定的 hash 也相同。
- 651 份 Go 來源與最終測試凍結一致，3,124 份範圍外既有檔案保持。完整一般與 race 各 57 套件、build、vet、gofmt、mod verify、tidy -diff、API discoverability 與八個格式／治理頂層檢查通過。
- Luna max 的獨立唯讀審查完成，CLEAN、無新增 finding，Root 已核對原始碼與實際證據。實作 685a090 已提交推送，遠端 main 核對一致，見 [delivery.json](delivery.json)。

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| 派工 | OpenCode／Claude／Antigravity／Luna | 沒有接收任何 worker 修改。原始碼存取的 OpenCode 呼叫被自動審查拒絕，隔離契約呼叫則 403；其他備援額度／容量受限。Root 自行完成小範圍實作。 |
| 提交掃描 | 本票 staged diff | scripts/scan-commit.sh 回傳 PASS，實際回條見 delivery.json。 |
| 本機 CLI | /tmp 與新的系統暫存目錄之執行檔、本證據目錄新報告 | 只建立本票人工報告，既有目的地拒絕覆寫。沒有部署或變更服務。 |
| 提交推送 | origin/main | 實作 685a090 已推送，git ls-remote 的 main 與本機一致。 |

宿主：Codex。
OpenCode / opencode/big-pickle / run --pure --agent build --model opencode/big-pickle --format json：原始碼派工拒絕，隔離人工契約實際 403。
Claude / claude-sonnet-5-5 / -p --permission-mode acceptEdits --restricted --safe-mode --strict-mcp-config --tools empty --output-format json：429 weekly limit，沒有模型回應。
Antigravity / claude-opus-4-6-thinking / --mode plan --sandbox --disable-slash-commands --print-timeout 2m --output-format json：429 Individual quota reached，沒有模型回應。CLI 另警告 plan 與 disable-slash-commands 搭配時無效，但保留 sandbox 且沒有提供既有原始碼。
Codex subagent / gpt-5.6-luna / max、fork_turns none：首次測試派工 Selected model is at capacity。後續唯讀審查成功，CLEAN、無新增 finding，詳見 adversarial-review.md。設定依使用者提供的 AGENTS.md，沒有接收 worker 修改。
原始工具錯誤分別為 OpenCode 403: Error from provider (Console): OpenCode's free tier can only be used from within OpenCode。Claude 429: You've hit your weekly limit。Antigravity 429: Individual quota reached. Please upgrade your subscription to increase your limits.。Luna: Selected model is at capacity. Please try a different model.
每份原始輸出與旗標見 delegation.json 及 isolated-* 檔案。原派工拒絕見 auto-review-original.txt，隔離方案經自動審查允許但服務失敗，沒有把既有原始碼交給外部模型。

## Notes

完整 staged whitespace 檢查只在三份原始 Go 輸出日誌出現提示：flag help 的空格接 tab，以及 go doc 結尾空行。原始輸出保留逐位元內容，排除這三份日誌後所有其他提交檔案通過；具體指紋與命令見 staged-whitespace-check.json。

軟體紀錄正確性與原有人工任務輸出保持，不新增學習或生物機制結論。只驗證本機 macOS arm64，沒有宣稱遠端、GPU 或新模型結果。需求為 89／91，TSK-11 與 OPS-05 保持 specified。

減法提醒：共用既有預設來源，移除報告層自行推定預設任務的分支。

Next：本票無剩餘工作；後續優先處理 AGENTS.md 的導航非有限設定 P2。完整 Changed 清單見 [changed-files.md](changed-files.md)。
