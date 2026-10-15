# Root 的 driver 驗收裁決

目前只有一項已重現的範例錯誤：載入成功後、建立公開 Trainer 前，用原始 bundle 設定作嚴格比對，會因未啟用核心的空陣列表示差異而拒絕。`restore-probe.log` 的差異限於三個空陣列，Options 相同。`checkpoint/bundle_kinds.go:236` 明示空陣列正規化，`learning/core.go:422` 的 LIF 建構則清空未啟用的 Dynamics。修正將比對移至公開 RestoreTrainer 後，仍完整比對設定與選項，不放寬有效拓樸、LIF、化學或訓練設定。

新回歸涵蓋 OFF／neutral／ON 的 52 節點化學宣告、保存、公開 Trainer 恢復及下一次更新。一般／競態版本另由獨立程序讀回相同 smoke 模型，以完整 snapshot、參數、最佳化器、評分及下一次更新雜湊核對。接續命令由外部 runner 啟動，driver 內的 clone 只提供預期值，兩者不能混為同程序證據。

審查者其餘觀察依協定裁決如下：

- Active 旗標是觀察值，協定明示沒有初始輸出差異不自動判錯，因此不加入從結果倒推的通過門檻。現有一般／競態各十筆 probe 的輸出及梯度差異均為 true，仍不代表學習有幫助。
- 新版 SDK probe 對三組都比較 Network／Trainer 輸出，OFF／neutral 另比較完整梯度與更新狀態。化學節點回歸已補 LIF 分支。
- `external_loss_contract` 只宣稱公開 StepFrom 的 Applied／LossKnown／Loss 契約，不能解讀成另寫一套 Adam 的獨立證明。框架梯度及最佳化器由正式完整測試驗證，本票沒有改演算法。
- 12 組正式訓練及跨程序接續目前尚未完成。runner 會核對退出碼、完整輪數、資料指紋、每個評分的預測／混淆矩陣及完整狀態雜湊，再發布個別 accepted 回條。沒有這些回條就不算完成。

這些結果沒有確認新的框架數值缺陷。原 812 份來源與 18 個模型原檔的指紋保持。正式訓練設定、資料、種子、容量與更新預算沒有因 smoke 結果改動。
