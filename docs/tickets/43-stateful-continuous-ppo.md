# 43 — 使用者可以接續連續核心的既有記憶進行 PPO 訓練

**Epic:** LRN-01／LRN-09 訓練框架
**User Story:** 使用者可以保存神經狀態，從同一個狀態重播與更新目前這段經驗。
**Blocked by:** 26、31、41（已交付）
**Status:** in_progress

## 交付與契約

CPU 純量連續核心的 PPO 可以使用合法的非零初始神經狀態，包含延遲歷史。算分與梯度從同一份狀態出發；目前這段之前的電位及歷史視為固定輸入，不對先前經驗回推。段內沿用既有 Truncation。BurnIn 只遮掉直接損失。

保留既有可用的從零開始與重算路徑。非零狀態的 LIF、mixed 與 Recompute 暫不支援，入口明確拒絕，不默默從零開始或回退。vector 與 GPU 的個體持續狀態原本就未提供。可塑性、化學機制及 MiniBatch=1 的限制保持。API 為新增方法，保存格式、依賴、模型與範例設定保持。

## Root 骨架與檔案責任

1. `Continuous.ForwardFromState(ctx, Parameters, State, inputs) (*Trace, error)`：完整驗證 State，擁有參數與歷史副本；輸出與 Advance 逐位一致，Backward 的參數與輸入梯度涵蓋目前段，保存歷史視為常數。Initial 只報告起點電位的局部敏感度，不由最佳化器更新。既有 Forward／Backward 保持原行為。
2. `Network.LossGradientFromState(ctx, Parameters, NeuralState, input, upstream, window) (Gradient, error)`：沿用 Insyra encoder／readout 梯度橋接、固定符號與共享參數處理，拒絕未支援核心或不合法狀態。
3. `Trainer.StepFromState(ctx, NeuralState, input, upstream) (StepResult, error)`：沿用既有鎖、AdamW、累積器、mask、裁切及提交流程。候選新參數必須從同一保存狀態前向驗證，錯誤不改 trainer／AdamW／未完成累積視窗；拒絕 Recompute。
4. `rl.Update`：驗證所有 rollout 後才更新候選副本，每個 epoch 用同一份 InitialNeural 算分與梯度，回傳個體保留原本現行神經狀態，原個體與 rollout 不變。

測試 agent 只可新增三個 stateful 測試檔及調整原本拒絕非零狀態的測試，以新明確契約驗收。實作 agent 不得修改測試檔。Root 固定契約、審 red／green、追蹤文件與證據、完整驗證及交付。

## 完整使用流程

- 已 Advance 的延遲核心 → 保存狀態 → 段內前向及反向 → 與獨立固定前綴參考／有限差分一致。
- 帶既有記憶的個體 → 收集 rollout → 多 epoch PPO → 手動同狀態算分與梯度更新一致；現行記憶與原個體保持。
- 個體保存 → 新程序載入 → 同一個 rollout 更新 → 與未中斷路徑的參數、最佳化器及神經狀態逐位一致。

## 失敗、空值與限制

| 情況 | 結果與驗收 |
| --- | --- |
| nil／取消 context、空序列、形狀錯誤、負 window | 回錯誤，結果清空，輸入及 trainer 保持 |
| NaN／Inf、錯 schema／config hash／history、步數溢位 | 回錯誤，沿用完整 State 驗證與容量限制 |
| 延遲讀取跨越段起點、steps 小於 delay、4 ULP 容許歷史 | 使用保存歷史原值，不重建或改寫 |
| 陳舊版本、第二份 rollout 不合法、機制不支援 | 更新前拒絕，原個體保持 |
| 候選新參數只從零狀態可執行、保存狀態溢位 | 提交前從保存狀態拒絕，參數、最佳化器及未完成累積保持 |
| 非零狀態的其他核心／Recompute | 明確拒絕，原 fresh-zero 路徑保持 |
| 重複或並行呼叫 | 各自 trace 擁有副本，trainer 依既有鎖順序更新 |

所有列均由本票驗收。無資料遷移或正式環境操作。小型人工數值證據不宣稱導航學習或生物機制成立。

## 驗收

- [x] Root 審過的公開入口失敗測試在骨架／原版本重現，保存 red 與來源指紋（`evidence/LRN-09/stateful-continuous-ppo-20261006/`）。
- [x] 保存歷史、手算／獨立參考、有限差分、段內截斷、非法值與相容性回歸通過。
- [x] 多 epoch PPO 及新程序保存恢復與獨立對照通過，原個體、rollout 與 fresh-zero 路徑保持。
- [x] 完整 gofmt、build、test、race（60m timeout）、vet、依賴、歷史原件檢查與 Root／獨立審查通過。
- [ ] 證據含命令、環境、來源指紋、結果及日誌，追蹤更新、提交、推送並核對遠端。

減法審查：共用現有 encoder／readout 及最佳化器流程，不新增 PPO 設定、保存格式或額外訓練範例。

最終工程驗收：[verification.json](../../evidence/LRN-09/stateful-continuous-ppo-20261006/verification.json)。22 個頂層專項、372 個相關頂層測試、完整一般與 race 各 57 個套件通過。第 5 項待實際提交、推送及遠端核對後完成。
