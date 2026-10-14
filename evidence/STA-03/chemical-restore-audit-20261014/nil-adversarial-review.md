# Chemical nil-collection adversarial review

## Problem

本輪只審查第一版 presence 修正後，`checkpoint/chemical_episode.go` 的第二版變更。審查基準是 `00bc7016c9380c23ab2154caac10666d798581a5`，並以 `source-fixed.json` 與 `source-fixed-v2.json` 核對來源。目標是確認 direct `TrainingSnapshot`／task bundle 的 `resources` 與 `pending_feedback` 必須存在，只有明示的 `null` 集合可在 validation-only copy 視為空集合；single-file episode 維持嚴格 null 規則。

## Done

### Scope

**Scope CLEAN。** `source-fixed.json` 有 808 筆指紋，`source-fixed-v2.json` 有 809 筆。兩者差異只有新增 `checkpoint/bundle_chemical_nil_test.go`，以及 `checkpoint/chemical_episode.go` 的指紋更新；未發現第二版把其他正式檔或 frozen test 納入變更。未見 DRIFT 或 MISSING。

**CONFIRMED P1/P2：無。** 源碼沒有形成 direct training／task 接受缺少必填集合、錯誤接受非空 map 或 feedback 中的 null／錯型元素、放寬 source window／state 步數，或讓 single-file episode 接受原本禁止的 null 的可具體重現缺陷。

### 旗標路由

- Single-file `decodeDocument` 在 `checkpoint/checkpoint.go:400-407` 先呼叫 `checkEpisodeRequiredFields`。該入口在 `checkpoint/chemical_episode.go:63-74` 把 `allowNilCollections` 固定傳為 `false`。`checkEpisodeChemistry` 的直接 wrapper 也在 `checkpoint/chemical_episode.go:143-145` 固定使用 `false`。
- Direct training bundle 由 `checkpoint/bundle_presence.go:45-46` 呼叫 `checkTrainingSnapshotRequiredFields`，再於 `checkpoint/chemical_episode.go:85-93` 以 `true` 呼叫 `trainingPresenceValidationCopy`。
- Task bundle 由 `checkpoint/task_bundle.go:158-159` 把 nested `training` 送進相同 direct-training helper，因此同樣只在這條 bundle 路徑使用 `true`。
- 沒有看到公開入口把 `true` 傳給 single-file 路徑；Individual checker 仍使用原有 `requireIndividualChemical`，本輪未修改。

### Validation-only copy 的界線

`trainingPresenceValidationCopy` 在 `checkpoint/chemical_episode.go:123-140` 先要求存在的 chemistry 邊界完成 `checkEpisodeChemistryAt`，再只從驗證副本刪除 chemistry。`checkEpisodeChemistryAt` 在 `:164-171` 只有 `allowNilCollections == true` 時才建立 `chemicalPartValidationCopy`。

`chemicalPartValidationCopy` 在 `checkpoint/chemical_episode.go:212-227` 逐鍵複製 `part`，只對**已存在且 raw value 是 JSON null** 的 `resources` 改成 `{}`，以及 `pending_feedback` 改成 `[]`。它不新增缺少的鍵，也不改其他欄位。原始 document 仍由 bundle reader 後續 strict decode（`checkpoint/bundle.go:326-335`），所以解碼後的 nil map／nil slice 語意保持。

### 缺欄位、錯型與非空元素

- `requireIndividualChemical` 在 `checkpoint/individual.go:268-280` 對 `config`、`state`、`resources`、`pending_feedback` 使用 required-object／struct checks；因此兩個集合缺欄位時，即使 validation copy 沒有替換，也會在 required-object 階段拒絕。
- `resources` 必須是 object；`checkpoint/individual.go:297-306` 逐項拒絕 null value。把它寫成 array、scalar 或含 null 元素不會被 `{}` replacement 掩蓋，會在 object／element 檢查或後續 strict decode 拒絕。
- `pending_feedback` 必須是 array；`checkpoint/individual.go:308-312` 交由 `checkArrayElements` 檢查，`checkpoint/required.go:122-155` 對 null element 或非 array 型別拒絕。`null` 整體只在 direct bundle 的驗證副本轉成空 array。
- `source_step` 的 null 仍在 `checkpoint/chemical_episode.go:148-154` 拒絕。存在的 `source_states` 仍須是完整陣列，controller 的 `window` 與 `filled` 仍在 `:174-199` 要求存在且非 null。`source_states` 自身的 null 也仍由 `requireIndividualChemical` 在 `checkpoint/individual.go:283-293` 拒絕。
- Chemical `state` 的必填 `steps` 等欄位仍走 `checkRequiredFields`（`checkpoint/individual.go:277-280`；`checkpoint/required.go:51-69`），本輪沒有用 nil 集合旗標跳過 state walk。

### Source evidence

`jq` 比對 808／809 筆指紋只得到兩筆差異：新增 nil regression test，以及 `checkpoint/chemical_episode.go`。目前檔案與 v2 指紋相符，且兩份 frozen test 指紋均相符。

## Changed

| 檔案 | 變更摘要 |
|---|---|
| `/Users/timlai/Developer/coimnet/runs/.tmp/chemical-restore-audit-20261014/review/nil-adversarial-review.md` | 新增本輪第二版 nil-collection source review。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| source read | `checkpoint/chemical_episode.go` | 已讀第二版旗標路由、validation copy、chemistry/source boundary 檢查及第一版 diff。 |
| source read | `checkpoint/bundle_chemical_nil_test.go` | 已讀 frozen regression cases：兩種 bundle、兩個 null collection、缺欄位、nonempty 對照與 single-file null 拒絕。未執行測試。 |
| source read | `evidence/STA-03/chemical-restore-audit-20261014/plan.md`、`nil-collections-fix.md`、`root-review.md` | 已讀補充契約與範圍；提供的 runtime／工程結果未當作本報告的 source proof。 |
| fingerprint comparison | `source-fixed.json`、`source-fixed-v2.json` | 已核對 808→809 筆，差異只有指定新 frozen test 與 `chemical_episode.go`。 |
| Go／測試／server／Git write | CoImNet repo | 未執行；沒有 commit、stash 或 reset。 |

## Verified

### SHA-256

| 來源 | SHA-256 |
|---|---|
| `checkpoint/chemical_episode.go`（第一版 `source-fixed.json`） | `0417604598b6109d83dc17343e387acaaf9946e48cd768704f7dc9f78938254c` |
| `checkpoint/chemical_episode.go`（目前第二版／`source-fixed-v2.json`） | `754cff1d3baacb22dd8c2b5435abe215c2dcf8eb019fbceeaf25bd75e039e37e` |
| `checkpoint/bundle_chemical_nil_test.go`（frozen） | `a579f204724b75c9aa348fb903c802ec3e97491b8719f9417f8f3d043c51607e` |
| `checkpoint/bundle_presence_test.go`（既有 frozen） | `6a89dc00a4a5a80f1cc9979faf6ff05daff96873dc2b6a5fd461fe6edadb0114` |
| `evidence/STA-03/chemical-restore-audit-20261014/source-fixed.json` | `51cc738b46c3b2c6c5fc4e705473dc81d861df7e4a19c2b1edd66f80f09048f9` |
| `evidence/STA-03/chemical-restore-audit-20261014/source-fixed-v2.json` | `216e541806f712a937b5617811b3fb7b639ee67d81831fcf0edf5738dc19cdbf` |

### 實際讀過的路徑

`checkpoint/chemical_episode.go`、`checkpoint/checkpoint.go`、`checkpoint/individual.go`、`checkpoint/required.go`、`checkpoint/bundle_presence.go`、`checkpoint/task_bundle.go`、`checkpoint/bundle.go`、`checkpoint/bundle_kinds.go`、`checkpoint/bundle_chemical_nil_test.go`、`checkpoint/bundle_presence_test.go`、`checkpoint/chemical_episode_validation_test.go`、`learning/chemistry.go`、`learning/chemical_episode.go`、`modulation/chemistry.go`、`evidence/STA-03/chemical-restore-audit-20261014/plan.md`、`nil-collections-fix.md`、`root-review.md`、`source-fixed.json`、`source-fixed-v2.json`。

## NEEDS INVESTIGATION

無 source-level 疑慮。Go 執行、測試、server 及完整 engineering 結果依本輪限制未獨立驗證，這些是未驗證項目，不是已確認缺陷。

## Notes

本報告不把 Root 提供的 RED／green、舊 OCR 模型相容性、SDK MSE 或完整 engineering 結果當作本輪 source finding。判斷只根據目前正式來源、既有 helper 契約、指紋與 frozen test 內容。

## Next

無。

Skills：無
