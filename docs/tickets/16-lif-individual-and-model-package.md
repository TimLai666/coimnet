# 16 — 研究者可以以 LIF 個體持續運作、啟用慢速穩定，並分別匯出三種保存物

Epic：數值參考核心與狀態保存

User Story：研究者可以用 LIF 核心建立持續個體（電位、突觸跡、適應、不應期、慢速穩定狀態一起
保存與接續），開關慢速活動穩定機制，並把模型包、個體快照與訓練快照分開匯出，只有權重的檔案
不會被當成完整恢復。

Blocked by：11 LIF 核心、12 LIF 持續狀態、05 保存

Status：verified（兩個階段皆已驗證。第二階段完成模型包、三種保存物的互相拒絕、由模型包建立個體、跨程序精確接續、COR-04／STA-01 證據與文件，見「第二階段證據」）

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

- [x] 第一階段：慢速穩定手算時序（含上限與回落）、關閉逐位相同、`Forward`／`Advance` 一致、
  狀態往返與非法狀態拒絕、既有 hash 不變；LIF 個體 `Advance` 等於 `Forward`、快照往返、
  舊連續個體檔仍可讀、`TrainEpisode` 更新 theta；`go test`、race、vet。
- [x] 第二階段：模型包保存讀回、篡改拒絕、模型包當個體讀取被拒（訊息明確）、由模型包啟動的
  個體與直接建立相同；新程序精確接續（subprocess）；`evidence/COR-04/`、`evidence/STA-01/`；
  ticket 11 的 COR-04 備註更新；README、ENG、機制文件。`delivery-status.md` 與
  `docs/requirements-status.json` 不在本階段的檔案範圍，未動。

### 第一階段證據（2026-09-15）

先寫失敗測試：`evidence/COR-04/red-dynamics.log`（`undefined: LIFHomeostasis` 等 11 個建置錯誤）、
`evidence/COR-04/red-learning.log`（`s.Neural.Continuous undefined`、`undefined:
learning.NeuralCoreContinuous`）。`checkpoint` 是例外：實作寫在測試之前，
`evidence/COR-04/red-checkpoint.log` 是把新測試放回 8187315 版 `checkpoint/individual.go` 補跑的
結果（8 個測試紅燈，含舊檔 fixture 讀不回），只能當鑑別力證明，不是第一次紅燈。

**手算時序**（`dynamics.TestLIFHomeostasisHandCalculatedTiming`）。兩顆神經元，`dt = ln 2`、
`tau = tau_syn = 1` 使 `lambda = alpha = kappa = 0.5`，`theta_raw = 0` 使 `theta_base = 1`；
`dt/tau_rate = 0.5`、`eta*dt = 1`、`target_rate = 0.25`、`h_max = 0.4`；不應期 0、適應關閉。
邊為 0→1、延遲 0、權重 2.5。`theta_eff` 用的是「上一步結束時的 `h`」。

節點 0（輸入 4,4,4,4,0,0,0,0）：

| 步 | v_cand | theta_eff | spike | v | x | r | h |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | 2 | 1 | 1 | −1 | 1 | 0.5 | 0.25 |
| 1 | 1.5 | 1.25 | 1 | −1 | 1.5 | 0.75 | 0.4（觸上限） |
| 2 | 1.5 | 1.4 | 1 | −1 | 1.75 | 0.875 | 0.4 |
| 3 | 1.5 | 1.4 | 1 | −1 | 1.875 | 0.9375 | 0.4 |
| 4 | −0.5 | 1.4 | 0 | −0.5 | 0.9375 | 0.46875 | 0.4 |
| 5 | −0.25 | 1.4 | 0 | −0.25 | 0.46875 | 0.234375 | 0.384375（開始回落） |
| 6 | −0.125 | 1.384375 | 0 | −0.125 | 0.234375 | 0.1171875 | 0.2515625 |
| 7 | −0.0625 | 1.2515625 | 0 | −0.0625 | 0.1171875 | 0.05859375 | 0.06015625 |

節點 1（drive = 2.5 × 節點 0 的 x）：

| 步 | drive | v_cand | theta_eff | spike | v | x | r | h |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0（觸下限） |
| 1 | 2.5 | 1.25 | 1 | 1 | −1 | 1 | 0.5 | 0.25 |
| 2 | 3.75 | 1.375 | 1.25 | 1 | −1 | 1.5 | 0.75 | 0.4（觸上限） |
| 3 | 4.375 | 1.6875 | 1.4 | 1 | −1 | 1.75 | 0.875 | 0.4 |
| 4 | 4.6875 | 1.84375 | 1.4 | 1 | −1 | 1.875 | 0.9375 | 0.4 |
| 5 | 2.34375 | 0.671875 | 1.4 | 0 | 0.671875 | 0.9375 | 0.46875 | 0.4 |
| 6 | 1.171875 | 0.921875 | 1.4 | 0 | 0.921875 | 0.46875 | 0.234375 | 0.384375 |
| 7 | 0.5859375 | 0.75390625 | 1.384375 | 0 | 0.75390625 | 0.234375 | 0.1171875 | 0.2515625 |

每個放電判定的餘裕都在 0.1 以上，沒有恰好相等的邊界，所以 `lambda` 與 `eta*dt` 的浮點誤差
（約 1e-16）不會翻轉任何事件；數值以相對誤差 1e-12 比對。三個突變都被這批測試抓到：
`r` 少掉衰減項、`h` 不加進 `theta_eff`、上限 clamp 拿掉。

**收斂證據**（`dynamics.TestLIFHomeostasisConvergesTowardTargetRate`，
[homeostasis-convergence.json](../../evidence/COR-04/homeostasis-convergence.json)、
[log](../../evidence/COR-04/homeostasis-convergence.log)）。一顆無邊神經元，固定輸入 2，
`dt = 1`、`tau = tau_syn = 1`、`theta_base = 1`、`v_reset = −0.5`、不應期 0；
`tau_rate = 50`、`target_rate = 0.2`、`eta = 0.05`、`h_max = 5`，跑 4,000 步、每 100 步記一次。
關閉時每一步都放電，放電比例正好 1.0，`r` 與 `h` 全程為零（陣列根本不存在）。
開啟時 `r` 由 0.4037（第 100 步）降到 0.1619（第 200 步）、0.2409（第 300 步），第 900 步起
停在 0.19992，最終 `r = 0.19991839399434197`（與目標差 8.16e−05，門檻 0.05），`h` 收在
0.9584081253769056，整段放電比例 0.207。同一組參數先用獨立的 Python 模擬算過，數值相同。
重現指令寫在 JSON 的 `reproduction_command`；證據檔只在
`COIMNET_COR04_EVIDENCE` 指向絕對路徑目錄時才寫出，平常跑測試不會動到工作目錄。

**既有 hash 不變**（[protocol-hash-unchanged.log](../../evidence/COR-04/protocol-hash-unchanged.log)）。
用工作樹的程式重算 `evidence/NAT-01/protocol-fullgraph-uniform.json` 的 protocol hash，得到
`befa9f1d340a49d8dbeed50a67ca72d711bc601710764d22cdfbef353ea6ce42`，與 NAT-01 紀錄相同。
`dynamics.TestLIFConfigJSONIsUnchangedWithoutHomeostasis` 另外把 NAT-01 的 LIF 區塊與一份有拓撲的
設定的 canonical JSON 逐位元釘住，並比對欄位出現前量到的摘要
`8a64827606764d7f9f36dab81bed2c003fee2e30db54dbc2cac7b699f9088172`；指標為 nil 時不輸出鍵，
所以沒有宣告該機制的設定（含全腦那一份）編碼完全不變。**未重測**：NAT-01 的
`core_config_hash 61e0e242…` 需要 25,563,197 條邊的 store 與約 90 秒的全圖執行，本階段沒有重跑，
只以編碼推論。

**LIF 個體**。`learning.TestLIFIndividualAdvanceMatchesForward` 以兩段 `Advance` 對上單次
`dynamics.LIF.Forward`（慢速穩定開與關各一組），並在快照往返、續跑後再比一次更長的
`Forward`；`TestLIFIndividualSnapshotOwnsItsBuffersAndRejectsMismatches` 蓋住聯集的六種不一致；
`TestLIFIndividualTrainsThresholdAndResetsNeural` 用 theta-only 最佳化器跑 20 步，`theta_raw` 改變、
其他參數群組與持續狀態不變，`ResetNeural` 只重設神經狀態。
`checkpoint.TestLoadIndividualReadsPreUnionContinuousCheckpoint` 讀
[testdata/continuous-individual-pre-union.json](../../checkpoint/testdata/continuous-individual-pre-union.json)，
那份檔案是用 `git archive HEAD` 匯出 8187315 的原始碼、在暫存目錄編譯後產生的舊形狀檔
（`neural` 直接是連續 `State`），載入後與現在重跑同一條軌跡的個體逐項相等。
`TestLIFIndividualCheckpointSubprocessResume` 在新程序讀回 LIF 個體續跑，與不中斷執行逐項相同。

**驗證指令與結果**（macOS arm64、go1.26.5、load average 1.50）：`gofmt -l .` 無輸出；
`go vet ./...` 通過；`go test -count=1 ./...` 20 個套件全過
（[test.log](../../evidence/COR-04/test.log)）；
`go test -race -count=1 ./dynamics/ ./learning/ ./checkpoint/ ./simulate/` 全過
（[race.log](../../evidence/COR-04/race.log)）。

**偏離與限制**：

- `coreModel` 加的是 `profile`／`newState`／`validateState`／`advance`，並移除 `continuous()`；
  票面的 `resetState` 沒有另開，`ResetNeural` 直接用 `newState`，行為相同。
- `TrainEpisode` 更新 theta 的測試用 `learning` 自己的 LIF fixture（ticket 11 第二階段的五步延遲脈衝
  加一條延遲邊），沒有改用 `experiment.NewDelayedLIFTrainer`，避免 `learning` 的測試反向依賴
  `experiment`。
- `checkpoint/required.go` 沒有改：既有的反射走訪本來就處理 `omitempty` 指標，聯集規則寫在
  `checkpoint/individual.go` 的 `requireIndividualNeural`，並呼叫同一個走訪檢查
  `config.lif`、`neural.continuous`、`neural.lif`。聯集的六種不一致多半在
  `decodeStrict` 與 `RestoreIndividual` 就被擋下，只有「最長延遲為零的模型漏掉 `steps`」這一項
  非靠必填檢查不可（已加成測試案例並以突變確認）。
- 宣告但關閉的 homeostasis 區塊不驗證數值（比照既有的關閉適應區塊），但它會改變設定指紋，
  因為那是一份不同的宣告。
- 建構檢查實際擋的是 `theta_min + h_max` 非有限；`v_reset < theta_min` 已經是既有條件，
  而 `h >= 0` 只會抬高閾值，所以 `v_reset < theta_eff` 恆成立。
- `LIFState` 的 `Rate`／`Homeostasis` 放在 `Refractory` 之後，舊文件是新編碼的前綴。
- README、ENG.md、`docs/model-and-mechanisms.md`、`docs/individual-state.md` 與
  `delivery-status.md` 仍寫著「LIF 個體尚未支援」，`docs/requirements-status.json` 的 COR-04 仍是
  `specified`；這些屬第二階段，本輪未動。（第二階段已更正前四份文件，`delivery-status.md` 與
  `docs/requirements-status.json` 不在其檔案範圍，仍待處理。）
- 只在 macOS arm64 實測；`evidence/COR-07/` 是同時段另一位 agent 產生的未追蹤目錄，未動。

### 第二階段證據（2026-09-15）

先寫失敗測試：`evidence/COR-04/red-package.log`。整份 `checkpoint/package_test.go` 在
`checkpoint/package.go` 存在以前就寫完並執行，輸出是 11 個 `undefined:`（`Units`、`ModelPackage`、
`NewModelPackage`、`ModelPackageSchemaVersion`、`CompatibleVersions`、`SaveModelPackage`、
`LoadModelPackage` 等）後編譯器放棄，`FAIL ... [build failed]`。

**模型包**（`checkpoint/package.go`）。`ModelPackage`（`coimnet-model-package/v1`）帶
`TopologyFingerprint{Nodes, Edges, SHA256}`、`learning.Config`、`learning.Parameters`、
`Units{TimeStep, TimeConstant}`、`EvidenceRegistry []string` 與
`CompatibleVersions{Individual, Training}`。`SaveModelPackage`／`LoadModelPackage` 沿用個體快照
那一套 envelope、SHA-256 payload checksum、`checkUniqueJSONRejectNull`、必填走訪、嚴格解碼、
同目錄暫存檔加排他 hardlink 與目錄同步。載入時以 `learning.NewTrainer` 重建整個模型（建核心、
檢查每組參數形狀、跑一次預測）並重算拓撲指紋，指紋與設定不符就拒絕。

**拓撲指紋的編碼**。sources 全部再 targets 全部、各以 little-endian uint32 編碼後取 SHA-256，
與 `simulate/nullmodel.go` 的 `topologyHash` 同一種編碼，但在 `checkpoint` 私下重寫，避免
`checkpoint` 依賴 `simulate`。一條邊 `0→1` 的指紋是
`01acecb507abfe1a354aa8064f4af5d3f1acd019e37db3c11c97523b71c76e9d`，就是 8 個位元組
`00 00 00 00 01 00 00 00` 的 SHA-256（在 Go 以外另算過）；測試對每個 fixture 另用一個最直白的
編碼器重算一次。同一組邊換順序指紋就不同，所以它認的是保存下來的邊陣列，不是抽象的圖。

**六種交叉載入全部拒絕**（`TestArtefactKindsRefuseEachOther`），實際訊息：

| 讀法 | 檔案 | 訊息 |
| --- | --- | --- |
| `LoadIndividual` | 模型包 | `"coimnet-model-package/v1" is a model package, which declares configuration and parameters only and is not an individual snapshot: seed a new individual with NewIndividualFromPackage instead of restoring one` |
| `LoadIndividual` | 訓練快照 | `"coimnet-episode-checkpoint/v1" is an episode training checkpoint, which carries no persistent neural state, and is not an individual snapshot` |
| `LoadModelPackage` | 個體快照 | `"coimnet-individual-checkpoint/v1" is an individual snapshot, which carries persistent neural state and optimizer moments, not a model package` |
| `LoadModelPackage` | 訓練快照 | `"coimnet-episode-checkpoint/v1" is an episode training checkpoint, which carries trainer state and a data cursor, not a model package` |
| `Load` | 模型包 | `unsupported checkpoint schema "coimnet-model-package/v1"` |
| `Load` | 個體快照 | `unsupported checkpoint schema "coimnet-individual-checkpoint/v1"` |

**往返與拒絕**。連續與 LIF 兩種模型各驗一次：保存後讀回 `reflect.DeepEqual` 相同，同一份包存兩次
位元組相同，把讀回的再存一次還是同樣的位元組，發布的文件裡沒有 `null`，改動讀回的值不影響下一次
載入。載入拒絕 23 種情況：翻轉 payload 位元組、歸零 checksum、過短 checksum、截斷、envelope 版本
超前、payload 版本超前、指紋摘要被改、指紋邊數被改、指紋節點數被改、缺 `units`、缺
`time_constant`、空的 `time_step`、缺 `evidence_registry`、缺 `compatible_versions`、宣告不認識的
individual schema、絕對路徑的證據、含 `..` 的證據、陣列裡的 `null`、缺 `parameters`、參數形狀與設定
不符、未知欄位、重複鍵、尾端多餘資料；同一個測試也確認未篡改的 fixture 讀得回來，所以這些拒絕不是
空的。取消的保存不會留下檔案或暫存檔，同一路徑第二次保存失敗且不改動已發布的位元組，空路徑與零值
包被拒絕。

**由模型包建立個體**（`TestNewIndividualFromPackageEqualsNewIndividual`）。
`NewIndividualFromPackage(pkg, options, initial)` 的快照與用同一份設定、參數、選項、初始電位呼叫
`learning.NewIndividual` 的快照 `reflect.DeepEqual` 相同，接著同一段輸入的 `Advance` 輸出與推進後的
狀態也都相同；連續模型與開啟慢速穩定的 LIF 模型各一組。建立之後改動模型包的陣列不影響已建立的個體。

**跨程序精確接續**（`TestLIFIndividualFromPackageSubprocessResume`）。由保存的模型包建立 LIF 個體，
推進四步、開啟 `Trainable.Theta` 訓練兩個 episode，保存快照；真正的新程序（exec 測試執行檔，以
`COIMNET_MODEL_PACKAGE_HELPER` 分流）讀回快照、跑完五步的尾段再保存。接續後的快照與不中斷執行
逐項相同：神經狀態（含活動估計與閾值偏移）、參數、Adam 一階與二階動量、每個參數的步數、更新次數，
尾段讀出也逐值相同。測試同時擋掉空洞比較：更新次數必須是 2、訓練必須真的改變參數、至少一個 Adam
動量不為零、接續後的閾值偏移不為零（實測 `[0.3748046875, 0.4, 0.0445312500000001]`，第 1 顆貼在宣告的
`h_max = 0.4`）、比較的視窗裡必須有放電。

**突變檢查**。把 `canonicalModelPackage` 的指紋重算關掉，三個測試立刻紅：被改摘要的篡改案例被接受、
`SaveModelPackage` 接受與設定不符的指紋、`NewIndividualFromPackage` 由這種包建立個體。把
`individualKindError` 換回原本的 `unsupported individual checkpoint schema`，交叉載入測試在模型包那一
項失敗。

**驗證指令與結果**（macOS arm64、go1.26.5、Insyra v0.3.2，load average 執行前 2.09、執行後 2.20）：
`gofmt -l .` 無輸出；`go vet ./...` 通過；`go test -count=1 ./...` 21 個套件全過；
`go test -race -count=1 ./checkpoint/ ./learning/ ./dynamics/` 全過。日誌見
[verification.log](../../evidence/STA-01/verification.log)。

**偏離與限制**：

- 票面第二階段的契約沒有列建構函式，但 `SaveModelPackage` 需要一份已經算好指紋的包，而指紋的編碼是
  套件私有的，呼叫者算不出來。因此另加 `NewModelPackage(config, parameters, units, evidence)`，
  由它驗證並算好指紋；`SaveModelPackage` 仍會重算一次並比對，所以「指紋與設定不符」還是可以測。
- `NewIndividualFromPackage` 的簽章比票面多一個 `learning.Options`。沒有它就得在 `checkpoint` 裡預設
  一組最佳化器設定，而那是呼叫者的決定；票面第二階段的契約本來也寫成收 options 的形式。
- 三種交叉載入裡，`Load`（episode）那兩種是靠 `checkpoint/checkpoint.go` 既有的版本檢查拒絕，訊息用
  schema 字串指名讀到的是哪一種，不是一句完整說明。`checkpoint.go` 不在本階段的檔案範圍，沒有改。
- `checkpoint/individual.go` 只動了 12 行：把 envelope 的嚴格解碼與版本判定移到必填走訪之前，並改叫
  新的 `individualKindError`。這樣讀錯檔案的人拿到的是「這是哪一種保存物」，而不是一份本來就不是個體
  快照的文件缺了哪個欄位。既有的拒絕案例全部仍然拒絕。
- 拓撲指紋與 `simulate` 的 `topologyHash` 同編碼這件事，是靠文件寫明的規則加上測試裡的獨立重算，不是
  呼叫 `simulate` 的私有函式。兩邊的實作有可能各自漂移而不被測試發現。
- `EvidenceRegistry` 的路徑原樣保存，只檢查是相對路徑、沒有反斜線、沒有 `..` 片段；框架不會開啟它們，
  也不驗證那些紀錄存在或真的描述這個模型。
- 單位字串只是宣告，框架不做任何換算，也不代表毫秒或任何量測到的生物時間尺度。
- 全部是 fixture：兩顆神經元的連續模型與三顆神經元的 LIF 模型，沒有用真實接線圖建過模型包。
- STA-03 維持 specified。訓練快照的「完整」只涵蓋訓練器實際擁有的東西（參數、最佳化器選項、Adam 動量、
  每個參數的步數、更新次數，episode 格式另有資料 seed 與樣本游標）。重播庫、教師訊號、快速權重、化學
  狀態與亂數流在整個框架都還不存在。
- `delivery-status.md` 與 `docs/requirements-status.json` 不在本階段的檔案範圍，未動；COR-04 與 STA-01
  的狀態登記是剩下的步驟。
- 記錄那次全綠的執行是在工作樹的隔離副本上跑的：同一時間另一位 agent 正在改 `learning/`（ticket 17），
  他未完成的檔案當下編譯不過。副本移除了他的未追蹤檔案並把 `learning/` 還原到 32a72fd，其餘與工作樹
  相同。ticket 16 的 21 個測試在工作樹本身也全過，工作樹當下唯一的失敗是那位 agent 自己尚未實作的紅燈
  測試。共存狀態記在 [concurrent-tree.log](../../evidence/STA-01/concurrent-tree.log)。
- 只在 macOS arm64 實測。

### root 審查（2026-09-15，第一階段）

逐檔讀過 dynamics 的慢速穩定實作、狀態聯集、checkpoint 的舊檔升級與必填規則；手算表中節點 0 的
`h` 序列由 root 依票面公式獨立重算（0.25、0.4 觸頂三步、0.4、0.384375、0.2515625、0.06015625）一致；
指標加 `omitempty` 的編碼推論成立，NAT-01 protocol hash 重算相同。root 重跑 gofmt／vet／
`go test ./...`／race 全數通過。接受第一階段的偏離（`resetState` 由 `newState` 取代、checkpoint 的
紅燈屬事後鑑別）。第二階段另派。

## 依據

- ticket 11（COR-04 未完成部分）、ticket 12 探索紀錄（Individual 綁定連續核心的清單）、
  ticket 05（STA-01／STA-03 待完成）、主規格 8.2、15.1–15.4。
