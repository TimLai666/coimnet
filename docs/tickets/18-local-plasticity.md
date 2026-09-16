# 18 — 研究者可以使用近期參與紀錄、學習閘門與兩種局部更新規則

Epic：訓練與持續學習（可塑性）

User Story：研究者可以在個體運作中開啟局部可塑性：每條指定連線保存近期參與紀錄與快速變化，
以活動相關或脈衝時序相關的規則更新，受學習閘門與衰退控制，延遲到達的回饋仍能作用；關閉時
還原原生行為；固定符號的連線不會因快速變化跨零。

Blocked by：11 LIF 核心、16 LIF 個體（第一階段）、17 固定符號（第一階段）

Status：in progress（第一階段已於 2026-09-15 實作並驗證，見「第一階段證據」；第二階段 runner 與
compare，NAT-06，尚未開始）

對應需求：LRN-04（活動相關與脈衝時序相關規則，各有方程式與前後時序測試）、LRN-05（近期參與
紀錄與調節控制：延遲回饋、閘門關閉、衰退、更新順序）、MOD-05 的閘門與時間窗部分（受體來源在
後續調節票）、NAT-06 的前置（第二階段接 runner）。主規格 10.3、11.5、7.2。

## Root 決策（2026-09-15）

1. **參考規則（主規格 10.3 原式）**：對每條啟用可塑性的邊 e=(j→i)：
   `elig_e(t+1) = decay_e * elig_e(t) + pre_j(t) * post_i(t+1)`，
   `plastic_e(t+1) = bounded(decay_p * plastic_e(t) + gate(t+1) * elig_e(t+1))`。
   `pre`／`post` 對 LIF 是突觸跡 x 與當步 spike（活動相關版）；連續核心是輸出 y。`bounded` 為
   `clamp(·, -plastic_max, plastic_max)`。
2. **脈衝時序規則（STDP，只對 LIF）**：每邊兩個跡 `pre_trace`（`decay_pre`，在 pre spike 時 +1）與
   `post_trace`（`decay_post`，在 post spike 時 +1）；post spike 時 `elig += a_plus * pre_trace`，
   pre spike 時 `elig -= a_minus * post_trace`；`plastic` 更新同上。規則以名稱選擇
   `rule ∈ hebbian_rate | stdp_pair`，兩者各有手算的前後時序測試（pre-before-post 增強、
   post-before-pre 減弱）。
3. **有效權重與符號安全**：自由邊 `w_eff = w_base + plastic`；固定符號邊（17 的 `EdgeSigns`）
   `w_eff = s * max(|w_base| + plastic, w_min)`，`w_min > 0`（設定），永不跨零；報告被 `w_min` 擋下的
   次數。快速變化不寫回基礎參數。
4. **閘門與延遲回饋**：`gate(t)` 是外部給定的時序（本票由呼叫端提供，可為常數、可為延遲脈衝；
   後續調節票才由受體產生）；閘門為 0 時 `plastic` 只衰退不新增（測試）；延遲 d 步到達的閘門仍以
   殘留的 `elig` 更新（測試以手算對照，且 `decay_e` 不同時結果可區分）。
5. **更新順序**：同一步內固定順序：核心前向 → 更新 elig 與 trace → 套 gate 更新 plastic →
   下一步前向使用新的 `w_eff`。與梯度更新同時存在時，梯度更新只改 `w_base`，在 episode 邊界進行，
   不與 plastic 更新交錯（主規格 10.3「唯一順序與合併方式」）。單一 goroutine。
6. **狀態與保存**：`PlasticState{Eligibility, Plastic, PreTrace, PostTrace []float64（依啟用邊順序）}`
   進 `IndividualSnapshot.Plastic`（omitempty；未啟用為 nil），schema 維持
   `coimnet-individual-checkpoint/v1`。關閉可塑性時前向逐位等於未啟用。
7. **第二階段（NAT-06 前置）**：`simulate` 的 protocol 可宣告 `plasticity` 區塊，runner 在 `Run` 中
   套同一套規則（第一階段的 `plasticity` 套件），`RunReport` 加 `plasticity{rule, enabled_edges,
   clamped, gate_source}`；`simulate compare` 可跑「學習前／學習後／關閉」三格同一刺激。

## 契約（第一階段：套件 `plasticity` 與 learning 整合）

```go
package plasticity
type Rule struct { Kind string; DecayE, DecayP, PlasticMax, WMin float64; DecayPre, DecayPost, APlus, AMinus float64 }
type Config struct { Rule Rule; Edges []int /*啟用的邊索引，遞增*/ }
type State struct { Eligibility, Plastic, PreTrace, PostTrace []float64 }
func New(c Config, edges int) (*Model, error)
func (m *Model) NewState() State
func (m *Model) Step(s State, pre, post, spikesPre, spikesPost []float64 /*節點層*/, gate float64, sources, targets []int) (State, Report, error)
func (m *Model) Effective(base []float64, signs []int8, s State) ([]float64, ClampReport, error)
// learning: Individual.EnablePlasticity(c plasticity.Config), Individual.Advance 接受 gate 序列
```

## 驗收

- [x] 第一階段：兩種規則各一組 2–3 神經元手算時序（含閘門關閉、延遲閘門、衰退、上限、`w_min`）；
  關閉可塑性逐位等於未啟用；固定符號邊不跨零；快照往返；`go test`、race、vet；`evidence/LRN-04/`、
  `evidence/LRN-05/`。
- [ ] 第二階段：runner 與 compare 的 `plasticity` 區塊、三格對照（前／後／關閉）在 fixture 與全圖
  各跑一次；`evidence/NAT-06/`；文件。

## 依據

- 主規格 10.3（局部可塑性原式與固定符號限制）、11.5（局部學習閘門、學習時間窗）、7.2（模式表）、
  研究方向 NAT-06。

## 第一階段證據

`evidence/LRN-04/`、`evidence/LRN-05/`（實作 2026-09-15，證據中的驗證日誌為 2026-09-16 重跑；
macOS arm64、go1.26.5；`uptime` 為 up 11 days, 25 mins，load 4.39／4.44／3.37，偏高是因為同一個
repo 上有其他 agent 同時在編譯與測試，本票沒有任何時間量測）。先寫失敗測試再實作，
三份 red log 對應三個階段。

### 紅燈

- `evidence/LRN-04/red-rules.log`、`evidence/LRN-05/red-gate.log`：手算測試寫在
  `plasticity/rules_test.go`、`plasticity/gate_test.go`，當時 `plasticity/` 只有測試檔，
  `no non-test Go files` 編譯失敗。
- `evidence/LRN-05/red-individual.log`：`learning/plasticity_test.go` 與
  `learning/plasticity_helpers_test.go` 先寫，`EnablePlasticity`、`DisablePlasticity`、
  `AdvanceGated`、`PlasticReport`、`IndividualSnapshot.Plastic` 全部 undefined。
- `evidence/LRN-05/red-snapshot.log`：**行為紅燈**。`checkpoint` 的 19 個畸形 `plastic` 區塊中，
  有 17 個在加必填走訪之前就已被嚴格解碼器、null 掃描或 `RestoreIndividual` 擋下；
  只有「省略 `decay_e`」與「省略 `decay_p`」會被 `encoding/json` 靜靜變成合法的 0，
  那兩格就是紅的，補上 `requireIndividualPlastic` 後轉綠。

### 手算表（節錄，完整版在兩份 `verification.json` 的 `observed_result`）

三神經元鏈，邊 0 = 0→1、邊 1 = 1→2、邊 2 = 0→2（不啟用）；
`decay_e = decay_p = 0.5`、`plastic_max = 8`、`w_min = 0.0625`，`gate = 1`。所有衰減都是 2 的冪，
因此每一格在 float64 下精確，測試用 `==` 比對。

| 規則 | 活動／脈衝 | 邊 0 eligibility | 邊 0 plastic | 邊 1 plastic |
| --- | --- | --- | --- | --- |
| hebbian_rate 相關 | y = [1,.5,0] → [.5,1,.5] → [0,.5,1] | 0.5, 0.75, 0.375 | 0.5, 1, 0.875 | 0, 0.5, 1 |
| hebbian_rate 反相關 | post 全部變號 | −0.5, −0.75, −0.375 | −0.5, −1, −0.875 | — |
| stdp_pair 前→後 | spikes [1,0,0] → [0,1,0] → [0,0,1] | 0, 0.5, 0.25 | 0, 0.5, 0.5 | 0, 0, 0.5 |
| stdp_pair 後→前 | spikes [0,0,1] → [0,1,0] → [1,0,0] | 0, 0, −0.5 | 0, 0, −0.5 | 0, −0.5, −0.5 |

`stdp_pair` 的兩個順序由 `TestSTDPPairCoincidentSpikePinsTheReadOrder` 釘住：同一步的前後脈衝
eligibility 為 0、兩個跡都變 1（跡在加自己的事件之前被讀）；第二次同步脈衝 eligibility 仍為 0、
兩個跡變 1.5（跡先衰退再加，不是加完再衰退）。

閘門與邊界（`evidence/LRN-05`）：

| 子句 | 手算 | 實測 |
| --- | --- | --- |
| 閘門 0 | plastic 2 → 1, 0.5, 0.25；eligibility 1, 1.5, 1.75 | 相同 |
| 延遲閘門 `decay_e = 0.5` | eligibility 1, 0.5, 0.25 → plastic 0.25 | 相同 |
| 延遲閘門 `decay_e = 0.25` | eligibility 1, 0.25, 0.0625 → plastic 0.0625 | 相同（兩者可區分） |
| `plastic_max = 1.5` | 無界 1, 2, 2.5 → 1, 1.5, 1.5，clamped 0, 1, 1 | 相同（負向對稱） |
| `w_min` 擋下 | base [.5,−.25,.75]、signs [+1,−1,0]、plastic −2 → [0.0625, −0.0625, 0.75]，held 2 | 相同；全自由時為 [−1.5, −2.25, 0.75]，held 0 |

### 個體整合

兩神經元 LIF，`dt = tau = tau_syn = 1`，`theta_raw = −2` → `theta = 0.16324277592101166`，
基礎權重 0.9，輸入 [1.5, 0, 1.5, 0, 0]。期望值由一份獨立的 Python LIF＋規則實作先算出來，
再寫進測試；Go 實作逐位吻合。

| step | 該列使用的權重 | eligibility | plastic | readout（閘門全開） | readout（閘門全關） |
| --- | --- | --- | --- | --- | --- |
| 0 | 0.9 | 0 | 0 | 0 | 0 |
| 1 | 0.9 | 0.36787944117144233 | 0.36787944117144233 | 1 | 1 |
| 2 | 1.2678794411714422 | 0.18393972058572117 | 0.36787944117144233 | 0.3678794503211975 | 0.3678794503211975 |
| 3 | 1.2678794411714422 | 0.5096363698321669 | 0.693576090417888 | 1.1353353261947632 | 1.1353353261947632 |
| 4 | 1.593576090417888 | 0.40846910704143036 | 0.7552571522503744 | 1.4176665544509888 | 0.417666494846344 |

延遲脈衝 `gate = [0,0,0,1,0]`：第 0–3 列與閘門全關逐位相同，閘門到達那一步的 plastic 正好等於殘留的
eligibility `0.5096363698321669`，它抬高的權重把第 4 步的放電翻過來，最後一列 readout 變成
`1.4176665544509888`（閘門全關是 `0.417666494846344`），之後閘門再關上，plastic 衰退到
`0.25481818491608343`。

固定符號：邊宣告 +1、存 `rho = log(0.9)`、`w_min = 0.9`、快速變化由快照還原為 −8。沒有地板時
`0.9 − 8 = −7.1` 會讓這條興奮邊變成抑制邊；有地板時五列全部擋在 0.9（`held_at_w_min` = 5），
readout 與未啟用可塑性的表完全相同，快速變化 −8 → −4 → −2 → −1 → −0.5 → −0.25。同一個 −8 放在
自由邊上 readout 全為 0，證明「真的會跨零」不是假設。

關閉即還原：LIF 與連續兩個 fixture 上，從未啟用、啟用後再關閉、以及全零閘門的 `AdvanceGated`
三者 readout 逐位相同，關閉的個體不寫 `plastic` 區塊，`PlasticReport` 為零值。

保存：`IndividualSnapshot.Plastic` 往返逐位相同且不共用緩衝，從還原後的個體接續等於原個體接續；
`RestoreIndividual` 拒絕 8 種畸形區塊，`checkpoint.LoadIndividual` 拒絕 19 種，
schema 維持 `coimnet-individual-checkpoint/v1`，未啟用時完全不寫 `plastic` 鍵，
`LoadModelPackage` 仍然拒絕這份文件，`checkpoint/individual_test.go` 既有測試一個都沒有動。

### 全套驗證

`evidence/LRN-04/verification-full.log`（與 LRN-05 同一份）：`gofmt -l .` 無輸出、`go vet ./...`
exit 0、`go test -count=1 ./...` 全綠（含其他 agent 當時未提交的 `modulation`、`signal`、
`simulate`、`experiment` 變更）。`verification-race.log`：
`go test -race -count=1 ./plasticity/ ./learning/ ./checkpoint/ ./experiment/ ./examples/...` 全綠。

### 偏離與決策

1. **`Individual.Advance` 的簽章不變，閘門走新的 `AdvanceGated`。** 票面寫「`Individual.Advance`
   接受 gate 序列」，但 `Advance(ctx, input)` 已經是公開契約，`learning`、`checkpoint`、
   `experiment`、`examples` 與 CLI 都在用。改為
   `AdvanceGated(ctx, input, gate) ([][]float64, PlasticReport, error)`；`Advance` 在可塑性開啟時
   等於 gate 全 0 的 `AdvanceGated`（跡與 eligibility 照常演化，plastic 只衰退），關閉時是完全沒動過的
   快路徑。這是 root 對本階段的決定。
2. **可塑性關閉時，非零閘門是錯誤而不是靜默的 no-op。** 票面只說關閉時前向逐位等於未啟用。
   如果 `AdvanceGated` 在關閉狀態下默默吃掉閘門，就多了一個被忽略的旋鈕（ENG.md 的既有原則），
   所以全零閘門走快路徑並回零值報告，任何非零值直接回錯。
3. **`stdp_pair` 在連續核心上於 `EnablePlasticity` 就被拒絕**，而不是等到第一步才發現沒有事件。
   票面把核心種類的檢查留給 `Step`；提早拒絕的位置是唯一知道核心種類的地方。
4. **`Config.Edges` 不得為空。** 票面只要求遞增且小於邊數。啟用一個什麼都不改的機制是設定錯誤，
   不是一種機制，因此 `New` 直接拒絕。
5. **`hebbian_rate` 不得攜帶四個 STDP 欄位。** 同樣是「不要留被忽略的旋鈕」，
   `decay_pre`／`decay_post`／`a_plus`／`a_minus` 非零時 `New` 拒絕。
6. **契約之外多了三個方法**：`(*Model).Config()`（快照要寫回宣告）、`(*Model).Edges()`
   與 `(*Model).ValidateState()`（`RestoreIndividual` 與 `checkpoint` 要驗證還原的狀態）。
7. **`coreModel.advance` 多回傳一個每步事件矩陣**（連續核心為 nil）。`lifCore.advance` 原本把
   `dynamics.LIF.Advance` 的事件丟掉，可塑性需要它當 post 訊號。這是套件私有介面，唯一的呼叫點在
   `learning/individual.go`。
8. **`State.PreTrace`／`PostTrace` 用 `omitempty`**，`hebbian_rate` 下不存在而不是零陣列，
   與 `LIFState.Rate`／`Homeostasis` 的既有做法一致。
9. **`plastic` 寫成 JSON `null` 時，個體快照既有的 null 掃描會先擋下**，因此「null 代表關閉」只在
   記憶體中的 `IndividualSnapshot` 成立，落盤文件的規則更嚴。必填走訪本身仍把 null 當關閉處理，
   與 `neural` 聯集的寫法一致。
10. **第二階段（runner、compare、NAT-06）完全沒有動**，`simulate/` 未修改。
