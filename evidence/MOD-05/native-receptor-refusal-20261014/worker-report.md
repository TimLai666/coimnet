# 原生受體宣告契約唯讀審查

## Scope

審查基準為 `HEAD e2dabd1f5f1c296ccbb1983564dcefbfc3368a5b`，內容是 Root 骨架、ticket 92 與 live source 的靜態契約追蹤。沒有執行 Go、測試、網路或 Git 寫入。`evidence/MOD-05/receptor-rule-ownership-20261014/native-receptor-audit.json` 的 `go test` receipt 僅作既有外部證據，不能算成本次執行。

## CONFIRMED

- `[P2] [CONFIRMED] simulate/plasticity.go:81-116`：只要 `Plasticity.Rule.GateReceptor` 或 `DecayEReceptor` 是非 nil，且其餘 Rule 欄位通過既有數值驗證，原生 protocol 目前仍被接受。`validatePlasticity` 在 113-115 只把 Rule 交給 `plasticity.New`，沒有在成功後拒絕受體宣告。`plasticity.New` 在 `plasticity/plasticity.go:124-145` 會呼叫 `validateRule`，而 `validateRule` 在 `plasticity/plasticity.go:148-225` 只驗證受體索引非負、GateScale／Decay-E 參數與規則數值，不驗證原生化學綁定。這使合法數值的 `gate_receptor=0/100`、`decay_e_receptor=0/100` 及 gate/window/both 宣告通過，`nil` 則維持缺省。

- `[P2] [CONFIRMED] simulate/runner.go:255-280, 669-703, 761-778`：上述宣告在原生執行中沒有實際服務。`enablePlasticity` 只保存 `GateChannel`、外層 `GateScale` 與 `Frozen`；每步的閘門在 777 由 `plasticScale * stimulus[plasticGate]` 計算，778 呼叫 `Model.Step` 時沒有傳入 `DecayE` override。`plasticity.Model.StepWith` 在 `plasticity/plasticity.go:346-363` 只使用 Rule 的固定 `DecayE`，受體欄位不參與此原生路徑。因此受體宣告會被默默忽略，結果與移除欄位的正常原生模型相同。這是 ticket 92 要拒絕的真正漏洞，不涉及生物成效判斷。

- `[CONFIRMED]` 所有要求的原生入口都匯入同一個檢查：`simulate.Protocol.Validate` 在 `simulate/protocol.go:217-295` 的 291 呼叫 `validatePlasticity`；`DecodeProtocol` 在 192-200 的 197 呼叫 `Validate`；`Build` 在 `simulate/runner.go:104-114` 先經 `buildPreflight`，而 `buildPreflight` 在 132 呼叫 `Validate`，之後 `BuildVariant` 在 154-169 的 167 再驗一次；直接呼叫 `BuildVariant` 也在 167 驗證。比較流程由 `simulate/compare.go:133-143` 的 `DecodeCompareProtocol`、205-210 的 `CompareProtocol.Validate` 及 288-301 的 `Compare` 間接涵蓋。原生變體建構 `OriginalVariant`／`DeriveNullModel` 也共用 `simulate/nullmodel.go:203-228` 的 `variantPreflight`，其中 215 呼叫 `protocol.Validate`。沒有找到可建立並執行原生 `Runner`、又繞過 `Protocol.Validate`／`validatePlasticity` 的公開 consumer；`simulate/parameters.go:147-177` 的 `FromDerived` 只產生參數集，不執行模型，不能算繞過。

- `[CONFIRMED]` 受體拒絕檢查若放在 `simulate/plasticity.go:113-115` 的既有 `plasticity.New` 成功之後，可保留目前錯誤優先序。`validateRule` 的順序是規則種類、`DecayE`／`DecayP`、`PlasticMax`／`WMin`、GateReceptor 與 GateScale、DecayEReceptor 與 window 參數、最後是 stdp 專用數值，見 `plasticity/plasticity.go:148-225`。因此 `-1`、非有限／不合法 GateScale、非法 Decay-E 參數與不相容 `continuous/stdp` 仍先回原本錯誤；通過數值驗證後，非 nil 的 index `0` 與 `100` 才回新的 native chemistry receptor 拒絕。`Frozen` 在 `Plasticity` 宣告中不改變驗證路徑，故 `Frozen=false/true` 都應拒絕非 nil 受體；nil 則維持既有行為。這個判斷不選擇新的數值政策。

- `[CONFIRMED]` 合法正常路徑可保持。`continuous/hebbian`、`LIF/hebbian` 與 `LIF/stdp` 的既有規則相容性由 `simulate/plasticity.go:110-114` 保留；`gate_channel` 範圍、不得與 injection 共用及外層 `GateScale` 有限性由 94-109 保留。只新增對非 nil 受體引用的拒絕，不需新增原生化學層、不改 `plasticity.Model.Step` 算式、不改 Frozen 流程，也不改正常刺激閘門。

- `[CONFIRMED]` 模型格式可保持。`plasticity.Rule` 的 JSON 欄位仍在 `plasticity/plasticity.go:66-82`，`ProtocolSchemaVersion` 在 `simulate/protocol.go:32-40` 不需變更，`Protocol.Hash` 在 203-211 仍對同一 JSON 結構計算 SHA-256。`Protocol.Validate` 是值接收器且拒絕前沒有寫回 protocol；`DecodeProtocol` 在驗證失敗時於 197-200 回傳零值。因此合法 nil protocol 的編碼／hash 不需改變，含受體的非法 protocol 只會在建 runner 前被拒絕。

- `[CONFIRMED]` ticket 89 的增益範圍、ticket 91 已修正的指標所有權，以及局部學習的 pre/post 時點歧義都不屬於這次原生入口契約。此審查沒有將它們重新分類或提出數值政策。

## NEEDS INVESTIGATION

- `[NEEDS INVESTIGATION] simulate/plasticity.go:81-116`：Root 尚未在本次唯讀審查基準上套用拒絕檢查，因此 post-fix 的實際錯誤文字、四入口拒絕矩陣、`Frozen`／三種合法 core-rule 組合、輸入 JSON 指紋不變及正常輸出／報告相等，都尚未由本次測試驗證。靜態證據只支持「應在既有 `plasticity.New` 成功後檢查任一非 nil 指標」及「不需改模型格式」，不能宣稱修正已通過。

- `[NEEDS INVESTIGATION] plasticity/plasticity.go:124-145`：`plasticity.New` 是 learning／chemistry 路徑也使用的公開 API，該路徑必須繼續支援已綁定 chemistry 的受體規則。是否要改變這個跨套件 API 的全域政策，ticket 92 沒有授權；目前只確認 native `simulate` 應在自己的共用入口拒絕，不能把 native 拒絕擴大成 `plasticity` 套件全面拒絕。

## 驗證界線

- 本次沒有執行 Go 或其他命令型測試。既有 receipt `evidence/MOD-05/receptor-rule-ownership-20261014/native-receptor-audit.json` 的 `diagnostic_expects_original_bug_not_repaired_behavior=true`，只證明歷史診斷預期觀察原始漏洞，不是本次 post-fix 通過證據。
- 審查期間重新計算 live source SHA-256，與讀取時一致；沒有偵測到外部 source 改動。

## HEAD 與讀取檔案 SHA-256

```text
HEAD e2dabd1f5f1c296ccbb1983564dcefbfc3368a5b
AGENTS.md 5a452345fda8743b84406d1e67684c4b16906f2b5b738df35a27e7068cd8b0c2
README.md d57e4c1ba66642fc83a7fc07b1b5dc8f72904e7bf9c57f2249433d90ebea2699
delivery-status.md 43821d6877aec744c4cf4d8c516326906dc9f8a10513cd0d4c4510afddaaf443
ENG.md 23dcfd9a41f458630abbeab5ce3448474c7e6f366404489111e2f54f0a2b49d7
docs/initialization.md 19a358028e3d55985b77f76f8d5f0b35c944da2bf933b5beca7fbb574eee18ec
docs/handoff/AGENT_START_HERE.zh-TW.md 126ec9a76ed5eead27fbff3c966be00cee91fd82018f9569c72318fe27248183
docs/handoff/CoImNet_Implementation_Plan.zh-TW.md b92126883a10cdb9fb852276796e0521111520709bb7ca82d06f98dbab8935ea
docs/tickets/92-native-receptor-declaration-refusal.md 7b644ff24a35e83873687ba31145cd908991f545d36fdc978be9255897d72439
evidence/MOD-05/native-receptor-refusal-20261014/plan.md fc228536f2ee55e9b5cf7bf11fdd119faa52739347a718e85480875cb9a0a8d3
evidence/MOD-05/receptor-rule-ownership-20261014/native-receptor-audit.json 2b6a0580893ec6afd6cca6011b58eecd75eac411d798817b84fe4c2f5e3a27d1
evidence/MOD-05/receptor-rule-ownership-20261014/target-flow.json e17aad8e613e8c7f36e9d82422f7e64c3ed57b6db78dce954be731739c5724f3
evidence/MOD-05/receptor-rule-ownership-20261014/root-review.md d7a89398b2629ad19b9d96660b04be9b07d33a38dd4ad5f390a41d935a3b39e2
simulate/plasticity.go f634199d2d59daf239dbede40bc2c502c2f544fb13a06c2df1eca1b89b64d863
simulate/protocol.go 3f5e374b3811ff69bf90dd9b3f8548f511f4f973360e05114ab9ed3f0b037d3d
simulate/runner.go 2fe1741d7f55939ae66a67bca5d10d5c03756bec996e05f8e16a91693502450e
simulate/compare.go c7988e32199fd12a91c8e82fc7c74f2369936683effbbe265125d72de22008a2
simulate/nullmodel.go 9e189beb7a008852adfbae96ec849e7338177316daba4b36540b2bf491ef6186
simulate/parameters.go 38bcdb81e807c24dd9f571cba48fd3f33a3c71196dbc87f8d4694f779f6fbc38
plasticity/plasticity.go ab73f4e5e448b8299fbd46aa2cf9ac0f81085298b5b0a8c8b865c2e15ae3ebb0
learning/individual.go 531d3ebba6829853a2b8a3eefaf81f744668a281c4afd926170898b5c88486d9
learning/chemistry.go 4e6164cbafd843c56c357558e9b8a580598b607daeccffbfc74a714052a52032
```

`runs/.tmp/native-receptor-refusal-20261014/review/report.md` 在寫入前不存在，故沒有讀取前 SHA-256。

## 唯一修改檔案

`runs/.tmp/native-receptor-refusal-20261014/review/report.md`
