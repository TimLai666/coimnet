# 串流分段完整狀態驗收

既有框架的 Feed 分段及恢復契約保持。這是任意分段的性質驗證，目前沒有失敗案例，不把缺少測試稱為產品 bug。

## 共用契約與責任

Root 固定預期、來源、驗收與刻意錯誤控制，親自審查及執行全部 Go。測試子代理只能新增 `tasks/asr/stream_partition_test.go`，不能改既有測試或產品程式，不能執行編譯或訓練。若確認產品缺陷，先凍結可重現回歸，再另外決定最小修正。

## 測試格子

`TestStreamPartitionFullState(t *testing.T)`：使用既有 `streamTestFixture(t)`，長度固定為 0、1、Window−1、Window、Window+Hop−1、Window+Hop、len(sample)−1、len(sample)。每個長度以單一 Feed 建立參考，分別在 Flush 前及 Flush 後比對 `json.Marshal(Snapshot())` 的完整位元組。分段方法為一樣本、循環邊界大小、三個固定 seed 的可變大小。每個 Feed／Flush 回傳的新增文字串接須等於 Text。空 Feed 不能改變快照。完整錄音控制必須有非空文字，不能只比較空字串。

`TestStreamPartitionRestoreFullState(t *testing.T)`：長度為 Window−1、len(sample)−1、len(sample)，分段方法為循環邊界大小與三個固定 seed。每個區塊後以 JSON 往返 StreamState，再透過公開 RestoreStream 重建，立即比較完整快照，最後 Flush 前／後都須與同段音訊的單一 Feed 參考相同。另驗證 flushed 狀態可恢復且快照保持。各函式結束確認共用 recognizer 快照及來源音訊保持。

共同純測試 helper 可放同一檔案，不複製 framing、CTC 或神經更新公式。亂數只決定分段大小，使用局部 PCG(17/29/43, 137)，不接全域亂數。循環大小固定為 Window−1、1、Hop−1、1、Hop+1。chunk 必須至少一個樣本，最後截到剩餘長度。

## 獨立有效性檢查

Root 在隔離 overlay 分別讓 Feed 重設前個 CTC class，及 RestoreStream 遺失 pending，新增測試必須拒絕這兩種刻意錯誤。這些失敗不算正式框架的新缺陷，不修改產品來源。

## 工程與清理

一般／競態專項與完整 gofmt、build、test、test -race、vet、模組驗證及固定 Insyra v0.3.4 都由 Root 執行。與上一輪 855 份來源及完整 test2json 終端事件比較，只允許新增測試。所有快取、overlay、工具與暫存限於本票 `runs/.tmp/stream-partition-20261017`，證據保存及無開啟程序後才清除，原有暫存保持。

減法審查：沿用已訓練且有非空輸出的共用人工 fixture，不另外訓練新模型或引入測試函式庫；沒有確認缺陷就保持產品程式不變。
