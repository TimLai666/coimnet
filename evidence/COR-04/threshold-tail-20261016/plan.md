# 閾值尾端數值修正

研究者可以在合法有限的閾值範圍及參數下，取得宣告公式的放電結果與解析斜率。本票只處理已重現的 sigmoid 尾端資訊損失，原始重現為 18 組中的 3 組前向差異、12 組閾值梯度差異。

## Root 骨架與責任

`LIF/Mixed 設定與參數 → 有界閾值及斜率 → 一般／分段／化學耦合訓練或持續模擬 → 輸出與梯度`。人工閾值範例的報告也必須使用相同的尾端規則。資料及模型格式保持。

共用模組 `internal/thresholdtail` 只提供 `Recover(raw, lo, hi float64) (base, slope float64, recovered bool)`。輸入已由呼叫端驗證有限且 lo < hi。正常區域回傳 recovered=false，由呼叫端保留完整原算式、順序與錯誤。raw >= 7 時，直接計算小的 sigmoid 補數，避免 1-s 的有效位數損失。raw <= -700 時，在 exp 進入次正規數前切換，避免先捨入再乘跨度。這是私有計算分支，不是使用者參數限制。

尾端以 e=exp(-abs(raw))、den=1+e 計算跨度乘 e。當 e 低於最小正規數時，改為 (span*exp(-abs(raw)/2))*exp(-abs(raw)/2)。跨度溢位時，正常 e 分別縮放端點再相減。e 次正規或為零時改成 (hi*h-lo*h)*h，讓最後一次乘法才捨入，避免兩個端點各自下溢後漏掉最後一個非零位元。offset=scaled/den，base 在 raw 正側使用 hi-offset，負側使用 lo+offset，slope=offset/den。真正無法表示的最終斜率保持零。所有原本的有限值檢查保持。

實作格子：`dynamics/lif.go`、`lif_segment.go`、`lif_coupled.go`、`lif_state.go`、`mixed.go` 的五個產生端及 `experiment/lif_threshold.go` 的報告端。報告端在非尾端但跨度溢位時也沿用核心已存在的端點加權式，避免報告與同一核心不一致。不改反向消費端、公開 API、相依性或儲存格式。共用 helper 獨立放檔，避免六份尾端算法分歧。

測試格子：`dynamics/threshold_tail_test.go`、`experiment/lif_threshold_tail_test.go`。測試作者與實作作者分開，Root 審查、執行修正前失敗、凍結測試後才派實作。參考值取自 Decimal 200／280 位一致的 `reference-input.json`，不能以正式 helper 或同一 sigmoid 算式當答案。正規範圍另以舊來源記錄逐位對照。

## 驗收與失敗處理（ticket 116 唯一負責）

| 情況 | 預期結果與證據 |
|---|---|
| 有限尾端與巨大跨度 | 公開 LIF、LIF 重算段長 1／2、耦合／耦合重算、Mixed 全 LIF 及真正混合節點的事件和所有梯度欄位符合獨立參考 |
| 持續模擬 | LIF／Mixed Advance 與同設定 Forward 的放電與電位一致，涵蓋三個原始前向錯誤 |
| 跨分段邊界 | 兩列 [0,1]、raw=40、[0,2]，upstream=[-.5,1]，bias=.75*psi，inputs=[.25*psi,.5*psi]，initial=.25*psi，theta=-psi*slope，tau=-.5*ln(2)*psi |
| 真正下溢 | 小跨度 raw=±1000 的最終斜率仍為零，不把所有零判成缺陷 |
| 一般值與合法舊保存 | 原來源與修正來源正常核心 JSON 完全一致，舊神經狀態與化學模型在新程序接續一致 |
| nil／空值／非法參數 | 原入口的檢查、錯誤及不發布部分狀態保持，沿用既有測試 |
| 取消／來源錯誤 | 原有錯誤傳遞及呼叫端所有權保持，不修改控制器或回滾契約 |
| 相同模型重複／並行呼叫 | helper 無可變狀態，完整一般／競態驗證 |
| 範例報告 | 尾端值使用相同規則，正常報告算式與固定研究協定保持 |

完成條件固定五項：重現與契約、凍結失敗測試、修正與相容性、完整工程驗收、保存清理及遠端交付。全域零 bug 目標沒有有限百分比，本票成功也不證明真實任務品質或生物正確性。
