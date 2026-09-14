# 16 — 研究者可以以 LIF 個體持續運作、啟用慢速穩定，並分別匯出三種保存物

Epic：數值參考核心與狀態保存

User Story：研究者可以用 LIF 核心建立持續個體（電位、突觸跡、適應、不應期、慢速穩定狀態一起
保存與接續），開關慢速活動穩定機制，並把模型包、個體快照與訓練快照分開匯出，只有權重的檔案
不會被當成完整恢復。

Blocked by：11 LIF 核心、12 LIF 持續狀態、05 保存

Status：ready（契約已於 2026-09-15 定案，分兩階段派工；驗收項目驗證後才勾選）

對應需求：COR-04（慢速穩定，補齊後才可標 passed）、STA-01（三種保存物）、STA-03 的一部分
（LIF 個體含最佳化器與資料游標的精確接續；快速權重與化學狀態尚不存在，STA-03 維持 specified
並註明範圍）。主規格 8.2（`theta_effective = bounded(theta_base + adaptation + homeostasis +
modulation)`）、15.1–15.4。

## Root 決策（2026-09-15）

1. **慢速穩定（homeostasis）定義**：每顆神經元一個活動估計 `r` 與一個閾值偏移 `h`：
   `r(t+1) = r(t) + (dt/tau_rate) * (spike(t+1) - r(t))`，
   `h(t+1) = clamp(h(t) + eta * dt * (r(t+1) - target_rate), 0, h_max)`，
   `theta_eff = theta_base + a + h`。`h` 只往上抬或回落到 0（「避免長期過強或沉默的受限調整」，
   主規格 S16 用語），`h_max` 保證 `v_reset < theta_eff` 仍成立（建構時檢查
   `theta_min + h_max` 在合法範圍且 `v_reset < theta_min`）。`r`、`h` 是狀態，不是可訓練參數，
   反向把 `h` 視為常數（與適應相同），文件寫明。關閉時 `r ≡ h ≡ 0`，前向與反向必須與未啟用逐位相同。
   設定：`LIFHomeostasis{Enabled bool; TauRate, TargetRate, Eta, HMax float64}`，
   `LIFConfig.Homeostasis *LIFHomeostasis json:"homeostasis,omitempty"`（指標加 omitempty，
   既有 `core_config_hash`、`protocol_hash` 與 NAT-01／02／03／05 的紀錄 hash 不變，有測試）。
   `LIFState` 加 `Rate []float64 json:"rate,omitempty"` 與 `Homeostasis []float64
   json:"homeostasis,omitempty"`，未啟用時為 nil 且 `ValidateState` 要求 nil；schema 維持
   `coimnet-lif-state/v1`（欄位可省略即向後相容）。
2. **LIF 個體**：`learning.Individual` 改為兩種 profile：既有
   `continuous-f64-insyra-f32-persistent-inference-episode-learning/v1` 不變；新增
   `lif-f64-insyra-f32-persistent-inference-episode-learning/v1`。`IndividualSnapshot.Neural` 改為
   聯集 `NeuralState{Core string; Continuous *dynamics.State; LIF *dynamics.LIFState}`（JSON
   `{"core":"continuous","continuous":{...}}` 或 `{"core":"lif","lif":{...}}`）；既有連續個體檔案
   （`coimnet-individual-checkpoint/v1`）維持可讀：`checkpoint/individual.go` 遇到舊形狀
   （`neural` 直接是連續 State）時視為 `core: continuous`，並在測試以舊檔 fixture 證明。
   `coreModel` 介面加 `newState`／`advance`／`validateState`／`resetState`，`NewIndividual`／
   `RestoreIndividual`／`Advance`／`TrainEpisode`／`ResetNeural` 對兩種核心走同一條路徑；
   LIF 個體的 `Advance` 逐位等於 `dynamics.LIF.Forward` 同輸入的輸出（含慢速穩定開啟時）。
3. **三種保存物（STA-01）**：新增 `checkpoint.ModelPackage`（schema `coimnet-model-package/v1`）：
   `TopologyFingerprint`（節點數、邊數、sources／targets 的 SHA-256，與 `simulate` 的 topology
   hash 同編碼）、`Config`（`learning.Config`）、`Parameters`、`Units`（dt 單位與時間常數單位的
   宣告字串）、`EvidenceRegistry []string`（來源證據路徑）、`CompatibleVersions`（可讀的
   individual／training schema 版本）。`SaveModelPackage`／`LoadModelPackage` 沿用 envelope、
   checksum、hardlink 不覆寫。`NewIndividualFromPackage(pkg, initial)` 由模型包啟動新個體；
   `checkpoint.LoadIndividual` 讀到模型包檔必須明確拒絕「只有權重／設定的模型包不是個體快照，
   不能當完整恢復」，反之亦然；訓練快照（episode）維持既有格式。
4. **精確接續（STA-03 的可做部分）**：LIF 個體的 `TrainEpisode` 之後 `Snapshot` → 新程序
   `RestoreIndividual` → 續跑，與連續執行逐項相等（神經狀態、參數、Adam 動量、更新計數、
   資料 seed 與游標）。快速權重與化學狀態尚未存在，STA-03 註明此範圍，不標 passed。
5. **證據**：COR-04 需要「慢速穩定確實影響活動」的證據：fixture 上一顆被持續驅動的神經元，
   開啟 homeostasis 後放電率向 `target_rate` 收斂（報告收斂曲線與最終偏差），關閉時不變；
   `evidence/COR-04/`、`evidence/STA-01/`。

## 契約（第一階段：dynamics 與 learning）

```go
type LIFHomeostasis struct { Enabled bool `json:"enabled"`; TauRate, TargetRate, Eta, HMax float64 }
// LIFConfig.Homeostasis *LIFHomeostasis `json:"homeostasis,omitempty"`
// LIFState.Rate, LIFState.Homeostasis []float64 (omitempty)
const IndividualProfileLIF = "lif-f64-insyra-f32-persistent-inference-episode-learning/v1"
type NeuralState struct { Core string; Continuous *dynamics.State; LIF *dynamics.LIFState }
func NewIndividual(c Config, p Parameters, o Options, initial []float64) (*Individual, error)   // LIF 設定不再回錯
```

## 契約（第二階段：checkpoint 與模型包）

```go
const ModelPackageSchemaVersion = "coimnet-model-package/v1"
type ModelPackage struct { SchemaVersion string; Topology TopologyFingerprint; Config learning.Config; Parameters learning.Parameters; Units Units; EvidenceRegistry []string; CompatibleVersions CompatibleVersions }
func SaveModelPackage(ctx, path string, pkg ModelPackage) error
func LoadModelPackage(ctx, path string) (ModelPackage, error)
func NewIndividualFromPackage(pkg ModelPackage, o learning.Options, initial []float64) (*learning.Individual, error)
```

## 驗收

- [ ] 第一階段：慢速穩定手算時序（含上限與回落）、關閉逐位相同、`Forward`／`Advance` 一致、
  狀態往返與非法狀態拒絕、既有 hash 不變；LIF 個體 `Advance` 等於 `Forward`、快照往返、
  舊連續個體檔仍可讀、`TrainEpisode` 更新 theta；`go test`、race、vet。
- [ ] 第二階段：模型包保存讀回、篡改拒絕、模型包當個體讀取被拒（訊息明確）、由模型包啟動的
  個體與直接建立相同；新程序精確接續（subprocess）；`evidence/COR-04/`、`evidence/STA-01/`；
  ticket 11 的 COR-04 備註更新；README、ENG、機制文件、delivery-status。

## 依據

- ticket 11（COR-04 未完成部分）、ticket 12 探索紀錄（Individual 綁定連續核心的清單）、
  ticket 05（STA-01／STA-03 待完成）、主規格 8.2、15.1–15.4。
