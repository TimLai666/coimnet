# Expression gradient review

## Problem

稽核基準 `318b7518eddef07c9925f7a8b97a3513123786c4` 的化學記憶讀出增益與訓練反向路徑，確認中性值、同一受體跨細胞／區域平均、多讀出節點、活動來源回授、夾限導數及分段／截斷時序沒有實際造成訓練品質或狀態錯誤。

## Result

**CONFIRMED P1/P2：無。**

逐項來源追蹤結果如下：

- **中性值：無缺陷。** `learning/memory.go:9-14,23-27,38-40,102-117` 明定增益為 `clamp(1 + Scale*meanOccupancy, Min, Max)`，邊界必須包含 1，沒有匹配記錄時平均值為 0。`learning/chemical_sequence.go:610-631` 以同一公式計算前向與是否在夾限內，故零佔用且合法範圍會得到精確增益 1；`modulation/effects.go:63-79,149-181` 的化學效果中性陣列也分別是 gain 1、offset／threshold 0。

- **同一受體跨細胞／區域平均：無缺陷。** `modulation/receptors.go:182-186,218-248` 產生每個「受體 × 細胞」記錄並保留 `Region`；`learning/memory.go:102-117` 對當列所有相同 `Receptor` 記錄加總後除以記錄數，不因區域切開。`learning/chemistry.go:235-264` 的每受體摘要採相同分母。反向在 `learning/chemical_sequence.go:397-420,455-467` 對每筆相同受體記錄各加上 `sum(up[node]*raw[node]) * Scale / count`，再由 `modulation/receptor_gradient.go:33-85` 依各記錄的 region/channel 累加。因此多區域細胞的全域平均與區域濃度梯度相互對應。

- **多讀出節點與 Network 反向路徑：無缺陷。** `learning/network.go:379-431` 先由 Insyra readout VJP 取得每個讀出節點的直接 adjoint，再按 `ReadoutNodes` 映射回 core row。`dynamics/coupled_readout.go:8-47` 以遞增 step 順序把未含回授的直接 adjoint 傳給 `BackwardReadout`。`learning/chemical_sequence.go:384-426` 對每個 expression node 做 `rawAdjoint[node] = up[node] * gain`，並以

  `seed(record) = Scale / count * sum(up[node] * rawOutputs[step][node])`

  對所有匹配受體記錄播種。這正是多節點讀出值同時乘同一增益時對平均佔用率的導數；readout 權重梯度仍由 transformed `h` 的 Insyra tape 計算。

- **活動來源回授：無缺陷。** `learning/chemical_sequence.go:131-167` 將前一列 raw core output 傳給化學 runtime，只有明示的 `InitialActivity` 在第一列取代它；`dynamics/coupled_modulation.go:171-218` 也確認 source 看到的是未套 expression 的 raw activity。來源為全域釋放時，`learning/chemistry.go:197-213` 將每 channel 釋放複製到所有 region，反向 `learning/chemical_sequence.go:530-543` 對所有 region 的 release gradient 求和，符合該前向複製。控制器的時間窗索引在 `learning/chemical_sequence.go:544-559`，神經來源對 `nonNegative(mean(activity)*Gain)` 僅在乘積大於零時回傳 `Gain/len(Nodes)` 導數，見 `learning/chemical_sequence.go:560-591`，與 `modulation/sources.go:135-157` 的前向相符。

- **夾限導數：無缺陷。** expression 增益在 `learning/chemical_sequence.go:619-631` 只有嚴格位於 `[Min, Max]` 內且有記錄時播種；邊界本身導數為零，與 `modulation/receptor_gradient.go:90-95,213-245,285-287` 對效果夾限採用的契約一致。神經來源的非負夾限在零點採零導數慣例，反向條件見 `learning/chemical_sequence.go:575-591`。控制器摘要方差在 `modulation/controller_state.go:103-117` 對 `rawVariance <= 0` 採零導數，並且其分母與 `modulation/controller.go:286-339` 的前向摘要相同。

- **分段與截斷時序：無缺陷。** `learning/chemical_recompute.go:10-49` 的 checkpoint 保存化學 state、資源、pending feedback、source states 與 source clock，只有原 episode 第一列保留明示初始活動；後續 segment 從 checkpoint 的 raw core output 接續。連續核心重算在 `dynamics/coupled_recompute_continuous.go:147-193,214-302` 以全域 segment start 建 checkpoint，反向依全域 `step/window*window` 載入，沒有把 local step 當成全域時間。化學反向在 `learning/chemical_sequence.go:491-510,594-607` 於 window 邊界清除 concentration carry，並拒絕跨邊界的 activity gradient。混合核心分段重算被 `learning/recompute.go:10-29` 與 `learning/chemical_episode.go:176-194` 明示限制為 continuous/LIF，屬已宣告能力邊界，不是本輪缺陷。

## NEEDS INVESTIGATION

1. Root 尚未完成的公開 `PredictAll`／`LossGradientFrom` 多區域、多讀出節點、正／負／零 `Scale`、夾限與固定 window 獨立參考矩陣，本輪沒有 runtime 結果。這是執行證據未完成，不足以從靜態流程推定 bug；依指示本輪未執行 Go，也未修改測試或診斷。

2. `modulation/receptors.go:225-238` 對 `unknown_skipped` 仍產生 `Occupancy=0` 記錄；`learning/memory.go:105-117` 與 `learning/chemical_sequence.go:397-420,634-641` 會把該記錄納入 expression 平均與分母，但 `modulation/receptor_gradient.go:33-36` 反向會跳過它。文件只說 unknown 未允許假設時計入 `unknown_skipped`，沒有決定 expression 平均應採「所有宣告細胞含零值」或「只平均可計算記錄」。用一個 known 加一個 unknown-skipped 的公開 fixture 即可區分兩種政策，但目前缺少既定預期，故不判為缺陷。

## Baseline and fingerprints

本輪讀取時指定 review 檔不存在，目標目錄只有 `.` 與 `..`。工作樹基準為：

```text
git rev-parse HEAD
318b7518eddef07c9925f7a8b97a3513123786c4
```

目前讀到的主要程式 SHA-256 指紋如下，均與基準 `source-before.json` 相符：

```text
learning/memory.go                         e43731d6ccecbbfc6c597be869bc9e6f499fa67d7a8332d1bae01eeec02e87d7
learning/chemical_sequence.go              c73b9c655e451ac5dd882547a260c69e48ea5379b4b08c53c4dd2cf3b5ac9305
learning/chemical_episode.go               928fb5dfea73a2ace4be49debf6084aad77173c5b6ff390b6f0df40cb4d941b7
learning/chemical_recompute.go             eb0d92c63a41127c19fd1e15236e2ebe4fdd10a64557f71fa0b8911bc523a953
learning/network.go                         56fe942df2304d60d13fe46221b43b407d8f343e6e7792258d8a7be751d2f2eb
dynamics/coupled_reverse.go                3a7a810e7c3dfda625d3b86f643bd33f9864aa76af679013e127244ffd78d529
dynamics/coupled_readout.go                7441f9d84e43300585d5e01b899ab5a08b8f38d5e89fa9e4f83e468ff72a08fe
dynamics/coupled_modulation.go             e7c20e0c7676fef695d0ffb769a5fe51c14cf3af654f26872eec86cff140c8df
dynamics/coupled_recompute.go              64d830ce7e4fbc09593ec170468d9ca61d4ef5cdc7600089ce5b67844daea48a
dynamics/coupled_recompute_continuous.go   cfd31afac8ddf4922739e52771f0f4169dda585695353e194efd8b70c95fb43f
dynamics/coupled_recompute_lif.go          841372c946c2113c13b02f32421705d61ed88432d33fda522371a25af6b645a9
modulation/receptors.go                     de4b3b2926bdeaa53c1805cab7898d815b79d81015d809542f16fdd12fb8120d
modulation/receptor_gradient.go             c87110f8a628c171a908c38695acd06a522634c7600a98f945618482bfb5c953
modulation/effects.go                       32e690166a02f14f2ca7f414c3e6de643864fb16bad42d342e249aaff2256a06
modulation/sources.go                       5280342db13372aff9ec3f3f540961135e762d97f0af4821a47e6986b85883af
modulation/controller.go                    1974c542a20f123cb3640d8ca6453d861f57afc4ff2a54c5552cbbe4011e1bab
modulation/controller_state.go              c568341acf0662a81ebc8a0ddc09eae04c80edbf001fda4a446e670b6aa2b58f
modulation/kinetics_gradient.go             310ee3977123be8e125d80c4db8ef0f9e4a62051896a8fc756e044ddf8465419
```

## Changed

| 檔案 | 變更摘要 |
|---|---|
| `runs/.tmp/expression-gradient-audit-20261014/review/expression-review.md` | 新增本輪靜態 expression gradient 審查；沒有修改正式程式、測試或診斷。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| `git status --short` | 工作樹 | 只有既有 `delivery-status.md`、`docs/tickets/96-expression-gradient-composition.md` 與 `evidence/MOD-06/expression-gradient-audit-20261014/` 變更，未改動。 |
| `git diff -- learning modulation dynamics` | 指定程式範圍 | 空 diff。 |
| codebase-memory MCP 查詢 | `coimnet-chemistry-ocr` 索引 | 已用於定位 `BackwardReadout`、`BackwardStep`、Network reverse 與重算流程，並以實際來源核對。 |
| Go 執行 | 本輪 | 未執行，遵守派工限制。 |
| Git 寫入 | repository | 未 commit、stash、reset、push 或修改 Git 狀態。 |

## Verified

- 已以 `git rev-parse HEAD`、指定 Git diff、來源 SHA-256 與逐段 `nl -ba` 讀取核對基準及行號。
- 已追蹤前向 `raw output → expression → readout` 與反向 `readout VJP → raw adjoint → occupancy seed → chemistry/source/core` 的完整鏈路。
- 已檢查同一受體記錄跨 region 的平均分母、每個 expression node 的 seed、global release 的 region sum、neural clamp 的正值分支、controller window 位置與 segment checkpoint clock。
- 未執行 Go，因此沒有宣稱 runtime 矩陣、數值誤差或 race 結果通過。

## Notes

- OpenCode `opencode/big-pickle --agent plan` 本輪回傳 `HTTP403 FreeTierError`；依派工資訊改由目前 agent 以 `gpt-5.6-luna/max` 完成靜態審查。
- 沒有找到可提供 P1/P2 的觸發輸入、根因與公共 API 最小重現，因此沒有修補建議。

## Next

Root 執行獨立參考矩陣並保存實際差異；若出現與上述公式不一致的可重現案例，再以該案例重新標定缺陷等級與最小修正範圍。

Skills：investigate、diff-inspector
