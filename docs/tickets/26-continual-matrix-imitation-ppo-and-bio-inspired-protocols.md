# 26 — 研究者可以量測先學 A 再學 B 的保持與適應、完成專家模仿與行動回饋學習，並執行有來源的生物啟發干預協定

Epic：訓練與持續學習；化學與荷爾蒙調節（有來源的協定）

User Story：研究者可以用固定 seed 跑「先學 A、再學 B、重測 A、循序加任務、規則改變後重新適應」，得到
每個階段對所有既有任務的完整矩陣，所有 seed 與失敗執行都保留，評估時調節狀態固定並附狀態切換對照；
可以先用專家路徑模仿、再用一套完整的行動回饋方法（遞迴 PPO）在小型環境上可重現地改善，收集經驗時保存
策略版本、行動機率與初始遞迴狀態；可以執行「改變時機影響記憶」與「狀態影響記憶表現」兩種有文獻登錄的
干預協定，人工數值只能用規格允許的假設名稱，不得標成定量生物重現。

Blocked by：22（記憶表現、穩定化、干預）、23（運行中學習、重播、評估）、21（化學狀態）、17（最佳化器）

Status：draft（契約已於 2026-09-15 定案；分三階段派工，待 22／23 完成後開始）

對應需求：LRN-08（產生完整階段矩陣，所有 seed 與失敗執行均保留）、LRN-09（至少一套完整算法可在環境改善；
遞迴狀態、策略版本及回饋時間一致）、MOD-09（至少時機與記憶表現兩種協定；人工數值不標為定量生物重現）。
主規格 3.1（S14、S15、S20）、10.1、10.4、10.5、11.7、11.8、13.6、14.5。

## Root 決策（2026-09-15）

### 第一階段：持續學習矩陣（LRN-08）

1. **協定**：`experiment.ContinualProtocol{Tasks []TaskSpec（名稱 + 有版本的 generator 與參數，例如
   `delayed-correlation/v1` 的不同延遲與通道）; Stages []Stage{Kind ∈ train_task|rule_change; Task string;
   Budget uint64}; Seeds []uint64（至少 3）; Evaluation{FixedChemistry bool（評估時把 21 的濃度固定，
   不釋放不清除）; StateSwitch *StateSwitch（另一組化學狀態下再評估一次，作「暫時抑制不是遺忘」的對照）;
   Metric string}; Comparison *PreRegistered{Method string ∈ paired_bootstrap; Interval float64; Baseline
   string}}`。每個階段結束後對**所有**任務評估，得到 `R[i][j]`（階段 i 後在任務 j 的成績）；遺忘
   `F_j(i) = max_{k ≤ i} R[k][j] − R[i][j]`（指標方向先統一為「越大越好」，不符者取負）。
2. **保留一切**：`ContinualReport{Matrix [][]float64 per seed; Forgetting; Mean, Std, Min, Max per cell;
   Runs []RunRecord{Seed; Stage; Status ∈ ok|failed; Error string}}`，失敗執行保留在報告裡，不從平均中
   悄悄剔除（平均只用成功者，並標示成功數／總數）。若協定宣告 `Comparison`，報告附事前選定的方法與
   不確定性區間，否則報告不做「勝過」的陳述。「每個任務重新初始化的獨立訓練」是對照組 `independent`
   而不是持續學習結果（報告分開列）。
3. CLI `examples run continual-matrix`；證據 `evidence/LRN-08/`（fixture：兩個延遲任務 + 一次規則改變，
   3 seed；表格與矩陣都在 JSON）。

### 第二階段：模仿與遞迴 PPO（LRN-09）

4. **梯度接口**：`learning.Network.LossGradientFrom(ctx, p, input, upstream [][]float64)`（呼叫端提供
   `dL/dy`，網路做 VJP，回傳與 `LossGradient` 相同形狀的 `Gradient`）與 `Trainer.StepFrom(ctx, input,
   upstream)`（走同一條裁切／累積／排程／遮罩／投影路徑），既有 `LossGradient` 改為呼叫它並傳
   `2(y − target)/n`（測試逐位相同）。這是 PPO 與模仿共用的唯一梯度入口。
5. **小型環境（新套件 `experiment/gridnav`）**：一維走廊長度 `L`（預設 7），起點在中央，`t = 0` 的觀察含
   一個提示位元指出目標在左或右，之後提示消失（部分可觀察，需要遞迴狀態記住）；動作 `left|right|stay`；
   獎勵到達目標 `+1`、每步 `−0.01`；時間上限 `20` 步；環境有 `Reset(seed)`、`Step(action)`、
   `Expert() action`（專家路徑：往提示方向走）。評估時**不**提供最短路徑或目標座標（主規格 13.6：
   `Observation` 型別沒有這些欄位，測試證明）。
6. **模仿**：`experiment.RunImitation`：以專家動作為標籤做監督（softmax 交叉熵，梯度經 `LossGradientFrom`），
   3 seed，報告專家一致率與環境回報。
7. **遞迴 PPO**：`learning/rl` 新套件：`Rollout{PolicyVersion uint64; InitialNeural NeuralState;
   InitialPlastic, InitialChemical（快照，可為 nil）; Steps []Transition{Obs, Action, LogProb, Value,
   Reward, Done}}`；收集時保存策略版本、行動機率、初始遞迴狀態（主規格 10.5）；更新：GAE（`gamma`,
   `lambda`）、裁切代理目標 `min(r·A, clip(r, 1−ε, 1+ε)·A)`、價值損失、熵；`burn_in` 步只前向不計損失；
   終止與時間上限分開處理（時間上限的最後一步用 bootstrap 價值）；更新前檢查 rollout 的
   `PolicyVersion` 與初始快速權重／化學狀態和目前一致，不一致就拒絕（主規格 10.5「不能沿用不一致的
   新舊快速權重或調節狀態」）。**機率比值測試**：同一 rollout 在未更新的策略下 `r = 1`（1e-12 內）；
   更新一步後 `r` 的方向與優勢符號一致；裁切區外梯度為 0（手算）。**可重現學習證據**：3 seed 各 200 次
   更新，平均回報高於隨機策略基線且 seed 間離散度報告；同 seed 兩次執行逐位相同。信用分配是完整的
   優勢估計，不是「reward 非零時增加所有活躍權重」（文件明寫）。
8. CLI `examples run gridnav --method imitation|ppo`；證據 `evidence/LRN-09/`。

### 第三階段：有來源的生物啟發協定（MOD-09）

9. **證據登錄**：`docs/biological-evidence-registry.md` 與 `data/evidence-registry.json`（`S14`：Ishimoto 等
   PNAS 2009，20E 與長期求偶記憶，給藥時機影響結果，不能簡化成濃度越高記憶越好；`S15`：Krashes 等
   Cell 2009，NPF／多巴胺迴路使內在狀態影響記憶表現，暫時不表現不一定是遺忘；`S20`：PPO 參考）。每條
   登錄：來源、原文主張、本專案採用的設計、明示不宣稱的事項。
10. **兩種協定（`experiment.RunBioInspired`）**：
    - `ecdysone_inspired`（時機）：同一學習 episode 前／中／後在不同步數注入一段 `ExternalTimeline`
      脈衝（21 的濃度 + 22 的 `Consolidate` 觸發由該通道受體驅動），量測 `slow` 幅度與重測成績隨脈衝
      時機的曲線；報告只給曲線與差值，明寫「工程數值，非定量重現」。
    - `npf_memory_expression_hypothesis`（狀態）：學完後同一個體在「表現抑制」（22 的
      `SetExpressionGain` 受體驅動，濃度高）與「中性」兩種狀態下評估，再切回中性重評：抑制期間成績下降、
      切回後恢復到抑制前（差 < 宣告容差），並比對 `Parameters`、`slow`、`plastic` 在三次評估間逐位不變
      （證明是表現不是遺忘）。
    協定名稱只能是這兩個；任何要標成 `ecdysone`／`npf` 定量模型的請求需要 `QuantitativeEvidence{Method,
    Data, Fit, HeldOutIntervention}` 四項齊全，否則拒絕（測試）。干預清單沿用 22 的八種，量測活動、
    快速權重、基礎參數與任務結果四種變化。
11. 證據：`evidence/MOD-09/`（fixture，3 seed）；文件：README 範例表、`docs/model-and-mechanisms.md`。

## 契約摘要

```go
package learning
func (n *Network) LossGradientFrom(ctx context.Context, p Parameters, input, upstream [][]float64) (Gradient, error)
func (tr *Trainer) StepFrom(ctx context.Context, input, upstream [][]float64) (StepResult, error)

package rl   // learning/rl
type Rollout struct { PolicyVersion uint64; InitialNeural learning.NeuralState; Steps []Transition }
type PPOConfig struct { Gamma, Lambda, ClipEpsilon, ValueCoef, EntropyCoef float64; BurnIn, TimeLimit int; Epochs, MiniBatch int }
func Update(ctx context.Context, ind *learning.Individual, rollouts []Rollout, c PPOConfig) (PPOReport, error)

package experiment
func RunContinualMatrix(ctx context.Context, p ContinualProtocol) (ContinualReport, error)
func RunImitation(ctx context.Context, c ImitationConfig) (ImitationReport, error)
func RunBioInspired(ctx context.Context, protocol string, c BioInspiredConfig) (BioInspiredReport, error)
```

## 驗收

- [ ] 第一階段：矩陣與遺忘手算（3 階段 × 2 任務）、失敗執行保留、固定調節狀態評估與狀態切換對照、
  `independent` 對照分開、事前比較方法；`go test`、race、vet；`evidence/LRN-08/`。
- [ ] 第二階段：`LossGradientFrom` 逐位等於 `LossGradient`；環境評估不洩漏目標；模仿一致率；PPO 機率比值
  三項、版本／狀態一致性拒絕、3 seed 改善且同 seed 逐位重現；`evidence/LRN-09/`。
- [ ] 第三階段：登錄三條、兩種協定各 3 seed、表現恢復且三層參數不變、定量命名拒絕；`evidence/MOD-09/`；文件。

## 依據

- 主規格 3.1（S14、S15、S20）、10.1（訓練途徑清單、PPO 參考）、10.4（持續學習基本功能、不得把重新
  初始化稱為持續學習）、10.5（回饋學習與探索：策略版本、行動機率、初始狀態、burn-in、終止、時間上限、
  機率比值測試）、11.7（生物命名限制）、11.8（對照與干預）、13.6（專家模仿、評估不洩漏）、14.5（矩陣、
  多 seed、失敗保留、固定調節狀態）。
