# Ticket 91 局部可塑性後續唯讀數值審查

本報告只審查 `plasticity.Rule`／`State`、狀態驗證、`StepWith`／`EffectiveWith`，以及 `learning` 與 `simulate` 的公開接入。受體指標所有權、`RestoreIndividual` 的受體／化學交叉檢查、ticket 90 有效權重組合與 ticket 89 增益範圍均排除。沒有執行 Go、網路、外部模型或任何 Git 寫入；數值反例只用 Python 手算，沒有把 Root 或其他執行者的 receipt 當成自己的驗證。

## 契約結論

目前可由公開入口觸發的真正缺口有兩項：

1. `hebbian_rate` 的個體與原生 runner 都把「核心一步之後的 source 值」當成 `pre`。主規格／ticket 18 要求的是該步開始時的 `pre_j(t)` 乘上該步結束時的 `post_i(t+1)`。延遲零的核心確實在步驟開始讀 source，因此這會改變 eligibility、plastic 及下一步有效權重。
2. `simulate.Protocol` 接受帶 `GateReceptor` 或 `DecayEReceptor` 的 Rule，但 simulate 沒有化學／受體佔用率來源；`Run` 仍使用刺激通道閘門並不傳 `DecayE` 覆寫。受體宣告因而被靜默忽略，且 `GateReceptor` 同時存在於直接閘門路徑，違反 ticket 22 的拒絕契約。

`plasticity` 套件本身的 Rule 複製、State 形狀／有限值／界限驗證、`StepWith` 的輸入與拓樸檢查及純函式狀態提交，沒有在本次來源審查中找到另一個可確認的反例。`EffectiveWith` 的算式與 ticket 90 組合不在本次判定範圍。Root 已套用的 Rule 指標隔離與 Restore 後 `checkReceptorRule` 綁定在目前來源仍保持。

## CONFIRMED

### C1：`hebbian_rate` 的 learning／simulate consumer 使用錯誤的 pre 時點

依 ticket 18 第 20–24 行，參考式是

```text
elig_e(t+1) = decay_e * elig_e(t) + pre_j(t) * post_i(t+1)
```

主規格第 10.3 節第 462–465 行保留同一個更新順序。ticket 25 第 27–33 行又明定延遲 0 讀本步開始的 `out`。`dynamics/continuous.go:168-185` 的 `forwardReader` 和 `dynamics/mixed.go:437-442` 的 reader 都在核心更新前讀 `out[past]`；LIF 呼叫相同 reader（`dynamics/lif.go:359-370`），而 LIF 的新 synaptic trace 在 `dynamics/lif.go:405-417` 才寫入 `syn[t+1]`，`Outputs()` 回傳的也是步驟後列（`dynamics/lif.go:455-463`）。

兩個公開接入點都把步驟後列拿來當 source：

- `learning/individual.go:749-755` 直接寫明「after this step」，以 `values[0]` 同時建立 `pre`；`learning/individual.go:808-811` 把它送進 `StepWith`。
- `simulate/runner.go:761-778` 有相同的「after this step」說明，`plasticStep` 以 `outputs` 當 `pre`，再呼叫 `Step`。ticket 18 第 42–44 行要求 runner 套用同一套規則，因此這不是只限個體的偏差。

最小可達手算反例使用既有兩節點 LIF 形狀：edge `0 -> 1`、delay `0`、初始 synaptic trace `[0,0]`、base weight `0.9`、`decay_e = decay_p = 0.5`、`plastic_max = 8`、gate `[1,1]`，輸入兩列 `[[1.5], [0]]`，`theta_raw = -2`、`theta_min = .05`、`theta_max = 1`、`dt = tau_syn = 1`。所需有限值為 `lambda = kappa = exp(-1) = 0.36787944117144233`、`alpha = 1-exp(-1) = 0.63212055882855767`、`theta = 0.16324277592101166`。

- 第 0 列結束時 source 的 synaptic trace 是 `1`，target 沒有 event。第 1 列開始時核心用 source trace `1`，所以 target candidate 是 `alpha * .9 = 0.56890850294570194`，大於 theta，target event 是 `1`。
- 依 ticket 18，這一列應取 `pre_j(t)=1`、`post_i(t+1)=1`，所以初始 eligibility 為零時新 eligibility 是 `1.0`，gate 開啟後 fast change 也是 `1.0`。
- 現行 `learning`／`simulate` 取第 1 列結束後的 source trace `0.36787944117144233`，再乘 target event `1`，得到 `0.36787944117144233`。ticket 18 第 127–133 行的個體表也把這個較小數值固定成第 1 列結果，與同文件第 21 行的正式公式矛盾。

這是可達的公開數值錯誤，不是生物效果推測；它會直接改變下一列套用的 fast weight。`plasticity.StepWith` 第 381–418 行本身依傳入的 `pre[j] * post[i]` 計算，缺口在 consumer 沒有傳入核心更新前的 source 活動。

### C2：simulate 接受受體 Rule 卻沒有受體／化學綁定，並靜默使用刺激閘門

`simulate.Plasticity` 的公開契約明定 gate 來自 `GateScale * stimulus[t][GateChannel]`（`simulate/plasticity.go:28-37`），該型別沒有化學設定或受體紀錄。`Protocol.Validate()` 在 `simulate/protocol.go:214-217`、`287-294` 會呼叫 `validatePlasticity`；`simulate/plasticity.go:110-115` 只拒絕非 LIF 的 `stdp_pair` 並呼叫 `plasticity.New`。`plasticity.New` 對受體指標只檢查非負（`plasticity/plasticity.go:168-196`），所以不存在化學資料的 simulate protocol 仍能通過這條驗證。

建構後沒有別的綁定路徑：`simulate/runner.go:255-279` 只保存 Rule、閘門 channel 與 scale；每列 `Run` 在 `simulate/runner.go:679-701` 先跑核心，再呼叫 `plasticStep`；`simulate/runner.go:771-778` 以刺激值計算 gate 並呼叫 `Step`。這裡沒有 `GateFor`、`WindowFor`、受體 occupancy 或 `StepWith` 的 `DecayE` 覆寫。`plasticity.StepWith` 在未提供 override 時固定取 Rule 的 `DecayE`（`plasticity/plasticity.go:357-363`），因此 `DecayEReceptor` 也不會生效。

可觸發的宣告是：一個已合法的二通道 protocol，`GateChannel = 1`、`GateScale = 1`，injection 只讀 channel 0，`All = true`，Rule 為 `hebbian_rate`、`DecayE=.5`、`DecayP=.5`、`PlasticMax=8`、`WMin=.0625`、`GateReceptor = ptr(0)`（或 `DecayEReceptor = ptr(0)` 並給合法的 base/span/min/max）。這組 Rule 對 `plasticity.New` 合法，simulate 沒有任何化學參照可解析。執行時 gate 仍是 `stimulus[t][1]`；GateReceptor 的 occupancy 不會被讀取，DecayEReceptor 的時間窗也不會被套用。

ticket 22 第 23–28 行要求受體 occupancy 產生 gate／decay window，且明定直接 gate 與受體 gate 同時宣告要拒絕；`docs/model-and-mechanisms.md:55` 同樣要求無化學時拒絕受體 Rule。這裡至少應在 simulate 的公開 Validate／Build 前拒絕，或另接明示的 chemistry 路徑。現況是接受後靜默忽略，屬真正的數值契約缺口，不是單純 report metadata 未列欄位。

## NEEDS INVESTIGATION

1. **Root 的公開動態重現仍缺失。** 本審查禁止執行 Go，因此沒有宣稱 `Protocol.Validate`、`Build` 或 `Run` 的實際 receipt。Root 應以合法二通道 protocol、`GateReceptor=ptr(0)`／`DecayEReceptor=ptr(0)`、GateChannel 1、injection channel 0，分別呼叫公開 `Validate`／`Build`／`Run`，保存「應拒絕但目前是否接受」及 occupancy／DecayE 實際值。這是對 C2 的 runtime 佐證，不改變上述由 live source 可達路徑確認的契約缺口。
2. **ticket 18 公式與其第 121–138 行 hand table 的時點定義互相衝突。** 目前依正式公式、ticket 25 的 core clock 語意及核心 reader 行為列 C1 為 CONFIRMED。若 Root 要把 hand table 的「步驟後 source」定為新政策，必須先明示更新 ticket／主規格及所有接入者，否則不能把現行 table 當成對正式公式的默認例外。
3. **沒有品質分數或生物成效結論。** 本審查只確認數值輸入、可達路徑與狀態契約；未執行任務、長期學習或生物定量評估。

## Rule／State 與 Root 骨架檢查

- `plasticity.New` 在驗證前複製 Rule，並複製 edge selection（`plasticity/plasticity.go:124-145`）；`Config()` 再複製 Rule 與 edges（`plasticity/plasticity.go:228-245`）。目前 live source 的受體指標不再共用。
- `ValidateState` 檢查 eligibility／plastic 長度與有限值、plastic bound、Hebbian 不帶 pair traces、STDP traces 的長度／非負／有限值（`plasticity/plasticity.go:282-327`）。
- `StepWith` 檢查 state、gate、DecayE override、完整拓樸、node-level signals、事件 0/1 與 endpoints，再以新 State 計算並在錯誤時不提交（`plasticity/plasticity.go:339-419`）。直接呼叫這個 API 時，輸入 pre／post 的時點由呼叫者承擔；C1 是接入端傳值錯誤。
- `Individual` 的 snapshot／restore 仍在建立全部部件後呼叫 `checkReceptorRule`（`learning/individual.go:194-288`），而該檢查對無受體參照的舊缺省回傳合法、對缺化學或越界索引回錯（`learning/individual.go:333-357`）。這部分不是本報告的缺口。
- ticket 90 的 `EffectiveWith` 組合與慢速層算式不在本輪判定；本報告沒有把它列為 confirmed 或品質分數。

## 讀過的來源與完整 SHA256

以下是本報告引用的 live source／規格來源，hash 以本次讀取位元組計算：

| 檔案 | 主要行號 | SHA256 |
| --- | --- | --- |
| `AGENTS.md` | 1–72 | `142445fa0a80972d74ab4aa33b018027df4e7214b01d117800fa0f611cedf5a5` |
| `ENG.md` | 57、63–65、213、233 | `23dcfd9a41f458630abbeab5ce3448474c7e6f366404489111e2f54f0a2b49d7` |
| `plasticity/plasticity.go` | 66–103、124–245、282–419、432–590 | `ab73f4e5e448b8299fbd46aa2cf9ac0f81085298b5b0a8c8b865c2e15ae3ebb0` |
| `learning/individual.go` | 174–191、291–357、434–464、644–830 | `531d3ebba6829853a2b8a3eefaf81f744668a281c4afd926170898b5c88486d9` |
| `learning/chemistry.go` | 300–344 | `4e6164cbafd843c56c357558e9b8a580598b607daeccffbfc74a714052a52032` |
| `simulate/plasticity.go` | 28–116、236–251 | `f634199d2d59daf239dbede40bc2c502c2f544fb13a06c2df1eca1b89b64d863` |
| `simulate/protocol.go` | 214–295 | `3f5e374b3811ff69bf90dd9b3f8548f511f4f973360e05114ab9ed3f0b037d3d` |
| `simulate/runner.go` | 99–249、252–280、622–703、761–778 | `2fe1741d7f55939ae66a67bca5d10d5c03756bec996e05f8e16a91693502450e` |
| `dynamics/continuous.go` | 158–185、324–352 | `cc85e745832cffddfe7a41fa65c5c1eb485ce1f995776b815cf19fcb41c8a7d4` |
| `dynamics/lif.go` | 344–420、455–463 | `95b4fd8f3c098f6ff49693f7025e2e91b6fd8394fede837d81fcdfbc3e84ec5f` |
| `dynamics/mixed.go` | 44–58、426–442 | `3d7741a6db48c31039d1fb50036ef24f24822db8ba5e29c0459bef5aae3d96f4` |
| `docs/tickets/18-local-plasticity.md` | 20–44、121–187 | `57447155d05a9ae77572908025856983af00db95d41e488fe36810f2f9ad53a2` |
| `docs/tickets/22-modulated-learning-memory-controller-and-interventions.md` | 23–28、46–57、134–167 | `2dd0e2b100f2a5232a786e99d529947c98a3b3692de245822b5ddddef312df2f` |
| `docs/tickets/25-mixed-types-vector-nodes-shared-parameters-and-recompute.md` | 22–43、47–54 | `ae26d494597535babfe27705333b9169f3abd2260963632a6cf4ab38e557e12c` |
| `docs/handoff/CoImNet_Implementation_Plan.zh-TW.md` | 458–479、539–548 | `b92126883a10cdb9fb852276796e0521111520709bb7ca82d06f98dbab8935ea` |
| `docs/model-and-mechanisms.md` | 55、59、65、69–71 | `43d9e559e9c25ea8767d0e65e9bb920406258cb3975912b21d9490b347184d61` |

完整 HEAD：`71033a8dce5663c2c6135798b8884872ef8ed2b0`。

## 唯一實際修改檔

`/Users/timlai/Developer/coimnet/runs/.tmp/receptor-rule-ownership-20261014/review/plasticity-next.md`
