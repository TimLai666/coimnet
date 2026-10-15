# `download.Fetch` cancellation resource review

審查範圍限定在現有 `download.Fetch`、`cancel_release_test.go` 及其直接呼叫。這是程式追蹤結果，不是新的 Go 執行結果。既有「goroutines=7、baseline=4」紀錄只能作為待查線索，不能單獨判定框架洩漏。

## 可確認的流程與資源所有權

| 資源 | 程式內建立／使用 | 可由原始碼確認的釋放時機 | 依據 |
|---|---|---|---|
| 取消 context 與 operation goroutine | `runCanceled` 建立 goroutine 執行 `op(ctx)`，再呼叫 `cancel()` | `runCanceled` 只有在 `done` 收到 operation 結果後才返回；30 秒逾時則測試失敗 | `cancel_release_test.go:95-110` |
| HEAD request / response body | `headSource` 以同一 context 建立 HEAD request 並呼叫傳入的 `*http.Client`；成功取得 response 後以 `defer` 關閉非 nil body | `headSource` 返回時執行 `response.Body.Close()`；若 `client.Do` 在取得 response 前回錯，該函式沒有 response body 可關閉 | `download/download.go:421-440` |
| GET response body | `downloadAttempt` 以同一 context 建立 GET request；成功取得非 nil body 後以 `defer` 關閉 | `downloadAttempt` 返回時執行 `response.Body.Close()`；body 讀取與 context 檢查在 `copyResponse` 中同步完成 | `download/download.go:487-515`, `608-655` |
| 部分下載檔案 | `Fetch` 在 HEAD 成功後建立／開啟 `.part`；`part` 由 `Fetch` 的 defer 關閉 | 取消或其他錯誤離開 `Fetch` 時由 `Fetch` defer 關閉；完成路徑先 `Sync`、`Close` 並將 `part=nil` | `download/download.go:196-202`, `204-271`, `291-305`, `860-884` |
| 部分檔案 metadata 暫存檔 | `writeMetadata` 用 `os.CreateTemp` 寫入 metadata | defer 會在未成功發佈時關閉並移除暫存檔；成功關閉後 `Rename` 發佈，並以 `removeTemp=false` 保留正式 metadata | `download/download.go:935-988` |
| download lock 與 lock file | `Fetch` 成功 `acquireLock` 後保留 `*os.File` | `Fetch` defer 先移除 `.lock`，再關閉 lock handle；移除失敗時會在關閉後重試 | `download/download.go:160-178`, `1223-1263` |
| 完成檔 hash handle 與目錄 handle | `hashFile` 開啟完成檔；`syncDirectory` 開啟目錄 | 各自以明確 defer 或同步後 `Close` 釋放 | `download/download.go:991-1012`, `1481-1495` |
| HTTP client／Transport | `Fetch` 只接收呼叫端傳入的 `*http.Client`，production `download` 程式沒有建立或關閉 `http.Transport` 的程式碼 | 本專案來源沒有定義 client、Transport 或 idle connection 的釋放責任 | `download/download.go:130`, `421-427`, `487-502` |

## 目前 cancellation 測試實際涵蓋的路徑

`testCancelDownloadFetch` 在 `httptest.NewServer` 的 handler 中只等待 `r.Context().Done()`，沒有回傳 HEAD 或 GET response；server 在函式結束時才依 defer 呼叫 `server.Close()`。測試順序是建立 server、註冊 defer、量 baseline、執行取消操作，最後才呼叫 `awaitReclaimed`：`cancel_release_test.go:174-188`。

`Fetch` 的第一個網路步驟是 `headSource`（`download/download.go:204-215`）。因此這個 handler 在現有測試中先阻塞 HEAD；取消後若 `client.Do` 回錯，`headSource` 走 `download/download.go:427-432`，不會產生可供 `downloadAttempt` 關閉的 GET response。`.part` 及 metadata 也只在 HEAD 成功後於 `download/download.go:227-263` 建立。這個案例可直接檢視 HEAD cancellation、lock defer 與提供給 Fetch 的 client/Transport 路徑，不能直接證明 GET body 的 `Close` 已被執行。

`server.Client()` 在 `cancel_release_test.go:182` 直接傳給 `Fetch`，測試沒有保留 client，也沒有顯式呼叫 Transport 的 idle connection 清理。程式碼只確認 client／Transport 是外部提供的；標準函式庫何時結束其內部 goroutine，不能由本專案原始碼推定。

## 為何現有 goroutine 數不足以定根因

`measureBaseline` 在 server 已建立、但 request 尚未發出時記錄 baseline（`cancel_release_test.go:58-68`, 呼叫於 `174-180`）。`runCanceled` 等待 `Fetch` 的 operation goroutine 返回後才把控制權交回測試（`95-110`）。然而 `awaitReclaimed` 在 `testCancelDownloadFetch` 的 deferred `server.Close()` 執行前就開始（`174-188`）。換句話說，回收檢查期間 server 仍然活著，handler、server connection 或 client/Transport 的清理時序都可能尚未完成。這是測試觀察窗口的已確認特性，不代表其中任一項已被證實洩漏。

目前沒有程式碼證據顯示 `download` package 自己額外啟動 goroutine，且 Fetch 的檔案與 response body 都有同步 defer／Close 路徑；但 `client.Do` 的網路實作屬於外部 client／Transport。故「7 比 4」在 `server.Close` 尚未發生的窗口取得，不能歸因於 `download.Fetch`。

## 最小可區分原因的 stack 觀察

不修改正式程式的前提下，下一次可用一次診斷性測試執行取得兩個 dump：

1. 在 `runCanceled` 返回後、`awaitReclaimed` 前（`cancel_release_test.go:185-188`）擷取 `runtime/pprof.Lookup("goroutine").WriteTo(..., 2)`。此時確認 operation goroutine 已返回，但 server 尚未 `Close`。
2. 將同一診斷流程的 server cleanup 明確放在第一個 dump 之後，呼叫既有 `server.Close()` 後立即擷取第二個 dump，再做回收等待。這個暫時性診斷只為切分時序，不能直接當成正式修正。

判讀界線如下：

- 只在第一個 dump 出現、第二個 dump 消失，且 stack 落在 `net/http` server／handler 或 client／Transport，表示現有測試窗口混入 server／Transport 回收時序，不能判為 Fetch leak。
- 在 server 關閉後仍存在的 stack，才值得進一步檢查其是否含 project 的 `download.Fetch` caller、`headSource`、`downloadAttempt` 或自訂 Transport frame。因目前 Fetch 路徑沒有明確啟動 goroutine，若只剩 `net/http` frame，仍須先確認 client／Transport 所有權。
- 若要直接證明 `Response.Body.Close`，stack dump 不夠。應另以最小 `RoundTripper`／body wrapper 記錄 HEAD 與 GET body 的 `Close`；但現有 `testCancelDownloadFetch` 在 HEAD 回應前取消，GET body 本來就未建立，不能以該測試聲稱 GET body close 已驗證。

## 尚未確認與本輪限制

- 尚未取得失敗當下的 goroutine stack，因此沒有決定性證據能判斷 7 個 goroutine 的實際所有者。
- 尚未驗證 `httptest.Server`、其 `Client` 或底層 Transport 的標準函式庫內部清理時序，亦不以標準函式庫慣例補足本專案證據。
- 本輪未執行 Go、測試、race、訓練、restore、網路或其他新程序；只使用 codebase-memory 搜尋／追蹤與唯讀原始檔案行號核對。
