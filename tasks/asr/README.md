# 音訊轉文字 SDK

`Recognizer` 將音訊轉成 log-mel 特徵，每一窗推進一次共用神經核心，再用 CTC 解碼成文字。
`TrainUtterance` 接收音訊與文字稿，`Transcribe` 處理整段音訊，`Evaluate` 計算 CER 與 WER。
完整公開介面可用 `go doc ./tasks/asr` 查閱。

## 因果串流

在已建立的辨識器 `r` 上呼叫 `NewStream()`，依收到的順序呼叫 `Feed(ctx, samples)`，最後呼叫 `Flush(ctx)`。
每次回傳這次新增的文字，`Text()` 回傳累計文字。每條串流保存自己的模型參數與神經狀態，建立後繼續訓練 `r` 不會改變既有串流。

- 傳入單聲道 `[]float64`，取樣率須符合 `RecognizerConfig.SampleRate`，串流不會自動重取樣。
- `Feed` 只處理已收到的完整視窗，保留下一窗會用到的重疊樣本，不讀未來音訊或整段未來統計。
- `Flush` 把尚未分析過的尾端補零成一窗。若只剩上一窗的重疊樣本，就不會重複分析。
- 空串流可以結束，結束後再次 `Feed` 或 `Flush` 都回錯。取消、非有限值或計算失敗時，該次呼叫不改變狀態，可以重送相同區塊。
- 同一條串流的操作由呼叫端依序執行，不支援同時呼叫 `Feed`、`Flush`、`Snapshot`。

整檔模式會捨棄不足一窗的尾端，串流模式會在 `Flush` 補零。
因此兩者在長度為 `Window + k*Hop` 時有相同分窗；其他長度可能得到不同文字，這是尾端處理方式的差異。
`TranscribeStreaming(ctx, samples, chunk)` 提供以固定區塊大小走完這套流程的便捷入口。

## 保存與恢復

`state := stream.Snapshot()` 取得獨立的 `StreamState`。
`r.RestoreStream(state)` 驗證前處理、取樣率、字母表與神經狀態後，建立可接續的串流。
快照中的模型參數是串流建立時的版本，恢復時使用那份參數。
快照包含待處理音訊與已輸出的文字，儲存位置及存取權限由呼叫端管理。

`StreamState` 是 SDK 的記憶體資料契約，可做 JSON 往返。它目前沒有獨立的檔案載入器、容量限制或原子落盤 API。
呼叫端若自行保存，須限制輸入大小並處理安全寫入，不能把 `checkpoint.SaveIndividual` 單獨保存的核心狀態當成整條音訊串流快照。

## 驗證範圍

```sh
go test -count=1 -v ./tasks/asr
```

測試涵蓋人工音調序列的訓練與保留集 CER／WER、相同設定重現、任意分塊、短尾、取消、快照往返與錯誤設定拒絕。
人工資料只有 `a`、`b`、`c` 對應的音調，不是自然語音資料。沒有自然語音辨識率、時間對齊精度或全腦模型訓練成績。
完整語音需求與尚待完成的資料匯入、說話者／場次分割及延遲報告，見 [ticket 29](../../docs/tickets/29-real-tasks-ocr-asr-text-and-media-generation.md)。
