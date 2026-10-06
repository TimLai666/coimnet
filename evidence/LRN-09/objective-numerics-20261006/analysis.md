# PPO 公開機率與損失數值修正

2026-10-06，ticket 41，LRN-09。來源基準為 `9202bbf348accada3a0bfe89638e176993257140`，環境為 macOS 27.0.1 arm64、Go 1.26.5、Insyra v0.3.4。

相同的兩個分數在 0、±1e16、±1e308 下，動作對數機率都是 -ln(2)，兩個動作各有 0.5 機率。舊機率以手算 -ln(2) 給定時，PPO 比值是 1，正負優勢的損失、熵與梯度都符合手算。公開函式共用位移後的 log-softmax，移除本次變更後不再使用的 logSumExp。

NaN、+Inf、全部動作被排除、非法純量與不可表示的結果會回錯誤。Loss 的錯誤結果為零分項、nil 動作梯度與零價值梯度。保留 -Inf 排除部分動作的用法，以及零機率的熵處理。關閉價值項時略過差值計算，啟用時先乘係數，讓有限的加權結果不因中間平方或倍增係數而被誤拒絕。函式不修改輸入。

## Test Report

新增頂層回歸測試 13 項，全部通過。原有 RL 頂層測試 20 項與 collector 頂層測試 5 項通過。完整工程命令、退出碼與日誌記於 [verification.json](verification.json)。

- 原始實作先執行新增 12 項，9 項失敗、3 項通過，見 [red.log](red.log)。通過的部分驗證既有動作排除與輸入錯誤行為。
- 第一版修正額外誤拒絕三種可表示的加權價值結果。Root 先補獨立回歸，三個子案例失敗，見 [review-final-red.log](review-final-red.log)。正式修正版讓它們通過，見 [green.log](green.log)。
- 手算、平移不變性、動作排除、有限差分、裁切、非法輸入、溢位與輸入位元保留都走公開函式。測試以獨立期望值核對，沒有把待測 LogProb 當成正確答案。

## Diff Inspector

Scope: CLEAN。範圍為 PPO 數值函式、兩份測試、公開文件、共用技術決策與追蹤證據。Root 親自讀完整 diff，追查 Update 與 experiment collector 的呼叫及錯誤傳遞，並自行執行工程命令。

本次是無共享可變狀態的純數值運算。沒有修改公開簽名、儲存格式、驗證機制、依賴、模型、訓練預算或非零狀態限制。所有歷史證據及交接原件依 baseline.json 的指紋核對，原盤點文件保留修正前來源與結果。

審查第一版發現的加權價值拒絕案例已修正。[獨立審查](review.md)沒有發現範圍內尚未修正的問題。value-target 溢位的拒絕，以及指數下溢成零後省略熵貢獻的既有 float64 限制已補入 README，公開入口與高精度重現另存。

## 已確認但另行處理的問題

[P2] distill/distribution.go:216-230,246 與 tasks/ocr/ctc/ctc.go:111-114 有同樣的相減精度流失，已記於 AGENTS.md。公開入口的重現程式為 [related-probe.go.txt](related-probe.go.txt)，結果為 [related-probe.log](related-probe.log)。

相同均勻師生分布、分數 [1e16,1e16] 的蒸餾損失回 -ln(2)，手算應為 0。單影格、標籤 1 的文字辨識損失回 0，手算應為 ln(2)，梯度也錯。這兩處沒有在本 ticket 修改，後續應各自補公開入口手算回歸再修正。

## 證據限制與減法審查

這是 macOS 本機的軟體數值驗證。沒有新增模型學習成績、全腦訓練、其他平台執行或生物機制證據，也沒有確認此缺陷是先前方向學習失敗的根因。需求狀態維持 89／91，只補 LRN-09 的數值驗證路徑。

合併現有機率計算可修正根因，並省掉 Loss 的重複正規化。第一版數學上不會溢位的中間檢查已簡化，保留輸入、有限差值、公開分項與梯度的必要檢查。新的訓練設定或人工模型調參不屬此修正。

## 分工工具

宿主：Codex desktop。
工具與模型：spawn_agent / gpt-5.3-codex-spark / xhigh，實際拒絕：``Unknown model `gpt-5.3-codex-spark` for spawn_agent.``
工具與模型：spawn_agent / gpt-5.6-luna / max，完成獨立測試與第一版實作。Root 審查後中斷尚未完成的實作分工，保留程式並親自修正運算順序與簡化檢查。
工具與模型：followup_task / gpt-5.6-luna / max，執行唯讀挑錯審查。

本輪使用者 AGENTS.md 的 Spark xhigh → Luna max 規則優先於 skill 的一般模型路由。Luna 未列在 Spark 失敗回條的模型清單，實際 spawn 呼叫獲接受；沒有假稱 Spark 已執行。
