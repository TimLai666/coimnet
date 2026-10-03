# 下載取消測試的未確認問題

完整 `scripts/verify.sh` 首輪的 race 執行失敗，`TestCancelReleases/download_Fetch` 在 20 次回收檢查中都是 7 個 goroutine，基準為 4，HeapInuse 在限制內。原始輸出見 [validation-first-attempt.log](validation-first-attempt.log)。同輪五個重型套件到達預設 10 分鐘上限，沒有觀察到 race detector 的資料競態報告。

`cancel_release_test.go:174-188` 的伺服器 handler 等待請求取消，伺服器清理延後到回收檢查後。`download/download.go:422-427` 與 `492-501` 的 HEAD、GET 都使用帶 context 的 request。下載、取消測試、既有 PPO 與模仿程式與基礎提交 `6fcd9e215affa38e4aff451db0081331c989f117` 相同。

可能原因包含 HTTP transport 連線清理時序及高負載下的清理延遲，缺少失敗當下的 goroutine stack，尚不能判定是哪一項。只在 Git 外建立 Go overlay，在失敗時追加 stack 診斷，沒有修改儲存庫程式。

`go test -race -count=3 -v -run '^TestCancelReleases$/download_Fetch$' -overlay <temporary-overlay> .` 三次通過。`go test -race -count=1 -v -overlay <temporary-overlay> .` 整個根套件也通過。[專項日誌](cancel-diagnostic.log)與[根套件日誌](cancel-full-diagnostic.log)保留實際結果，沒有取得失敗 stack，不能宣稱根因已確認或問題已修好。

後續完整 race 保留所有測試，改用 `-p 2 -timeout 30m` 限制同時執行的套件數並允許重型套件完成。排程改變後的通過不證明根因。下次重現時應保存失敗 stack 並比對取消、transport 關閉與測試伺服器回收順序，避免放寬原回收門檻。
