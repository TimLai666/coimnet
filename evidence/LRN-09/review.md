# LRN-09 整合審查

Scope: CLEAN。範圍是 ticket 26 的人工走廊收集器、PPO 執行器、CLI 與證據。沒有更改既有模仿拓撲、核心梯度、依賴或交接原件。

Adversarial review: RUN。由 Luna max 獨立審查循環狀態、取樣、時間上限、失敗記錄、亂數來源、評估隔離與 CLI。主 agent 逐檔檢查並自行執行兩平台驗證。Spark 本輪呼叫回 `Unknown model gpt-5.3-codex-spark`，沒有使用成功；CLI 由 OpenCode `opencode/big-pickle` 實作。

## 已修正

- P2 CONFIRMED：原始報告直接平方離均差，有限的 `GoalReward=1e160` 會讓 `StdReturn=+Inf`，`RunPPO` 沒有回錯，但 JSON 無法序列化。修正為按最大絕對值縮放後計算平均與母體標準差，並要求 `StdReturn` 有限才可能通過。`aggregate-red.log` 保存失敗，`aggregate-green.log` 保存極端正負值、同號最大值與公開 RunPPO-to-JSON 回歸。獨立複審使用 `1e160` 至 `1e300` 的有限獎勵均能輸出 JSON，此 finding 已關閉。
- 初版訓練取樣與讀出初始化同用 `PCG(seed, 2)`。收集訓練、評估與隨機基線改為三個獨立來源，測試檢查不與初始化來源重疊。
- CLI 說明誤稱一維走廊專家為 BFS，已依實際實作修正。既有 OCR 命令的說明縮排也已還原。

## 保留的範圍限制

- `gridnav.New` 原有的非有限獎勵設定缺口列於 `AGENTS.md` Follow-ups。新 PPO 公開入口與 collector 都拒絕非有限設定；底層環境的修正不混入本票。
- PPO 只處理零初始狀態的完整 episode，不支援可塑性／化學機制或切斷 BurnIn 前綴梯度。
- 這是人工走廊，兩種方法的模型結構不同，不作演算法優劣、全腦學習、GPU 或生物能力宣稱。

最終變更未留下其他已確認的審查缺陷。各平台的實際測試、來源指紋與學習結果以 [verification.json](verification.json) 為準。

## 先紅後綠紀錄的界線

`runner-red.log` 同時缺 runner 與 collector 符號，只能當成整合前缺少實作的證據。collector 最初命令未另存日誌，不能宣稱已有乾淨、獨立的先紅紀錄。主 agent 在實作後以只編譯 `ppo_collect_test.go` 的方式補做隔離基準，見 `collector-isolated-baseline.log`；再加入 collector 實作檔得到 `collector-isolated-green.log`。這是事後的缺少實作對照，不冒充先前的執行時間順序。

`cli-red.log` 是 CLI 實作前的實際失敗日誌。`aggregate-red.log` 是統計修正前的實際回歸失敗日誌。最終完整測試另由主 agent 重跑。
