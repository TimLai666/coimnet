# 文字模型詞表尺寸驗收

NewModel 對無法以 int 表示的 encoder／readout float64 陣列，在配置前回傳錯誤。有效模型的初始化、訓練與保存接續保持。

- 凍結回歸：一般／競態各 18 個原始失敗、6 個正常通過，修正後各 24 個通過。
- 相容性：8 組新舊模型配對與 16 次舊保存新程序接續的完整 JSON 相同，16 個模型都有實際參數更新。
- 完整工程：一般／競態各 58 套件通過，5,915／5,914 成功、22／23 略過、零失敗，格式、build、vet、固定依賴及最後 31 項治理通過。
- 自有暫存已清除。原模型、必要資料與兩個舊暫存目錄保持。

結果與限制見 [verification.json](verification.json)，交付見 [delivery.json](delivery.json)。這是建構拒絕與相容性驗收，不是模型準確率提升或生物機制定量證據。

## 重現來源與保存物

原來源為 cbb22ac，820 份指紋見 [source-before.json](source-before.json)。原 NewModel 所在檔案逐位保存於 [source-before-model.go.txt](source-before-model.go.txt)。修正後的 822 份來源見 [formal-source.json](formal-source.json)，凍結測試見 [regression-freeze.json](regression-freeze.json)。

[work-sources.tar.gz](work-sources.tar.gz) 保存 driver、compat、tests、skeleton、candidate、overlay 與完整工程 runner，成員與雜湊見 [work-preservation.json](work-preservation.json)。四份 compatibility tar.gz 保存 16 個人工模型及初始、三次更新後、下一次更新與恢復輸出，成員見 [compatibility-preservation.json](compatibility-preservation.json)。每個成員都與原檔逐位比對後才清理。

重現時在專案 runs/.tmp 的新子目錄取回所需原檔，依各命令 receipt 的參數與 [command-environment.json](command-environment.json) 執行。overlay 的路徑須改成當前 checkout 與工作子目錄。重現原始缺陷使用保存的原 model.go 或 cbb22ac checkout；候選驗收使用 candidate 與凍結測試。先核對來源指紋，不要把目前已修正的正式來源當成原失敗版本。

極大公開案例只使用舊 encoder 乘積嚴格為負的三種配置，避免巨大正值配置。其他極端界線只呼叫不配置參數陣列的私有長度函式，與 BigInt 參考比較。自訂 lazy 詞表只證明公開建構契約，並未新增自訂詞表保存功能。

所有實際命令、結束碼與日誌雜湊都在對應 JSON receipt。工程 unit／race 的 .log.gz 解壓後 SHA256 與 raw_bytes 在同名 JSON。沙箱本機連線失敗、第一次接續腳本的檔名錯誤及相容性 helper 編譯錯誤亦保留，正式完整驗收以 engineering-local-* 與 engineering-verification.json 為準。
