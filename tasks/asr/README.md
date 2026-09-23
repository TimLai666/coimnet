# 音訊轉文字 SDK

`Recognizer` 將音訊轉成 log-mel 特徵，每一窗推進一次共用神經核心，再用 CTC 解碼成文字。
`TrainUtterance` 接收音訊與文字稿，`Transcribe` 處理整段音訊，`Evaluate` 計算 CER 與 WER。
完整公開介面可用 `go doc ./tasks/asr` 查閱。

## 匯入錄音與分割資料

`ReadDataset("manifest.json", 16000, 0.8)` 讀取使用者指定的資料清單。清單的 `schema` 為 `coimnet-asr-dataset/v1`，`license` 須填 `holder`、`terms`、`source`；每筆 `recordings` 須填相對於清單所在目錄的 `path`、`text`、`speaker`、`session`、`sample_rate`、`channels`。匯入器只接受 PCM 16-bit WAV，會核對音檔的取樣率與聲道數，並依序混成單聲道、線性重取樣、按峰值正規化。回傳的 `Dataset.Metadata[i]` 對應 `Dataset.Utterances[i]`，記錄原檔 SHA-256 與每一步的設定及結果。

`SplitBySpeakerSession(dataset.Utterances, 0.25, 42)` 以說話者和錄音場次的連通群分成訓練集與測試集。同一說話者或場次會留在同一邊；如果所有錄音都連成一群，函式會回錯。群組大小不一時，在搜尋容量與工作量限制內找全域最接近指定比例的可行分割；超過限制會回錯，不靜默選擇偏差較大的分割。

清單上限 16 MiB。`ReadDataset` 預設限制每個原始 WAV 64 MiB、每筆處理緩衝估算 512 MiB、全批處理後樣本估算總額 2 GiB；超過時回錯。需要更大的授權語料時，可在估算可用記憶體後以 `ReadDatasetWithLimits` 明確指定 `DatasetLimits`。這些限制是配置前的資料緩衝估算，不是程序 RSS 上限；匯入器仍會把全部處理後樣本留在記憶體。清單和錄音透過固定的資料夾根目錄開檔，會拒絕指向資料夾外的連結；來源 SHA-256 與解碼使用同一次讀到的 WAV 位元組。授權欄位是來源紀錄，匯入器不會替使用者判定授權是否涵蓋預定用途。

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
`MeasureStreaming(ctx, samples, chunk)` 執行同一條逐塊路徑，回報整次推論、最慢單塊與 flush 的本機計算耗時，以及第一次輸出文字時已收到的樣本數。這是離線計算量測，沒有包含麥克風、傳輸或播放等待時間，也不表示字詞時間對齊精度。比較不同機器的結果時，連同 `go version`、`uptime`、資料指紋及區塊大小一起記錄。
`EvaluateStreaming(ctx, utterances, chunk)` 對每筆錄音建立獨立串流，彙總文字、CER／WER 與上述逐筆量測。耗時欄位每次執行都會變動，重現性比對應排除耗時欄位。

## 離線範例

`coimnet examples run asr --out asr-fixture.json` 在記憶體產生四組 `a`、`b`、`c` 人工音調，按說話者及場次分割，訓練小型辨識器，並在保留集跑整檔與串流評估。報告列出固定組態、資料範圍、CER／WER、逐筆串流計算耗時及限制。`--out` 只建立新檔，不覆寫既有成果。這個範例不讀真實 WAV，也不保存訓練後模型；匯入使用者授權資料請從 `ReadDataset` SDK 開始。

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

測試涵蓋人工音調序列的訓練與保留集 CER／WER、相同設定重現、任意分塊、短尾、取消、快照往返、資料清單匯入、說話者／場次分割、逐塊耗時報告與錯誤設定拒絕。端到端測試以人工 WAV 實際走過匯入、分割、訓練及兩種評估。
人工資料只有 `a`、`b`、`c` 對應的音調，不是自然語音資料。沒有自然語音辨識率、時間對齊精度或全腦模型訓練成績。
完整語音需求與尚待完成的真實資料驗收及完整保存介面，見 [ticket 29](../../docs/tickets/29-real-tasks-ocr-asr-text-and-media-generation.md)。
