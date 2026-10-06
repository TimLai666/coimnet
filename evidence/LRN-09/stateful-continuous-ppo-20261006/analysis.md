# 保存神經記憶後接續 PPO 訓練

這次補上訓練框架的一項缺口：修正前的 `learning/rl.Update` 只接受新個體的零初始神經狀態。對已運行的模型，單純移除保護會讓算分使用保存記憶、梯度卻從零開始，兩者不一致。修正前來源為 `16718541d6adce7cbc15b74308ccfe37c25c461f`，公開入口失敗重現見 `learning-rl-red.log`、`checkpoint-red.log` 與 `ppo-delays-red.log`。

## 現在可以做什麼

CPU 純量連續核心可從合法的保存狀態接續目前段的訓練，包含延遲歷史。`Continuous.ForwardFromState` 使用保存的原始歷史值，`Network.LossGradientFromState` 接上現有 Insyra 編碼器與讀出，`Trainer.StepFromState` 共用原最佳化器流程。PPO 每輪重新計分和更新都使用同一份 rollout 初始狀態，回傳個體保留呼叫開始時的現行神經記憶。

保存狀態視為固定起點。參數與輸入梯度涵蓋目前這段，段內依原 Truncation 設定處理，不回推先前經驗。Initial 只報告起點電位的局部敏感度，最佳化器不使用它。沒有另建訓練器、設定欄位或保存格式。

## 可以核對的證據

| 行為 | 依據 |
| --- | --- |
| 延遲跨段起點、合法 4 ULP 歷史、最大延遲與步數 | `dynamics/state_forward_test.go`、`dynamics/state_forward_limits_test.go`，與 Advance、獨立固定歷史參考及有限差分比較 |
| 完整編碼器／核心／讀出梯度與現有最佳化器 | `learning/stateful_test.go`、`learning/stateful_options_test.go`，完整有限差分、第一輪 AdamW 公式、固定符號與共享 mask |
| 同一初始狀態的多輪 PPO 更新 | `learning/rl/stateful_test.go`、`stateful_delayed_test.go`，與公開 Loss 加 StepFromState 的獨立更新路徑比較 |
| 新程序恢復 | `learning/rl/stateful_checkpoint_test.go`，保存既有 AdamW 與未完成累積梯度，由新程序載入，完整 checkpoint bytes 相同 |
| 原從零開始的使用流程 | 相關套件完整測試，以及 CLI delayed 的 80 步加恢復 40 步與連跑 120 步逐位相同 |

最終版 `feature-final.log` 的 22 個頂層專項與 `related-final.log` 的 372 個相關頂層測試通過。最終完整一般與 race 各 57 個有測試套件通過。各命令、環境及來源指紋保存在同目錄；最終完整驗收以 `verification.json` 為準。

原測試 helper 曾錯誤複製空的歷史列。Root 先以獨立控制重現，再只修正複製來源，沒有放寬原斷言。原與最終凍結都保存，詳見 `root-test-review.md`。

## 結論適用範圍

本輪是 Mac arm64 的人工數值、整合及保存恢復證據。非零 LIF／mixed、非零 Recompute 與可塑性／化學 PPO 梯度不支援；vector／GPU 個體持續狀態原本就未提供。既有可用的 fresh-zero 路徑保持。沒有新增全圖效能、其他平台、導航學習或生物機制證據，也沒有證明這個限制是先前方向學習失敗的原因。

減法審查：保留單一編碼器、讀出及最佳化器流程，只在核心前向選擇保存狀態；不用平行維護另一套 PPO 訓練器。

獨立審查另外抓到候選更新驗證仍從零開始的缺口。Root 用公開入口重現立即更新與未完成累積兩種錯誤提交，修正後候選必須從同一保存狀態可執行才提交。前後差異見 candidate-red.log、feature-final.log、candidate-probe-final.log；審查收尾確認修正有效。修正前完整 race 已取消並保留退出紀錄，沒有以取消結果宣稱通過。

CLI 證據腳本的結果參數曾被迴圈變數覆蓋，所有 CLI 命令及比較成功後，寫入回條失敗。僅修正證據腳本變數，CLI 及比較保持；cli-final-green.log 與 cli-validation-v2.json 已重跑通過。
