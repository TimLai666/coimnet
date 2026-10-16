# 下一個可驗證候選

候選：化學與慢速活動穩定並用時，公開個體持續狀態的最終 rate/homeostasis，是否與獨立前向值一致。

ticket124 驗證的是每次重新開始的 Network／Trainer episode，公開介面只回傳輸出與梯度。手算 r/h 直接讀取私有 reference，無法單獨抓到「最後一步狀態寫錯，但尚未影響當次輸出」的錯誤。因此這是驗證覆蓋候選，沒有已確認缺陷。

下一輪先盤點既有 AdvanceGated、分批接續與保存恢復檢查。只針對確實缺少的獨立對照建立最小案例，同時核對 raw neural activity 與化學讀出是否被正確區分。沿用目前停止梯度、模型格式與既有來源授權，不因驗證方便改變生物規則。

## 已核對的現有覆蓋

| 檔案 | 現有檢查與差異 |
| --- | --- |
| [learning/lif_test.go:412](../../../learning/lif_test.go:412) | Homeostasis 開關、分批 Advance 與 RestoreIndividual；沒有化學，最終 h 只要求存在且至少一顆上升。 |
| [learning/chemistry_test.go:785](../../../learning/chemistry_test.go:785) | 化學與局部塑性共用 AdvanceGated；其 LIF fixture 沒有 Homeostasis。 |
| [learning/chemical_stateful_test.go:265](../../../learning/chemical_stateful_test.go:265) | 保存的化學邊界恢復與輸出重現；使用 continuous 核心。 |
| [dynamics/lif_state_test.go:629](../../../dynamics/lif_state_test.go:629) | 直接核對最終 Rate/Homeostasis，但對照為正式 Forward 與 Advance，沒有活動化學。 |

Root 已讀上述來源並確認範圍。唯讀 agent 另盤點 chemical_recompute 與 Individual 化學訓練入口，沒有確認正式缺陷；不將 production 路徑互相比較等同於獨立手算。
