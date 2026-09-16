# 20 — 研究者可以把觀察、目標與事後回饋分開，並選擇調節訊號的來源

Epic：訊號與時間、化學調節（第一層：來源）

User Story：研究者可以確認前向路徑與調節控制器只看得到觀察，目標只由訓練器持有，回饋依可取得
時間之後才進入；並能從外部時序、具名神經元活動、內在資源與記錄重播四種來源產生非負的調節釋放率，
把原始獎懲以明示規則映射成調節輸入而不改寫原始值。

Blocked by：03 訊號、12 runner、14 具名集合

Status：verified_scoped（2026-09-15 實作、2026-09-16 重跑驗證：`signal.AvailableFeedback`、`simulate.ResolvedSet.Nodes`、
`modulation` 四種來源與獎懲映射、SIG-03 的靜態檢查腳本與執行期汙染測試、三份證據。範圍限制：可訓練控制器
（MOD-07）只保留名字沒有實作；濃度動態屬 21，釋放率目前沒有下游；四種來源都還沒接上 runner 或訓練器；
只在 macOS arm64 的 fixture 上跑過）

對應需求：SIG-03（資料流測試證明目前答案不進入前向或調節控制器）、MOD-01（外部、神經、控制器、
身體或重播來源都有實際執行路徑；目前測試答案不流入來源）、MOD-08（原始回饋不可被模型修改；負回饋
不當成負濃度）。可訓練控制器（MOD-07）另票。主規格 6.3、6.5、11.1、11.2、11.3（非負）、11.6。

## Root 決策（2026-09-15）

1. **三分離型別**（`signal` 套件）：`Observation{Step uint64; Values []float64}`、`Target{Step;
   Values}`、`Feedback{ActionID uint64; ProducedAt, AvailableAt uint64; Source string; Score float64;
   ModelVersion string}`。前向與調節來源的公開函式只接受 `Observation`；`Target` 只進 `learning` 的
   損失函式；`Feedback` 進來源前先依 `AvailableAt <= 目前步` 過濾，早於可取得時間的回饋不得使用。
2. **資料流證明**：以「編譯期型別」為主（前向與來源函式的簽章沒有 `Target`），加一個執行期的
   汙染測試：把 `Target` 填成 NaN 標記值跑完整訓練器一步的前向與調節來源，前向輸出與來源輸出必須
   有限；另以 `go vet`-style 的靜態檢查腳本（`scripts/check-target-flow.sh`，用 `go list -deps` 與
   grep）確認 `dynamics`、`simulate`、`modulation` 不 import `Target`。測試模式（evaluate）不呼叫教師
   的規則在教師票落實，這裡先在 `Feedback.Source` 記錄來源字串。
3. **調節來源**（新套件 `modulation`，只做第一層「來源 → 非負釋放率 q[r,k]」，濃度動態屬 21）：
   `Source` 介面 `Release(step uint64, ctx SourceContext) ([]float64 /*每通道 q ≥ 0*/, error)`；四種實作：
   `ExternalTimeline`（宣告的 (step, channel, q) 序列，經合法性檢查：非負、有限、步遞增）、
   `NeuralActivity`（具名集合的平均輸出乘增益，14 的 `ResolvedSet`；不得自動假設未知神經元是調節
   細胞，集合必須明示）、`InternalResource`（由環境給的資源量與消耗規則產生 q，只在任務宣告有資源時
   啟用）、`Replay`（重播已記錄的釋放時序，禁止向任何外部取值）。可訓練控制器（MOD-07）留空介面位置但
   不實作。每種來源都有實際執行路徑與測試；來源的 `SourceContext` 只含觀察摘要與已可取得的回饋。
4. **獎懲映射（MOD-08）**：`RewardMapper{Baseline string ∈ none|running_mean(window); Kind ∈ relu|
   split}`：`delta = raw − expected`；`relu`：q = max(delta, 0)、負值轉為「清除率增加」的另一通道值
   （交給 21 的濃度方程作為額外清除項，本票先記錄成 `clearance_boost ≥ 0`）；`split`：正負各一通道。
   四個值分開保存：`raw`、`expected`、`transformed`、`applied`；`raw` 一律唯讀（測試證明模型路徑
   無法改它，來源只拿副本）。
5. **證據**：`evidence/SIG-03/`、`evidence/MOD-01/`、`evidence/MOD-08/`（fixture）。

## Root 修正（派工時）

2026-09-15 派工時 root 對上面的契約做了三項修正，實作以修正後的版本為準，下面的「契約」區塊保留原始草稿。

1. 三個型別已經存在於 `signal`（`Observation`、`Target`、`Feedback` 加 `FeedbackSpec`，私有欄位、建構子、
   存取器與 `Feedback.ValidateAvailableAt` 都已實作）。決策 1 因此改成「保留它們並證明分離」，不得再建一套；
   只新增 `func AvailableFeedback(all []Feedback, now Timestamp) ([]Feedback, error)`，要求同一時間單位、
   過濾 `AvailableAt <= now`、依輸入順序回傳副本、單位不符回錯誤。
2. `ResolvedSet` 的節點索引維持私有，改在 `simulate/metrics.go` 加讀取用的 `func (s ResolvedSet) Nodes() []int`
   （遞增、呼叫端自有的副本），讓 `modulation.NeuralActivity` 能對已解析的集合取平均而不必偽造一個。
   `simulate` 其他部分不動。
3. `SourceContext` 帶 `Observation *signal.Observation`（忽略觀察的來源可為 nil）、`Activity []float64`
   （當步的節點活動，由呼叫端提供，取不到時為 nil）、`Feedback []signal.Feedback`（已由 `AvailableFeedback`
   過濾，來源仍必須自己再檢查 `ValidateAvailableAt`，呼叫端傳入太早的回饋就報錯）、
   `Resources map[string]float64`。`modulation` 任何地方都沒有 `Target` 欄位，也不 import `learning`。

### 實作時另外定案的細節

- 釋放步號記在 model step 時鐘上（`modulation.ReleaseTimeUnit = signal.TimeUnitModelStep`）。來源比對的是
  「步號對上 `AvailableAt`」，所以進到來源的回饋必須是同一個時鐘；其他單位一律拒絕，不換算。
- `ExternalTimeline` 需要宣告 `ChannelCount`，`Channels()` 就是它。同一個 (step, channel) 宣告兩次視為
  宣告錯誤而拒絕，框架不自行決定要相加還是覆寫。
- `NeuralActivity` 與 `InternalResource` 的 `Channels()` 是 `Channel + 1`：釋放向量以通道索引，宣告通道
  以下的通道釋放 0。`InternalResource` 因此也有 `Channel` 欄位，與神經來源對稱。
- `RewardMapper` 的 `none` 必須把 `Window` 留成 0（避免出現一個永遠不會被讀的旋鈕），`running_mean` 的
  `Window` 至少為 1；歷史不足 `Window` 筆時取現有筆數的平均，第一筆沒有歷史，期望值為 0。
- 可訓練控制器只保留名字：`type Controller struct{}` 與 `NewController()` 回 `ErrControllerNotImplemented`，
  沒有 `Release`，所以它不是 `Source`。

## 契約

```go
package signal
type Observation struct { Step uint64; Values []float64 }
type Target struct { Step uint64; Values []float64 }
type Feedback struct { ActionID uint64; ProducedAt, AvailableAt uint64; Source string; Score float64; ModelVersion string }
func AvailableFeedback(all []Feedback, step uint64) []Feedback

package modulation
type SourceContext struct { Observation signal.Observation; Feedback []signal.Feedback; Resources map[string]float64 }
type Source interface { Channels() int; Release(step uint64, c SourceContext) ([]float64, error) }
type ExternalTimeline struct{...}; type NeuralActivity struct{ Set simulate.ResolvedSet; Gain float64 }; type InternalResource struct{...}; type Replay struct{ Trace [][]float64 }
type RewardMapper struct { Baseline string; Window int; Kind string }
type RewardRecord struct { Raw, Expected, Transformed float64; Applied []float64; ClearanceBoost float64 }
func (m *RewardMapper) Map(raw float64) (RewardRecord, error)
```

## 驗收

- [x] 型別分離與汙染測試、靜態 import 檢查腳本；早於 `AvailableAt` 的回饋被過濾（測試）。
- [x] 四種來源各有手算輸出、非負與有限檢查、重播不可外取（無網路介面）；神經來源必須明示集合。
- [x] 獎懲映射：四個值分開、raw 唯讀、負回饋不產生負值而是清除項；`go test`、race、vet；證據與文件。

## 證據

證據目錄：`evidence/SIG-03/`、`evidence/MOD-01/`、`evidence/MOD-08/`，各有 `verification.json` 與 `test.log`。

### 先寫失敗測試

| 紅燈日誌 | 當時的失敗內容 |
| --- | --- |
| `evidence/SIG-03/red-available.log` | `undefined: AvailableFeedback`，函式還不存在 |
| `evidence/MOD-01/red-nodes.log` | `ResolvedSet has no field or method Nodes`，存取器還不存在 |
| `evidence/MOD-01/red-sources.log` | `undefined: Source`／`SourceContext`／四個來源型別，套件只有 package 宣告 |
| `evidence/MOD-08/red-mapper.log` | `undefined: RewardMapper`，映射器還不存在 |
| `evidence/SIG-03/red-taint.log` | 汙染測試第一次執行：`learning` 當下是另一個 agent 改到一半的狀態，整包編譯失敗 |

汙染測試本身沒有真正的紅燈，因為它驗的是既有程式碼的性質（03 的三個型別加本票的來源），第一次跑只可能因為
不相干的原因失敗。改用兩份突變檢查證明它抓得到問題，兩次都當場還原：

- `evidence/SIG-03/red-taint-mutation.log`：把 `checkedRelease` 暫時換成 `rates[0] = math.NaN(); return rates, nil`，
  測試失敗於 `replay: channel 0 released NaN on a tainted episode`。
- `evidence/SIG-03/check-target-flow-violation.log`：暫時放一個 `modulation/zz_violation_probe.go` 宣告
  `var probeTarget signal.Target` 並 import `learning`，腳本印出違規行與違規相依並以 1 結束。

### 四種來源的手算表

`ExternalTimeline{ChannelCount: 2}`，宣告 (0,0,0.5)、(0,1,1.25)、(2,1,2)、(3,0,0)：

| step | 0 | 1 | 2 | 3 | 9 |
| --- | --- | --- | --- | --- | --- |
| 釋放 | [0.5, 1.25] | [0, 0] | [0, 2] | [0, 0] | [0, 0] |

`NeuralActivity{Set: alpn（節點 1、2）, Gain: 0.5, Channel: 1}`，節點 0 在集合外，數值再大也不影響：

| activity | 集合平均 | ×Gain | 釋放 |
| --- | --- | --- | --- |
| [10, 0.25, 0.75] | 0.5 | 0.25 | [0, 0.25] |
| [10, -1, -3] | -2 | -1 | [0, 0]（夾在 0） |
| [10, 0, 4] | 2 | 1 | [0, 1] |

`InternalResource{Resource: "energy", Coefficient: 2, Threshold: 0.25}`，規則 `2 × max(energy − 0.25, 0)`：

| energy | 1.25 | 0.75 | 0.25 | 0.1 |
| --- | --- | --- | --- | --- |
| 釋放 | [2] | [1] | [0] | [0] |

`Replay{Trace: [[0,1.5],[2,0],[0.25,0.25]]}`：step 0 → [0, 1.5]、step 1 → [2, 0]、step 2 → [0.25, 0.25]、
step 3 → 錯誤（錄音結束）。回傳的是副本，把三列都覆寫掉之後原始 trace 不變；`Replay` 只有一個欄位、兩個方法
（以 reflection 驗證），沒有任何可以往外取值的東西。

四種來源都會自己再檢查回饋：在 step 2 餵進「step 3 才可取得」的回饋、餵進毫秒時鐘的回饋、餵進無效回饋，
四種來源全部報錯。

### 獎懲映射的手算表（raw = [1, −2, 0.5, 0, 3]）

| baseline | kind | expected | delta | applied | clearance_boost |
| --- | --- | --- | --- | --- | --- |
| none | relu | 0, 0, 0, 0, 0 | 1, −2, 0.5, 0, 3 | [1] [0] [0.5] [0] [3] | 0, 2, 0, 0, 0 |
| none | split | 0, 0, 0, 0, 0 | 1, −2, 0.5, 0, 3 | [1,0] [0,2] [0.5,0] [0,0] [3,0] | 0, 0, 0, 0, 0 |
| running_mean(2) | relu | 0, 1, −0.5, −0.75, 0.25 | 1, −3, 1, 0.75, 2.75 | [1] [0] [1] [0.75] [2.75] | 0, 3, 0, 0, 0 |
| running_mean(2) | split | 0, 1, −0.5, −0.75, 0.25 | 1, −3, 1, 0.75, 2.75 | [1,0] [0,3] [1,0] [0.75,0] [2.75,0] | 0, 0, 0, 0, 0 |

`running_mean` 的歷史依序是 []、[1]、[1,−2]、[−2,0.5]、[0.5,0]，每個值都能用二進位浮點精確表示，所以測試比對
的是完全相等而不是容差。非有限的 raw 會被拒絕而且不進歷史（連拒三次之後期望值仍是先前唯一一筆的平均）。

### 分離證明

- 編譯期：`bash scripts/check-target-flow.sh` 掃過 `dynamics`（4 個非測試檔）、`simulate`（9）、`modulation`（3）、
  `plasticity`（1）、`learning`（8），沒有任何 `signal.Target` 或 `NewTarget(`；四個前向／調節套件的
  `go list -deps` 都沒有 `learning`。輸出見 `evidence/SIG-03/check-target-flow.log`。
- 執行期：`experiment/target_taint_test.go` 把 episode 的目標填成 NaN，`Predict` 對乾淨與被汙染的 episode
  回傳完全相同且有限的 `[-0.004081378225237131]`，四種來源在 step 2 的輸出都有限、非負且寬度等於 `Channels()`；
  帶 NaN 目標的 `Step` 以 `tensor[0] cannot be represented as finite float32` 失敗，參數與更新次數與失敗前逐位相同，
  同一步換成乾淨目標就正常更新。測試檔頭列出被走過的每一個函式邊界。

### 命令與結果

`gofmt -l .` 無輸出、`go vet ./...`、`go test -count=1 ./...`、
`go test -race -count=1 ./signal/ ./simulate/ ./modulation/ ./experiment/`、`bash scripts/check-target-flow.sh`
全部通過（2026-09-15，go1.26.5 darwin/arm64）。

## 依據

- 主規格 6.3（三分離）、6.5（洩漏）、11.1–11.3、11.6（獎懲與損失的角色）；規格抽取紀錄。
