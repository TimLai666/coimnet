# 分類訓練誤差的共同偏移稽核

本目錄保存 ticket146 的隔離重現與候選，正式程式尚未採用。`verification.json` 為目前階段索引，不是完整工程通過紀錄。

媒體請求頭與文字生成的分類機率相同時，原版會因大分數的相加相減而回傳錯誤誤差。Root 以相同機率的解析參考、原版失敗回歸，以及合法有限的受控 trainer 經公開 Step／NLL 證明。受控狀態由測試建立，不據此推論預設訓練會遇到相同數值。

## 證據入口

- `root-loss-plan.json`：根因、修正位置、先失敗的測試與候選限制。
- `loss-tests-frozen.json`、`textgen-tests-frozen.json`：兩份回歸與原來源的指紋。
- `loss-candidate-focused-verification.json`：媒體原版各 13 fail／2 pass、候選各 15 pass，以及每個建置內 12 次正常完整更新對照。
- `textgen-root-review.json`、`textgen-candidate-focused-verification.json`：文字生成公開重現、原版各 8 fail／1 pass、候選各 9 pass。
- `media-package-unit-root-review.json`、`textgen-package-unit-root-review.json`：兩個套件完整一般結果保留既有事件，只新增 15／9 個 pass。
- `independent-review.json`：唯讀子代理的媒體候選審查與 Root 採用範圍。文字生成候選沒有計入這份同行審查。

## 重現與來源限制

`*.go.txt` 保存當時實際使用的探測、回歸、控制或候選來源。`*-overlay.json` 的 Replace 指向專案內 `runs/.tmp/request-head-audit-20261018/`；重跑前須從對應 `.txt` 恢復相同內容，見 `work-preservation.json`。它們不覆蓋正式 Go 檔案。

本目錄中複製的原始命令收據保持原位元組，其 `environment_file`、`source_manifest` 路徑以原執行目錄 `evidence/TSK-02/mel-capacity-adoption-20261018/` 為基準。`verification.json` 另以完整專案相對路徑列出環境及 866 份正式來源的指紋。每份收據的 command 與 log SHA 對應實際命令和日誌，不把 `source_count=866` 解讀成隔離候選已通過完整框架。

正常回傳誤差的最後位元可能因修正而改變。梯度公式沒有改動；指定正常媒體控制的狀態與誤差保持，不能外推任意模型。真正不可表示的媒體平均改為先回錯，與舊版帶著 Inf 繼續更新的行為不同。

## 尚待完成

媒體完整一般／競態各 121 pass、4 skip，原有 110 個結果保持，只新增 15 pass，見 `media-package-race-root-review.json`。七份隔離暫存共 38,443 bytes 已清除，見 `isolated-work-cleanup.json`。

v1 凍結來源及日誌保留。v2 加入明確有限值斷言與非有限輸入控制，原版一般／競態各 21 fail、11 pass，候選各 32 pass，見 `v2-focused-root-verification.json`。文字生成四個人工舊存檔已完成原版保存、原版／候選各自新程序載入及下一次更新對照，所有保留狀態的原始 JSON 位元組相同，見 `textgen-resume-root-verification.json`。

四個人工舊模型保存在 `original-textgen-checkpoints.tar.gz`，六份完整控制輸出分別以 gzip 保留。23 份共 7,817,137 bytes 控制暫存已清除，見 `controls-cleanup.json`。正式採用及完整工程仍待完成，本票 2／5。

母快取的原完整程序 97205 已實際結束，ticket145 最後治理與快取清理已完成，尚待交付。清理只依各自清冊處理，不移除原有模型、資料或其他工作。

減法審查：修正兩處現有算式，不增加數值框架、取消誤差門檻、模型版本或詞彙表 API 改造。
