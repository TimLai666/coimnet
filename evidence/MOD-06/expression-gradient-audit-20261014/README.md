# 記憶讀出調節的梯度稽核

160 組公開 `PredictAll`／`LossGradientFrom` 一般與 race 案例通過，最大梯度絕對差為 1.11022302e-16。80 組觀察到同一受體在兩個區域的不同佔用率。四種刻意錯誤的獨立參考都出現預期的數值失敗。既有化學回歸一般／race 各通過 100 個測試事件，沒有新的正式程式缺陷。

矩陣涵蓋控制器／神經活動來源、單一／兩個讀出節點、正／負／零增益係數、夾限／固定中性增益、明示／核心初始活動及完整／一列截斷視窗。兩個神經元、四步與一個化學通道是人工驗證圖。這份證據用來核對工程公式，不驗收真實任務品質、生物校準、全腦效能或完整零 bug。

`comparison-summary.json` 保存實際矩陣計數及四種數值錯誤。`root-review.md` 整合獨立審查與 producer 可達性核對。`verification.json` 是 MOD-06 的需求證據入口。正式 806 份來源和既有測試沒有更動。前次同來源的完整一般／race 各 58 套件證據已重新核對指紋，沒有宣稱這輪重跑全套。

## 重現

本輪 overlay 路徑只供原始命令紀錄，執行目錄清理後不能直接拿原 overlay 重跑。先在專案內建立新的 `runs/.tmp/expression-gradient-replay`，把 `expression_reference_v2_test.go.txt` 複製為該目錄的 `reference_test.go`，再建立 JSON overlay：`Replace` 的 key 是專案根目錄加 `learning/chemical_mixed_reference_test.go`，value 是新檔的絕對路徑。正式檔案不寫入。`source-before.json` 是執行來源的 SHA-256 清單。

將 `GOCACHE`、`GOMODCACHE`、`GOTMPDIR`、`TMPDIR` 指向新目錄內各自的子目錄。原環境採用本機唯讀 module proxy，設定見 `environment.json`；其他電腦須自行準備 `go.sum` 所固定的相依性。執行 `go test -overlay <新 overlay> -json -count=1 -run '^TestExpressionCompositionReference$' ./learning`，再加 `-race` 重跑。兩次各有 160 個子案例及一個頂層 pass。

錯誤控制的完整來源保存在 `missing-*_test.go.txt.gz`，用標準 gzip 解壓至自己的執行目錄，再建立對應 overlay。四份 `control-*.json` 記錄各自的完整命令與精確案例篩選，預期 exit 1，且實際日誌必須有 `core.weights[0]` 的數值差異，不能以編譯失敗代替。來源及無損封存指紋見 `reference-v2-provenance.json`、`archive-provenance.json`。

保存新證據後，只清除自己建立的 replay 目錄。本輪原執行目錄的完整清理回條另見 `cleanup.json`。

## 減法審查

沿用既有獨立微分參考及公共入口，沒有新增正式驗證器或更改正常公式。重複的錯誤參考與大型原始日誌採無損 gzip 保存，降低證據重複內容。
