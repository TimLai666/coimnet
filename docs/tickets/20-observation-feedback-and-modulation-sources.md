# 20 — 研究者可以把觀察、目標與事後回饋分開，並選擇調節訊號的來源

Epic：訊號與時間、化學調節（第一層：來源）

User Story：研究者可以確認前向路徑與調節控制器只看得到觀察，目標只由訓練器持有，回饋依可取得
時間之後才進入；並能從外部時序、具名神經元活動、內在資源與記錄重播四種來源產生非負的調節釋放率，
把原始獎懲以明示規則映射成調節輸入而不改寫原始值。

Blocked by：03 訊號、12 runner、14 具名集合

Status：draft（契約已於 2026-09-15 定案；待 18 派工後接著派）

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

- [ ] 型別分離與汙染測試、靜態 import 檢查腳本；早於 `AvailableAt` 的回饋被過濾（測試）。
- [ ] 四種來源各有手算輸出、非負與有限檢查、重播不可外取（無網路介面）；神經來源必須明示集合。
- [ ] 獎懲映射：四個值分開、raw 唯讀、負回饋不產生負值而是清除項；`go test`、race、vet；證據與文件。

## 依據

- 主規格 6.3（三分離）、6.5（洩漏）、11.1–11.3、11.6（獎懲與損失的角色）；規格抽取紀錄。
