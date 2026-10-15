# 文字生成詞表尺寸拒絕驗收

## 問題與依據

基準為 cbb22ac，820 份 Go／module 指紋記於 source-before.json。NewModel 接受公開 tokenizer.Vocabulary，在驗證 Size 至少為 2 及既有 ModelConfig 後，以 `vocabSize*Embed` 與 `Hidden*vocabSize` 配置 float64。通用 Trainer 的形狀檢查在配置之後，無法攔下已發生的乘法溢位。

公開 probe 使用真正的 lazy 詞表：0..255 是原始 bytes，其餘 IDs 由 `extra:{decimal-index}` 規則宣告，特殊符號解碼為空字串，範圍外 ID 回錯，Hash 是完整演算法宣告的 canonical JSON SHA256。逐一驗證介面的五個方法，不依賴假的 Size 回傳。這個 provider 只作建構控制，不宣稱高階 bos／eos 任務或 ByteVocab 保存支援。

三個失敗案例使用 Size=MaxInt、Embed=4／8／256、Hidden=4、Settle=1，確認第一個 encoder 配置的舊乘積嚴格小於零才呼叫 NewModel。此前圖與權重均有固定小型上限。禁止在未修正建構器執行巨大正值配置。合法控制 Size=256／260。日誌及獨立 canonical hash 核對保留全部結果。

## 最小修正骨架

- 新的私有 `modelParameterLengths(vocabSize, embed, hidden int) (encoder, readout int, err error)` 不配置陣列。要求 vocabSize 至少為 2、embed／hidden 為正，兩個 float64 陣列的元素及位元組均可由平台 int 表示。錯誤回傳，不依任意實體容量上限縮減模型。
- NewModel 維持現有 Size／ModelConfig 驗證及錯誤順序，在任何可變大小配置前取得兩個長度。後面的 PCG 串流、迭代順序、參數與 Trainer 保持。
- Save／Load 既有建構路徑共用 NewModel，不新增格式或遷移。既有合法 ByteVocab 模型必須保持初始化、三次更新、保存、新程序載入與下一次更新。

## 分工與固定驗收

Root 負責重現、骨架、期望結果、審查、執行、提交與推送。測試 worker 只可寫 `runs/.tmp/textgen-dimensions-20261015/tests/model_dimensions_test.go`，不得改實作。另一個實作 worker 只可寫隔離 candidate 的 `tasks/textgen/model.go` 與 `model_dimensions.go`，不得改測試。正式來源在 Root 驗收前保持不變。

失敗驗收 overlay 保留原 NewModel，只對新的私有函式提供未檢查乘法骨架。公開負容量案例必須實際失敗，唯讀私有長度測試用 BigInt 參考涵蓋 encoder／readout 各自的元素與位元組界線，不執行巨大配置。正常控制必須通過。Root 審查後凍結回歸，再交付實作。

候選與正式驗收包含：公開拒絕、正常初始化／訓練、新程序保存接續、完整 textgen／tokenizer 套件、格式、build、完整一般與 race、vet、固定依賴與治理。所有命令、環境與指紋保存，不依測試結果調整正常模型或預算。

## 保存與減法審查

暫存限 `runs/.tmp/textgen-dimensions-20261015`，必要證據保存後才清理。保護兩個原有暫存目錄、18 個原模型與六份必要資料，不更動其他未完成成果。

不新增公開設定、詞表實作產品、動態記憶體上限或 recover 包裝。只集中兩個配置長度的非配置檢查，避免在保存入口另寫不同公式。工程驗收不作零 bug、完整任務品質或跨平台執行的保證。
