# 25 — 研究者可以依神經元類型混合連續與脈衝規則、使用向量節點與共享參數，並以重算降低反向歷史記憶體

Epic：神經核心與梯度；訓練與持續學習

User Story：研究者可以把每群神經元指派給連續或 LIF 規則，同一核心時鐘下跨類型傳訊有明確契約且不
雙重計入輸出；可以讓每個節點帶多維狀態並記錄維度，可以按類型共享參數或逐項參數，展開與回收梯度的索引
對照可查；可以用重算代替保存完整反向歷史，得到與完整歷史一致的梯度，且重算不觸發第二次學習、隨機事件
或教師呼叫。

Blocked by：11 LIF 核心、16 LIF 個體、17 最佳化器、21 化學狀態（重算檢查點要包含它；未啟用時為 nil）

Status：draft（契約已於 2026-09-15 定案；分三階段派工）

對應需求：COR-05（跨類型傳訊與同步更新正確，不雙重計入輸出）、COR-06（形狀與共享梯度正確；不同容量
分開報告）、LRN-02（與完整歷史參考一致，無重複學習、重複隨機事件或教師呼叫）。主規格 2.1 D11、7.3、
8.1、8.2、8.4、9.2、10.2、16.4、21。

## Root 決策（2026-09-15）

### 第一階段：按類型混合（COR-05）

1. **型別指派**：`dynamics.MixedConfig{Nodes int; Sources, Targets []int; Delays []int; DT float64;
   NodeRule []uint8 /*0 = continuous, 1 = lif；長度 N*/; Continuous ContinuousRuleConfig{Activation};
   LIF LIFRuleConfig{TauSyn, ThetaMin, ThetaMax, VReset, RefractorySteps, Adaptation, Homeostasis, Surrogate}}`；
   `NewMixed(c) (*Mixed, error)`。每個節點只有一種規則（主規格 8.4：不是全部神經元同時輸出兩次）；
   全 0 或全 1 的指派必須逐位等於既有 `Continuous`／`LIF`（測試兩者）。
2. **跨類型輸出契約（同一時鐘、同步更新，所有連線只讀上一步或更早的狀態）**：每個節點對外只有一個
   輸出序列 `out_j(t)`：連續節點 `out_j = y_j = phi(v_j)`；LIF 節點 `out_j = x_j`（衰減突觸跡，與 11 一致，
   不是原始 0/1 事件）。突觸輸入 `I_i(t) = external_i(t) + Σ_j w_ji · out_j(t − delay_ji)`，對兩種目標
   相同；連續目標把 `I_i` 當 8.1 的輸入，LIF 目標把 `I_i` 當 8.2 的突觸輸入。延遲 0 也讀「本步開始時」的
   `out`（代數循環不存在，順序無關；測試：改變節點編號順序結果不變）。不雙重計入：一個 LIF 節點的事件
   只透過 `x` 進入下游一次（測試：把一個 LIF 節點同時接到連續與 LIF 目標，下游收到的輸入等於手算的
   `w·x`，沒有第二條 spike 路徑）。
3. **梯度**：連續節點沿 8.1 的反向；LIF 節點沿 11 的替代梯度；跨類型的邊只是一般的 `w·out` 乘積，反向
   對 `out` 的梯度分派回各自的節點規則。有限差分只對「全連續」與「LIF 用平滑測試模式」驗證（沿用 11 的
   規則：硬事件不做有限差分）；混合的正確性以「單一類型子圖」的逐位等價與手算 3 節點混合圖釘住。
4. **learning 整合**：`learning.Config.Mixed *dynamics.MixedConfig`（與 `Dynamics`、`LIF` 三選一），
   `coreModel` 加第三個實作；`Parameters.ThetaRaw` 長度 = LIF 節點數（索引對照 `LIFIndex []int` 由
   核心提供並寫進 `Config()`），`Trainable.Theta` 只作用於它們；個體 profile
   `mixed-f64-insyra-f32-persistent-inference-episode-learning/v1`，`NeuralState` 聯集加 `Mixed
   *dynamics.MixedState{Continuous State（只含連續節點）; LIF LIFState（只含 LIF 節點）; Index}`。
   混合是可選能力，文件明寫「全腦一律混合未經實驗支持，不是預設」（主規格 2.1 D11）。
5. 證據：`evidence/COR-05/`（fixture：3 節點手算混合圖、全 0／全 1 逐位等價、編號順序無關、不雙重計入）。

### 第二階段：向量節點與共享參數（COR-06）

6. **向量節點**：`dynamics.Config.StateDimension int`（預設 1；`> 1` 時每個節點狀態是 `C` 維向量，
   `v_i ∈ R^C`，`b_i ∈ R^C`，`tau_i` 仍是純量，邊權重 `w_ji ∈ R^{C×C}` 或純量廣播（`EdgeShape ∈
   scalar|matrix`）；`I_i = Σ_j W_ji · y_j`）。`Parameters.Core.Weights` 的排列與長度由 `EdgeShape` 決定並
   寫進 `Config()`；`StateDimension = 1, EdgeShape = scalar` 逐位等於既有實作（測試）。**容量報告**：
   `learning.Network.Capacity() CapacityReport{Nodes, Edges, StateDimension, ParameterCount,
   FreeParameterCount, MultAddsPerStep}`，寫進模型包與 `model inspect`；文件明寫「節點數相同不代表
   容量相同」。本階段只做連續核心的向量節點；LIF 的向量節點不做（LIF 的閾值語意對向量沒有定義，
   拒絕並說明）。
7. **共享參數**：`learning.Config.Sharing *ParameterSharing{Weights []int32 /*每邊的群組索引；−1 = 逐項*/;
   Bias []int32; LogTau []int32; Groups int}`；`Parameters` 存展開後的逐項值（核心不變），
   `Sharing` 決定訓練時的回收：`flatGradient` 之後 `reduce`：同群組的梯度**相加**（不是平均，主規格
   9.2）到群組代表，AdamW 在群組層更新，再 `expand` 回每個成員（成員逐位相同）；索引對照
   `SharingIndex{Expand []int; Reduce [][]int}` 由 `Network.Config()` 給。與 17 的遮罩、固定符號、
   範圍相容：遮罩以群組為單位（群組內任一成員被遮罩則整組凍結，並回報衝突數），固定符號要求同群
   同號否則拒絕。手算：兩條邊共享一個權重，梯度 = 兩邊梯度之和（不是平均）；`Sharing = nil` 逐位等於
   改前。
8. 證據：`evidence/COR-06/`（fixture：向量節點 `C = 3` 的手算線性圖、對稠密參考、零邊／孤立節點／
   自我連線、共享梯度相加、容量報告）。

### 第三階段：重算（LRN-02）

9. **重算檢查點**：`learning.Options.Recompute *Recompute{SegmentSteps int}`：前向只保存每段起點的
   `RecomputeCheckpoint{Step; Neural NeuralState; Plastic *plasticity.State; Chemical
   *modulation.ChemistryState; RNG []byte（若有）; Inputs 起點索引; Transaction uint64}`，反向時逐段
   從檢查點重跑前向得到該段的中間值再回傳梯度；記憶體從 `O(T)` 降到 `O(T/S + S)`（報告實測 RSS）。
   **一致性**：對同一 fixture，`Recompute = nil` 與 `SegmentSteps ∈ {1, 4, 7}` 的梯度在 1e-12 內相同
   （連續與 LIF 各一；有截斷 `Truncation` 時亦同）。
10. **無副作用**：重算路徑經 `pureReplay` 旗標執行：可塑性更新、化學釋放、教師與環境呼叫在該旗標下
    被拒絕（測試以計數的假來源證明呼叫次數與不重算時相同）；隨機事件重放來自檢查點的 RNG 狀態
    （測試：帶隨機的 fixture 兩次前向逐位相同）；`Transaction` 序號在重算中不遞增（測試）。
    「時間窗之外仍可保留個體狀態，但不能宣稱損失可追溯到無限久以前」：`StepResult` 加
    `GradientHorizonSteps`，文件明寫。
11. 證據：`evidence/LRN-02/`（fixture；附 `T = 256, S = 16` 的 RSS 對比）。

## 契約摘要

```go
package dynamics
type MixedConfig struct { ... }; func NewMixed(c MixedConfig) (*Mixed, error)
// Config.StateDimension int; Config.EdgeShape string

package learning
// Config.Mixed *dynamics.MixedConfig; Config.Sharing *ParameterSharing
type CapacityReport struct { Nodes, Edges, StateDimension, ParameterCount, FreeParameterCount, MultAddsPerStep int }
func (n *Network) Capacity() CapacityReport
type Recompute struct { SegmentSteps int }   // Options.Recompute *Recompute
// StepResult.GradientHorizonSteps int
```

## 驗收

- [ ] 第一階段：全 0／全 1 逐位等價、3 節點混合手算、編號順序無關、不雙重計入、混合個體快照；
  `go test`、race、vet；`evidence/COR-05/`。
- [ ] 第二階段：`C = 3` 手算與稠密參考、退化逐位等價、共享梯度相加手算、與遮罩／符號的相容規則、
  容量報告進模型包；`evidence/COR-06/`。
- [ ] 第三階段：三種分段與完整歷史 1e-12 一致、零副作用三項（可塑性／來源／教師計數）、隨機重放、
  交易序號不變、RSS 對比；`evidence/LRN-02/`；文件。

## 依據

- 主規格 2.1 D11、7.3（純量／向量、共享／逐項、索引對照）、8.1、8.2、8.4（混合規則、輸出契約、
  延遲 0）、9.2（共享梯度相加、試金石清單）、10.2（有限時間窗與重算、禁止副作用）、16.4、21。
