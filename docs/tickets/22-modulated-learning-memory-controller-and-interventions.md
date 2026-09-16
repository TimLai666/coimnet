# 22 — 研究者可以讓化學狀態調節局部學習與記憶表現、施加具名干預、訓練小型控制器並和簡單替代模型對照

Epic：化學與荷爾蒙調節（第三層：效果進學習與記憶、干預、控制器、對照）

User Story：研究者可以讓受體佔用率決定局部學習閘門與參與紀錄的時間窗，讓記憶表現受調節而不被
誤算成永久學習，讓暫時變化寫入較慢狀態時受觸發、預算與去重規則約束；可以在實驗設定中授權具名的
細胞與通道干預並追查其開始、結束、對象與效果；可以訓練一個只看得到歷史摘要與先前回饋的小型控制器，
並用無調節、直接獎懲、固定規則、可訓練與容量匹配五組同資料同預算的對照比較。

Blocked by：18 局部可塑性（第一階段）、20 調節來源、21 化學濃度與效果（兩階段）、17 最佳化器

Status：第一階段已驗證（2026-09-17）；第二階段（干預）、第三階段（控制器與對照）待派工

對應需求：MOD-05（閘門、衰退、延遲回饋與關閉效果分開驗證；受體來源）、MOD-06（不誤算為永久學習；
重複寫入防護與恢復）、COR-11（干預開始、結束、對象與效果可追查；正常推論不能任意鉗制）、MOD-07
（梯度路徑及容量可查；無題目旁路；容量匹配對照）、MOD-10（無調節、直接獎懲、固定規則、可訓練與容量
匹配組均有實驗流程）。主規格 6.1、7.2、10.3、10.4、11.2、11.5、11.8、14.4、14.5、16.2（`ablate`）。

## Root 決策（2026-09-15）

### 第一階段：受體驅動的閘門與時間窗（MOD-05）、記憶表現與穩定化（MOD-06）

1. **閘門與時間窗來自受體**：18 的 `plasticity.Rule` 加 `GateReceptor *int`（受體索引）與
   `DecayEReceptor *int`；每步 `gate(t) = GateScale * occ_gate(t)`（`occ` 由 21 的佔用率給），
   `decay_e(t) = clamp(DecayEBase + DecayESpan * occ_decay(t), DecayEMin, DecayEMax)`，區間必須在
   `(0, 1)` 內（建構時驗證）。呼叫端仍可直接給閘門序列（18 的路徑），兩者同時宣告時拒絕。測試分開：
   閘門關（`occ = 0`）時 `plastic` 只衰退不新增；不同 `decay_e` 讓延遲回饋的結果可區分；
   `occ` 從 1 降到 0 的關閉效果與 18 的常數閘門手算一致。
2. **記憶表現（MOD-06 前半）**：`learning.Individual.SetExpressionGain(nodes []int, receptor int,
   scale, min, max float64)`：讀出前對指定節點的輸出乘 `clamp(1 + scale*occ, min, max)`。
   只作用於讀出路徑，不動 `Parameters`、`plastic`、`eligibility`（測試：跑 200 步三者逐位不變）；
   增益回到 1 後輸出逐位恢復（狀態切換測試：抑制期間輸出下降、解除後與從未抑制的軌跡在同一步
   起相同，因為核心狀態未被改動）。報告欄位 `expression_gain_applied`，文件明寫「這不是遺忘」。
3. **穩定化寫入較慢狀態（MOD-06 後半）**：三層記憶 = `plastic`（暫時）、`Parameters.Core.Weights`
   （基礎）之間加一層 `slow`（較慢，`SlowState{Values []float64（依啟用邊順序）; LastEpisode uint64;
   Budget, Used uint64}`）。寫入規則明示：`Consolidate(ctx, trigger ConsolidationTrigger)` 只在
   `trigger.Receptor` 的 `occ ≥ Threshold` 且本 episode 尚未寫入（`LastEpisode < 目前 episode`）且
   `Used < Budget` 時執行，寫入量 `slow += Rate * plastic`（`Rate ∈ (0, 1]`），然後 `plastic` 依
   `Retain ∈ [0, 1]` 縮放；同一 episode 第二次呼叫回 `ErrAlreadyConsolidated` 且狀態不變；預算用完回
   `ErrBudgetExhausted`。有效權重變成 `w_base + slow + plastic`（固定符號規則同 18）。`slow` 不是
   永久學習：報告分開列 `base_l2`、`slow_l2`、`plastic_l2`，梯度 `Step` 不動 `slow`。
   快照：`IndividualSnapshot.Plastic` 加 `Slow *SlowState`。恢復測試：中途快照接續後 `Consolidate`
   的去重判斷與連續執行相同。
4. 證據：`evidence/MOD-05/`、`evidence/MOD-06/`。

## 第一階段證據（2026-09-17）

驗證指令：`cd /Users/timlai/Developer/coimnet && go test -count=1 -race -v -run 'TestReceptor|TestExpression|TestConsolidat|TestSlowState|TestRuleReceptor|TestStepWith|TestWindowAndGate|TestExplicitGate|TestDisableChemistry' ./plasticity/ ./learning/ > evidence/MOD-05/test.log 2>&1; cp evidence/MOD-05/test.log evidence/MOD-06/test.log`

19 個頂層測試全 PASS、0 FAIL、無 panic，`-race` 下兩套件都 `ok`（plasticity 1.447s、learning 1.735s；go1.26.5 darwin/arm64）。四張小票的證據與手算常數：

- **plasticity 受體欄位**（`plasticity/receptor_test.go`）：`TestRuleReceptorFieldsValidated` 拒絕 `gate_scale` 無受體、負受體索引、`decay_e` 的 min/max/base/span 各種錯誤組合；`TestStepWithDecayEOverride` 手算每步覆寫（eligibility = `decay_e*elig + pre*post`：0.5×0+1×1=1、0.25×1+1=1.25、0.75×1.25+1=1.9375，`decay_e`=1 或 NaN 拒絕）；`TestWindowAndGateFor` 夾限 `WindowFor`（occ 0→0.5、1→0.8、−1→0.3、NaN→0.5）與 `GateFor(0.25)=2×0.25=0.5`，無受體時回 false。
- **個體受體閘門**（`learning/receptor_gate_test.go`）：閘門關時 plastic=0、forward 與化學-only 逐位相同、eligibility 0.4579164488274385 持續追蹤；閘門開時 occ1=0.611481100422978、mid plastic=0.1315724355959273=occ1×elig1（elig1=0.2151700772189281）、最終 plastic 0.3052627141695012；時間窗 `decay_e(occ1)=0.5−0.4×0.611481100422978=0.2554075598308088`，最終 eligibility 0.4158818857906775 低於固定 0.5 衰退的 0.4579164488274385；無化學拒絕、越界受體（7）拒絕、明確閘門與受體閘門不得並用（失敗呼叫不提交任何狀態）、規則指向受體時 `DisableChemistry` 拒絕。
- **表現增益**（`learning/memory_test.go`）：`TestExpressionGainScalesOnlyTheReadout` 證明只動讀出（兩快照除 Expression 外 `DeepEqual`）、row 0 增益=1 逐位相同、row 1 讀出=twins×`1−0.5×0.611481100422978`；`TestExpressionGainStateSwitchIsNotForgetting` 清除後下一列逐位恢復；驗證與快照往返各過。
- **穩定化**（`learning/consolidation_test.go`）：`TestConsolidateHandComputed` 以 Rate 0.5／Retain 0.25 手算一次寫入（slow=0.5×P1、plastic=0.25×P1，三層 L2 分開：BaseL2=0.9、SlowL2=|0.5×P1|、PlasticL2=|0.25×P1|，計數器 {episode 0, last 1, used 1}）；同 episode 第二次拒絕（slow `DeepEqual`、plastic fast `sameBits` 逐位不變）；預算用盡、閘門關各拒絕且狀態不變；slow 折進基礎後輸出／報告／電位逐位相同；JSON 快照接續後去重判斷與連續執行相同。

`evidence/MOD-05/verification.json`（閘門／窗口）、`evidence/MOD-06/verification.json`（表現／穩定化），皆 `profile: fixture`。限制：fixture 規模（一至兩顆神經元、單一受體、數列）；只有 `hebbian_rate` 在個體上驗證受體閘門（`stdp_pair` 共用同一 `Model` 機械但沒有個體受體閘門測試）；時間窗只驗證單一受體；穩定化閘門只看最近一次 advance 的佔用率報告；無真實任務；本 log 只含上述 `-race` 目標測試，完整 `go test ./...` 與 `go vet ./...` 不在本次指令範圍。

### 第二階段：具名干預（COR-11）

5. **干預是實驗設定的授權模式，不是推論 API**：`simulate.Protocol.Interventions []Intervention`
   （`omitempty`，既有 hash 不變）與 `learning.Individual.Intervene(ctx, plan InterventionPlan)`
   （回傳 `InterventionLog`）。`Intervention{Kind string ∈ clamp_voltage|force_spike|silence|
   block_channel|fix_concentration|shuffle_delays|swap_regions|remove_channel; Targets Selector 或
   節點索引; Channel int; Value float64; Start, End uint64（步，半開區間）; Restore bool}`。
   每種在 `dynamics.AdvanceModulated` 之上以「每步覆寫」實作：`clamp_voltage` 在 `[Start, End)` 把
   目標電位設為 `Value`；`force_spike` 強制事件；`silence` 把目標的輸入與輸出置零；`block_channel`
   把該通道的佔用率置零；`fix_concentration` 把該通道濃度固定為 `Value`；`shuffle_delays`
   以 seed 打亂目標邊的延遲（只影響該次執行的設定副本）；`swap_regions` 對調兩區域的濃度；
   `remove_channel` 把通道釋放率置零。`End` 之後（`Restore` 為 true）狀態依規則恢復正常演化，不
   回填被覆寫期間的歷史。
6. **可追查**：`InterventionLog{Entries []struct{Kind; Targets（已解析的節點或通道）; Start, End;
   AppliedSteps uint64; ActivityDelta, PlasticDelta, BaseParameterDelta float64（與同 seed 無干預執行
   的 L2 差）; TaskDelta *float64}}` 寫進 `RunReport.interventions` 與個體的 `AdvanceReport`。
   主規格 11.8 要求分別量測活動、快速權重、基礎參數與任務結果的變化：前三者每次干預都算，任務結果
   只有在協定宣告指標時給。
7. **正常推論不能鉗制**：`Advance`／`AdvanceGated` 沒有任何參數能改電位；`Intervene` 需要
   `InterventionPlan.Authorized = true` 且 `Reason` 非空，否則拒絕；測試證明未授權呼叫不改任何狀態。
8. 證據：`evidence/COR-11/`（fixture：八種干預各一組手算或不變量測試；`clamp_voltage` 期間電位
   等於 `Value`、結束後自由演化；`fix_concentration` 期間濃度不受釋放率影響）。

### 第三階段：小型可訓練控制器與對照流程（MOD-07、MOD-10）

9. **控制器**：`modulation.Controller{Inputs ControllerInputs{SummaryWindow int; UseFeedback bool;
   UseResources bool}; Hidden int; Channels int; Parameters []float64}` 是一個小 MLP：輸入 =
   最近 `SummaryWindow` 步節點輸出的均值與方差摘要（不是完整活動）+ 已可取得回饋的分數 + 資源；
   輸出 `softplus` 保證非負的 `q`。它實作 20 的 `Source` 介面（取代 `ErrControllerNotImplemented`）。
   **梯度路徑**：控制器參數的梯度只經由它自己的輸出對一個宣告的「控制目標」計算
   （`ControllerObjective ∈ reward_proxy|activity_target`），用有限差分驗證；本票不把梯度穿過
   濃度→受體→核心（那條可微路徑另立需求時再開），文件明寫。參數量與計算量寫進 `CapacityReport`
   （`parameters`, `mult_adds_per_step`），並計入整體模型容量報告。**無題目旁路**：控制器的輸入型別沒有
   `Target`；`scripts/check-target-flow.sh` 把 `modulation` 納入檢查（20 已做）；執行期汙染測試同 20。
10. **容量匹配對照**：`modulation.MemoryController` 是同參數量的一般記憶控制器（同 MLP 形狀，
    但輸出直接加到讀出，而非釋放率），建構時 `parameters` 必須與控制器相等（測試）。
11. **對照流程（MOD-10）**：`experiment.RunAblation(ctx, AblationConfig)` 與 CLI `examples run ablate`：
    五組 `no_modulation | direct_reward | fixed_decay | trainable_controller | capacity_matched`
    用同一資料分割、同 seed、同更新預算與同指標；每組結果獨立保存為 `ablation/<group>.json`，
    彙總報告列任務指標、參數量、活動變化（與 `no_modulation` 的 L2 差）。`direct_reward` 是把獎懲
    直接當損失權重（主規格「不得僅將 reward 改名為 hormone」的對照）；`fixed_decay` 是 20 的
    `ExternalTimeline` + 21 的濃度。報告不做文字判斷。
12. 證據：`evidence/MOD-07/`、`evidence/MOD-10/`（fixture：小型延遲任務，五組各 3 seed）。

## 契約摘要

```go
// 第一階段
// plasticity.Rule 加 GateReceptor, DecayEReceptor *int; GateScale, DecayEBase, DecayESpan, DecayEMin, DecayEMax float64
func (i *Individual) SetExpressionGain(nodes []int, receptor int, scale, min, max float64) error
type SlowState struct { Values []float64; LastEpisode, Budget, Used uint64 }
type ConsolidationTrigger struct { Receptor int; Threshold, Rate, Retain float64 }
func (i *Individual) Consolidate(ctx context.Context, t ConsolidationTrigger) (ConsolidationReport, error)
// 第二階段
type Intervention struct { Kind string; Targets []int; Selector *simulate.Selector; Channel int; Value float64; Start, End uint64; Restore bool; Seed uint64 }
type InterventionPlan struct { Authorized bool; Reason string; Items []Intervention }
func (i *Individual) Intervene(ctx context.Context, plan InterventionPlan, input [][]float64) ([][]float64, InterventionLog, error)
// 第三階段
type Controller struct { ... }  // 實作 modulation.Source
func (c *Controller) Gradient(objective ControllerObjective, history ...) ([]float64, error)
type CapacityReport struct { Parameters int; MultAddsPerStep int }
func RunAblation(ctx context.Context, c AblationConfig) (AblationReport, error)
```

## 驗收

- [x] 第一階段：閘門關、時間窗可區分、關閉效果、記憶表現狀態切換（不是遺忘）、穩定化去重與預算、
  三層 L2 分開、快照接續；`go test`、race、vet；`evidence/MOD-05/`、`evidence/MOD-06/`。
- [ ] 第二階段：八種干預各有測試、未授權不改狀態、期間與結束後行為、四種差值計算；
  `evidence/COR-11/`；`simulate run` 的 `interventions` 區塊與 `RunReport.interventions`。
- [ ] 第三階段：控制器有限差分、容量報告、無題目旁路、容量匹配參數量相等、五組對照流程各 3 seed、
  CLI；`evidence/MOD-07/`、`evidence/MOD-10/`；文件。

## 依據

- 主規格 6.1（不得在預設推論覆寫電位）、7.2（生物干預模式）、10.3、10.4（三層記憶、穩定化不是
  相加）、11.2（控制器輸入限制與容量計入）、11.5（閘門、時間窗、記憶使用、穩定化閘門）、11.8
  （對照與干預清單）、14.4、14.5（固定調節狀態的記憶測試、狀態切換測試）、16.2（`ablate`）。
