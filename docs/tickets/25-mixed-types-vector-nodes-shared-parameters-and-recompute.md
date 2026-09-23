# 25 — 研究者可以依神經元類型混合連續與脈衝規則、使用向量節點與共享參數，並以重算降低反向歷史記憶體

Epic：神經核心與梯度；訓練與持續學習

User Story：研究者可以把每群神經元指派給連續或 LIF 規則，同一核心時鐘下跨類型傳訊有明確契約且不
雙重計入輸出；可以讓每個節點帶多維狀態並記錄維度，可以按類型共享參數或逐項參數，展開與回收梯度的索引
對照可查；可以用重算代替保存完整反向歷史，得到與完整歷史一致的梯度，且重算不觸發第二次學習、隨機事件
或教師呼叫。

Blocked by：11 LIF 核心、16 LIF 個體、17 最佳化器、21 化學狀態（重算檢查點要包含它；未啟用時為 nil）

Status：stage1_verified（契約已於 2026-09-15 定案；分三階段派工。第一階段於 2026-09-16 實作並驗證，證據見下；第二、三階段未開始）

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

- [x] 第一階段：全 0／全 1 逐位等價、3 節點混合手算、編號順序無關、不雙重計入、混合個體快照；
  `go test`、race、vet；`evidence/COR-05/`。
- [x] 第二階段：`C = 3` 手算與稠密參考、退化逐位等價、共享梯度相加手算、與遮罩／符號的相容規則、
  容量報告進模型包；`evidence/COR-06/`。
- [ ] 第三階段：三種分段與完整歷史 1e-12 一致、零副作用三項（可塑性／來源／教師計數）、隨機重放、
  交易序號不變、RSS 對比；`evidence/LRN-02/`；文件。

## 第一階段證據（2026-09-16）

先寫失敗測試再實作：`evidence/COR-05/red-mixed.log`（`dynamics` 的 11 個混合測試在型別存在前的編譯失敗）、
`evidence/COR-05/red-learning.log`（`learning` 的混合測試在 `Config.Mixed` 存在前的編譯失敗）、
`evidence/COR-05/red-checkpoint.log`（`checkpoint` 讀到 `"mixed"` 核心時的
`$.payload.neural.core "mixed" is not a known core`）。

### 3 節點手算混合圖

節點 0 連續、節點 1 LIF、節點 2 連續；邊 0 是 `0 -> 1` 延遲 0 權重 4，邊 1 是 `1 -> 2` 延遲 1 權重 2。
`dt = ln 2`、`log_tau = 0`、`tau_syn = 1`，所以 `lambda = alpha = kappa = 0.5` 剛好；
`theta_min = 0`、`theta_max = 2`、`theta_raw = 0`，所以 `theta_base = 1` 剛好；`v_reset = -1`、不應期 1 步。
外部輸入只進節點 0：`[4, 0, 0, 0]`。

| 步 | v0 | out0 | 節點 1 的 drive | v1 | event1 | x1 | 節點 2 的 drive | v2 | out2 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 2 | tanh 2 | `4 * out0(0) = 0` | 0 | 0 | 0 | `2 * x1(0) = 0` | 0 | 0 |
| 2 | 1 | tanh 1 | `4 * tanh 2` | −1（放電後重設） | 1 | 1 | `2 * x1(0) = 0` | 0 | 0 |
| 3 | 0.5 | tanh 0.5 | （不應期，輸入不進膜電位） | −1 | 0 | 0.5 | `2 * x1(1) = 0` | 0 | 0 |
| 4 | 0.25 | tanh 0.25 | `4 * tanh 0.5` | `−0.5 + 2 tanh 0.5 = 0.4242…` | 0 | 0.25 | `2 * x1(2) = 2` | 1 | tanh 1 |

實測與上表相符（`TestMixedHandCalculatedThreeNodeGraph`，容差 1e-12，超越部分為 `tanh` 的浮點值）。
同一份 fixture 走持續路徑（`Advance` 一次跑完，以及切成 2+2 兩段）得到逐位相同的輸出、事件與兩半狀態。

**不雙重計入**（`TestMixedNoDoubleCounting`）：節點 0 是 LIF，同時接到連續目標 1（權重 2）與 LIF 目標 2（權重 3），
兩條邊延遲皆 0。手算：第 1 步節點 0 放電、`x0 = 1`，兩個目標當步收到的都是 `w * x0(0) = 0`；
第 2 步目標 1 收到 `2 * 1 = 2`（`v = 1`、`out = tanh 1`），目標 2 收到 `3 * 1 = 3`（`cand = 1.5 ≥ 1`，放電）；
第 3 步分別是 `2 * 0.5 = 1` 與 `3 * 0.5 = 1.5`。實測逐項相符，事件沒有第二條路徑進入下游。

**編號順序無關**（`TestMixedNodeNumberingOrderIndependence`）：4 節點、5 條邊、兩個 LIF 節點的 fixture，
以置換 `0→1, 1→3, 2→0, 3→2` 重新編號（`NodeRule`、`bias`、`log_tau`、初始電位、輸入列一起置換，
`theta_raw` 依新編號的遞增 LIF 順序重排，邊順序不變只改端點標號），每個節點的輸出、事件與電位逐位相同。

**梯度**（`TestMixedHandCalculatedTwoStepGradient`）：同一 3 節點圖跑 2 步，`upstream = [[.3,−.2,.5],[.7,.4,−.6]]`。
依宣告規則手算（`dlambda = lambda*dt/tau = .5 ln2`、`theta` 斜率 `= (2−0)·sigma(0)(1−sigma(0)) = .5`、
`psi(u) = 1/(1+|u|)^2`；`dv0 = b0(1−tanh²1)`、`du1 = b1·psi(2 tanh 2 − 1)`、
`dv0b = .5 dv0 + (a0 + 2 du1)(1−tanh²2)`、`dv2b = a2 + .5 b2`）：

| 群組 | 手算值 |
| --- | --- |
| weights | `[.5 du1 tanh 2, 0]` |
| bias | `[.5 dv0 + .5 dv0b, .75 du1, .5 b2 + .5 dv2b]` |
| log_tau | `[ln2 (dv0 − 2 dv0b), −2 ln2 du1 tanh 2, 0]` |
| theta_raw | `[−.5 du1]`（長度 1，只有 LIF 節點） |
| inputs[0] | `[.5 dv0b, .25 du1, .5 dv2b]` |
| inputs[1] | `[.5 dv0, .5 du1, .5 b2]` |
| initial | `[.5 dv0b + du1, .25 du1, .5 dv2b]` |

實測逐項相符（容差 1e-12）。第二個權重恰為 0，正是「延遲 1 的 LIF → 連續邊在兩步內只讀到零的突觸跡前史」。

有限差分只做宣告允許的兩處：`TestMixedSmoothModeFiniteDifference` 用套件私有的平滑模式
（4 節點、2 連續 2 LIF、6 條邊含延遲 0/1/2、適應開啟、6 步）對 weights、bias、log_tau、theta_raw、
初始電位與每一步輸入做中心差分，相對誤差 1e-4、近零看絕對 1e-7，涵蓋兩個方向的跨類型邊；
硬放電事件不做有限差分。全連續指派的梯度另以逐位等價釘在 `Continuous.Backward` 上，而後者已有既有的有限差分測試。

### 逐位等價與 `-race` 的例外

全 0 指派對 `Continuous`：前向輸出、最終電位、五組梯度、`Advance` 的輸出與狀態（步數、電位、歷史）在
一般建置與 `-race` 建置都逐位相同（tanh 與 softplus 各一組）。

全 1 指派對 `LIF`：事件、適應、慢速穩定、不應期計數與全部結構欄位在兩種建置都逐位相同；
膜電位、突觸跡與梯度在**一般建置**逐位相同，在 `-race` 建置相差 1e-16 以內（實測最大絕對差 1.39e-17）。
原因是既有 `LIF.Forward` 自己的行為，不是混合核心的：`cand = lambda*v + alpha*drive` 這一行
Go 允許用融合乘加（只捨入一次），編譯器逐基本區塊決定，於是**同一行在不同建置會給出相鄰的兩個 float64**。
實測（`TestMixedMembraneContraction`，會在常數與觀測不符時失敗）：`Continuous.Forward` 在兩種建置都選融合，
`LIF.Forward` 在一般建置選融合、在 `-race` 建置不融合；混合核心把這一行放進自己的 `membrane` 函式
（`//go:noinline`）因此兩種建置都固定選融合。放寬只套用在「全 1 對 LIF」的膜電位衍生值上，
其餘比較維持逐位相等。**這是既有核心的性質，不是本階段放寬的驗收標準**，記在此處供後續核對。

### learning、checkpoint 與文件

`learning.Config.Mixed` 與 `Dynamics`、`LIF` 三選一（缺一或多於一都拒絕）；個體 profile 為
`mixed-f64-insyra-f32-persistent-inference-episode-learning/v1`；`NeuralState` 聯集加入 `Mixed`，
三個半邊只能出現宣告的那一個；`Parameters.ThetaRaw` 長度等於 LIF 節點數，索引對照由核心寫進
`Config().LIFIndex`（其他兩種核心不得帶這個欄位）。`Trainable.Theta` 只作用於那幾項，
`Options.Masks.Nodes` 經 `LIFIndex` 對應到正確的 theta 項（遮住 LIF 節點就凍結它的 theta，
遮住連續節點不會）。可塑性（18）在混合核心上 `pre = out`，`post` 對 LIF 目標取事件、對連續目標取輸出，
以閘門為 0 的四步手算釘住 eligibility；`stdp_pair` 在混合核心被拒絕，因為連續節點沒有事件。
`checkpoint` 的聯集走訪接受第三種核心，並拒絕同時帶兩個半邊或同時宣告兩種核心的文件。

未完成／已知限制：`checkpoint.ModelPackage` 尚未支援混合核心（拓撲指紋只讀連續與 LIF 設定），
因此明確拒絕並指向持續個體快照；`simulate` 的原生 runner 不提供混合核心；向量節點與共享參數是第二階段。

## 第二階段證據（2026-09-23）

向量節點接進 learning 的訓練路徑：`StateDimension > 1` 由向量核心執行，encoder、讀出與反向梯度都按分量展開，
`StateDimension 1` 與純量核心逐位相同。三分量 matrix 邊的向量核心（4 節點、含自我連線與孤立節點、延遲 0–2）
與展開成 12 個純量節點的稠密參考比對，前向最大誤差 8.3e-17、兩個截斷窗的反向最大誤差 5.6e-17 與 2.8e-17；
零邊時每個分量與手算洩漏逐位相符；每步讀出的向量梯度與中央差分相對誤差 2.2e-6。

共享參數：`Config.Sharing` 把 weights／bias／log_tau 的條目綁成群組，一步之中群組梯度換成成員梯度的和
（手算：兩條邊 −0.017171 與 −0.017194，共享後用 −0.034365，AdamW 一步後兩個成員同為 0.30999999709），
梯度範數每群只算一次，遮罩凍結任一成員就凍結整群並計入衝突，成員必須同初值、同固定號誌；群組數超過可共享的值
總數會在配置前被拒絕。與 Root 決策 7 的差異：沒有另建 `SharingIndex` 與群組層最佳化器狀態，而是讓每個成員
拿到相同的梯度和、做相同的 AdamW 更新，數學上等價且不改快照格式。

容量：`Network.Capacity`／`Trainer.Capacity` 分開回報節點、邊、分量數、參數量、可更新參數量與每步乘加數
（純量 3 節點 2 邊為 13 個參數、5 次乘加；C = 3 matrix 2 節點 1 邊為 26 個參數、15 次乘加），
模型包新增 `capacity` 欄位，舊模型包讀回為 nil、指紋不變；向量核心拒絕固定邊號誌。
完整命令與輸出見 [COR-06 證據](../../evidence/COR-06/verification.json)。

未完成／已知限制：向量核心的持續個體狀態（`NewIndividual`）仍明確拒絕；容量數字是計數，不是實測記憶體；
全部是 fixture。

## 依據

- 主規格 2.1 D11、7.3（純量／向量、共享／逐項、索引對照）、8.1、8.2、8.4（混合規則、輸出契約、
  延遲 0）、9.2（共享梯度相加、試金石清單）、10.2（有限時間窗與重算、禁止副作用）、16.4、21。
