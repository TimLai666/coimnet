# ASR 模型建構尺寸驗證

只調查公開 RecognizerConfig.Validate／NewRecognizer 的衍生尺寸是否在配置前驗證。不改正式來源、學習算式、資料、模型或既有執行。

目前程式以 mels+Hidden 計算節點，以 mels×Hidden+Hidden² 計算邊容量。先以精確整數及 32／64 位有號整數取整證明五組非法宣告的第一個配置容量必為負，再以公開 API 實測。probe 在每次非法呼叫前重算並要求容量為負，避免進入任何正值的大型配置。recover 只保存 panic 內容，不把 panic 改判為 API 回錯。三組 tiny 合法宣告作正常控制。

只有公開重現、根因與適用契約確認後才建立修正 ticket。任何修正都要覆蓋同一建構函式的全部衍生尺寸，在配置前回錯，不增加任意容量上限。合法可表示值不等於實體記憶體足夠。正式採用仍須遵守 104 固定來源／分析及 106 工程驗收的鎖定。

減法審查：先做不配置大型陣列的最小公開重現，不啟動訓練、不下載資料、不引入新的資源管理層。

## 已確認的根因與修正骨架

公開原始碼一般／競態各五個案例的 Validate 回成功，NewRecognizer 在第一個 make 的負邊容量發生 panic，三個 tiny 合法控制成功。原始觀察、完整設定、指紋與命令見 public-reproduction-verification.json。既有 Trainer 及保存形狀檢查晚於公開建構的配置，不能保護這條路徑。

流程維持「公開基本欄位驗證 → 共用衍生尺寸 → 陣列與參數初始化 → Trainer」。共用私有結果保存 nodes、edges、classes、encoder 及 readout 長度，全部先以既有 checkedAdd／checkedProduct 檢查，再確認實際陣列的位元組可由目前平台 int 表示。邊數使用同義的 nodes×Hidden，合法值的邊序與亂數流保持。Validate 與 NewRecognizer 使用同一尺寸計算；保存形狀檢查沿用相同結果核對密集重建陣列。不能以 recover 包住正式建構來代替驗證。

候選來源只允許私有 copies 的 tasks/asr/recognizer.go、persistence.go 與新 recognizer_dimensions.go。以 107 的四份凍結候選為底，不修改 107 原候選或四份凍結測試。正式 Go／module 仍為 815 份，107／104／106 的輸入保持。正式採用須接在 107 之後。

| 情況 | 公開結果與責任 | 驗收 |
| --- | --- | --- |
| 基本欄位非法、空 alphabet | 保留既有錯誤順序，由 Validate 拒絕 | 原有套件測試及 Unicode 回歸 |
| 加法、乘法或 float64 位元組不能表示 | Validate 與建構在配置前回錯，建構回 nil | 本票公開回歸，不使用大型配置 |
| 合法 tiny 模型 | 初始化、訓練、保存與下一次更新保持 | 修改前後完整快照及新程序接續對照 |
| 大而可表示的宣告 | 只驗證尺寸，不增加任意上限 | Validate-only 接近界線控制，不呼叫建構 |
| 保存重建形狀不相符 | 載入回錯，不放大稀疏保存物 | 保留既有 sparse amplification／非法 payload 測試 |

## 凍結公開回歸

測試 agent 只寫 runs/.tmp/asr-dimensions-20261015/tests/recognizer_dimensions_test.go，package asr_test，TestRecognizerDimensionRepresentability。前端固定 16000 Hz、Window 16、Hop 8、FMin 0、FMax 8000、Floor 1e-10，alphabet 為 A，LearningRate .01，Seed 7。

十一個非法宣告要求 Validate 回錯，原設定逐位保持。前五個與公開 probe 相同，只有它們可以呼叫 NewRecognizer，且每次呼叫前必須檢查包回的 edges<0，保證原來源不會嘗試正值巨量配置。五個配置為 mels／hidden：1／MaxInt、2／MaxInt、4／MaxInt、8／MaxInt、2／(MaxInt/2+1)。捕捉原來源的 panic 並讓測試失敗，候選必須無 panic、回 nil/error。

其餘六個只呼叫 Validate：1／(floor(sqrt(MaxInt))+1)、1／(floor(sqrt(MaxInt/8))+1)、(floor(sqrt(MaxInt))+1)／2、(floor(sqrt(MaxInt/8))+1)／2、1／(MaxInt/8+1)、MaxInt／2。整數平方根以標準 math/big 的精確整數 Sqrt 計算，不能用浮點近似挑界線。不要為了涵蓋每條拒絕而實際配置超大陣列。

兩個大型合法控制只呼叫 Validate：floor(sqrt(MaxInt/8))／2，以及 1／(floor(sqrt(MaxInt/8))-1)。用精確整數先證明所有所需 float64 陣列元素數都不超過 MaxInt/8，驗證不引入任意容量上限。三個 tiny 合法建構控制為 mels／hidden：1／2、2／2、4／2，已知 nodes／edges／encoder／readout 長度分別為 3／6／1／4、4／8／4／4、6／12／16／4。不得修改既有測試。

Root 審查及凍結測試後，先在 107 候選底上取得一般／競態的實際行為失敗，再填尺寸驗證。候選須保留原有 353 個套件測試事件並使新回歸通過。後續正式採用與完整 build／一般／race／vet／相依性／治理，依既有工程規則完成。
