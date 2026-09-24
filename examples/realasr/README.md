# 真實授權語音範例

這個獨立範例讀取使用者提供的 `coimnet-asr-dataset/v1` manifest 與其中列出的 PCM 16-bit WAV，實際走過資料匯入、說話者／場次分割、一次訓練、獨立推論及 CER／WER 評估。它不下載資料，也不保存模型或 checkpoint。

```sh
go run ./examples/realasr --manifest /path/to/manifest.json
```

`--help` 會列出完整格式、固定設定及錯誤條件。成功時只在 stdout 輸出一份縮排 JSON，失敗訊息寫到 stderr 且不輸出部分報告。

manifest 必須符合 `coimnet-asr-dataset/v1`，並在 `license` 提供非空的 `holder`、`terms`、`source`。每筆 recording 需要相對於 manifest 的 `path`、`text`、`speaker`、`session`、`sample_rate` 及 `channels`。匯入器只接受 PCM 16-bit WAV，會依 SDK 規則混音、線性重取樣及峰值正規化；範例固定目標取樣率 16 kHz、峰值 0.8。

固定執行設定如下：

- 以 speaker/session 的連通群切分，測試比例 `1/3`、seed `42`。
- 從訓練集文字建立排序去重的 rune 字母表，避免把保留集文字洩漏到模型設定。
- log-mel 前端為 window/hop/mels `256/128/12`、`FMin=0`、`FMax=8000`、`Floor=1e-10`。
- hidden `24`、learning rate `0.001`、模型 seed `5`。
- 每個訓練樣本只呼叫一次 `TrainUtterance`。無合法 CTC 對齊的樣本會計入 `impossible_count`，不會被當成成功更新。

JSON 會列出匯入、訓練與保留集數量、實際解析的 manifest SHA-256、每筆解碼 WAV 的 SHA-256 與前處理紀錄、授權欄位、訓練字母表、固定設定、保留集訓練前後的 CER/WER，以及訓練後對第一個保留樣本另做一次 `Transcribe` 的結果。CER/WER 與文字結果是觀察值，低準確率不會被宣稱為模型學會。

結果只適用於命令當次提供的授權資料與上述固定設定。一次訓練通過不代表收斂、自然語音品質、跨說話者泛化、字詞時間對齊、GPU 加速、跨平台執行或全腦模型能力。資料授權範圍由 manifest 的持有者與條款決定。

本範例的測試使用暫存資料夾內自製 WAV，不依賴網路或外部資料：

```sh
go test -count=1 ./examples/realasr
```
