# 146 — 分類訓練誤差保留共同偏移下的機率

Epic：訓練與評分的數值正確性

User Story：模型對每個類別給出相同機率時，訓練與評分應回傳相同誤差，不受分數的共同偏移影響。

Blocked by：無。完整工程、最後治理與暫存清理已完成，等待提交推送及遠端核對。

Status：in_progress

相關需求：TSK-03 文字生成評分、TSK-07 外部工具模式的核心請求頭。

## Root 決策（2026-10-18）：根因與修正骨架

`tasks/media/external_tool.go` 的 requestLossGradient 與 `tasks/textgen/model.go` 的 lossAndUpstream 都先將最大分數加回 log-normalizer，再減去目標分數。大數加法先捨去機率資訊，後續相減不能復原。

媒體三分類的同分輸入應得到 ln(3)，原版在共同偏移 ±2^54 時得到 0 或 2。文字生成 259 類應得到 ln(259)，原版得到 4 或 6。兩處均以合法有限的受控 trainer 狀態，經公開 Step 重現誤差 0；文字生成公開 NLL 也重現，且 NLL 未改變完整狀態。受控狀態由測試建立，不宣稱預設訓練會自然達到此狀態。

修正限於這兩處誤差算式。媒體逐組計算半個 shifted target difference 與半個 log-normalizer，再累加，避免個別差值或未除平均的中間總和溢位。最後平均確實不可表示時回傳零、nil、error。文字生成保留 summed NLL／count 契約、既有有限性拒絕及遮罩，只把相減移到最大分數加回之前。兩處梯度公式、模型格式及公開簽名保持。

Root 已先凍結回歸並執行原版失敗，再建立隔離候選。媒體原版一般／競態各 13 fail、2 pass，候選各 15 pass。文字生成原版各 8 fail、1 pass，候選各 9 pass。這些數字是包含父測試的終端事件，不是完整框架驗收。

正常回傳誤差的最後位元可能因運算順序而改變，不宣稱所有正常輸入逐位相同。媒體的指定 12 次更新對照在每個建置內，完整訓練快照、StepResult、請求與回傳誤差均保持。真正不可表示的媒體平均會由原版繼續更新改為先回錯；此改變不得混入「所有更新保持」宣稱。

## 固定完成條件

- [x] 兩處均以獨立機率參考、受控公開呼叫及凍結失敗回歸證明根因。
- [x] 隔離候選一般／競態、相關既有測試與正常訓練對照通過並由 Root 核對。
- [x] 正式採用兩處最小修正與凍結回歸，專項驗證通過。
- [x] 固定最終來源通過完整工程、相關保存接續與最後治理。
- [ ] 保存必要證據、清除自有暫存、提交推送並核對遠端。

## 減法審查

改正現有算式，不增加公用數值抽象、依賴、模型版本或取消誤差門檻。可變詞彙表的不等長 panic 另列契約待查，不在本次修改公開詞彙 API。

## 證據

[Root 骨架與輸入指紋](../../evidence/TSK-04/request-head-audit-20261018/root-loss-plan.json)、[媒體專項](../../evidence/TSK-04/request-head-audit-20261018/loss-candidate-focused-verification.json)、[文字生成公開重現](../../evidence/TSK-04/request-head-audit-20261018/textgen-root-review.json)、[文字生成專項](../../evidence/TSK-04/request-head-audit-20261018/textgen-candidate-focused-verification.json)、[媒體完整一般](../../evidence/TSK-04/request-head-audit-20261018/media-package-unit-root-review.json)。正式 866 份來源不變，完整競態與後續驗收尚未完成。

## Root 決策（2026-10-18）：強化回歸與舊模型接續通過

v1 凍結來源及日誌保持。v2 明確拒絕 NaN／Inf 比較，另加入每處三種非有限輸入及父測試，原版一般／競態合計各 21 fail、11 pass，候選各 32 pass，舊事件結果保持，見[Root 核對](../../evidence/TSK-04/request-head-audit-20261018/v2-focused-root-verification.json)。

兩種建置各由原版保存兩個正常小型模型，原版與候選各用新程序讀取同一份檔案，完整快照、最佳化器、logits 與下一次更新的 JSON 位元組保持，指定 NLL 與 mean 也保持，見[接續對照](../../evidence/TSK-04/request-head-audit-20261018/textgen-resume-root-verification.json)。此證據只涵蓋四個人工存檔，不外推所有模型。

必要模型與輸出已封存，23 份共 7,817,137 bytes 控制暫存已清除，見[清理回條](../../evidence/TSK-04/request-head-audit-20261018/controls-cleanup.json)。目前 2／5，正式採用與完整工程待完成。

## 追加驗證：極小尾項尚未被候選修正

原版與先前 shifted 候選在兩組 `[0,-40,-40]` 的媒體輸入都回傳 0，解析平均為 `Log1p(2*Exp(-40))`。文字生成 259 類只移動 winner 的位置，誤差便從 0 變成不同的正值，解析值應相同。四份一般測試日誌各有 4 個失敗終端事件，兩版同位置的梯度 bits 相同，見[Root 核對](../../evidence/TSK-03/categorical-loss-adoption-20261018/tail-red-root-verification.json)。

根因是先累加 `1 + tiny` 已捨去尾項，後面的 Log 無法復原。既有 2／5 只記錄前述隔離驗證；新增尾項驗收尚未通過，不得直接正式採用舊候選。接續需分開計算尾項與梯度分母，檢查極小但可表示的平均，再重跑候選與保存接續。正式 866 份 Go／模組來源保持，這次新增證據沒有公開 trainer 動態重現、競態或完整工程驗收。

## Root 決策（2026-10-18）：尾項、平均與公開呼叫正式採用

追加回歸確認逐項 exponential、逐組平均、跨位置加總及上限半差值的中間捨入缺陷。多項尾部另有 99／113／288 ULP 的舊候選誤差。原失敗與候選版本保持，最終使用補償加總、直接相乘的尾項正規化、延後轉回的極小值累加，以及僅極端上限採用的精確平均，見[修正骨架](../../evidence/TSK-03/categorical-loss-adoption-20261018/root-tail-sum-plan.json)及[上限骨架](../../evidence/TSK-03/categorical-loss-adoption-20261018/root-mean-boundary-plan.json)。

兩處修正及十份凍結回歸已正式採用。876 份最終來源固定，一般／競態專項各 65 個 pass 終端事件，14 組梯度及三個受控公開觀察保持狀態與更新，見[候選核對](../../evidence/TSK-03/categorical-loss-adoption-20261018/stable-focused-root-verification.json)及[正式核對](../../evidence/TSK-03/categorical-loss-adoption-20261018/formal-focused-root-verification.json)。公開 Step／NLL 已動態重現並修正極小誤差，不推論預設訓練會自然達到該狀態。

四份既有原版人工存檔在新程序接續時，完整快照、最佳化器、logits 與下一次更新位元組保持；媒體十二次正常更新也保持，指定回報誤差最多一個 ULP 差異，見[最終候選控制](../../evidence/TSK-03/categorical-loss-adoption-20261018/stable-controls-root-verification.json)。舊候選完整套件競態因已被反例推翻而主動停止，其部分結果不列驗收。完整工程、最後治理、清理與交付仍待完成，目前 3／5，沒有全域零 bug 或模型品質完成宣稱。

## Root 決策（2026-10-18）：長序列累加補償

獨立審查發現 tiny 累加仍會重複捨去小項，Root 在預設 259 類、2049 個計分位置動態重現 821 ULP 誤差，並在一般加總重現同根因的 79 ULP 誤差。兩個子案例及父測試在正式 876 份來源全部失敗，見[失敗日誌](../../evidence/TSK-03/categorical-loss-adoption-20261018/aggregate-prior-unit.log)。

文字生成的一般及 tiny 跨位置累加均改用補償加總，保留有限性拒絕、遮罩、count 及梯度。十一份凍結回歸、877 份來源正式採用後，一般／競態專項各 68 pass，原 65 個結果及 18 個梯度／公開觀察指紋保持，見[追加驗收](../../evidence/TSK-03/categorical-loss-adoption-20261018/aggregate-focused-root-verification.json)及[正式第二版驗收](../../evidence/TSK-03/categorical-loss-adoption-20261018/formal-v2-focused-root-verification.json)。四份原版模型及十二次媒體更新的狀態與更新仍保持，文字回報誤差最多兩個 ULP、媒體一個 ULP，見[最新控制](../../evidence/TSK-03/categorical-loss-adoption-20261018/aggregate-controls-root-verification.json)。

876 份來源的一般工程測試雖已通過，完整驗收因上述缺陷停止，原 manifest 及所有日誌保持。877 份來源使用版本化 manifest 重新執行全套，避免舊收據指向新來源。目前仍為 3／5。

## Root 驗收（2026-10-18）：877 份來源的完整工程

九項完整工程均通過，一般 8,777 pass／23 skip、競態 8,776 pass／24 skip，各 58 套件通過。原 8,732 個終端結果保持，只新增凍結的 68 pass，Root 已獨立逐筆比對日誌與目前來源，見[完整核對](../../evidence/TSK-03/categorical-loss-adoption-20261018/full-engineering-root-review.json)。九份正常報告、21 項已核准增益控制、四份舊人工模型接續與媒體十二次更新保持。最後 31 項治理通過，2,041,297,098 bytes 本票暫存已清除，見[清理](../../evidence/TSK-03/categorical-loss-adoption-20261018/cleanup.json)。本票 4／5，提交推送及遠端核對待完成。

另有 [ticket147](147-categorical-gradient-stability.md) 的既有分類梯度缺陷，已透過公開更新對照確認，尚未修正。它與本票的回報誤差修正分開追蹤，不以本票工程通過宣稱框架沒有 bug。
