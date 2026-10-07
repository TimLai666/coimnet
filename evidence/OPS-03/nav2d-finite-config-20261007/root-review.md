# 導航環境有限值驗證

Problem：環境設定的單向或範圍比較會接受 NaN，部分欄位也接受正無限大。非法設定可能進入訓練，或直到 JSON 匯出才失敗。

Done：沿用既有共用驗證，在四個浮點欄位各自的範圍檢查前拒絕 NaN 與正負無限大。公開簽名、有限值限制、零值預設及原有錯誤次序保持。

## Diff Inspector

Scope: CLEAN。Root 完整讀過 env.go 的修改與兩份新增回歸。既有 production 修改只有一個檔案，測試 worker 只改指定的報告回歸檔。檔案指紋、consumer 清單與限制見 [scope-review.json](scope-review.json)。

No confirmed findings in the submitted fix.

四個數值欄位採相同檢查。New、兩個 Validate 與三個 Run 入口沿既有路徑取得共用驗證；suite 回錯時保留原有 SchemaVersion，沒有 task 結果。context 的 nil／取消檢查仍在設定驗證前。Go doc 已明列有限值限制，README 與共用決策一致。

## Tests

新增四個頂層回歸。修正前，兩個非法輸入回歸各有七組失敗，兩個合法控制通過，見 [red-all.json](red-all.json)及 [red-all.log](red-all.log)。Root 審查後先凍結測試，再修改實作，沒有為通過而修改凍結測試。

修正後，四個新增回歸及全部 11 個選取的頂層測試通過，見 [green-scoped.log](green-scoped.log)。每份否定回歸各涵蓋四欄乘上 NaN、正 Inf、負 Inf，共 12 組。控制涵蓋正負零的預設、極小正數、有限極值、有限負值、密度範圍、Resolve 重複解析與先前錯誤的優先次序。

兩個新程序的八份合法報告、24 筆 run 都與修正前完整相同，見 [valid-comparison.json](valid-comparison.json)。八個真實 CLI 流程通過，涵蓋單任務、四任務 suite、歸因、兩個 help、兩個非法任務與既有輸出保護，見 [cli-workflow.json](cli-workflow.json)。

完整一般測試 57 個套件通過，沒有快取結果。build、vet、離線 tidy -diff、模組校驗、gofmt 與治理／格式索引通過。完整 race 57 個套件通過，沒有快取結果。命令、環境與輸入指紋由本目錄的紀錄保存。

## 後續缺口

- [P2] [CONFIRMED] experiment/attribution.go:99：Comparison.Interval=NaN 會通過 Validate，encoding/json 卻拒絕該設定。重現見 [related-comparison.log](related-comparison.log)，建議另外補有限值驗證。
- [P2] [CONFIRMED] experiment/nav2d_suite.go:371：報告與呼叫端共用 Policies，之後修改切片會讓設定與既有雜湊不符。既有公開入口證據見 [ownership-probe.json](../nav2d-effective-config-20261007/ownership-probe.json)，建議另票處理雙向切片所有權。

這兩項已記入 AGENTS.md，均不阻擋本票的四個環境欄位驗收。

## 減法審查與限制

建議簡化：所有入口共用既有驗證，只有四組檢查，不增加驗證器或依賴。有限極值可能在後續算術溢位，本票不新增數值上限。這是本機軟體驗證，沒有新的學習成效或生物機制證據。

Recommendation: SHIP。軟體驗證通過，提交推送與遠端核對待完成。
