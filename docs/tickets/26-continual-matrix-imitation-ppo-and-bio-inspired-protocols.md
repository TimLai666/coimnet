# 26 — 研究者可以量測先學 A 再學 B 的保持與適應、完成專家模仿與行動回饋學習，並執行有來源的生物啟發干預協定

Epic：訓練與持續學習；化學與荷爾蒙調節（有來源的協定）

User Story：研究者可以用固定 seed 跑「先學 A、再學 B、重測 A、循序加任務、規則改變後重新適應」，得到
每個階段對所有既有任務的完整矩陣，所有 seed 與失敗執行都保留，評估時調節狀態固定並附狀態切換對照；
可以先用專家路徑模仿、再用一套完整的行動回饋方法（遞迴 PPO）在小型環境上可重現地改善，收集經驗時保存
策略版本、行動機率與初始遞迴狀態；可以執行「改變時機影響記憶」與「狀態影響記憶表現」兩種有文獻登錄的
干預協定，人工數值只能用規格允許的假設名稱，不得標成定量生物重現。

Blocked by：22（記憶表現、穩定化、干預）、23（運行中學習、重播、評估）、21（化學狀態）、17（最佳化器）

Status：第一階段已驗證（2026-09-19）、第三階段已驗證（2026-09-20）；第二階段模仿已實作，PPO 更新入口於 2026-09-22 完成局部驗證，完整學習驗收待完成

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
7. **遞迴 PPO**：`learning/rl` 新套件：`Rollout{PolicyVersion string; InitialNeural NeuralState;
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
type Rollout struct { PolicyVersion string; InitialNeural learning.NeuralState; InitialPlastic *learning.PlasticPart; InitialChemical *learning.ChemicalPart; Steps []Transition }
type PPOConfig struct { Gamma, Lambda, ClipEpsilon, ValueCoef, EntropyCoef float64; BurnIn, TimeLimit int; Epochs, MiniBatch int }
func PolicyVersion(ind *learning.Individual) string
func Update(ctx context.Context, ind *learning.Individual, rollouts []Rollout, actions int, c PPOConfig) (*learning.Individual, PPOReport, error)

package experiment
func RunContinualMatrix(ctx context.Context, p ContinualProtocol) (ContinualReport, error)
func RunImitation(ctx context.Context, c ImitationConfig) (ImitationReport, error)
func RunBioInspired(ctx context.Context, protocol string, c BioInspiredConfig) (BioInspiredReport, error)
```

## 驗收

- [x] 第一階段：矩陣與遺忘手算（3 階段 × 2 任務）、失敗執行保留、固定調節狀態評估與狀態切換對照、
  `independent` 對照分開、事前比較方法；`go test`、race、vet；`evidence/LRN-08/`。
- [ ] 第二階段：`LossGradientFrom` 逐位等於 `LossGradient`；環境評估不洩漏目標；模仿一致率；PPO 機率比值
  三項、版本／狀態一致性拒絕、3 seed 改善且同 seed 逐位重現；`evidence/LRN-09/`。
- [x] 第三階段：登錄三條、兩種協定各 3 seed、表現恢復且三層參數不變、定量命名拒絕；`evidence/MOD-09/`；文件。

## 第一階段證據（2026-09-19）

以「先學 A、再學 B、規則改變 A」的 fixture 協定完成 LRN-08 的第一階段，證據在 `evidence/LRN-08/`（`test.log`、`continual-matrix.json`、`verification.json`）。`go test -count=1 -race -v -run 'Continual|PairedBootstrap|Comparison|Forgetting|AggregateCells|FreezeChemistry|StateSwitch|Independent' ./experiment/ ./internal/cli/ ./learning/` 共 30 個頂層測試全 PASS（experiment 16、internal/cli 3、learning 5，另有 34 個 validate 子測）`grep -c "^--- PASS"` 回 24；CLI 以 `--chemistry` 產出 continual-matrix.json：3 seeds、3 階段 × 2 任務、0 個失敗 run、protocol_hash `4bdb08cda13eabdfd2831785e31d2cd55d61b9336e743ca5b6f6c148a69792cd`。九張小票對照：

1. **凍結化學**：`learning/chemistry_freeze_test.go` 的 `TestFreezeChemistryHoldsConcentration`、`TestFreezeChemistryTwiceFromSameSnapshotIsBitIdentical`、`TestFreezeChemistryIsNotInTheSnapshot`、`TestFreezeChemistryNeedsChemistry`。凍結後濃度逐位保持、無釋放，同一快照凍結兩次輸出與報告逐位相同，凍結旗標只存在 runtime 不在快照。
2. **協定型別**：`experiment/continual.go` 的 `ContinualProtocol`／`Stage`／`TaskSpec`／`Evaluation`／`PreRegistered` 與 `Validate`，`TestContinualProtocolValidateAcceptsTheFixture`／`TestContinualProtocolValidateRejects`（34 個子測全過）。
3. **純函式**：`experiment/continual_math.go` 的 `ContinualEpisode`（協定版 delayed-correlation/v1：`delay+2` 列、channel 上帶脈衝、目標 `gain*a`、flip 取負）、`Forgetting`、`AggregateCells`，`TestForgettingHandTable`／`TestAggregateCellsSkipsFailedSeeds`／`TestContinualEpisode*`。
4. **fixture 與評估**：`experiment/continual_fixture.go` 的 `ContinualFixture`／`ContinualFixtureWithChemistry` 與 `continual_eval.go` 的 `evaluateTask`／`scoreTwin`：評估在快照孿生上做，`FreezeChemistry` 後重設神經狀態取 `-MSE`，結束檢查 Parameters 與 Plastic 逐位不變。
5. **矩陣執行**：`experiment/continual_run.go` 的 `RunContinualMatrix`，`TestContinualMatrixUntrainedCellsMatchDirectScores`（未訓練格子逐位等於直接計算、rule_change 只變 A 欄）、`TestContinualMatrixTrainsAndKeepsEveryRecord`、`TestContinualMatrixKeepsFailedRuns`、`TestContinualMatrixIsDeterministic`、`TestContinualMatrixRejects`。
6. **對照**：`experiment/continual_controls.go` 的 `runIndependent`／`evaluateSwitched` 與 `ControlStateSwitch`／`BaselineIndependent`。
7. **比較**：`experiment/continual_comparison.go` 的 `pairedBootstrap`／`compareToIndependent`，`TestPairedBootstrapConstantDifferences`（常數差 `{1,1,1}` 的 mean/lower/upper 全等 1）、`TestPairedBootstrapBoundsContainTheMean`、`TestComparisonAbsentWithoutDeclaration`、`TestComparisonUsesOnlyPairsWhereBothSucceeded`。
8. **CLI**：`internal/cli/continual.go`，`TestContinualMatrixWritesAReport`／`TestContinualMatrixWithChemistry`／`TestContinualMatrixRejects`。
9. **證據**：`evidence/LRN-08/test.log`、`evidence/LRN-08/continual-matrix.json`、`evidence/LRN-08/verification.json`。

手算常數：遺忘 `F_j(i) = max_{k ≤ i, 未失敗} R[k][j] − R[i][j]`（越大越好；失敗格 F=0）。矩陣 JSON 每 seed 只有 (2,A) 非零，例如 seed 7 `0.12465654281343898 = max(-0.06845514226710238, -0.035994941232014147, -0.16065148404545312) − (-0.16065148404545312)`；forgetting_cells 的 (2,A) mean `0.12043799274790447`。B 欄第 1/2 列三個 seed 全部逐位相同，證明 rule_change 只改 A。手算表測試 `TestForgettingHandTable` 定案 `F = [[0,0],[0,1],[0.3,0]]`。

與 [Root 決策](#root-決策2026-09-15)的出入（實作依定案的契約摘要調整）：`TaskSpec` 沒有 Episodes 欄位，評估期數統一放 `Evaluation.Episodes`（所有任務共用）；`RunRecord` 在 Seed／Stage／Status／Error 之外多了 `Task`（omitempty）與 `Control`（omitempty，命名 state_switch 與 independent 的記錄）；`independent` 對照的訓練方式是在 fresh 個體上只依協定順序重放會碰到該任務的那些 stage（同 `trainSeed`、同 budget、依序套 rule_change），再以該任務的 `evalSeed` 評分，而不是以整套協定重跑。

## 第三階段證據（2026-09-20）

兩種有來源的干預協定都在 fixture 上跑完 3 個 seed，證據在 `evidence/MOD-09/`（`test.log`、`bio-inspired-ecdysone.json`、`bio-inspired-npf.json`、`verification.json`）。`go test -count=1 -race -v -run 'TestBioInspired|TestEcdysone|TestNPF|TestRunBioInspired|TestQuantitative|TestFreezePlasticity|TestScoreTwinFreezes' ./experiment/ ./learning/` 共 17 個頂層測試全 PASS（experiment 14、learning 3），`grep -c "^--- PASS"` 回 17、`^--- FAIL` 回 0；`TestBioInspiredValidateRejects` 另含 14 個全過的子測。驗收四項對照：

1. **登錄三條**：`docs/biological-evidence-registry.md` 的 `[S14]`（第 8 行）、`[S15]`（第 26 行）、`[S20]`（第 44 行），每條都有「原文主張」「本專案採用的設計」「明示不宣稱的事項」「對應主規格章節」四段；「明示不宣稱的事項」分別在第 20、38、56 行。程式端由 `experiment/bioinspired.go` 的 `Validate` 落實：`ecdysone_inspired` 必須宣告 `S14`、`npf_memory_expression_hypothesis` 必須宣告 `S15`，少了就拒絕（子測 `ecdysone_registry_missing_S14`、`npf_registry_missing_S15`）。
2. **兩種協定各 3 seed**：`ecdysone_inspired`（seeds 1/2/3、6 episodes、pulse_steps 0/1/3）的 `TestEcdysoneInspiredCurveHasOnePointPerPulseStep` 通過，三個 seed 的 `slow_magnitude` 都隨脈衝後移單調遞增——seed 1 `0.00029968768656378226 → 0.000410345722329368 → 0.0005279158451362891`、seed 2 `0.0002218320276589831 → 0.0003037579688905269 → 0.0007662567254308275`、seed 3 `0.00017507433279512777 → 0.00023969513711926045 → 0.0009457480937260429`；`retest_score` 三點兩兩不同（測試明文拒絕「三個時機同分」的平曲線），末減首依序 `1.8403789113496938e-07`、`5.0651842484206178e-07`、`5.5983916738555628e-07`，三個 seed 同向但量級只有 1e-7，只能當方向與可分辨的證據，不是效應量宣稱。曲線全數存在 `evidence/MOD-09/bio-inspired-ecdysone.json`。
3. **表現恢復且三層參數不變**：`npf_memory_expression_hypothesis`（seeds 1/2/3、12 episodes、suppressed 0.4、tolerance 1e-9）的 `TestNPFSuppressionLowersScoreAndRecovers` 通過。seed 1 before `-0.062355363585436537`、suppressed `-0.06565613733775605`、after `-0.062355363585436537`（抑制期間下降 `0.0033007737523195121`）；seed 2 `-0.08630233458884023` ／ `-0.09029425705668197` ／ `-0.08630233458884023`（下降 `0.0039919224678417325`）；seed 3 `-0.057882453427303186` ／ `-0.060853987034420166` ／ `-0.057882453427303186`（下降 `0.0029715336071169801`）。三個 seed 的 after 都逐位等於 before，`|after − before| = 0` 遠低於宣告容差 1e-9；`ParametersUnchanged`／`SlowUnchanged`／`PlasticUnchanged` 三個旗標全為 true（`experiment/bioinspired_npf.go` 以 `reflect.DeepEqual` 比對第一次評分前與三次評分後的 `Parameters`、`Plastic.Slow`、`Plastic.State`）。分數變、三層逐位不變，就是「表現不是遺忘」。三次評估存在 `evidence/MOD-09/bio-inspired-npf.json`。
4. **定量命名拒絕**：`TestQuantitativeNamingIsRefusedWithoutFourItems` 通過——協定名稱帶 `ecdysone` 而缺 `QuantitativeEvidence` 時回 `ErrQuantitativeNaming`；補齊 `Method`／`Data`／`Fit`／`HeldOutIntervention` 四項後 `Validate` 過，但 `RunBioInspired` 仍以「quantitative protocols are not implemented」拒絕執行，填滿欄位不會換來數字。`TestBioInspiredValidateRejects` 的 14 個子案例涵蓋未知名稱、seed 少於 3、重複 seed、episodes 越界、兩種協定各自不該出現的欄位（`ecdysone` 的 `suppressed`／`tolerance` 必須為 0、`npf` 的 `pulse_steps` 必須為空）與登錄缺項。

其餘不變量：`TestEcdysoneInspiredIsDeterministic`、`TestNPFIsDeterministic`（同 config 兩次執行 `Seeds` 逐位相同）、`TestEcdysoneInspiredHonoursCancellation`、`TestNPFHonoursCancellation`（已取消的 context 回 `context.Canceled`，不記成失敗 seed）、`TestEcdysoneInspiredRejectsPulseOutsideEpisode`（`pulse_step` 4 超出一個 episode 的 4 列，開跑前就拒絕）、`TestNPFReportRoundTrips` 與 `TestBioInspiredReportJSONShape`（報告 JSON 往返逐位相同）。評估側由 `TestScoreTwinFreezesPlasticTwins`（評分用凍結孿生，評分後個體快照逐位不變）與 `learning` 的 `TestFreezePlasticityHoldsFastState`／`TestFreezePlasticityIsNotInTheSnapshot`／`TestFreezePlasticityNeedsPlasticity` 守住，所以 npf 的三次評分之間唯一的差別是受體驅動的讀出增益。

與 [Root 決策](#第三階段有來源的生物啟發協定mod-09)的出入：決策 10 寫「干預清單沿用 22 的八種，量測活動、快速權重、基礎參數與任務結果四種變化」，實作沒有用到那八種 kind 的任何一種——時機協定用 `ExternalTimeline` 脈衝、狀態協定用 `SetExpressionGain`，量測落實成 `slow_magnitude`、`retest_score` 與三個逐位不變旗標，沒有產生 `ActivityDelta`／`PlasticDelta`／`BaseParameterDelta` 數值。決策 11 寫「README 範例表」，實際補的是 README 的「能力狀態」表（README 沒有範例表）。本階段沒有 CLI 子指令，`evidence/MOD-09/` 的兩份曲線／評估 JSON 是從 `test.log` 的 `t.Logf` 輸出逐字整理的，不是 CLI 產物，整理過程沒有新增程式檔。其餘限制（fixture 三神經元、脈衝時機只在一個 episode 的 4 列內、重測分數差 1e-7 量級、`Suppressed` 由 `Validate` 限制在 (0,1) 且只測 0.4、真實資料未跑、單次 macOS arm64 執行）見 `evidence/MOD-09/verification.json` 的 `limitations`。

## 第二階段更新入口（2026-09-22，局部驗證）

`learning/rl.Update` 已接上既有梯度與最佳化器流程。版本摘要識別完整設定與六組參數，所有 rollout 驗證完才建立更新，成功回傳新個體，失敗保留原個體。每份 rollout 必須由新建或重設至零電位的個體開始收集，`MiniBatch` 限 1，可塑性與化學機制目前明確拒絕。

直接重跑 `go test -count=1 -v ./learning/rl` 的 20 個頂層測試通過。20 個 transition 在更新前的機率比偏差最大為 0，更新後 24 個正優勢 transition 的機率比全部大於 1。同 seed 的報告與個體快照逐位相同。測試使用人工走廊、貪婪動作與明示為 0 的 timeout bootstrap，只驗證更新入口，不能當成完整策略學習成績。

`BurnIn` 現階段只遮掉前綴的直接損失，後續損失仍能經核心回傳梯度到前綴。任意初始狀態與切斷前綴梯度需要相符的反向路徑。3 seed 各 200 次更新、取樣探索、正確的 timeout bootstrap、隨機策略對照與 `examples run gridnav --method imitation|ppo` 尚未完成，`LRN-09` 維持 `specified`。

日誌與輸入指紋見 [本輪證據](../../evidence/rl-asr-takeover-20260922/verification.json)，使用方式見 [PPO SDK](../../learning/rl/README.md)。

## 依據

- 主規格 3.1（S14、S15、S20）、10.1（訓練途徑清單、PPO 參考）、10.4（持續學習基本功能、不得把重新
  初始化稱為持續學習）、10.5（回饋學習與探索：策略版本、行動機率、初始狀態、burn-in、終止、時間上限、
  機率比值測試）、11.7（生物命名限制）、11.8（對照與干預）、13.6（專家模仿、評估不洩漏）、14.5（矩陣、
  多 seed、失敗保留、固定調節狀態）。
