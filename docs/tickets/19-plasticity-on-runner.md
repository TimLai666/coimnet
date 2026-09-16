# 19 — 研究者可以在原生模擬之上開啟可塑性，並報告原有輸出被增強、修改或破壞

Epic：原生模擬（可學習模式的第一步）

User Story：研究者可以用同一份刺激與判讀協定，比較「無可塑性」「開啟可塑性」「學過之後關閉」
三種執行，說出原有輸出被增強、修改或破壞了哪些；關閉可塑性時逐位還原原生行為。

Blocked by：14 空模型與判讀、18 局部可塑性（第一階段）

Status：done（契約 2026-09-15 定案，2026-09-16 實作與驗證，見「證據」）

對應需求：NAT-06（學習前後在相同刺激與判讀協定下的對照矩陣；關閉學習可還原原生行為）。
主規格 10.3、14.4（對照組）、研究方向第五層。

## Root 決策（2026-09-15）

1. **protocol 的 `plasticity` 區塊**：`Plasticity{Rule plasticity.Rule; Edges *Selector 或 "all";
   GateChannel int; GateScale float64}`，閘門取自刺激矩陣的一個通道（與注入通道分離），每步
   `gate(t) = gate_scale × stimulus[t][gate_channel]`；未宣告 = 關閉。`RunReport.plasticity{rule,
   enabled_edges, gate_channel, clamped_by_w_min, plastic_l2_before, plastic_l2_after}`。
2. **runner 內的順序**：沿用 18 的固定順序（核心前向 → 更新跡 → 套閘門 → 下一步用新的
   `w_eff`）。`w_eff` 每步重算成核心的權重陣列（`dynamics` 的 `Advance` 接受每次呼叫的參數，
   因此分塊推進時在每個分塊前更新一次；為了逐步更新，分塊大小在開啟可塑性時強制為 1，並在報告寫明
   耗時代價）。
3. **三種執行**（`simulate compare` 新增 `learning_variants`）：`original`（無可塑性，逐位等於
   NAT-01／02 的報告）、`plastic`（開啟，報告學後的 plastic 統計）、`learned_then_frozen`（先跑
   `plastic` 得到 `plastic` 陣列，再以該陣列固定為 `w_eff` 且關閉更新，重跑同一刺激）。指標與門檻
   沿用 14；每格的 `deltas_from_original` 直接回答「增強／修改／破壞」：報告不做文字判斷，只給差值
   與百分位。
4. **關閉即還原**：`plastic` 陣列全零且關閉更新時，RunReport 與未宣告 `plasticity` 的報告逐位相同
   （測試）。
5. **真實資料**：推導參數集 + NAT-02 protocol + `hebbian_rate` 規則、閘門為刺激後 20 步的一段常數
   （`gate_channel` 1），啟用全部邊；三格各跑一次，`evidence/NAT-06/`。

## 契約

```go
// simulate
type Plasticity struct { Rule plasticity.Rule; Edges *Selector; All bool; GateChannel int; GateScale float64 }
// Protocol.Plasticity *Plasticity `json:"plasticity,omitempty"`（omitempty，既有 hash 不變）
type PlasticityReport struct { Rule string; EnabledEdges int; GateChannel int; ClampedByWMin uint64; PlasticL2Before, PlasticL2After float64 }
// RunReport.Plasticity *PlasticityReport `json:"plasticity,omitempty"`
// CompareProtocol.LearningVariants []string  // original | plastic | learned_then_frozen
```

## 驗收

- [x] fixture：三格對照的手算（小圖、hebbian_rate、常數閘門）、關閉即逐位還原、分塊為 1 不改結果、
  `learned_then_frozen` 的 `w_eff` 等於 `plastic` 跑完後的值；`go test`、race、vet。
- [x] 真實資料：三格各一次，記錄命令、環境、指紋、耗時；報告只給差值與百分位。
- [x] 文件與 `evidence/NAT-06/`。

## 依據

- 研究方向 NAT-06、主規格 10.3、14.4、7.2（模式表：運行中局部學習）。

## 證據

`evidence/NAT-06/`（實作與驗證 2026-09-16；macOS arm64、Darwin 25.6.0、go1.26.5、8 顆邏輯 CPU、
17,179,869,184 B 實體記憶體、insyra v0.3.2，`evidence/NAT-06/compare-plastic-v1/doctor.json`）。
先寫失敗測試再實作，兩份 red log 對應兩個階段。本票只動 `simulate/`、`internal/cli/simulate*.go`、
`scripts/plasticity-evidence.sh`、`evidence/NAT-06/` 與三份文件，`plasticity/`、`learning/` 一行都沒有改。

### 紅燈

- `evidence/NAT-06/red-runner.log`：`simulate/plasticity_test.go` 先寫，`Protocol.Plasticity`、
  型別 `Plasticity`、`RunReport.Plasticity`、`Runner.PlasticState`、`ParameterSet.Signs` 全部
  undefined，套件編譯失敗。
- `evidence/NAT-06/red-compare.log`：runner 那一段轉綠之後才加 `simulate/compare_plasticity_test.go`，
  `CompareProtocol.LearningVariants`、`VariantPlastic`、`VariantLearnedThenFrozen` undefined。

### 手算表（三神經元 fixture，`simulate/plasticity_test.go`）

`fixtureGraph` 是 0→1（邊 0，原始 weight 2）與 1→2（邊 1，原始 weight 3），uniform `gain` 2 讓基礎
權重是 4 與 6；uniform 來源沒有符號，兩條邊都是自由邊，`w_eff = w_base + plastic` 逐位精確。LIF 的
`dt = tau_syn = 1`，所以 `lambda = kappa = k = exp(-1) = 0.36787944117144233`、`alpha = 1 - k`，
`theta_raw` 0 讓基礎閾值等於 1。pre 是來源的突觸跡、post 是目標當步的事件，閘門走刺激的通道 1
（注入走通道 0）。`decay_e = decay_p = 0.5`、`plastic_max = 8`、`w_min = 0.0625`。

| 情境 | 步 | pre | post | 邊 0 eligibility | 邊 0 plastic | 邊 1 eligibility | 邊 1 plastic |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `gate_scale` 1 | 0 | [1,0,0] | [1,0,0] | 0 | 0 | 0 | 0 |
| | 1 | [k,1,0] | [0,1,0] | k | k | 0 | 0 |
| `gate_scale` 4 | 1 | [k,1,0] | [0,1,0] | k | 4k = 1.4715177646857694 | 0 | 0 |
| | 2 | [k²,k+1,1] | [0,1,1] | 0.5k+k² = 0.31927500382233387 | 0.5·4k + 4·(0.5k+k²) = 2.0128588976322195 | k+1 = 1.3678794411714423 | 4(k+1) = 5.471517764685769 |

實測與上表逐位相同（測試用 `==` 比對，每一格都照實作的運算順序寫出來，不是四捨五入後的近似）。

快速變化真的進到下一步的權重：`gate_scale` 4 時第 2 步整合的是 `4 + 4k = 5.471517764685769` 而不是
4，節點 1 的候選電位變成 `k·(−0.5) + alpha·(4+4k)·k = 1.0884 ≥ 1` 因此放電；沒有可塑性時是
`0.7463 < 1`，不放電。`spike_fraction` 因此是 [1/3, 1/3, 2/3] 對 [1/3, 1/3, 1/3]。跑完後的
`Effective` 回 `[4 + 2.0128588976322195, 6 + 5.471517764685769]`，`held_at_w_min` 為 0。

另外兩組：連續核心（`TestPlasticContinuousCoreUsesOutputsAsBothSignals`）沒有事件，規則兩邊都讀
啟動後的輸出，期望值在測試裡由 `v(t+1) = k·v(t) + a·drive(t)`、`y = tanh(v)`、`w = base + plastic`
三條式子重新推一次，不是讀實作的輸出；`stdp_pair`（`TestSTDPPairRunsOnTheSpikingCore`）的跡先衰退
再加自己的事件，前→後兩步後 eligibility 0.5、plastic 0.5、兩個跡是 0.5 與 1。

### hash 不變

`TestProtocolWithoutTheBlockKeepsTheRecordedHash` 把 `evidence/NAT-01/protocol-fullgraph-uniform.json`
原樣寫進測試，解碼後 `Hash()` 仍是
`befa9f1d340a49d8dbeed50a67ca72d711bc601710764d22cdfbef353ea6ce42`（`evidence/NAT-01/verification.json`
記錄的值），而且編碼裡沒有 `plasticity` 這個鍵。真實資料那一段另有一個實測：本票 `original` 那一格
的四個探針序列、monitors、assumptions、`core_config_hash` 與 `parameter_hash` 與 NAT-02 的全腦單次
執行逐位相同。

### 關閉即還原

兩個位置各測一次，因為「逐位相同」的界線在兩處不同：

1. compare（完全逐位）：宣告 `learning_variants` 之後，`original` 那一格跑的是把區塊拿掉的 protocol，
   所以整份 `RunReport`（含 `protocol_hash`）與同一份比較在沒有宣告區塊時的第 0 格位元組相同，空模型
   那一格也相同（`TestCompareOriginalCellIsTheReportWithoutTheBlock`）。
2. runner（排除兩個必然不同的欄位）：宣告區塊、快速變化全零、`frozen: true` 的四步執行，整份
   `RunReport` JSON 在拿掉 `plasticity`、`protocol_hash` 與 `assumptions` 之後與沒有宣告區塊的執行
   位元組相同，神經狀態快照也逐位相同；`assumptions` 單獨比對，確認只是多出寫明規則的那一句。
   `protocol_hash` 必然不同，因為它就是宣告本身的指紋。

分塊為 1 不改結果：`TestPlasticityForcesChunkSizeOne` 用 `gate_scale` 0（快速變化永遠是零）跑五步，
逐步推進的報告在同樣三個欄位之外與一次推進五步的報告位元組相同；`TestChunkBoundariesDoNotChangeTheRun`
已經釘住分塊大小本身不改任何值。

`learned_then_frozen` 的權重：`TestLearnedThenFrozenUsesThePlasticCellsFinalWeights` 先獨立跑一次
`plastic`，用它跑完的快速狀態算 `Effective(base, signs, state)`，把那組權重當成基礎參數再跑同一份
刺激，四個探針序列與 compare 第 2 格逐位相同。compare 的三格指標（`TestCompareLearningVariantsRunTheThreeCells`）：
ALPN 的 `mean_rate` 原圖 2/6、`plastic` 與 `learned_then_frozen` 都是 3/6，差值是兩個已捨入的比率
相減（不是 1/6 的捨入），`alpn_fraction` 三格都是 1、差值 0，原圖那一格的所有差值都是 0。

### 真實資料

`scripts/plasticity-evidence.sh` 跑了兩次，同一份 store 與同一份參數集，只有 `decay_p` 不同。
兩次都是 165,122 個神經元、25,563,197 條邊、300 步，`all: true` 啟用全部 25,563,197 條邊，
刺激是 NAT-02 的同一個脈衝（通道 0，第 10–29 步，振幅 2）加上通道 1 的閘門（第 30–49 步常數 1，
其餘為 0），`gate_channel` 1、`gate_scale` 1、`plastic_max` 0.01、`w_min` 0.0001、`decay_e` 0.5，
參數來源 `derived_release/v1`（`unknown_sign: exclude`、`weight_scale: 21.6`）。

| | `compare-plastic-v1`（`decay_p` 0.5） | `compare-plastic-persistent-v1`（`decay_p` 0.999） |
| --- | --- | --- |
| 牆鐘 | 1252.44 s（user 874.32、sys 268.43） | 1217.28 s（user 874.65、sys 254.32） |
| 最大 RSS | 6,605,570,048 B | 6,558,416,896 B |
| 峰值記憶體 | 14,563,535,768 B | 14,069,049,880 B |
| 每格牆鐘 | 137.51 / 611.82 / 494.04 s | 89.58 / 598.64 / 519.41 s |
| `clamped_by_plastic_max` | 33,391,125 | 34,549,873 |
| `clamped_by_w_min` | 0 | 0 |
| `plastic_l2`（前→後） | 0 → 8.083189933121256e-75 | 0 → 11.902496517509498 |
| 退出碼 | 0 | 0 |

`uptime`：第一次 `up 11 days, 54 mins` load 3.33／3.56／3.47 → `up 11 days, 1:15` load
3.40／3.68／3.94；第二次 `up 11 days, 1:16` load 2.06／3.24／3.76 → `up 11 days, 1:37` load
3.26／3.94／4.08。這台機器同時有其他 agent 在編譯與測試，牆鐘含這些干擾。指紋：store
`a6c0ddffd507e0f7c2941b2d76d65ebecb7a19d2da0523d282b1cd1161bb9fcd`、參數集
`c4db0f3f93390b66c417cee2a96d805179e4fb178678615ec537260710c58c77`、protocol
`0e30d8d817f72816931bc487e003fe160cf089f84aaa5f2343061df35c9cc030` 與
`4189458e7dae96317753f1206bb0e425b156e852ec4161b832013ab90bfd91f9`。兩次的 `bin/coimnet` 指紋不同
（`64ad7153…` 與 `26be3cdc…`），因為同一個 repo 上有其他 agent 在改 `learning`、`modulation`、
`replay`，兩次建置之間那些套件變了；`simulate` 與 `internal/cli` 的原始碼兩次完全相同，而且兩次的
`original` 那一格的整份 `RunReport` 位元組相同，所以那些改動沒有碰到本票的路徑。

估算與實測：先用一份 25,563,197 條邊的量測程式測出每步的可塑性額外成本約 0.30 s
（`Effective` 約 0.12 s、`Step` 約 0.19 s），加上 NAT-01 的每步核心成本約 0.31 s，估每個可塑性
格 300 步約 3 分鐘；實測 `plastic` 格 611.82 s 與 598.64 s，遠低於 prompt 訂的 90 分鐘門檻，
因此沒有縮短 protocol。

第二次（`decay_p` 0.999）的三格差值，只有差值與位置，沒有文字判斷：

| 指標 | `original` | `plastic` | `plastic` 差 | `learned_then_frozen` | `frozen` 差 |
| --- | --- | --- | --- | --- | --- |
| alin_spike_fraction | 1 | 1 | 0 | 1 | 0 |
| alin_mean_rate_gate | 0.2479166667 | 0.2916666667 | +0.04375 | 0.3145833333 | +0.06666666667 |
| alin_mean_rate_after | 0.2463333333 | 0.3048333333 | +0.0585 | 0.3191666667 | +0.07283333333 |
| alin_latency | 0 | 0 | 0 | 0 | 0 |
| alin_activity_ratio_after | 0.7298765432 | 0.9032098765 | +0.1733333333 | 0.9011764706 | +0.1712999274 |
| descending_neuron_spike_fraction | 0.5639269406 | 0.4916286149 | −0.07229832572 | 0.5136986301 | −0.0502283105 |
| descending_neuron_mean_rate_gate | 0.08523592085 | 0.09075342466 | +0.005517503805 | 0.08694824962 | +0.001712328767 |
| descending_neuron_mean_rate_after | 0.08811567732 | 0.08479756469 | −0.003318112633 | 0.08480974125 | −0.003305936073 |
| descending_neuron_latency | 13 | 13 | 0 | 13 | 0 |
| descending_neuron_activity_ratio_after | 56.48 | 54.35317073 | −2.126829268 | 6.331818182 | −50.14818182 |
| vnc_motor_spike_fraction | 0.5988700565 | 0.6016949153 | +0.002824858757 | 0.5889830508 | −0.00988700565 |
| vnc_motor_mean_rate_gate | 0.06334745763 | 0.07556497175 | +0.01221751412 | 0.07026836158 | +0.006920903955 |
| vnc_motor_mean_rate_after | 0.07705084746 | 0.08655932203 | +0.009508474576 | 0.07901129944 | +0.001960451977 |
| vnc_motor_latency | 21 | 21 | 0 | 18 | −3 |
| vnc_motor_activity_ratio_after | undefined | undefined | undefined | 32.90588235 | undefined |

沉默比例 0.6439844478627924 → 0.5921197659912065 → 0.5944937682440862，每步全群放電比例的最大值
0.07934739162558593 → 0.08569421397512142 → 0.08362301813204782；三格都沒有穩定旗標，五個門檻三格
全過。`vnc_motor_activity_ratio_after` 在 `original` 與 `plastic` 未定義（baseline 窗的放電率為 0），
在 `learned_then_frozen` 才有值，因此那一格的差仍然未定義——對沒有量到的東西不給差值。

第一次（`decay_p` 0.5）的結果是另一件事：閘門結束後 `plastic` 每步衰減一半，250 步後整個陣列只剩
`8.083189933121256e-75`，所以 `learned_then_frozen` 的整份 `RunReport` 在 `plasticity`、
`protocol_hash` 與多出的那一句 `assumptions` 之外與 `original` 位元組相同，十五個差值裡有定義的十四個全部是 0，剩下那一個（`vnc_motor_activity_ratio_after`）三格都未定義。
同一份 `plastic` 格仍然有差（ALIN 閘門窗 +0.04375、descending_neuron 的 `spike_fraction` −0.03272、
vnc_motor 的 `spike_fraction` +0.04096），也就是「學習當下有效、事後全部衰退掉」。要讓學到的東西撐
到執行結束，`decay_p` 必須接近 1；這是規則的時間常數決定的，不是實作的選擇。

### 偏離與決策

1. **`Plasticity` 與 `PlasticityReport` 各多一個 `frozen`**（`omitempty`）。票面的契約沒有這兩個
   欄位，但 `learned_then_frozen` 需要「同一套規則、同一組啟用邊、不更新」這個狀態，而且讀者要能
   從報告分辨那一格。
2. **`clamped` 拆成 `clamped_by_w_min` 與 `clamped_by_plastic_max`**，另外加 `chunk_size` 與
   `wall_clock_penalty_note`。兩種上限是兩件事：一個擋住跨零，一個擋住快速變化的大小；分塊代價
   寫在數字旁邊而不是留給文件。
3. **逐位相同的界線**。決策 4 寫「逐位相同」，字面上只有在「區塊不存在」時成立，因為
   `protocol_hash` 就是宣告的指紋，而 `assumptions` 會多一句寫明規則。完全逐位的版本放在 compare
   的 `original` 那一格，runner 上的版本排除那兩個欄位並單獨比對 `assumptions`。
4. **`edges` 選擇器啟用「兩端都在集合內」的邊**。票面只寫 `Edges *Selector`，而選擇器解析的是節點；
   邊的定義是本票的決定，也就是那個族群內部的連線。零命中由 `plasticity.New` 拒絕。
5. **`ParameterSet` 多 `Signs []int8`，且不進 `fingerprint()`**。`Effective` 需要符號才能讓固定符號
   邊不跨零，而 `ParameterSet` 原本只留下已經乘過符號的權重。把欄位放進指紋會改掉 NAT-02／NAT-03／
   NAT-05 已記錄的 `parameter_hash`；derived 的指紋已經指名參數集檔、規則 hash、unknown 政策與
   scale，符號就是這四者的函數，所以放在指紋外面不會少記任何東西。
6. **`unknown_sign: exclude` 的邊在可塑性下是自由邊**。它們的符號是 0、基礎權重是 0，`Effective`
   走 `base + plastic`，所以快速變化可以讓一條被排除的邊帶上權重。這是「exclude 政策」遇上「啟用
   全部邊」的必然結果，不是額外假設；全腦這份參數集有 2,754,056 條這種邊（NAT-02 的推導報告）。
   要避免就改 `unknown_sign` 或改用 `edges` 選擇器。
7. **`simulate run` 的 `--state-in`／`--state-out` 與 `plasticity` 區塊互斥**。狀態快照只有核心狀態、
   沒有快速變化，分兩段跑會把快速變化靜靜歸零；直接回錯，而不是丟掉一半的狀態。
8. **空模型那幾格不帶可塑性**。票面沒有要求「學習 × 空模型」的乘積，本票把那個問題留給另一份
   protocol，並在 `Compare` 裡把空模型用的 protocol 明確去掉區塊。
9. **`reproduction` 只在宣告 `learning_variants` 時多接一段**，所以既有比較（NAT-03、NAT-05）重跑
   時字串一字不變。
10. **ticket 18 的第二階段方塊沒有勾**，因為本票的檔案範圍不含其他 ticket；18 的「第二階段：runner
    與 compare」即是本票，證據在這裡。

### 限制

- `plastic_max` 0.01、`decay_e` 0.5、`decay_p` 0.5／0.999、`w_min` 0.0001 與 `gate_scale` 1 都是
  宣告的工程選擇，沒有量測依據；規則本身也是主規格 10.3 的參考式，不是果蠅可塑性的測量。報告的
  `assumptions` 會原樣寫出這些值。
- 閘門是刺激矩陣的一個通道，由使用者給，不是由受體或調節訊號產生；受體來源在調節票。
- 兩次全腦執行各一次，沒有重複量測，牆鐘與 RSS 受同機其他 agent 影響。
- 沒有跑空模型格與學習格的乘積，所以本票不能回答「學習造成的改變有多少是接線本身的」。
- `w_min` 在兩次全腦執行都沒有擋下任何邊（`clamped_by_w_min` 0），因此固定符號邊不跨零這件事在
  真實資料上沒有被觸發，只有 fixture 上有實測（`TestPlasticFixedSignEdgeIsHeldAtWMin`）。
- CLI 沒有保存或載入快速變化的入口，所以跨行程接續一段可塑性執行目前做不到（見偏離 7）。
