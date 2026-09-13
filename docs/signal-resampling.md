# 訊號取樣與時間對齊

`signal.NewStreamingResampler` 將單一通道的連續值取樣到指定神經步號。`signal.ResampleOffline` 另提供離線線性插值。兩者沿用 `Signal`，適用於 `continuous`、`activity` 與 `modulation` 的時間點樣本，輸入 `Duration` 必須為零。

可執行範例見 [Go example](../signal/resample_example_test.go)，執行 `go test ./signal -run ExampleNewStreamingResampler -v`，或用 `go doc ./signal.ResampleConfig` 查閱設定。

## 三種時間分開

`SourceUnit` 宣告輸入時間戳的單位。`SourceTicks / SimulationSteps` 指定每個神經步號對應多少來源時間刻度：

```text
來源位置 = 絕對神經步號 × SourceTicks / SimulationSteps
輸出時間 = Clock.TimestampAt(絕對神經步號)
```

來源與輸出共用零起點。比率必須由呼叫者明示，框架不自動推定生理時間。未經生理校準時，輸出 `Clock` 使用 `model_step` 或 `normalized_time`。`Clock.CurrentStep` 不控制取樣範圍，範圍由 `[StartStep, EndStep)` 決定。

例如，人工設定 `SourceUnit=ms`、`SourceTicks=1`、`SimulationSteps=2` 時，神經步號 0、1、2 對應來源 0、0.5、1 ms。若來源在 0 ms 與 2 ms 分別為 2、6，即時模式在步號 0 至 4 輸出 `2, 2, 2, 2, 6`，離線線性插值輸出 `2, 3, 4, 5, 6`。這個對應只是一個工程範例。

時間比較使用整數乘除及餘數，避免每步捨入造成累積漂移。線性插值的數值部分使用 float64。實際電腦耗時由外部量測，取樣器不讀系統時鐘，也不把神經步數當成執行秒數。

不同頻率的通道各建一個取樣器，使用同一個輸出 `Clock` 與步號範圍。各通道的結果可依輸出時間對照。每個通道仍是獨立的 `Observation`，目前沒有把多個取樣器自動合併成共用來源序號的操作。

## 即時與離線模式

| 模式 | 取值規則 | 入口 |
| --- | --- | --- |
| `CausalHold` | 使用來源位置當下或以前，最近一筆已確定的樣本 | `NewStreamingResampler` 或 `ResampleOffline` |
| `OfflineLinear` | 使用前後兩筆樣本線性插值，可讀取後面的資料 | `ResampleOffline` |

回傳 `ResampledBatch.Mode` 標明模式。即時建構器拒絕 `OfflineLinear`，沒有整段標準化或使用未來統計的處理。呼叫者須保留模式與設定，離線插值結果不能用來宣稱即時延遲。

一個輸入串流的經驗、串流、通道、種類、單位、形狀、有效範圍、品質與編碼器版本都必須一致。輸出保留這些數值屬性，串流 ID 改為 `OutputStreamID`，來源序號改為絕對神經步號，時間改為輸出時鐘，持續時間保持零。要追溯原始來源序號時，請連同原始資料與取樣設定保存。

## 分塊與結尾

`Push(ctx, chunk, watermark)` 的 `watermark` 是來源時間，意思是「這個時間以前的資料都已提供」。本次樣本時間必須位於前一次與本次 watermark 之間，含兩端。watermark 可以重複，也可用空 chunk 向前推進。

恰好等於 watermark 的樣本群暫不封閉，下一塊可以補上同時樣本。群組封閉後，按來源序號排序，以最大序號的值覆寫。同一時間分在不同塊，結果一致。時間倒退、重複序號、封閉後補送舊資料、後面時間使用較小序號都會回傳錯誤。

`Push` 只輸出來源位置嚴格小於 watermark 的步號。`Finish(ctx)` 封閉最後一群，再以明示的 `HoldLast` 策略延續最後值至 `EndStep`。第一筆樣本以前不產生訊號，空串流回傳明確空陣列。結尾不補零，第二次 `Finish` 或結束後 `Push` 都會失敗。

資料錯誤、容量超限或取消時不回傳部分結果，取樣器狀態不變，可以修正輸入後重試。對同一實例的呼叫須依序執行。

## 容量與適用範圍

`ResampleLimits` 的三項限制都必須大於零：

- `MaxBufferedSignals`：尚未封閉的樣本加上本次輸入的數量上限。
- `MaxOutputSignals`：整段 `[StartStep, EndStep)` 的步數上限，包含沒有樣本的前段。
- `MaxValues`：本次輸入、尚未封閉樣本、一筆保留樣本與整段輸出範圍的邏輯數值數量上限。以每筆樣本的數值個數乘上上述筆數，在複製數值前檢查。

這是容量契約，沒有包含切片與字串、驗證暫存副本、Go 執行環境及配置餘量，不能當成程序記憶體上限。離線入口須一次提供完整樣本，受同樣的 buffer 限制。

目前只支援時間點樣本及共用零起點。持續區間、離散脈衝、實驗干預、音訊解碼、抗混疊濾波及媒體特徵編碼需要各自的規則，尚未接入。這次取樣功能不包含神經核心的可微編碼器，也不宣稱可用於一般音訊頻率轉換。完整工作仍由 [ticket 03](tickets/03-signals.md) 追蹤。
