# 41 — 研究者可以在大分數下正確計算 PPO 機率與損失，並收到非法數值錯誤

Epic：訓練與持續學習。

User Story：研究者可用同一公開介面取得正確的動作機率、損失與梯度；不可表示的計算回錯誤，不偽造成功結果。

Blocked by：無。ticket 26 的更新入口已存在，框架盤點已重現缺陷。

Status：completed，5／5 驗收通過。實作 `6d44edf` 已推送並核對遠端，見[交付紀錄](../../evidence/LRN-09/objective-numerics-20261006/delivery.json)。

範圍：修正通用 `learning/rl` 公開機率與損失計算，對應 LRN-09。原始缺陷與手算重現見 [框架盤點](../../evidence/LRN-09/framework-audit-20261006.md)。不改模型、訓練配方、依賴、零初始狀態或可塑性／化學機制限制。

## Root 決策（2026-10-06）：修正骨架與共用契約

- 根因：先把最大分數加回 log-sum-exp，再相減，會丟失正規化量。`LogProb` 與 `Loss` 要共用位移後的 log-softmax，現有私有 `softmax` 同步消除此模式。
- 公開簽名保持不變：`LogProb(logits []float64, action int) (float64, error)`；`Loss(logits []float64, action int, oldLogProb, advantage, value, target float64, c PPOConfig) (StepLoss, []float64, float64, error)`。
- 所有相等分數包含 0、±1e16、±1e308，兩動作各為 0.5。以手算舊機率 -ln(2) 驗證比值 1、熵及正負優勢梯度，不用待測函式產生期望。
- 不相等但可精確表示的分數，加同一常數後機率、損失與梯度不變。原有有限差分與裁切行為保持。
- 機率指數運算下溢成零時，保留跳過熵乘積的既有行為，大係數不恢復尾端貢獻。限制與公開入口重現見[獨立審查](../../evidence/LRN-09/objective-numerics-20261006/review.md)。
- NaN／+Inf 分數拒絕。保留原可用的 `-Inf` 動作遮罩：至少一個有限分數時可正規化，被遮罩動作的 `LogProb` 為 -Inf，機率為零，熵不產生 NaN；全部遮罩拒絕。有限分數相減溢位則回錯誤。
- `Loss` 的 oldLogProb、advantage、value、target 必須有限。非有限結果（比值、損失分項、總損失或任一梯度）回錯誤，不回成功的半套結果。遮罩選中動作可保持零機率比值及既有有限結果。
- 關閉的價值項不計算差值平方或梯度，因此有限極值在 ValueCoef=0 時不造成 0×Inf。啟用時的 value-target 須能以有限 float64 表示，否則拒絕。已啟用的價值項保留係數加權的乘法順序，不先算可能溢位的未加權平方或 2×係數。手算驗證小係數／大差值，以及大係數／零或小差值，見 `review-final-red.log`。
- 失敗不得修改輸入。錯誤回傳 `StepLoss{}`、nil 梯度、零 value 梯度，不能偽造裁切成功。

## 檔案責任

- 測試 agent 只可新增 `learning/rl/objective_numerical_test.go`，不可修改實作或既有測試。
- 實作 agent 只可修改 `learning/rl/objective.go`，不得修改測試檔，認為測試錯就停下回報。
- Root 審核、執行失敗／成功測試、更新 `learning/rl/README.md`、`ENG.md`、追蹤文件與證據，完成提交及遠端核對。

## 行為與失敗路徑

呼叫端 → `LogProb`／`Loss` → 共用位移正規化 → 有限損失與梯度，或錯誤回呼叫端。這條路徑不寫個體、最佳化器或快照，不需要資料遷移。

| 操作與例外 | 處理者與結果 | 驗證責任 |
| --- | --- | --- |
| 正常分數、可表示的共同位移 | 公開入口回正確機率與梯度 | 本 ticket 的手算與平移測試 |
| nil／空分數、非法 action | 公開入口回錯誤 | 新增或既有公開入口測試 |
| 全遮罩、NaN、+Inf、非法純量 | 公開入口回錯誤，Loss 結果清空 | 本 ticket 的非法輸入測試 |
| 正規化、比值、損失或梯度溢位 | 公開入口回錯誤，不交付部分結果 | 本 ticket 的數值極值測試 |
| 零係數與零機率 | 關閉項跳過計算，零機率跳過熵乘積 | 本 ticket 的關閉項與遮罩測試 |
| 同一輸入重複呼叫 | 結果一致且輸入不變，無共享可變狀態 | 本 ticket 的輸入保留與 race 驗證 |

## 驗收

- [x] 先跑新增測試，確認原始實作失敗，保存完整日誌：`evidence/LRN-09/objective-numerics-20261006/red.log`。
- [x] 手算相等分數、平移不變性、遮罩、非法輸入、溢位與有限加權價值回歸全部通過：`green.log`，13 項新增頂層測試。
- [x] 既有 RL／collector 測試與完整 build、test、race、vet、相依性檢查通過。
- [x] 審查完整 diff，確認公開簽名、模型、歷史證據及交接原件保留。
- [x] 保存命令、環境、來源指紋與日誌，提交並推送後核對遠端提交。

減法審查：修正現有共用機率計算，不新增人工模型的調參選項，也不在本次加入帶記憶的訓練功能。
