# 11 — 研究者可以執行並訓練 LIF 放電核心

Epic：數值參考核心

User Story：研究者可以用與連續核心同一套拓撲、延遲與同步時鐘執行 LIF 放電模型，
以宣告的替代梯度訓練權重、偏置、時間常數與基礎閾值，並分開驗證前向事件時序與
反向近似。

Blocked by：02 稀疏算子、04 連續核心訓練

Status：verified_scoped（三個階段皆已驗證。COR-03、COR-09 標為 passed；COR-04 的基礎閾值與短期適應在本票完成，慢速穩定已於 [ticket 16](16-lif-individual-and-model-package.md) 補齊，COR-04 三項機制到齊，`docs/requirements-status.json` 的狀態更新由該票後續處理）

對應需求：COR-03（LIF 時序）、COR-09（替代梯度）、COR-04 的基礎閾值可訓練與短期
適應部分。慢速穩定（homeostasis）與 LIF 個體持續狀態另開 ticket 16，並已於 2026-09-15
實作與驗證；化學調節與按類型混合（COR-05）仍未實作。本票不加無作用欄位。

## Root 決策（2026-09-14）

1. **膜電位**：沿用連續核心的指數漏電，`lambda_i = exp(-dt/tau_i)`、
   `alpha_i = -expm1(-dt/tau_i)`，`v_cand_i(t+1) = lambda_i*v_i(t) + alpha_i*(b_i + I_i(t))`。
   `tau_i = exp(log_tau_i)` 可訓練，與連續核心相同。
2. **突觸輸出**：放電神經元對外的輸出是衰減突觸跡 `x_j`，不是原始 0／1：
   `x_j(t+1) = kappa*x_j(t) + spike_j(t+1)`，`kappa = exp(-dt/tau_syn)`，`tau_syn` 為
   模型設定（所有神經元相同，非可訓練）。邊讀取 `x_j(t-delay)`，讀出也看 `x`。
   `tau_syn` 極小時 `x` 退化為原始脈衝，因此不另設「原始脈衝輸出」模式。
3. **閾值參數化**：`theta_base_i = theta_min + (theta_max-theta_min)*sigmoid(theta_raw_i)`，
   `theta_raw` 可訓練；設定須滿足 `v_reset < theta_min < theta_max`，建構時檢查，
   因此 `v_reset < theta_effective` 恆成立（適應只會抬高閾值）。反向對 `theta_raw` 要乘
   `(theta_max-theta_min)*sigmoid'(theta_raw)`。
4. **放電與重設**：`theta_eff_i(t+1) = theta_base_i + a_i(t)`；
   `spike_i(t+1) = 1 若非不應期且 v_cand_i(t+1) >= theta_eff_i(t+1)，否則 0`；
   `v_i(t+1) = v_reset 若放電，否則 v_cand`。每步最多一個 spike，這是離散步的表示上限，
   不是生物頻率。
5. **不應期**：語意固定為「保持並忽略輸入」：放電後接下來 `refractory_steps` 步
   `v = v_reset`、不放電、該步的輸入與邊貢獻不進入膜電位（drive 仍可算但不用），
   反向對這些步的 `v_cand` 與輸入梯度為零。`refractory_steps >= 0`。
6. **短期適應（可開關）**：`a_i(t+1) = rho*a_i(t) + beta*spike_i(t+1)`，
   `rho = exp(-dt/tau_adapt)`，`beta >= 0`；關閉時 `a ≡ 0`，前向與反向必須與未啟用完全
   相同（還原參考行為）。`tau_adapt`、`beta` 為設定，非可訓練。
7. **替代梯度（宣告）**：`kind = "fast_sigmoid"`，`psi(u) = 1 / (1 + scale*|u|)^2`，
   `u = v_cand - theta_eff`，`scale > 0` 為設定並寫入報告／快照。
   `d spike/d v_cand = psi(u)`、`d spike/d theta_eff = -psi(u)`。
   **重設不傳梯度**：`v_next = (1-spike)*v_cand + spike*v_reset` 的 `spike` 在此分支
   視為常數，`dv_next/dv_cand = 1-spike`；不應期步 `dv_next/dv_cand = 0`。
   梯度沿 `x`（突觸跡）、`a`（適應）與 `v` 三條狀態跨時間傳遞，時間窗語意與連續核心
   `Backward(window)` 相同。
8. **驗證判準**：硬 spike 前向只驗事件時序與狀態值（手算）；反向對照兩種參考：
   (a) 小模型上以宣告規則的獨立前向模式（tangent）實作算出替代梯度，與 `Backward`
   比到 `1e-12` 相對誤差，涵蓋放電、不應期與安靜步；(b) 套件內測試專用的「平滑模式」，
   前向改用 `sigma(u) = 0.5*(1 + scale*u/(1+scale*|u|))` 產生連續值 spike，其精確導數為
   `0.5*scale*psi(u)`（`scale = 2` 時等於宣告的 `psi`），且平滑模式的重設分支保留
   `d v_next/d spike = v_reset - v_cand`，因此 `Backward` 是該平滑模型的精確梯度，
   可用中心有限差分驗證整條鏈（延遲、突觸跡、適應、閾值參數化、時間窗）。產品硬模式
   維持第 7 點的重設不傳梯度。平滑模式只存在於測試（套件私有欄位），不是產品模式，
   也不得拿硬 spike 做有限差分。
9. **零／邊界**：零邊合法；空序列拒絕；非有限值拒絕且不留半次結果；`dt`、`tau_syn`、
   `tau_adapt`、`scale` 須為正；`theta_raw` 任意有限值；初始狀態只給電位，
   `x`、`a`、不應期計數從零開始。

## 第一階段契約（dynamics）

在 `dynamics` 套件新增 `lif.go` 與 `lif_test.go`，公開型別與方法名稱依既有慣例：

```go
type LIFConfig struct {
	Nodes           int     `json:"nodes"`
	Sources         []int   `json:"sources"`
	Targets         []int   `json:"targets"`
	Delays          []int   `json:"delays,omitempty"`
	DT              float64 `json:"dt"`
	TauSyn          float64 `json:"tau_syn"`
	ThetaMin        float64 `json:"theta_min"`
	ThetaMax        float64 `json:"theta_max"`
	VReset          float64 `json:"v_reset"`
	RefractorySteps int     `json:"refractory_steps"`
	Adaptation      LIFAdaptation `json:"adaptation"`
	Surrogate       LIFSurrogate  `json:"surrogate"`
}
type LIFAdaptation struct { Enabled bool; TauAdapt float64; Beta float64 }   // json: enabled, tau_adapt, beta
type LIFSurrogate  struct { Kind string; Scale float64 }                     // json: kind, scale；Kind 目前只接受 "fast_sigmoid"
type LIFParameters struct { Weights, Bias, LogTau, ThetaRaw []float64 }      // json: weights, bias, log_tau, theta_raw
type LIFGradient   struct { Weights, Bias, LogTau, ThetaRaw []float64; Inputs [][]float64; Initial []float64 }

func NewLIF(c LIFConfig) (*LIF, error)
func (m *LIF) Config() LIFConfig
func (m *LIF) Forward(ctx context.Context, p LIFParameters, initial []float64, inputs [][]float64) (*LIFTrace, error)
func (tr *LIFTrace) Outputs() [][]float64      // 每步後的突觸跡 x（讀出用）
func (tr *LIFTrace) Spikes() [][]float64       // 每步後的 0/1 事件
func (tr *LIFTrace) Voltages() [][]float64     // 每步後的膜電位（重設後）
func (tr *LIFTrace) FinalVoltage() []float64
func (m *LIF) Backward(ctx context.Context, tr *LIFTrace, upstream [][]float64, window int) (LIFGradient, error)
```

`upstream` 是損失對每步輸出 `x` 的梯度。Trace 擁有自己的緩衝，不與呼叫端共享；
`Config()`、`Outputs()` 等回傳副本。錯誤與取消不留半次狀態，與 `Continuous` 相同。
平滑模式以套件私有欄位或建構參數提供，只供 `_test.go` 使用。

## 第一階段驗收

- [x] 手算時序：三顆神經元固定邊與延遲，逐步核對 `v`、spike、`x`、不應期計數與適應值，
  含「達閾值放電並重設」「不應期內忽略輸入」「延遲邊讀到舊 `x`」「突觸跡衰減」。
- [x] 事件決定性：相同輸入兩次 `Forward` 位元相同；spike 只出現 0／1。
- [x] 替代梯度：2–3 神經元、3–4 步的小例子，`Backward` 與依宣告公式的獨立手算相等
  （含 `theta_raw` 的有界轉換導數、重設不傳梯度、不應期零梯度）。
- [x] 平滑模式有限差分：每組參數、輸入與初始電位的中心差分，相對誤差 `1e-4`，近零
  梯度看絕對誤差；覆蓋延遲、突觸跡、適應開／關、時間窗。
- [x] 適應關閉時前向／反向與未啟用逐位相同；零邊、空序列、非有限值、非法 `theta`／
  `v_reset`／`tau`／`scale`／負不應期／錯形狀皆拒絕；取消不留半次結果。
- [x] `go test`、`go test -race`、`go vet ./dynamics/` 通過；實作前先有失敗測試紀錄。

### 第一階段證據

Opus subagent 先寫 `dynamics/lif_test.go`（11 個測試、59 個子測試）取得紅燈，再實作
`dynamics/lif.go`；root 逐行審查前向／反向與契約一致。紅燈摘錄、突變測試與手算表見
[stage1-subagent-report.md](../../evidence/COR-03/stage1-subagent-report.md)，綠燈日誌見
[lif-dynamics-test.log](../../evidence/COR-03/lif-dynamics-test.log)、
[race](../../evidence/COR-03/lif-dynamics-race.log)、[vet](../../evidence/COR-03/lif-dynamics-vet.log)。
未驗證：只在呼叫前取消的情形；逐步取消檢查點是鏡射連續核心，未實際觸發。

## 第二階段契約（learning、experiment、examples）

第二階段在第一階段驗證並提交後開始，只改 `learning`、`experiment`／`examples` 與必要的
`checkpoint` 測試，不改 `dynamics`。

1. **核心選擇**：`learning.Config` 新增 `LIF *dynamics.LIFConfig` `json:"lif,omitempty"`。
   `Dynamics`（連續）與 `LIF` 二選一：`LIF != nil` 時 `Dynamics` 必須為零值，否則
   `NewNetwork` 回錯；兩者皆缺也回錯。節點數、輸入／讀出節點檢查改用共用的
   `topology` 取值（連續取 `Dynamics.Nodes`，LIF 取 `LIF.Nodes`）。內部以私有介面包住
   兩種核心（forward／backward／nodes／config），不公開新抽象。
2. **參數與梯度**：`learning.Parameters` 新增 `ThetaRaw []float64` `json:"theta_raw,omitempty"`；
   LIF 時長度須等於節點數，連續時必須為空（否則拒絕）。`learning.Gradient` 對應新增
   `ThetaRaw`。AdamW 攤平順序固定為 `weights, bias, log_tau, theta_raw, encoder, readout`；
   連續模型 `theta_raw` 長度為 0，既有快照與最佳化器狀態位置不變。
3. **遮罩**：`Trainable` 新增 `Theta bool` `json:"theta,omitempty"`；連續模型設 `true`
   回錯。`DefaultOptions()` 保持不變（Theta 預設 false，LIF 使用者需明確開啟）。
4. **讀出與 Insyra 邊界**：LIF 的核心輸出是突觸跡 `x`，讀出與 `LossGradient` 的
   upstream 直接對 `x`；float32 邊界與現有路徑相同。
5. **快照**：`TrainingSnapshot` schema 維持 `coimnet-episode-training/v1`，新增欄位皆
   `omitempty`；舊檔不含新欄位仍可讀（`checkpoint` 的必填檢查對 `omitempty` 放行），
   LIF 快照可 Save／Load 往返並 `RestoreTrainer`。`NewIndividual`／`RestoreIndividual`
   遇到 LIF 設定回明確錯誤「LIF individuals are not supported yet」，不退回連續模型；
   個體持續狀態（電位、突觸跡、適應、不應期）另開票。
6. **範例與任務證據（COR-04）**：`experiment` 新增 `NewDelayedLIFTrainer(seed, rate,
   trainable Trainable)`，沿用既有五步延遲脈衝資料產生器，以三顆 LIF 神經元（含一條
   延遲邊）建立訓練器；`examples/lifthreshold` 提供可執行範例與測試：固定三組 seed，
   對照組為 (a) 全部可訓練、(b) 只訓練 `theta`、(c) 凍結全部，報告訓練前後保留資料
   MSE、`theta_base` 變化與放電率；驗收門檻事前寫死：(b) 的保留損失須低於 (c)，
   `theta_base` 至少一顆改變超過 `1e-3`，放電率在 (0,1) 內。門檻沒過回傳非零並輸出報告。
7. **驗收**：`go test`、`go test -race`、`go vet` 覆蓋 `learning`、`experiment`、
   `examples/lifthreshold`、`checkpoint`；既有連續模型的所有測試與 `scripts/verify.sh`
   維持通過；實作前先有失敗測試紀錄。

### 第二階段證據

Opus subagent 先寫 `learning/lif_test.go`、`learning/lif_flatten_test.go`、
`experiment/lif_delayed_test.go`、`checkpoint/lif_snapshot_test.go`、
`examples/lifthreshold/main_test.go` 取得紅燈（未定義欄位／函式），再實作
`learning/core.go`（私有 `coreModel` 介面與兩個 adapter）、`Config.LIF`、`ThetaRaw`、
`Trainable.Theta`、攤平順序、`Trainer.Spikes`、`experiment.NewDelayedLIFTrainer` 與
`examples/lifthreshold`。root 審查 diff 後接受以下偏離：

- `checkpoint/required.go` 加入指標欄位規則（`omitempty` 指標可省略或為 null，存在時
  依結構走訪），否則含 `Config.LIF` 指標的所有 episode 快照都無法讀回；連續快照序列化
  仍無 `lif`／`theta_raw`／`theta` 鍵，最佳化器 13 個位置順序不變。
- `learning/projections.go` 改用共用節點／邊數取值，並帶入 `ThetaRaw`。
- 範例的 LIF 設定與初始值由 subagent 重調（`theta_min=0.05`、`theta_max=1`、讀出神經元
  tonic bias 0.3、`theta_raw=-2`），因為契約建議值下閾值訓練反而讓保留損失變差；門檻
  未放寬。三個 seed 只擾動第一條邊權重，theta-only 結果相同，這證明可重現而非初始化
  變異，README 已註明。
- 「無 spike 時 `ThetaRaw` 梯度為零」不成立（替代梯度處處為正），改測「無讀出路徑的
  神經元梯度為零」。
- 全部 `go test ./...`、race（learning／experiment／checkpoint／examples）、vet、gofmt
  通過；`go run ./examples/lifthreshold` 三個 seed：theta-only 保留 MSE 0.2324→0.0522、
  `theta_base` 最大變化 0.496、放電率 0.198→0.091；all 0.0535～0.0541；frozen 不變。
  LIF 個體與 `Advance` 持續狀態尚未支援（明確錯誤）。

## 第三階段契約（CLI、文件、證據）

1. **CLI**：`coimnet examples list` 多列 `lif-threshold`（profile `fixture`），
   `coimnet examples run lif-threshold [--updates N]` 執行 `examples/lifthreshold` 的同一
   套流程（把流程放進可被 CLI 與範例 main 共用的套件函式，避免兩份邏輯），輸出報告
   JSON，門檻未過回非零；help、未知旗標、取消與輸出失敗有測試。
2. **文件**：README「Go SDK」加 `dynamics.NewLIF`／`learning.Config.LIF` 說明與範例命令；
   `docs/model-and-mechanisms.md` 的機制表把「LIF、適應性閾值」改為已實作（按類型混合
   仍待實作），寫明產品硬模式、替代梯度公式、重設不傳梯度、不應期語意與適應開關；
   `ENG.md` 共用決策加一條 LIF／替代梯度／theta 參數群組與快照相容規則。
3. **需求證據**：`evidence/COR-03/verification.json`（LIF 時序：手算、延遲、不應期、重設、
   突觸衰減）、`evidence/COR-09/verification.json`（替代梯度：前向硬事件與反向近似分開驗證、
   tangent 參考與平滑模式有限差分）、`evidence/COR-04/verification.json`（基礎閾值可訓練與
   短期適應：梯度、範圍、更新與任務影響；當時慢速穩定未實作，紀錄明寫為部分，該檔已由
   ticket 16 改寫成涵蓋三項機制的完整紀錄）。三者的
   `reproduction_command`、`environment`（新的 `scripts/verify.sh` 輸出目錄 doctor.json）、
   `input_fingerprints`（source.sha256）、`observed_result`、`test_log` 都指向實際檔案；
   `docs/requirements-status.json` 只把 COR-03、COR-09 標 `passed`；COR-04 在本票完成時因慢速穩定
   尚未實作而維持 `specified`，已完成的閾值訓練與短期適應部分只在 ticket 與 `evidence/COR-04/`
   記錄，不提前標通過。慢速穩定於 ticket 16 補齊後，COR-04 的三項機制到齊，狀態更新屬該票範圍。
4. **驗證**：`scripts/verify.sh` 新目錄全套通過（含 race），Windows／Linux 交叉編譯紀錄；
   `delivery-status.md` 更新 11 的狀態與下一步（LIF 個體狀態與快照、按類型混合）。

### 第三階段驗收

- [x] `coimnet examples list` 多一列 `lif-threshold`（profile `fixture`），`coimnet examples run
  lif-threshold [--updates N]` 輸出同一份報告 JSON，門檻未過回非零。
- [x] 範例流程移到 `experiment.RunLIFThreshold`，`examples/lifthreshold` 只留旗標解析、
  列印與退出狀態，CLI 與範例 main 共用同一份實作，沒有第二套邏輯。
- [x] CLI 測試涵蓋清單項目、小預算通過門檻的可解析報告、未過門檻仍輸出報告、非法預算、
  未知旗標、多餘參數、取消與 help 寫入失敗；實作前先有紅燈紀錄。
- [x] README 補 `dynamics.NewLIF`、`learning.Config.LIF`／`Trainable.Theta` 與新命令；
  `docs/model-and-mechanisms.md` 的機制表拆成已實作與待實作兩列；`ENG.md` 共用決策新增
  一條 LIF、替代梯度、theta 群組與快照相容規則。
- [x] `evidence/COR-03/`、`evidence/COR-09/`、`evidence/COR-04/` 各有 verification.json，
  指向新的 `scripts/verify.sh` 輸出目錄與實際日誌；`docs/requirements-status.json` 只把
  COR-03、COR-09 標為 passed。
- [x] `scripts/verify.sh evidence/cpu-reference-20260914/macos-v26` 全套通過（含 race），
  Windows／Linux 交叉編譯通過。
- [ ] Ubuntu 與 Windows 實機重跑本階段：本輪只有 macOS arm64 實際執行。

### 第三階段證據

Root 先寫失敗測試：`experiment/lif_threshold_test.go` 與 `internal/cli/run_test.go` 的新測試
先取得建置紅燈（`undefined: RunLIFThreshold`、`undefined: experiment.LIFThresholdGateDescription`），
移入 `experiment.RunLIFThreshold` 後 CLI 測試仍紅（`examples list has no lif-threshold entry`、
`failing run did not print an honest report`），再加上 `internal/cli/run.go` 的命令才轉綠。

`scripts/verify.sh evidence/cpu-reference-20260914/macos-v26` 的 build、`go test`、
`go test -race`、`go vet`、`go mod verify`、doctor 與既有 CLI 續訓比對全部通過
（[validation.log](../../evidence/cpu-reference-20260914/macos-v26/validation.log)、
[doctor.json](../../evidence/cpu-reference-20260914/macos-v26/doctor.json)、
[source.sha256](../../evidence/cpu-reference-20260914/macos-v26/source.sha256)，依慣例已移除目錄內複製的
`coimnet` 執行檔）。`GOOS=windows GOARCH=amd64` 與 `GOOS=linux GOARCH=amd64` 的 `go build ./...` 通過。

`./bin/coimnet examples run lif-threshold` 的 300 更新報告存為
[lifthreshold-report.json](../../evidence/COR-04/lifthreshold-report.json)：三組 seed 的
theta-only 保留 MSE 皆 0.232354 → 0.052194，frozen 維持 0.232354，all 為 0.053478／0.054142／
0.053964，`theta_base` 由 0.163243 移到 0.551178／0.651735／0.659591（最大變化 0.496348），
放電率 0.197917 → 0.090625，三項門檻全過。

需求證據：[COR-03](../../evidence/COR-03/verification.json)（手算時序、延遲、不應期、重設、
突觸衰減）、[COR-09](../../evidence/COR-09/verification.json)（前向硬事件與反向近似分開驗證，
tangent 參考 1e-12、平滑模式中心差分，硬放電未做有限差分）、
[COR-04](../../evidence/COR-04/verification.json)（基礎閾值可訓練與短期適應開關的完成部分）。
本票完成時 COR-04 因慢速穩定未實作而在 `docs/requirements-status.json` 維持 `specified`，完成部分
只記錄在本票與 `evidence/COR-04/`。

**2026-09-15 更新**：慢速穩定（homeostasis）已在 [ticket 16](16-lif-individual-and-model-package.md)
實作並驗證，`evidence/COR-04/verification.json` 改寫為完整紀錄，保留本票的閾值訓練與短期適應
證據，並補上慢速穩定的規則、手算時序表位置、收斂數字、關閉逐位相同與既有指紋不變的證明。
COR-04 的三項機制（可訓練基礎閾值、短期適應、慢速穩定）到此到齊，且可各自獨立開關；反向把
適應與慢速穩定都視為常數。`docs/requirements-status.json` 的狀態更新不在 ticket 16 第二階段的
檔案範圍內，是唯一剩下的登記步驟。

## 依據

- [主規格 8.2、8.3、9.3–9.5、10.2](../handoff/CoImNet_Implementation_Plan.zh-TW.md#chapter-08)。
- 替代梯度來源 S08（Neftci、Mostafa、Zenke 2019）、適應性閾值參考 S09（Bellec 等 2018）；
  重設不傳梯度為工程選擇，不宣稱生物定律。
- [共用工程設計](../../ENG.md)：固定邊順序、同步更新、錯誤不留半次更新。
