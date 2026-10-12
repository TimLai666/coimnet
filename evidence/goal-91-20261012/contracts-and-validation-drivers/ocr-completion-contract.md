# OCR 原規格功能補齊

## 目標、入口与支援範圍

補主規格 §13.2 已要求的 PNG/JPEG 讀取、單欄水平頁面分行送入同一模型、尺寸／色彩／縮放／方向／座標 metadata、固定與可訓練 encoder 對照、空輸出率。沿用現有 glyphs.Image、Recognizer 的欄序列 CTC 和 Page/Block，不增加第二個OCR模型、外部辨識器或新依賴，不宣稱任意頁面版面或自然文字品質。

資料流：有授權或測試自行產生的PNG/JPEG → 有上限的解碼／明示灰階與ink polarity → 尺寸和來源指紋metadata → 單行TrainLine/Infer，或 SegmentLines → Crop → 固定nearest縮放到同一Recognizer的Height → Infer → 每個Block.Text按原ReadingOrder回填。預設固定encoder保留；可選trainable identity-initialized encoder與既有核心一起更新，設定與checkpoint一起保存。

## API、資料形狀與實作格子

- 新 raster.go：`ImageOptions{MaxEncodedBytes int64, MaxPixels int, Height int, LightInk bool}`。零bytes/pixels限制使用明定16MiB／1,048,576 pixels預設，負限制拒絕；Height0不縮放，正height保持長寬比用nearest。
- `ImageMetadata{SchemaVersion,SourceSHA256,Format,SourceColorSpace,ScalingMethod,Orientation,TextDirection,CoordinateSpace string; SourceWidth,SourceHeight,Width,Height int; ScaleX,ScaleY float64; Region Rect}`。
- `Raster{Image glyphs.Image; Metadata ImageMetadata}`。
- `LoadImage(ctx context.Context, path string, options ImageOptions) (Raster,error)`。
- 新 page_recognizer.go：`PageOptions{Threshold float64; MaxLinePixels int}`。MaxLinePixels0使用1,048,576預設，負數拒絕。`(*Recognizer).InferPage(ctx context.Context, image glyphs.Image, options PageOptions) (Page,error)`。
- 既有 RecognizerConfig 增加 `TrainableEncoder bool json:"trainable_encoder,omitempty"`，false保留舊格式與行為；既有 EvalReport 增加 `EmptyOutputs int` 和 `EmptyOutputRatio float64`。空輸出定義為字串完全空，不去掉空白或改原文。
- realocr --encoder fixed|trainable，預設fixed；保留--model-input/output和原分割、seed、資料量；report寫明encoder模式，encoder權重數、核心/讀出權重數與實際更新，避免把新增可訓練容量當核心效果。

不可更改 glyphs/ctc/learning/通用checkpoint契約。既有page.go需要補同類已確認安全缺口：finite threshold/background、乘法溢位、Crop像素形狀缺失不能panic。NewRecognizer的新增設定必須沿用Save/Load比對；舊v1無trainable欄仍可讀。

## 影像metadata規則

記錄同一次有上限的來源 bytes 的 SHA256，DecodeConfig驗證原尺寸及pixel上限後才完整Decode。只允許PNG/JPEG。以Go non-premultiplied RGB轉明示版本灰階，alpha先合成白色背景；預設dark ink=1-background intensity，LightInk反轉。保存實際原尺寸與輸出尺寸／比例，Region為來源inclusive-exclusive像素座標。方向固定horizontal-left-to-right、stored-pixel-order，未實作EXIF自動旋轉、ICC轉換或其他方向，文件不可宣稱支援。先檢查保持比例後的pixel容量和所有乘法，不能因height/width放大而先配置巨量陣列。取消前後檢查，不返回部分Raster/Page。

## 正常、錯誤、空值與互動驗收

| 情境／操作 | 結果 | 驗證入口 |
| --- | --- | --- |
| PNG與JPEG、alpha/灰階/RGB、自製Unicode標籤 | 欄序列輸入與metadata可核對，PNG精確、JPEG按解碼後值 | LoadImage + TrainLine/Infer |
| 缺失/破損/空/不支援格式、nil/cancel、超encoded/pixel限制、縮放溢位 | 明確error，零結果，前處理不先巨量配置 | LoadImage |
| 多行、不同line高度／寬度、blank page | 同一Recognizer逐行Infer，reading order與來源coords保持；blank page空非nilBlocks | InferPage |
| 缺pixel／非有限／非法threshold或background／overflow Rect | error，不panic，呼叫端image不改 | SegmentLines/Stack/Crop/InferPage |
| 固定/可訓練encoder | 相同seed、初始全參數、資料、核心/讀出容量與預算；固定encoder位元不變，可訓練encoder有gradient更新；不要求後者贏 | public constructors、TrainLine、Snapshot |
| 部分或全部空預測、參考空字串 | EmptyOutputs與比例正確，CER沿用原空參考規則，原文字不改 | Evaluate |
| 保存可訓練設定→新模型Load→下一步 | mode/optimizer/parameters/inference/next-update相同；舊fixed v1兼容 | Save/LoadRecognizer |
| realOCR未知encoder／model模式不一致 | 未訓練前error，沒有partial report，不重寫既有模型 | realocr run |

## 三條完整使用流程

1. 自製有来源與fingerprint的多行PNG/JPEG → LoadImage → 區塊辨識 → 與直接同一Recognizer.Infer各正規化crop比對 → 保存與Load後相同，呼叫端頁面／模型快照不被改。
2. 同seed、同資料的fixed/trainable兩組 → TrainLine/Evaluate → EmptyOutputs、編碼器權重/核心權重數/梯度與更新對照 → 可訓練模型Save/Load後續訓。無事後品質門檻，不要求模型勝出。
3. 真實KMNIST固定同分割／seed／1epoch／1,000train/200test，fixed與trainable各跑一次，保存原始report、encoder容量、空輸出比例及來源；同模型讀取新程序推論一致，完整相關測試與本機full checks通過後保存證據、清owned暫存。

## 分工與驗證

主agent寫以上API空骨架，test worker只改新測試檔；審查並實機跑RED後，實作worker只填raster/page/config/realocr指定格子，不得改tests。Root完整審diff和自己跑完整流程，重跑所有受影響一般/race及必須full validation。仍未驗證的自然OCR品質保持未驗證，不能以metadata或synthetic PNG成功冒充。
