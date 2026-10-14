# Ticket 91 受體宣告所有權與恢復契約唯讀稽核

## 契約結論

目前完整 HEAD `71033a8dce5663c2c6135798b8884872ef8ed2b0` 的 live source 尚未達 ticket 91。Root 提出的骨架方向正確，真正需要處理的缺口只有兩組：`Rule.GateReceptor`／`Rule.DecayEReceptor` 的 pointee 所有權，以及 `RestoreIndividual` 缺少既有 `checkReceptorRule` 交叉檢查。沒有受體參照的舊缺省可合法保留。

本稽核只讀取來源與既有證據，沒有執行 Go、測試、網路、外部模型或 Git 寫入。既有 Root receipt 只作來源交叉核對，不能算成本 worker 的執行結果。

## CONFIRMED 問題與精確觸發

### 1. New／Config／Enable／Snapshot／Restore 共用兩個受體指標

- `plasticity.Rule` 在 `plasticity/plasticity.go:66-83` 以兩個 `*int` 保存受體編號。`New` 在 `:124-145` 先驗證 `c.Rule`，再以 `rule: c.Rule` 保存，因此只複製 Rule 外層，沒有複製兩個 pointee。`Config` 在 `:227-232` 又直接回傳 `m.rule`，其兩個指標仍可被外部修改。
- 可達觸發是：呼叫 `New` 或 `Individual.EnablePlasticity` 時傳入 `idx` 的位址，成功返回後改寫 `*idx`；或改寫 `Model.Config().Rule`／`Individual.Snapshot().Plastic.Config.Rule` 所流出的 pointee。模型或持續個體會讀到外部新編號。`RestoreIndividual` 在 `learning/individual.go:228-241` 也經 `newPlasticRuntime` 走同一條淺拷貝路徑，改寫原 snapshot 的 pointee 會影響已恢復個體。
- 受體值一旦被改成不存在的正編號，`learning/individual.go:772-792` 會在 `:774` 或 `:787` 直接索引 `chemical.receptorOccupancy`；負編號也能由外部改寫造成同樣的越界 panic。這不是執行期間並行修改的判定，呼叫端並行改參數本來就不在保證內；這是操作完成後仍保留別名的公開所有權違約。
- 修正邊界是兩個非 nil pointee 各自複製成值後再驗證／保存，`Config` 再各自建立新 pointee；nil、0、正值與同一輸入指標的值語義保持，不需複製其他核心參數，也沒有證據要求保留指標身份別名。

### 2. RestoreIndividual 接受未綁定化學受體的快照

- `RestoreIndividual` 在 `learning/individual.go:228-241` 建立 plastic runtime、驗證 fast state 與 slow state，在 `:244-281` 建立 Chemical runtime，直到 `:283` 返回；這條路徑沒有呼叫 `checkReceptorRule`。該檢查目前只在 `EnablePlasticity:317-324` 的 `:321` 與 `EnableChemistry:301-305` 的 `:302` 出現。
- 精確觸發 A：快照的 `Plastic.Config.Rule.GateReceptor` 或 `DecayEReceptor` 非 nil，但 `Chemical` 缺失。`checkReceptorRule` 本來會在 `learning/individual.go:332-345` 拒絕；目前 Restore 卻成功，之後 `advanceStepwise` 的 `:773-792` 因 `i.chemical != nil` 條件不成立而略過受體 gate／window，宣告的作用被靜默停用。
- 精確觸發 B：快照有 Chemical，但受體指標大於等於 `len(Chemical.Config.Receptors.Records)`。`chemicalRuntime.finishRow` 在 `learning/chemistry.go:251-264` 只配置該長度的 `receptorOccupancy`，目前 Restore 仍接受，後續在 `individual.go:774` 或 `:787` 索引越界而 panic。
- 負受體編號不是 Restore 漏洞：`plasticity.New` 的既有 `validateRule` 已在 `plasticity/plasticity.go:167-180` 拒絕負值。Restore 應補的是 chemistry 是否存在及上界交叉驗證，並在所有部件成功建立後用已擁有的 Rule 呼叫既有 `checkReceptorRule`。
- 既有 Root 稽核也記錄相同來源路徑：`evidence/MOD-06/slow-memory-composition-20261014/root-review.md:10,14,16`；`ownership-restore.json:18-21` 的 Go receipt 不是本 worker 執行，僅作既有證據引用。

## NEEDS INVESTIGATION 與缺少的證據

- 本 worker 沒有執行 Go，因此不能替 Root 宣稱修正已通過。仍缺 New 輸入隔離、Config 輸出隔離、Enable 輸入隔離、Snapshot 輸出隔離、Restore 輸入隔離的回歸證據，且兩個指標都要涵蓋 nil／0／正值、同一輸入指標及修改後仍可獨立推進。
- 仍缺 Restore 的四類邊界證據：無受體參照且無 Chemical 的舊缺省可恢復；有受體參照但無 Chemical 被拒絕；Chemical records 不足被拒絕；合法受體索引恢復後的快照與新程序接續保持相同。拒絕時需確認呼叫者快照與原個體均未被改寫。
- 三核心差異必須由 Root 實測補齊：continuous 與 mixed 都因 `c.LIF == nil` 在 `learning/individual.go:294-300` 拒絕 `stdp_pair`；LIF 可接受 `stdp_pair`；mixed 的 `hebbian_rate` 仍須按 `learning/core.go:504-517` 的逐節點 spiking mask 執行。受體綁定本身只依 Chemical records 數量，沒有核心特例。三核心均需覆蓋正常、缺省、拒絕、合法 snapshot continuation。
- `EnablePlasticity` 目前把呼叫端的 `c.Rule` 傳給 `checkReceptorRule`（`individual.go:321`），而 runtime 已由 `newPlasticRuntime` 建立。依「呼叫期間不得並行修改參數」的既有前提，這不是已確認的額外 bug；實作審查時仍應確認檢查使用的值與保存到 model 的 owned Rule 完全相同，Restore 則直接使用 owned model Rule。
- 不需要把 ticket 89 的增益合法性、BaseL2 單位或生理方程式列入本票。它們不是本次兩個 `*int` 所有權與 Restore 綁定缺口的證據。

## Root 骨架是否保持正常公式與合法快照

- 保持。`plasticity/plasticity.go:28-39` 的 eligibility／plastic 更新順序與 `:494-515` 的 effective-weight 算式不需要改動。只複製兩個受體 pointee 不會改變正常規則的數值欄位；`NewState`、`ValidateState`、`StepWith`、`EffectiveWith` 的合法輸入語義也不需擴張。
- 保持。`IndividualSnapshot` 的 v1 結構與可選欄位在 `learning/individual.go:93-110`，Snapshot 的四部件複製在 `:394-426`，Restore 的正常建構在 `:194-283`。合法 nil 受體欄位仍省略／缺省；合法 Chemical 與受體索引只增加交叉驗證，不改保存格式、正常公式或三核心既有事件規則。
- `checkReceptorRule` 在 `learning/individual.go:332-342` 明確把兩個指標都為 nil 視為合法，因此「沒有受體參照的舊缺省」在有無 Chemical 時都可保留。相關可選數值若沒有對應指標，仍依 `plasticity.go:174-195` 的既有規則要求為零。
- 核心差異只影響可接受的事件訊號：continuous 無事件，LIF 的 `stdp_pair` 讀 0／1 事件，mixed 對局部規則逐節點判斷。受體所有權與 chemistry 綁定檢查是共用契約，不應為三核心另立一套。

## 讀過的檔名行號及完整 SHA256／完整 HEAD

完整 HEAD：`71033a8dce5663c2c6135798b8884872ef8ed2b0`

| 檔名 | 本次核對行號 | SHA256 |
| --- | --- | --- |
| `evidence/MOD-05/receptor-rule-ownership-20261014/plan.md` | 1-25 | `72a8c619e27265427cb469286f4a6e2a96a56d9e992f64648cf574e34c339dcb` |
| `docs/tickets/91-receptor-rule-ownership-and-restore.md` | 1-16 | `c936cde0a4abacc63165a5f6aa2756067a8983859bd718376cb93973d69676f3` |
| `AGENTS.md` | 1-72 | `142445fa0a80972d74ab4aa33b018027df4e7214b01d117800fa0f611cedf5a5` |
| `ENG.md` | 197-217, 221-255 | `c705912363988e378ba1b39d1a546440f61116336c333ac6816a7ded9c575a15` |
| `plasticity/plasticity.go` | 1-577 | `ca835416a04696cfb70122b74d1763d2c201d3a732af1c9c29685237e04dbd9f` |
| `learning/individual.go` | 1-1031 | `f5e626c5532df1a95d1abdae3bf836784030fedba8382d63b92f2f1c32bf80e2` |
| `learning/chemistry.go` | 1-510 | `4e6168cbafd843c56c357558e9b8a580598b607daeccffbfc74a714052a52032` |
| `learning/network.go` | 14-25, 157-179 | `56fe942df2304d60d13fe46221b43b407d8f343e6e7792258d8a7be751d2f2eb` |
| `learning/core.go` | 470-517 | `fbf4f8238b352a9b6351c5207b92c9b0c4e30e50fd8e2b7dce507ced0855ece8` |
| `learning/receptor_gate_test.go` | 204-247 | `07ba3a525bc69403b1a268ac07309541208a548115af2f7b2421913e477eaa90` |
| `plasticity/receptor_test.go` | 11-67, 139-188 | `6109cfeb6740fe4db58b010dd371a1d05cd528d5eb70cd9f9aa88b2755d4b211` |
| `learning/plasticity_test.go` | 240-336 | `f39dd9dc5d73374ec54bf8675bc45e34fc7e5a6974c688817964fd3acd3a962e` |
| `evidence/MOD-06/slow-memory-composition-20261014/root-review.md` | 1-16, 43-51 | `44ffb0102cf9d7b52d446ee0d2b0e7bdc472faff485daea3e77116eea65ff54e` |
| `evidence/MOD-06/slow-memory-composition-20261014/ownership-restore.json` | 1-23 | `14f44413fefb360c73fb5da9c8a500ecf35b2e4834784de4fedd479b651e030e` |

## 唯一實際修改檔

`runs/.tmp/receptor-rule-ownership-20261014/review/report.md`（本檔）。沒有修改來源、測試、證據、Git 或設定。
