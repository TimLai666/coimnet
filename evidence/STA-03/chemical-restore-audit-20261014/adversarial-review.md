# Final adversarial review

## Problem

本輪只審查 HEAD `00bc7016c9380c23ab2154caac10666d798581a5` 到目前工作樹的 bundle presence 變更：`checkpoint/bundle.go`、`checkpoint/chemical_episode.go`、`checkpoint/task_bundle.go`、新增 `checkpoint/bundle_presence.go`，以及 frozen `checkpoint/bundle_presence_test.go`。檢查目標是四種公開 loader 對原始缺欄位／null 的拒絕、合法 Chemistry.Resources Map、4097 大陣列、legacy `SourceStates`／`InputNodes`／`Delays`／`Capacity`，並嘗試從 manifest schema/type routing 找公開驗證繞路。

## Done

### Scope

**Scope CLEAN。** 目前差異只落在指定的三個既有正式檔、指定的新 helper、指定的 frozen test，以及 `evidence/STA-03/chemical-restore-audit-20261014/source-fixed.json` 指紋檔；未發現額外正式程式漂移。

**CONFIRMED P1/P2：無。** 源碼沒有形成已確認的公開 loader 接受缺欄位、錯誤接受 required null、合法 Chemistry Map 被 generic walker 誤拒，或可由 manifest routing 繞過 presence 驗證的缺陷。

### 四個公開 loader 的實際路徑

- `LoadModelPackageBundle`、`LoadIndividualBundle`、`LoadTrainingBundle`、`LoadTaskTrainingBundle` 分別把固定型別指標傳給 `readBundle`（`checkpoint/bundle_kinds.go:38-55,86-108,142-167`；`checkpoint/task_bundle.go:61-85`）。
- `readBundle` 先驗證要求的 `kind`、manifest kind、檔案 checksum 與 document checksum（`checkpoint/bundle.go:279-327`），再於 strict decode 前執行 `bundleDocumentPresenceApplies`／`checkBundleDocumentPresence`（`checkpoint/bundle.go:326-330`）。
- `bundleDocumentPresenceApplies` 對 model、individual、training 同時要求固定 document schema 與固定 Go destination type，task 則要求 `*TaskTrainingSnapshot`（`checkpoint/bundle_presence.go:14-31`）。公開 loader 沒有可注入另一種 destination type 的入口。
- model、individual 的 manifest document schema 不符時，helper 不會因 routing 被誤套用，但 public loader 隨後在 `checkpoint/bundle_kinds.go:45-50,93-99` 拒絕；manifest kind 不符則更早在 `checkpoint/bundle.go:340-345` 拒絕。因此以 schema/type 改路由不能跳過公開驗證。

### Presence 與 null

- Individual 與 model document 由 helper 包成既有 single-file envelope，再重用 `requireIndividualFields`／`requireModelPackageFields`（`checkpoint/bundle_presence.go:38-76`）。`checkUniqueJSONRejectNull` 先拒絕顯式 null，既有 required-object walk 再拒絕 required key 缺失。
- Training document 走 `checkTrainingSnapshotRequiredFields`（`checkpoint/bundle_presence.go:45-46`；`checkpoint/chemical_episode.go:77-94`）。required scalar、struct、required array key 的缺失會由 generic walker 在 strict decode 前拒絕；`updates`、optimizer shape 等其餘語意則由 `RestoreTrainer` 在 `checkpoint/bundle_kinds.go:152-167` 接續驗證。
- Task document 的 `task_schema`、`config`、`training` 由 `checkTaskTrainingDocumentFields` 要求存在且非 null，nested training 走同一個 training presence helper（`checkpoint/task_bundle.go:135-160`）。
- Chemistry 的 `source_step`、`part`、ChemicalPart required fields、resources object、pending feedback 與 source controller `window`／`filled` 由 `checkEpisodeChemistryAt` 和既有 `requireIndividualChemical` 驗證（`checkpoint/chemical_episode.go:141-197`；`checkpoint/individual.go:261-313`）。這些檢查在 strict decode 與 runtime restore 前發生。

### 合法 Chemistry Map

`ChemicalPart.Resources` 是 `map[string]float64`（`learning/chemistry.go:59-71`），generic `checkRequiredFields` 本身仍不支援 Map。變更沒有把 Map 塞回 generic walker，而是先取出 `training.config.chemistry`，用 `checkEpisodeChemistryAt`／`requireIndividualChemical` 檢查 resources 必須是非 null object，逐值拒絕 null，再把 chemistry 從 validation copy 移除（`checkpoint/chemical_episode.go:96-138,162-163`）。原始 JSON 仍交給 strict decoder，沒有丟失 Map 內容。

因此合法 task-training Chemistry producer 由 `SaveTaskTrainingBundle` 寫出的 Map 有專用檢查路徑；model package 仍由 `canonicalModelPackage` 明確拒絕 `Config.Chemistry`（`checkpoint/package.go:223-228`），不會形成合法 model 的 Map false rejection。standalone episode 也維持同一個 chemical-aware path（`checkpoint/chemical_episode.go:50-74`）。

### 4097 numeric arrays 與 placeholders

`bundleSkeleton` 對長度至少 4096 的一維 numeric slice 把資料移到 `arrays.bin`，document 只留下空 skeleton（`checkpoint/bundle.go:490-500`）。presence 檢查在 `restoreBundleArrays` 前執行（`checkpoint/bundle.go:326-335`），所以不會要求 document 內的原始長度。既有 generic array walker 接受已存在的 `[]`，byte slice 也保留 base64／空 byte slice 規則；`restoreBundleArrays` 隨後要求 skeleton 長度為零並填回 manifest 宣告的長度（`checkpoint/required.go:102-114,122-155`；`checkpoint/bundle.go:428-455`）。

對 `omitempty` 的大陣列，document 可能沒有欄位，但 manifest entry 仍可在 decode 後找到 struct field 並還原；這是欄位本來就允許省略的情況，不是 required presence bypass。required 欄位缺 key 則在 helper 階段拒絕。

### Legacy omission

- `SourceStates` 的 absent 是既有 legacy fresh boundary（`learning/chemical_source_state.go:9-19`）；individual chemical validator 對 absent 保持合法，present list 才逐項檢查。
- `InputNodes` 是 `omitempty`，nil 表示完整 core 接收 encoded observations（`learning/network.go:26-42`）；generic walker 不會把省略誤判為 required 缺失。
- `dynamics.Config.Delays` 是 `omitempty`，空值由 dynamics 使用零 delay 的既有規則補足（`dynamics/continuous.go:14-19,105-113`）；model／individual presence walk 允許省略。
- `ModelPackage.Capacity` 是 `omitempty` 的舊 package 欄位；`normalizeModelPackage` 不會捏造 report，canonical validation 對 nil 保持合法（`checkpoint/package.go:73-84,245-253,411-433`）。

## Changed

| 檔案 | 變更摘要 |
|---|---|
| `/Users/timlai/Developer/coimnet/runs/.tmp/chemical-restore-audit-20261014/review/final-adversarial-review.md` | 新增本輪對抗性源碼審查報告。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| `git diff 00bc7016c9380c23ab2154caac10666d798581a5 -- checkpoint/bundle.go checkpoint/chemical_episode.go checkpoint/task_bundle.go` | 三個正式變更檔 | 本輪重新實際執行，exit code `0`；輸出確認 `bundle.go` 的 presence insertion、`chemical_episode.go` 的 chemistry-aware validation refactor、`task_bundle.go` 的 nested training helper，未帶出其他路徑。 |
| source read | `checkpoint/bundle_presence.go`、`checkpoint/bundle_presence_test.go` | 已讀取 helper、四種 loader fixture、4097 arrays、legacy omissions 與 source-state preservation case。測試未執行，沒有以測試空白推論通過。 |
| caller inventory | `checkpoint`、`tasks`、`examples`、`experiment`、`internal` | 已確認公開 bundle callers 使用固定 loader，未發現可把任意 destination type 傳入的公開入口。 |
| source fingerprint check | `evidence/STA-03/chemical-restore-audit-20261014/source-fixed.json` | 808 檔指紋檔未修改；本輪直接審查檔案指紋與檔案內容相符。 |
| Go／測試／server／Git write | CoImNet repo | 未執行；沒有 commit、stash 或 reset。 |

## Verified

### Direct source fingerprints

| 來源 | SHA-256 |
|---|---|
| `checkpoint/bundle.go` | `ff6bea1da0c6fa22aada2d58b1c6a6f626bb3d4be8c40842773effdf3c2bd80b` |
| `checkpoint/chemical_episode.go` | `0417604598b6109d83dc17343e387acaaf9946e48cd768704f7dc9f78938254c` |
| `checkpoint/task_bundle.go` | `8927e171f40a7019aec7a9b075617ca7bd26322aa1d5a5f2fd39dbc2debe64bf` |
| `checkpoint/bundle_presence.go` | `32e8cc05bc3208463890dc0a88961645d2ecc2811e20e0fc4ccf1c96f01cad2b` |
| `checkpoint/bundle_presence_test.go` | `6a89dc00a4a5a80f1cc9979faf6ff05daff96873dc2b6a5fd461fe6edadb0114` |
| `evidence/STA-03/chemical-restore-audit-20261014/source-fixed.json` | `51cc738b46c3b2c6c5fc4e705473dc81d861df7e4a19c2b1edd66f80f09048f9` |

### 實際讀過的路徑

`checkpoint/bundle.go`、`checkpoint/chemical_episode.go`、`checkpoint/task_bundle.go`、`checkpoint/bundle_presence.go`、`checkpoint/bundle_presence_test.go`、`checkpoint/bundle_kinds.go`、`checkpoint/individual.go`、`checkpoint/package.go`、`checkpoint/required.go`、`checkpoint/presence.go`、`learning/network.go`、`learning/chemical_episode.go`、`learning/chemistry.go`、`learning/chemical_source_state.go`、`dynamics/continuous.go`、`modulation/config.go`、`modulation/chemistry.go`、`modulation/controller_state.go`、相關公開 callers，以及 `evidence/STA-03/chemical-restore-audit-20261014/source-fixed.json`。

## Notes

- 這是 source-only review。Root 回報的 29 pass、48 份舊 Individual 新程序一致與完整 engineering 結果未被本報告當作源碼缺陷證據，也未重新執行。
- 舊版報告中的 baseline SHA 是文字誤植，該行不能視為正確 diff 的證據；本輪以完整正確 SHA 重新執行上述唯讀命令並取得 exit code `0`，因此以下結論以本次實際輸出為準。
- 未驗證項目只有本輪明確禁止的 runtime 行為、Go 編譯與測試執行；因此沒有把任何未執行結果寫成新的 CONFIRMED finding。
- source-fixed 指紋檔與指定正式檔、helper、frozen test 的目前內容一致；沒有 DRIFT 或 MISSING 項目。

## Next

無。若完整 engineering 後出現與上述 source path 矛盾的 loader 結果，再以具體 artifact、manifest、document bytes 與錯誤位置另開缺陷。

Skills：無
