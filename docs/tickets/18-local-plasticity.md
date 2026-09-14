# 18 — 研究者可以使用近期參與紀錄、學習閘門與兩種局部更新規則

Epic：訓練與持續學習（可塑性）

User Story：研究者可以在個體運作中開啟局部可塑性：每條指定連線保存近期參與紀錄與快速變化，
以活動相關或脈衝時序相關的規則更新，受學習閘門與衰退控制，延遲到達的回饋仍能作用；關閉時
還原原生行為；固定符號的連線不會因快速變化跨零。

Blocked by：11 LIF 核心、16 LIF 個體（第一階段）、17 固定符號（第一階段）

Status：draft（契約已於 2026-09-15 定案，分兩階段派工；待 16／17 第一階段完成後派工）

對應需求：LRN-04（活動相關與脈衝時序相關規則，各有方程式與前後時序測試）、LRN-05（近期參與
紀錄與調節控制：延遲回饋、閘門關閉、衰退、更新順序）、MOD-05 的閘門與時間窗部分（受體來源在
後續調節票）、NAT-06 的前置（第二階段接 runner）。主規格 10.3、11.5、7.2。

## Root 決策（2026-09-15）

1. **參考規則（主規格 10.3 原式）**：對每條啟用可塑性的邊 e=(j→i)：
   `elig_e(t+1) = decay_e * elig_e(t) + pre_j(t) * post_i(t+1)`，
   `plastic_e(t+1) = bounded(decay_p * plastic_e(t) + gate(t+1) * elig_e(t+1))`。
   `pre`／`post` 對 LIF 是突觸跡 x 與當步 spike（活動相關版）；連續核心是輸出 y。`bounded` 為
   `clamp(·, -plastic_max, plastic_max)`。
2. **脈衝時序規則（STDP，只對 LIF）**：每邊兩個跡 `pre_trace`（`decay_pre`，在 pre spike 時 +1）與
   `post_trace`（`decay_post`，在 post spike 時 +1）；post spike 時 `elig += a_plus * pre_trace`，
   pre spike 時 `elig -= a_minus * post_trace`；`plastic` 更新同上。規則以名稱選擇
   `rule ∈ hebbian_rate | stdp_pair`，兩者各有手算的前後時序測試（pre-before-post 增強、
   post-before-pre 減弱）。
3. **有效權重與符號安全**：自由邊 `w_eff = w_base + plastic`；固定符號邊（17 的 `EdgeSigns`）
   `w_eff = s * max(|w_base| + plastic, w_min)`，`w_min > 0`（設定），永不跨零；報告被 `w_min` 擋下的
   次數。快速變化不寫回基礎參數。
4. **閘門與延遲回饋**：`gate(t)` 是外部給定的時序（本票由呼叫端提供，可為常數、可為延遲脈衝；
   後續調節票才由受體產生）；閘門為 0 時 `plastic` 只衰退不新增（測試）；延遲 d 步到達的閘門仍以
   殘留的 `elig` 更新（測試以手算對照，且 `decay_e` 不同時結果可區分）。
5. **更新順序**：同一步內固定順序：核心前向 → 更新 elig 與 trace → 套 gate 更新 plastic →
   下一步前向使用新的 `w_eff`。與梯度更新同時存在時，梯度更新只改 `w_base`，在 episode 邊界進行，
   不與 plastic 更新交錯（主規格 10.3「唯一順序與合併方式」）。單一 goroutine。
6. **狀態與保存**：`PlasticState{Eligibility, Plastic, PreTrace, PostTrace []float64（依啟用邊順序）}`
   進 `IndividualSnapshot.Plastic`（omitempty；未啟用為 nil），schema 維持
   `coimnet-individual-checkpoint/v1`。關閉可塑性時前向逐位等於未啟用。
7. **第二階段（NAT-06 前置）**：`simulate` 的 protocol 可宣告 `plasticity` 區塊，runner 在 `Run` 中
   套同一套規則（第一階段的 `plasticity` 套件），`RunReport` 加 `plasticity{rule, enabled_edges,
   clamped, gate_source}`；`simulate compare` 可跑「學習前／學習後／關閉」三格同一刺激。

## 契約（第一階段：套件 `plasticity` 與 learning 整合）

```go
package plasticity
type Rule struct { Kind string; DecayE, DecayP, PlasticMax, WMin float64; DecayPre, DecayPost, APlus, AMinus float64 }
type Config struct { Rule Rule; Edges []int /*啟用的邊索引，遞增*/ }
type State struct { Eligibility, Plastic, PreTrace, PostTrace []float64 }
func New(c Config, edges int) (*Model, error)
func (m *Model) NewState() State
func (m *Model) Step(s State, pre, post, spikesPre, spikesPost []float64 /*節點層*/, gate float64, sources, targets []int) (State, Report, error)
func (m *Model) Effective(base []float64, signs []int8, s State) ([]float64, ClampReport, error)
// learning: Individual.EnablePlasticity(c plasticity.Config), Individual.Advance 接受 gate 序列
```

## 驗收

- [ ] 第一階段：兩種規則各一組 2–3 神經元手算時序（含閘門關閉、延遲閘門、衰退、上限、`w_min`）；
  關閉可塑性逐位等於未啟用；固定符號邊不跨零；快照往返；`go test`、race、vet；`evidence/LRN-04/`、
  `evidence/LRN-05/`。
- [ ] 第二階段：runner 與 compare 的 `plasticity` 區塊、三格對照（前／後／關閉）在 fixture 與全圖
  各跑一次；`evidence/NAT-06/`；文件。

## 依據

- 主規格 10.3（局部可塑性原式與固定符號限制）、11.5（局部學習閘門、學習時間窗）、7.2（模式表）、
  研究方向 NAT-06。
