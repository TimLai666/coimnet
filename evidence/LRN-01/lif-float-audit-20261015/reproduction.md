# LIF 跨建置差異的驗收範圍

本人工案例的差異來自 `dynamics/lif.go:383` 的乘加融合項不同。普通建置先取整 `lambda*voltage`，再融合 `alpha*drive` 與加法。競態建置先取整 `alpha*drive`，再融合 `lambda*voltage` 與加法。兩種做法都符合 [Go 浮點運算規格](https://go.dev/ref/spec#Floating-point_operators)，本機 Go 1.26.5 的規格節錄與指紋另存於 `go126-floating-spec.html.txt`、`environment.json`。

最早不同的儲存值是 `cand[1][0]`，但神經元處於不應期，該值沒有進入後續電位與梯度。第一個傳到梯度的差異在 `cand[4][2]`，接著影響替代導數、電位和 `Inputs[4][2]`，最後造成 `Bias[2]` 的一個 binary64 間距差異。最終核心梯度的其他值相同。`trace-comparison.json`、`public-comparison.json` 保留完整差異。

公開探針使用實際 Insyra float32 編碼器與讀出 `MatMul`，再以相同輸入與上游比較公開 `Network.LossGradient` 和 `LIF.Forward/Backward`。兩種建置各自的完整核心梯度逐位相同。唯讀 trace 觀察只透過新增測試檔案的 overlay，正式方法沒有插入 log；觀察後的所有公開值與原始控制相同。4 次新程序重跑與移除探針無用陣列後的結果也保持。同一案例的公開持續狀態 `Advance`，一次六步與六次單步的輸出、放電、各步電位及最終完整狀態也符合 `Forward`，見 `state-parity.json`。

`complete-reference-bits.py.txt` 以精確有理數乘加及最接近值取整重建完整前向和反向。它只取用實際編碼後輸入、上游、參數，以及已觀察的 lambda、alpha、theta slope 常數，不拿觀察到的 drive、candidate、電位或替代導數當前向答案。兩種建置的全部前向 bits 與完整梯度 bits 都符合參考。這不包含對 `Exp/Expm1` 函式本身的獨立正確性證明。

原執行檔的 `Forward/Backward` 機器碼及來源 PC 對應已保存。靜態來源對應結合運算值支持上述來源層因果，沒有捕獲每次執行的 CPU 指令、暫存器與節點配對，不能宣稱已證明所有指令的實際執行順序。分工報告與 Root 採用／修正判斷見 `root-review.json`。

## 重現

在此專案建立 `runs/.tmp/lif-float-audit-20261015`。將 `public-core-min.go.txt` 和 `audit_lif_trace_test.go.txt` 複製到該目錄，副檔名分別改為 `.go`。沿用 `cache-prepare.py.txt` 的專案內快取設定。該工具依賴本機既有、相同版本的模組來源，不能在缺少來源時直接套用。

`public-min-build.json`、`public-min-race-build.json` 保存實際建置命令，`public-normal.json`、`public-race.json` 保存控制執行命令。執行探針時可指定一個尚不存在的專案內 JSON 路徑，作為 trace 測試的 fixture。`trace-normal.json`、`trace-race.json` 保存 overlay 測試命令和 fixture／輸出環境變數。overlay 只將虛擬 `dynamics/audit_lif_trace_test.go` 映到本輪測試檔，不替換正式方法。`complete-reference-bits.py.txt` 可直接用 Python 3 執行並比對已保存證據。

每份命令回條包含環境相關設定、來源核對、耗時、結果和日誌指紋。重跑後先保存必要證據，確認程序結束，再依 `cleanup.py.txt` 的範圍保護清除自己的暫存。該清理工具只適用於此票的既有目錄身分與回條，不能改名後任意清除別的目錄。

## 裁決與限制

沒有確認本案例的正常公式缺陷。主規格 9.4 沒有明示普通與競態建置必須互相逐位相同，既有同建置重現測試保持原判準。本票沒有更動模型格式、正常公式、公開 API 或相依性，也沒有放寬既有測試。

本案例的預測、損失、放電紀錄與突觸輸出相同。長期訓練品質、真實單字辨識分數、全腦及跨硬體行為沒有在本票量測，不能從一個間距差異推論品質好壞，也不能以本票通過宣稱整個框架沒有 bug。

【減法審查】建議移除已取代的中間算術檔與本輪快取、執行檔、虛擬測試檔。保留能重現結論的最小探針、完整參考、原始控制與證據，不增加公開診斷 API。品質量測依原有任務獨立驗收。
