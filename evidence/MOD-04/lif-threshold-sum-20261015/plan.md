# 多項門檻相加的可表示性稽核

前一輪已完成 ticket 111 的文件修正、驗收與推送，現況 HEAD 為 `d4c2fca1e00eed0a081b2b2313668ad7a00f6a1f`，分類為有進展。本輪仍以整體除錯為目標，不把局部通過當成零 bug。

## 診斷骨架與預期結果

1. 核對 ticket 21 的 `max(base + adaptation + homeostasis + shift, theta_min)` 與各公開路徑的合法輸入。先區分最終門檻不可表示與中間部分和溢位。
2. 使用 `B=2^1023`、`theta_min=B/2`、`theta_max=1.5B`、raw=0 得到 base=B。令 adaptation=B、shift=−B，精確有理數總和為 B。所有輸入及最終門檻有限。
3. 先從零狀態用合法輸入產生一次放電，確認 adaptation=B 是正常可達狀態，再檢查持續 AdvanceModulated、整段 AdvanceModulated 與 ForwardCoupled。診斷如實記錄錯誤，不先改正式程式。
4. 若公開重現且契約確認為缺陷，再凍結回歸、處理同類路徑、驗證正常相容性與專案必要工程檢查。若無法成立，保存排除原因，不為了有改動而改模型。
5. 必要證據保存後，只清除本輪 `runs/.tmp/lif-threshold-sum-20261015/`，保留原模型、六份必要資料與兩個既有工作目錄。

預期控制：單步暖機可成功且產生一次放電；獨立精確總和為 B；正常範圍、零偏移及真正不可表示結果須分別驗證。實作前的診斷不表示上述疑點已確認。

## 減法審查

優先沿用既有門檻公式與運算順序，不新增設定或改變正常模型。只在獨立證據確認缺陷後處理必要算術，不先重構全部核心。

## 分工與暫存

宿主 Codex。當輪 OpenCode 列模型失敗：`Unknown: FileSystem.open (/Users/timlai/.local/share/opencode/log/opencode.log)`。依專案規則，既有 gpt-5.6-luna／max worker 唯讀核對契約與同類位置，Root 負責公開重現及審查。所有新快取和工作檔位於本輪專案內目錄。

## 已確認根因與修正格子

公開診斷已確認：暖機放電為 1、產生的 adaptation=B 通過 ValidateState；精確總和為有限 B，但持續／整段 AdvanceModulated 及 ForwardCoupled 都在第二步回非有限錯誤。ENG 的限制是供放電比較使用的最終門檻，未要求每個部分和可表示。這是相加途中丟失有限項的數值缺陷，不是要改變學習公式。

正式修改範圍為 `dynamics/modulation.go`、`lif_state.go`、`mixed_state.go`、`lif_coupled.go`、`mixed_coupled.go`。正常運算順序保持。只有核心三項部分和為正無限、且合法化學偏移為負時，用原始四個有限加數重算完整總和，再沿用原有下限投影及有限值拒絕。精確重算可使用標準函式庫 `math/big.Rat`，不新增外部依賴。零偏移及無調節路徑保持，正向不可表示的最終值仍拒絕。LIF 重算共用 coupled 前向，不另寫一份算法。

測試格子只寫 `tests/modulated_threshold_sum_test.go`，不改實作。覆蓋 adaptation-only、homeostasis-only、各半三種正常可達機制狀態，非地板／落地板兩種結果，以及 LIF 持續、整段、coupled、coupled 重算及 Mixed 持續／coupled。使用精確有理數總和驗證，不能把相加順序換一遍當參考。

可微控制用 log_tau=-10，使 alpha=1；第一步 input=MaxFloat64 產生一次放電且該步替代導數為零。第二步 input 等於精確最終門檻，替代導數為 1。非地板時只給第二列輸出上游 1，預期該列 threshold shift 導數 −1、theta_raw 導數 −(theta_max−theta_min)/4；地板時這兩項為零。真正溢位、零偏移不救回及輸入非有限繼續拒絕，失敗不修改原狀態或發布部分 trace。Root 凍結測試並執行紅燈後，另一位執行者才填候選實作，不能修改測試。
