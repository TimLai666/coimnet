# 22 — 研究者可以讓化學狀態調節局部學習與記憶表現、施加具名干預、訓練小型控制器並和簡單替代模型對照

Epic：化學與荷爾蒙調節（第三層：效果進學習與記憶、干預、控制器、對照）

User Story：研究者可以讓受體佔用率決定局部學習閘門與參與紀錄的時間窗，讓記憶表現受調節而不被
誤算成永久學習，讓暫時變化寫入較慢狀態時受觸發、預算與去重規則約束；可以在實驗設定中授權具名的
細胞與通道干預並追查其開始、結束、對象與效果；可以訓練一個只看得到歷史摘要與先前回饋的小型控制器，
並用無調節、直接獎懲、固定規則、可訓練與容量匹配五組同資料同預算的對照比較。

Blocked by：18 局部可塑性（第一階段）、20 調節來源、21 化學濃度與效果（兩階段）、17 最佳化器

Status：三階段皆已驗證（2026-09-23）

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

## 第二階段證據（2026-09-19）

驗證指令：`cd /Users/timlai/Developer/coimnet && go test -count=1 -race -v -run 'TestIntervention|TestIntervene|TestClampVoltage|TestClampOnLIF|TestSilence|TestForceSpike|TestUnauthorizedPlan|TestUnsupportedKind' ./learning/ ./simulate/ > evidence/COR-11/test.log 2>&1`

`evidence/COR-11/test.log` 記錄最後一次完整重跑的 `-race` 執行：25 個頂層測試全 PASS（learning 13、simulate 12）、55 個子案例全 PASS、0 FAIL、無 panic，兩套件都 `ok`（learning 1.387s、simulate 1.659s；go1.26.5 darwin/arm64，Insyra v0.3.2）。`evidence/COR-11/verification.json` 是完整驗收紀錄，`profile: fixture`。四張小票的證據：

- **宣告與八條驗證規則**（`learning/intervention_test.go`）：`TestInterventionPlanValidate` 44 個子案例逐條走 rule 1（未授權／空白原因回 `ErrInterventionNotAuthorized`）、rule 2（空項目、未知 kind 拒絕、八種全部接受）、rule 3（空區間、NaN／無限 value 拒絕）、rule 4（節點目標升冪與範圍、`force_spike` 需要放電核心）、rule 5（通道範圍、`fix_concentration` 非負）、rule 6（`swap_regions` 兩個不同且界內區域）、rule 7（`shuffle_delays` 需要正延遲與界內邊）、rule 8（同類同目標時間重疊拒絕）。
- **節點類干預**（`learning/intervene_test.go`）：`TestInterveneClampVoltageHoldsThenReleases` 用 tau = dt = 1（lambda = k = exp(−1)、alpha = 1 − k）證明鉗制期間電位等於 `Value`、結束後自由演化：out = [tanh(alpha)、tanh(0.25)、tanh(0.25)、tanh(k·0.25 + alpha)]，鉗制窗 [1,3) 的 `AppliedRows` 2、`ActivityDelta` 0.684772379365581，下一呼叫從釋放電位 k·0.25 + alpha 續跑（電位 0.8984985375725405）；日誌 JSON 往返保留。`TestInterveneSilenceZeroesOutput` 輸出與持久跡置零、下游零驅動（電位恰為 k·alpha = 0.23254415793482963）。`TestInterveneForceSpikeOnLIF` 只在 LIF 上強制事件（膜電位 = VReset −0.5、不應期 3、跡 +1），連續核心拒絕；`TestInterveneKeepsBaseParameters` 強制放電讓 eligibility 到 (k+1)·1 = 1.3678794411714423、`PlasticDelta` 同值、`BaseParameterDelta` 0 且快照參數逐位不變。`TestInterveneUnauthorizedChangesNothing` 七條拒絕路徑全部 `Snapshot` 逐位相同。
- **通道與結構干預**（`learning/intervene_channels_test.go`）：`remove_channel` 濃度兩個區域恰為 0、佔用率 0、`ReleaseTotal` [0]、`ConcentrationDelta` 1.3644775709818022；`fix_concentration` 固定後自由行回到 2·exp(−0.5) = 1.2130613194252668、續跑 2·exp(−1) = 0.7357588823428847、`ConcentrationDelta` 3.4678116724044341；`block_channel` 佔用率歸零、輸出與電位和無化學執行逐位相同、濃度和 `ConcentrationDelta` 0；`swap_regions` 交換兩區域（同釋放率下值無效）；`shuffle_delays` seed 7 把 [1 2] 置換成 [2 1]，輸出等於獨立宣告 [0 2 1] 的核心、呼叫後延遲還原 [0 1 2]。
- **原生 runner 干預**（`simulate/interventions_test.go`）：`TestInterventionsKeepProtocolHash` 證明無干預區塊的協定編碼與 hash（NAT-01 記錄值 `befa9f1d...`）不變、加區塊後 hash 改變；`TestClampVoltageHoldsThenReleases`（連續）讀 [tanh(alpha·2)、tanh(.25)、tanh(.25)、tanh(k·.25)]、`AppliedSteps` 2、報告帶 reason；`TestSilenceZeroesOutputAndDownstream` 下游收到 0（探針 [1, k, k², k³]）；`TestForceSpikeOnLIF` 事件、v_reset、跡、不應期齊全。另 `TestClampOnLIFHoldsTheMembraneVoltage`（鉗制覆寫事件重設）、`TestSilenceNeedsAZeroActivation`、`TestInterventionConflictsRejected`、`TestInterventionTargetsOutOfRangeRejected`、`TestInterventionTargetsMustBeStrictlyIncreasing`、`TestForceSpikeRejectedOnContinuous`、`TestUnauthorizedPlanRejected`、`TestUnsupportedKindRejected` 逐一釘住語意與拒絕。

限制：fixture 規模（一至三顆神經元、單一通道、至多數列）；runner 沒有通道類干預（沒有化學層），五種通道／結構 kind 在 `simulate` 一律被拒；runner 的 `[Start, End)` 以每次 `Run` 呼叫的列數計而非全域步數（`AppliedSteps` 是該次重疊步數）；任務結果差（`TaskDelta`）未實作；節點類干預在 mixed core 被拒（未實作）；真實資料未跑；本 log 只含上述目標測試，完整 `go test ./...` 與 `go vet ./...` 不在本次指令範圍。

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

## 第三階段證據（2026-09-23）

驗證指令：`cd /Users/timlai/Developer/coimnet && go test -count=1 -race -v -run 'Controller|MemoryController|CapacityMatched|SourceSpecBuildsAController' ./modulation/ > evidence/MOD-07/test.log 2>&1; grep -c "^--- PASS" evidence/MOD-07/test.log; go test -count=1 -race -v -run 'Ablation|Ablate' ./experiment/ ./internal/cli/ > evidence/MOD-10/test.log 2>&1; grep -c "^--- PASS" evidence/MOD-10/test.log; rm -rf evidence/MOD-10/ablation; go run ./cmd/coimnet examples run ablate --out-dir evidence/MOD-10; scripts/check-target-flow.sh`

MOD-07 的 12 個頂層測試全 PASS、MOD-10 的 13 個頂層測試全 PASS（experiment 10、CLI 3）、0 FAIL、無 panic，`-race` 下都 `ok`（modulation 1.474s、experiment 2.353s、internal/cli 2.022s；go1.26.5 darwin/arm64）。`examples run ablate` 實跑寫出六個檔（五組 JSON + summary.json，0 failed runs），`scripts/check-target-flow.sh` 全 PASS（dynamics 10、simulate 11、modulation 10、plasticity 1 個非測試檔無 `signal.Target`／`NewTarget(`；learning 18 檔；四個 forward／modulation 套件都不依賴 `learning`，最後一行 `RESULT: a target reaches no forward or modulation package`）。七張小票的證據：

- **控制器來源**（`modulation/controller.go`、`controller_test.go`）：`TestControllerHandForward` 手算前向——Nodes 2、Hidden 1、視窗 2 行，首步視窗 {0,0}、{1,1} 讀 mean 0.5、population variance 0.25，channel 1 釋放 `softplus(tanh(0.5))`、channel 0 恰為 0（Channels = Channel+1 = 2）；下一步視窗滿 {1,1} 讀 mean 1、variance 0，釋放 `softplus(tanh(1))`；錯寬度的活動拒絕、nil 活動讀成宣告節點數的零。`TestControllerReadsFeedbackAndResources` 證明 InputWidth 4（均值、方差、回饋 0.5、energy 2）時釋放 `softplus(tanh(0.5+2))`，缺資源時 `softplus(tanh(0.5))`（讀 0 不是拒絕）、無回饋時 `softplus(tanh(2))`、未到達的回饋拒絕。`TestControllerValidateAndCounts` 釘住 Hidden 3、InputWidth 4 下參數量 19 = 3·4+3+3+1、每步乘加 15 = 3·4+3，十一種壞宣告全拒絕。`TestSourceSpecBuildsAController` 證明 Build 回 `*Controller`、spec 改動不影響已建來源（釋放仍 `softplus(tanh(0.5))`）、五種歧義 spec 拒絕。
- **控制目標梯度**（`controller_objective.go`、`controller_objective_test.go`）：`TestControllerGradientMatchesFiniteDifferences` 用 eps=1e-6 的中央差分逐參數對 25 個參數（Nodes 3、Hidden 4、InputWidth 4）驗證，reward proxy 最差相對誤差 4.919340696604421e-09、activity target 6.235998255320076e-09；`TestControllerUpdateReducesLoss` 20 次 Update(0.05) 讓 loss 逐次單調不增、由 0.9084832839974664 降到 0.010770769812422113；`TestControllerGradientRejects` 無 release、reward proxy 但沒讀回饋、未知 objective、非有限 target、gradient 長度不符或 rate 不合法一律拒絕且參數逐位不變。
- **記憶控制器**（`memory_controller.go`、`memory_controller_test.go`）：`TestCapacityMatchedHasEqualParameterCount` 證明與控制器同 InputWidth 4、ParameterCount 19、MultAddsPerStep 15，各持自己的參數與資源副本；`TestMemoryControllerHandForward` 手算線性位移 `2*tanh(1)+0.5`、同一輸入同一參數下控制器釋放 `softplus(2*tanh(1)+0.5)`（唯一差別是讀出走線、位移沒有 softplus）；`TestMemoryControllerGradientMatchesFiniteDifferences` 19 參數最差相對誤差 3.358017227354867e-09；`TestMemoryControllerValidateRejects` 短參數、0 hidden、`NewCapacityMatched(nil)`、未 Offset 即梯度全拒絕。
- **對照前三組**（`ablation.go`、`ablation_test.go`）：`TestAblationSharesDataAcrossGroups` 證明共用同資料分割、TrainSeed 1001／TestSeed 1003、`DataHash` 等於共用 episode 的 hash；`TestAblationNoModulationMatchesDirectTraining` 逐行手算重放後與組內 seed 7 分數完全相同、activity delta 0；`TestAblationDirectRewardDiffersAfterReward` Episodes 1 時每 seed 都與 no_modulation 相同（首個閘門是尚未產生的 0）、6 時出現差異與正 delta；`TestAblationFixedDecayRuns` 三 seed 全成功、分數有限、delta 非負；`TestAblationValidate`（八個子案例）與 `TestAblationIsDeterministic` 釘驗證與重現。
- **對照後兩組**（`ablation_controllers.go`、`ablation_controllers_test.go`）：`TestAblationControllerParametersMatch` 兩組同為個體 13 + 控制器 21（Hidden 4、InputWidth 3 = 均值、方差、回饋）= 34 參數；`TestAblationTrainableControllerChangesRelease` 六 episode 真的動控制器參數、seed 7 又不是 no_modulation 藏身；`TestAblationControllerContextHasNoTarget` 逐欄名檢查無 Target、episode 0 無回饋、之後只有前一 episode 分數；`TestAblationFiveGroupsDeterministic` 五組兩次執行 JSON 逐位相同、所有 seed 成功。
- **CLI**（`internal/cli/ablate.go`、`ablate_test.go`）：`TestAblateWritesEveryGroup` 五組檔 + summary.json、schema `coimnet-ablation/v1`、每組三 run、固定順序；`TestAblateSubsetOfGroups` 子集只寫宣告的組；`TestAblateRejects` 缺 `--out-dir`、既有 ablation 目錄、`--groups` 缺 no_modulation、少於三 seed、越界預算與位置參數全拒絕並命名錯誤。
- **證據**：`evidence/MOD-07/verification.json`、`test.log`；`evidence/MOD-10/verification.json`、`test.log`、`ablation/`（五組 JSON + summary.json）。CLI 實跑（seeds 7,42,123、episodes 40、eval 32、learning_rate 0.02、controller_hidden 4、controller_rate 0.05；config_hash `d5e78257…`）summary.json 五組表：no_modulation mean −0.01995946228553269、std 0.0010604794160278639、3/3、13；direct_reward mean −0.0198638117588819、std 0.0010597663490686862、3/3、13；fixed_decay mean −0.0017328064162344026、std 0.0004836227033012419、3/3、13；trainable_controller mean −0.0066941676446041695、std 0.0020598217077653896、3/3、34；capacity_matched mean −0.037647698381520364、std 0.004521682380114229、3/3、34；每 seed 的 activity_delta（與同 seed no_modulation 保留評估讀出的 L2 差）與分數見 `summary.json`，報告記錄數字不做文字判斷。

與 Root 決策的出入：`ControllerInputs.Resources` 是宣告要讀的資源**名稱清單**（`[]string`，缺資源讀 0），不是決策寫的 `UseResources bool`；在對照流程裡控制器不會直接當 `Source` 實例掛在個體上，而是每次 episode 釋放後經 `SetResource` 寫出名為 `controller` 的資源、由 `ablationControllerChemistry` 的 `SourceInternalResource` 進化學層（與 fixed_decay 的 `ExternalTimeline` 只差釋放來源、其餘全等）；控制器與記憶控制器的活動視窗是跨呼叫的狀態、**不在任何快照**（本階段復原後視窗從零重來）。

限制：fixture 任務（三顆神經元延遲脈衝、CLI 實跑 40 training／32 eval episode）與單機；控制器視窗以 episode 為單位而非模型步；capacity_matched 的位移只進該組指標、不進基礎參數的梯度；只有 reward_proxy 與 activity_target 兩種 proxy；視窗不在快照；真實資料未跑；本 log 只含上述目標測試，完整 `go test ./...` 與 `go vet ./...` 不在本次指令範圍。

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
- [x] 第二階段：八種干預各有測試、未授權不改狀態、期間與結束後行為、四種差值計算；
  `evidence/COR-11/`；`simulate run` 的 `interventions` 區塊與 `RunReport.interventions`。
- [x] 第三階段：控制器有限差分、容量報告、無題目旁路、容量匹配參數量相等、五組對照流程各 3 seed、
  CLI；`evidence/MOD-07/`、`evidence/MOD-10/`；文件。

## 依據

- 主規格 6.1（不得在預設推論覆寫電位）、7.2（生物干預模式）、10.3、10.4（三層記憶、穩定化不是
  相加）、11.2（控制器輸入限制與容量計入）、11.5（閘門、時間窗、記憶使用、穩定化閘門）、11.8
  （對照與干預清單）、14.4、14.5（固定調節狀態的記憶測試、狀態切換測試）、16.2（`ablate`）。
