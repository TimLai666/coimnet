# Ticket 43 公開入口測試審查

Root 親自確認公開契約、測試內容與失敗日誌。測試及實作由不同 agent 負責，實作 agent 不可修改測試；來源凍結於 `tests-frozen.json`，原始測試副本同目錄保存。

| 驗收 | 獨立依據 | Root 的修改前重現 |
| --- | --- | --- |
| 保存狀態的連續核心 | 與 `Advance` 逐位比較，tanh／softplus、單／多 worker、延遲跨段起點及不足歷史 | `dynamics-red.log`、`dynamics-limits-final-red.log` |
| 固定歷史的反向 | 參數／輸入有限差分，Initial 使用固定歷史的獨立參考，段內截斷對照 | `dynamics-red.log` |
| 完整梯度橋接 | `RestoreIndividual` 加 `Advance` 的有限差分，沿用 Insyra float32 邊界的 ENG 誤差規則 | `learning-rl-red.log` |
| 最佳化器沿用 | 第一輪 AdamW 的獨立公式與 moments，固定符號、InputNodes、共享參數及整組 mask | `learning-rl-red.log` |
| 帶記憶的 PPO | 每個 epoch 以保存的 InitialNeural 重新計分，與公開 Loss 加 StepFromState 手動路徑比較 | `learning-rl-red.log`、`ppo-delays-red.log` |
| 保存與新程序 | 保存既有 Adam 更新與未完成累積視窗，由另一個程序讀回實際檔案，完整 checkpoint bytes 比較 | `checkpoint-red.log`、`learning-rl-red.log` |

非法值、取消、容量、步數溢位與未支援模式檢查錯誤結果及原資料保持。原有 fresh-zero 與 Recompute 控制在舊版本已通過，不將這些控制算為新能力的失敗重現。第二份非法 rollout 必須在最佳化器開始更新前被拒絕。

梯度數值對照固定 `eps=2e-3`、`atol=1e-5 + 1e-4*abs(fd)`，來自既有 ENG 的 float32 橋接驗收。fresh-zero 的參數及輸入梯度與舊入口逐位比較；Initial 的差異是保存歷史固定與初始 activation 可微的明確契約差異，最佳化器不使用 Initial。

未以此測試推論導航學習、完整真實圖效能、生物機制或跨 CPU 逐位一致。

## 測試資料複製缺陷與最終凍結

Root 另外加入 `TestStatefulRolloutClonePreservesHistoryAndIndependence`，以實際 Advance 產生的合法歷史獨立檢查測試用複製函式。`test-fixture-red.log` 證實原 helper 把新建的 nil 歷史列當來源，複製後的比較基準本身錯誤。Root 僅把來源改為 `s.Continuous.History[i]`，保留所有原測試斷言；實作 agent 未修改測試。

原始八檔指紋與副本保留於 `tests-frozen.json`。`tests-frozen-v2.json` 凍結修正後的九檔；`source-integrity.py` 驗證與原始副本恰好只差這一行，其他原測試保持，新增的 helper 控制也檢查修改複本不影響原資料。

`feature-green.log` 的 21 個頂層專項通過，包含原失敗案例、延遲歷史多 epoch、新程序載入 AdamW 與未完成累積梯度的逐位對照。`related-tests.log` 的 dynamics、learning、learning/rl 完整套件通過。這些日誌由 Root 執行，並非以 agent 的敘述取代驗證。

## 獨立審查發現的候選驗證缺口

審查發現 StepFromState 在提交流程前仍用從零開始的 Predict 檢查新參數。Root 加入 `TestStepFromStateRejectsCandidateThatOnlyRunsFromZero`，一節點 softplus、合法 1e36 電位／歷史與有限 1e10 學習率會在保存狀態重播時超出 float32，但原候選從零驗證錯誤成功。`candidate-red.log` 的立即更新與未完成累積兩種情境皆重現 Applied=true。

新測試要求候選被拒絕、StepResult 清空，參數、AdamW、更新次數及原累積視窗完全保持，另保留 fresh-zero 候選可成功的控制。Root 審過測試後凍結於 `tests-frozen-v3.json`，共十檔。實作 agent 只可改 trainer.go 與 stateful.go，不可改測試；最終成功驗收另存，不覆寫先前記錄。

`feature-final.log` 通過最終版 22 個頂層專項，含新候選拒絕與新程序恢復；`related-final.log` 通過 372 個相關頂層測試。最終來源 646 個 Go 檔凍結於 source-manifest-v2.json，十份測試凍結於 tests-frozen-v3.json。修正前完整 race 已取消並記錄，不計通過。
