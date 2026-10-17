# 146 — 分類訓練誤差保留共同偏移下的機率

Epic：訓練與評分的數值正確性

User Story：模型對每個類別給出相同機率時，訓練與評分應回傳相同誤差，不受分數的共同偏移影響。

Blocked by：正式採用須等 ticket145 的固定來源工程驗證與交付結束。隔離驗證可獨立進行。

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
- [ ] 正式採用兩處最小修正與凍結回歸，專項驗證通過。
- [ ] 固定最終來源通過完整工程、相關保存接續與最後治理。
- [ ] 保存必要證據、清除自有暫存、提交推送並核對遠端。

## 減法審查

改正現有算式，不增加公用數值抽象、依賴、模型版本或取消誤差門檻。可變詞彙表的不等長 panic 另列契約待查，不在本次修改公開詞彙 API。

## 證據

[Root 骨架與輸入指紋](../../evidence/TSK-04/request-head-audit-20261018/root-loss-plan.json)、[媒體專項](../../evidence/TSK-04/request-head-audit-20261018/loss-candidate-focused-verification.json)、[文字生成公開重現](../../evidence/TSK-04/request-head-audit-20261018/textgen-root-review.json)、[文字生成專項](../../evidence/TSK-04/request-head-audit-20261018/textgen-candidate-focused-verification.json)、[媒體完整一般](../../evidence/TSK-04/request-head-audit-20261018/media-package-unit-root-review.json)。正式 866 份來源不變，完整競態與後續驗收尚未完成。

## Root 決策（2026-10-18）：強化回歸與舊模型接續通過

v1 凍結來源及日誌保持。v2 明確拒絕 NaN／Inf 比較，另加入每處三種非有限輸入及父測試，原版一般／競態合計各 21 fail、11 pass，候選各 32 pass，舊事件結果保持，見[Root 核對](../../evidence/TSK-04/request-head-audit-20261018/v2-focused-root-verification.json)。

兩種建置各由原版保存兩個正常小型模型，原版與候選各用新程序讀取同一份檔案，完整快照、最佳化器、logits 與下一次更新的 JSON 位元組保持，指定 NLL 與 mean 也保持，見[接續對照](../../evidence/TSK-04/request-head-audit-20261018/textgen-resume-root-verification.json)。此證據只涵蓋四個人工存檔，不外推所有模型。

必要模型與輸出已封存，23 份共 7,817,137 bytes 控制暫存已清除，見[清理回條](../../evidence/TSK-04/request-head-audit-20261018/controls-cleanup.json)。目前 2／5，正式採用與完整工程待完成。
