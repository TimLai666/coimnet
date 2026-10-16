# 下一個可驗證候選

慢速活動穩定的獨立梯度參考尚未完成，不是已確認的新缺陷。ticket122只處理有限活動率更新，沒有新增梯度規則。

目前手算表核對 h 的時序、夾限與回落；停用機制有逐位對照。一般／分段／耦合的相等測試仍共用正式反向算式。既有 lifForwardTangent 只包含 base 與 adaptation，沒有維護 homeostasis 的 primal r/h。

下一個最小驗收：沿用八步手算 fixture，在測試私有 forward tangent 維護 r/h 的數值，但令它們不回傳導數，與完整 Backward 的 weights、bias、log_tau、theta_raw、initial、inputs 逐座標比對。需要同時觸發 h 上限、下限與回落；另外用錯把 r/h 傳回的對照確認判準能拒絕錯誤。

先固定獨立算式與期望結果，再補正式測試。不能用硬放電事件的有限差分改寫既定停止梯度，不能據測試通過宣稱生物機制或真實任務品質。沒有確認錯誤前保持正常模型與公開介面。
