# Ticket 91 Root 修正唯讀契約稽核

## 契約結論

live source 的兩個正式修正符合 `plan.md:13-19` 與 ticket 91 的既有契約，沒有找到新的已確認實作反例。`plasticity.Model` 現在在 `New` 驗證與保存前擁有兩個受體 pointee，`Config` 重新提供獨立 pointee；`RestoreIndividual` 在 plastic 與 Chemical 部件都成功建立後，使用 owned Rule 沿用 `checkReceptorRule`。沒有受體參照的舊快照仍可沒有 Chemical。

這是來源與資料流層級的結論，不是修正已通過測試的宣稱。本次沒有執行 Go、gofmt、build、test、race 或 vet。`ownership-red.log` 是既有 RED receipt，不能算本 worker 的執行結果，也不能用來宣稱 Root 修正後仍失敗或已通過。

## CONFIRMED 問題與精確觸發

### 原始兩個缺口已由 live source 對準

- `plasticity.Rule` 的兩個可變欄位是 `GateReceptor` 與 `DecayEReceptor`（`plasticity/plasticity.go:66-83`）。`New` 在 `:124-145` 的 `:130` 先呼叫 `copyRule`，再於 `:131-132` 驗證並以已複製的 Rule 保存。`copyRule` 在 `:228-237` 對兩個非 nil pointee 各自複製值，`Config` 在 `:240-245` 再複製一次。
- `copyRule` 對同一個輸入指標會保留兩個欄位的數值語義，但會建立兩個 owned pointee。這符合計畫明定的值語義，不需要保留指標身份別名。`plasticity/rule_ownership_test.go:19-55` 也以 `both-shared` 覆蓋同一輸入指標的值隔離；測試本身未執行。
- `validateRule` 的 Gate 負值檢查仍在 `plasticity/plasticity.go:168-176`，Decay-E 負值檢查仍在 `:178-186`，且現在是複製後驗證。既有 Gate 負值案例位於 `plasticity/receptor_test.go:11-65`。來源未見負值繞過。
- `RestoreIndividual` 的 plastic runtime 建立在 `learning/individual.go:228-242`，Chemical runtime 建立在 `:244-282`；新增的 `:283-287` 在兩者完成後呼叫 `checkReceptorRule(restored.plastic.model.Config().Rule, restored.chemical)`。這使用已由 Model 擁有的 Rule，而不是快照外部指標。
- `checkReceptorRule` 在 `learning/individual.go:333-357` 對無受體參照在 `:345-346` 回傳 nil；有參照但無 Chemical 在 `:348-349` 回錯；每個索引在 `:351-355` 以 `0 <= idx < len(records)` 檢查。因此受體 0、1 在至少兩筆 records 時沒有特例，越界與負值會被拒絕。拒絕發生在 `RestoreIndividual` 回傳前，並回傳 nil。

### Enable／DisableChemistry 邊界仍一致

- `EnablePlasticity` 仍先在 `learning/individual.go:316-330` 建立已擁有 Rule 的 runtime，再以既有 `checkReceptorRule` 檢查，直到 `:329` 才替換 `i.plastic`。在呼叫期間禁止並行修改參數是 `learning/individual.go:129-133` 的既有前提，因此 `:326` 使用呼叫端 `c.Rule` 與 runtime 的值在此契約下相同，沒有足夠證據把它列為新 bug。
- `EnableChemistry` 在 `learning/chemistry.go:291-316` 先建立候選 runtime，於 `:301-304` 以 `i.plastic.model.Config().Rule` 檢查，再在 `:315` 提交；不相容的化學宣告不會取代舊狀態。`DisableChemistry` 在 `:327-344` 以同一 owned Config 檢查仍引用受體的 plastic rule，保留既有「先 DisablePlasticity 才能關閉」邊界。既有 gate/window 回歸在 `learning/receptor_gate_test.go:268-297`，本次未執行。
- `DisablePlasticity` 在 `learning/individual.go:360-370` 仍只清除 fast runtime。這使沒有受體參照的 rule 可以合法關閉 Chemical，也沒有因 deep copy 改變原有 lifecycle。

### 正常公式與快照格式未被修正碰到

- `plasticity/plasticity.go:28-45` 的 eligibility、plastic 更新順序及固定符號邊界，`StepWith` `:339-419` 與 `EffectiveWith` `:515-587` 都未出現在 live diff。正式 diff 只有 Rule 所有權複製與 Restore 交叉檢查，沒有改數值欄位或方程式。
- `IndividualSnapshot` 的結構與 `omitempty` 欄位在 `learning/individual.go:80-110` 未變；`Snapshot` `:399-431` 仍透過 `model.Config()` 輸出完整獨立設定。新增檢查只會拒絕未綁定的受體宣告，不改合法快照 JSON、合法 nil 缺省、正常啟用、閘門或時間窗計算。
- 新回歸的缺省矩陣 `learning/receptor_restore_ownership_test.go:159-180` 覆蓋 continuous、lif、mixed，且各測無 Chemical 與有 Chemical；這與 `checkReceptorRule` 的無參照合法分支一致。測試結果未由本 worker 執行。

## NEEDS INVESTIGATION 與缺少的證據

- **修正後 runtime 綠證據仍缺。** 本 worker 沒有執行 Go。現有 `evidence/MOD-05/receptor-rule-ownership-20261014/ownership-red.log:1-382` 只記錄 Root 提供的既有 RED receipt，包含 ownership 測試在 `:54`、learning 測試在 `:103` 與 `:145` 的原始失敗，以及缺省子測試的 PASS；它不能代替修正後的 New／Config／Enable／Snapshot／Restore 驗證。
- **合法 index 1 沒有直接 Restore 案例。** `receptorOwnershipSeed` 固定以 index 0 建立（`learning/receptor_restore_ownership_test.go:23-33`）。`TestReceptorCopyMutationPreservesLearning` 的 `:73-81` 把 `seed` 或 Snapshot 副本改成 1 來測隔離，但實際恢復的個體仍由 index 0 建立；`TestRestoreReceptorBindingMatchesEnable` 的 `:121-127` 只把值改成越界 2。來源 `checkReceptorRule` 已能由長度 2 接受 0 與 1，但尚缺一個直接以 index 1 序列化、Restore、再接續的 runtime 證據。這是覆蓋缺口，不是目前已確認的 source bug。
- **learning 路徑沒有帶受體的 `stdp_pair` Restore／Snapshot 案例。** `plasticity/rule_ownership_test.go:32-59` 直接覆蓋 `RuleHebbianRate` 與 `RuleSTDPPair` 的 New／Config；但 `learning/receptor_restore_ownership_test.go:49-108` 的三核心矩陣由 `compositionIndividual` 的 Hebbian rule（`learning/consolidation_composition_test.go:18-70`，啟用在 `:48`）建立。既有 `learning/plasticity_test.go:270-336` 有 LIF 的 stdp 與快照，但沒有受體欄位。`copyRule` 與 `checkReceptorRule` 對 rule kind 沒有分支，故目前沒有 source 反例；若 ticket 91 要把兩種 rule kind 都算進 learning 邊界，仍需補證據。
- **Decay-E 負值沒有新的 runtime 回歸。** live `validateRule` 在 `plasticity/plasticity.go:178-180` 明確拒絕負值，既有測試目前明示 Gate 負值（`plasticity/receptor_test.go:22-25`）。在本輪禁止執行 Go 的限制下，不能把該來源判讀升格成修正後測試通過。
- **三核心有效性目前只能確認矩陣存在。** 新測試的正常所有權矩陣在 `learning/receptor_restore_ownership_test.go:49-108` 迴圈三個 profile、三種 refs、三個 boundary；拒絕矩陣在 `:110-158` 迴圈三個 profile、三種 refs、三個問題；缺省矩陣在 `:159-180` 迴圈三個 profile。混合核心的逐節點 event 選擇由 live `learning/core.go:504-517` 定義，`stdp_pair` 對 continuous／mixed 的拒絕由 `learning/individual.go:291-305` 定義。這些資料流與矩陣相符，但沒有本 worker 的實際執行結果，不能宣稱三核心逐位通過。

## Root 骨架是否保持正常公式與合法快照

**來源層級：CONFIRMED 保持。** 修正未改正常公式、Rule 的數值欄位、快照 schema 或 JSON tag；只增加兩個 pointee 的深複製及 Restore 的既有交叉驗證。合法 nil、index 0，以及來源上可接受的 index 1 都沿用既有判斷，無受體參照的舊缺省仍可缺少 Chemical。

**執行層級：NEEDS INVESTIGATION。** 沒有執行 Go，也沒有把 Root 的 receipt 當成自己的測試。仍需 Root 以目前四個檔案和同一 HEAD 實跑三核心正常／缺省／拒絕、合法 index 1 Restore 接續、帶受體 stdp 的 learning 邊界，以及完整工程檢查。

## 讀過的檔名、行號與完整 SHA256

完整 HEAD：`71033a8dce5663c2c6135798b8884872ef8ed2b0`。目前 `.git/HEAD`：`ref: refs/heads/codex/receptor-chemistry-training`。

| 檔名 | 核對行號 | SHA256 |
| --- | --- | --- |
| `AGENTS.md` | 1-72 | `142445fa0a80972d74ab4aa33b018027df4e7214b01d117800fa0f611cedf5a5` |
| `ENG.md` | 197-255 | `c705912363988e378ba1b39d1a546440f61116336c333ac6816a7ded9c575a15` |
| `evidence/MOD-05/receptor-rule-ownership-20261014/plan.md` | 1-25 | `72a8c619e27265427cb469286f4a6e2a96a56d9e992f64648cf574e34c339dcb` |
| `docs/tickets/91-receptor-rule-ownership-and-restore.md` | 1-16 | `c936cde0a4abacc63165a5f6aa2756067a8983859bd718376cb93973d69676f3` |
| `plasticity/plasticity.go` | 1-590; 124-245; 339-419; 475-587 | `ab73f4e5e448b8299fbd46aa2cf9ac0f81085298b5b0a8c8b865c2e15ae3ebb0` |
| `learning/individual.go` | 80-185; 194-450; 644-825; 867-885 | `531d3ebba6829853a2b8a3eefaf81f744668a281c4afd926170898b5c88486d9` |
| `learning/chemistry.go` | 225-344 | `4e6164cbafd843c56c357558e9b8a580598b607daeccffbfc74a714052a52032` |
| `learning/network.go` | 14-105; 470-517 | `56fe942df2304d60d13fe46221b43b407d8f343e6e7792258d8a7be751d2f2eb` |
| `learning/core.go` | 470-517 | `fbf4f8238b352a9b6351c5207b92c9b0c4e30e50fd8e2b7dce507ced0855ece8` |
| `plasticity/rule_ownership_test.go` | 1-77 | `c5a5538d39df539460bacc4276b96106737de7c19b73e21752e0cf2da70a84b8` |
| `learning/receptor_restore_ownership_test.go` | 1-181 | `875dc3d0a221e8d37dbc5be404feb0973de3c9fa339d3d442225fd1379ed1793` |
| `learning/consolidation_composition_test.go` | 18-70 | `4622c659e33a063e66f1da5c5a7598471afdbc1e5865b2748acb51ba7f197823` |
| `learning/receptor_gate_test.go` | 204-247; 268-297 | `07ba3a525bc69403b1a268ac07309541208a548115af2f7b2421913e477eaa90` |
| `plasticity/receptor_test.go` | 11-67 | `6109cfeb6740fe4db58b010dd371a1d05cd528d5eb70cd9f9aa88b2755d4b211` |
| `learning/plasticity_test.go` | 240-336 | `f39dd9dc5d73374ec54bf8675bc45e34fc7e5a6974c688817964fd3acd3a962e` |
| `evidence/MOD-05/receptor-rule-ownership-20261014/ownership-red.log` | 1-382 | `2f2acca960ea608915f391ef0a3cbfe0f50fb3ec4830f161e31f6af2aef1e8b9` |

## 唯一實際修改檔

本 worker 唯一新增：`runs/.tmp/receptor-rule-ownership-20261014/review/fix-review.md`。沒有修改來源、測試、證據、Git、設定或既有 `report.md`。沒有執行 Go、網路、Git 寫入、commit、stash、reset。
