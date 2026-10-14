# Bundle presence scope audit

## Problem

本補充稽核只處理 bundle presence 來源契約。目標是確認省略必填但可為零的欄位，是否從 chemical individual bundle 延伸到 `training`、`model_package`、`task_training`，並核對四種 `readBundle` 入口、四種 standalone JSON presence validator、`arrays.bin` placeholder、legacy `SourceStates` 與 `Map` 的可達性。

稽核基準是 `/Users/timlai/Developer/coimnet` 的 HEAD `00bc7016c9380c23ab2154caac10666d798581a5`，來源索引為 codebase-memory project `coimnet-chemistry-ocr`，並以 `evidence/STA-03/chemical-restore-audit-20261014/source-before.json` 的來源指紋核對。沒有執行 Go、測試或 Git 寫入，因此本檔不把任何新候選標成已實跑確認的缺陷。

## Done

### 四種 bundle 與 standalone validator inventory

| kind | 寫入與讀取入口 | bundle document 的 presence 行為 | 對應 standalone JSON validator |
|---|---|---|---|
| `model_package` | `checkpoint/bundle_kinds.go:14-56` 的 `SaveModelPackageBundle`／`LoadModelPackageBundle` | `readBundle` 只做 strict decode，之後由 `canonicalModelPackage` 驗證；沒有 `checkRequiredFields` 的 document walk。 | `checkpoint/package.go:436-492` 的 `decodeModelPackageDocument`／`requireModelPackageFields`，會對 `ModelPackage` 呼叫 generic `checkRequiredFields`。 |
| `individual` | `checkpoint/bundle_kinds.go:58-109` 的 `SaveIndividualBundle`／`LoadIndividualBundle` | decode、array restore 後交給 `normalizeIndividualSnapshot` 與 `learning.RestoreIndividual`；沒有 raw required-field walk。三個 chemical fields 的 bundle 缺省問題在原報告範圍內。 | `checkpoint/individual.go:76-169` 的 `LoadIndividual`／`DecodeIndividual` 先呼叫 `requireIndividualFields`；chemical part 由 `:261-313` 的 `requireIndividualChemical` 自訂檢查。 |
| `training` | `checkpoint/bundle_kinds.go:111-168` 的 `SaveTrainingBundle`／`LoadTrainingBundle` | decode、`learning.RestoreTrainer`、normalize、topology check；沒有 raw required-field walk。 | `checkpoint/checkpoint.go:157-180,372-414` 的 `Load`／`decodeDocument` 走 `checkEpisodeRequiredFields`；該 helper 先把 chemical boundary 拿出單獨驗證，再對移除 chemistry 的 `State` 走 generic walker。 |
| `task_training` | `checkpoint/task_bundle.go:24-85` 的 `SaveTaskTrainingBundle`／`LoadTaskTrainingBundle` | manifest 只有此 kind 在 `checkpoint/bundle.go:296-300` 先走 required walk；document decode 會觸發 `TaskTrainingSnapshot.UnmarshalJSON`，在 `checkpoint/task_bundle.go:109-145` 檢查 task 三個頂層欄位與 nested training。 | 沒有另一個 typed single-file task-training loader。`DecodeTaskConfig` `checkpoint/task_bundle.go:88-106` 對 caller 的 struct 呼叫 generic walker；`LoadPayload` 則明確把 payload 欄位驗證留給 caller（`checkpoint/checkpoint.go:204-207,237-287`）。 |

### NEEDS INVESTIGATION：Training bundle 的 required zero-valued options

**來源可達候選，未實跑，候選 P2。** `learning.Options` 的 `weight_decay`、`clip_norm`、`truncation`、`trainable` 在 `learning/trainer.go:27-35` 都沒有 `omitempty`，因此 standalone episode validator 會要求欄位存在。可是 bundle 的 `LoadTrainingBundle` 在 `checkpoint/bundle_kinds.go:140-167` 沒有 presence walk，只把解碼結果交給 `learning.RestoreTrainer`。

`RestoreTrainer` `learning/trainer.go:194-224` 只檢查 schema、`NewTrainer`、optimizer shape、finite/range、optimizer steps、accumulator 與 shared state。`validateOptions` `learning/trainer.go:620-642` 明確允許 `WeightDecay == 0`、`ClipNorm == 0`、`Truncation == 0`；`validateTrainable` `learning/trainer.go:644-650` 只拒絕無 LIF core 卻要求 `Theta`，沒有拒絕全 false 的 `Trainable`。

具體 producer 是 `SaveTrainingBundle` `checkpoint/bundle_kinds.go:111-137`：它接受一個先由 `learning.RestoreTrainer` 接受、且例如 `WeightDecay > 0`、`ClipNorm > 0`、`Truncation > 0`、至少一個 trainable group 為 true 的 `TrainingSnapshot`，再把 `trainer.Snapshot()` 寫成 bundle。公開觸發輸入是只刪除 `document.json` 的 `training.options.weight_decay`、`clip_norm`、`truncation` 或整個 `trainable`，同時重算 document 與 manifest 指紋以通過檔案完整性檢查。來源流程預測 strict decode 會填入零值，`RestoreTrainer` 仍可接受，因而改變 optimizer、gradient horizon 或 trainable mask；但本輪禁止執行，故尚未把此預測升級為 CONFIRMED。

同一路徑中，刪除 `schema_version` 會被 `LoadTrainingBundle` 直接拒絕；刪除 optimizer arrays 通常在 `RestoreTrainer` 的 shape check 拒絕；刪除非零執行狀態的 `updates` 會因 `optimizer.steps[i] > s.Updates` 拒絕。`loss_scale`、`accumulate_steps`、`schedule`、`ranges`、`masks`、`recompute` 是 `omitempty` 欄位，且各自註解定義缺省或零值語意，不能把它們列為同一個缺口。

### NEEDS INVESTIGATION：Model bundle 的 evidence registry 缺省

**來源可達候選，未實跑，候選 P2，影響是 provenance metadata 遺失。** `ModelPackage.EvidenceRegistry` 在 `checkpoint/package.go:73-84` 是無 `omitempty` 的 required slice。standalone `requireModelPackageFields` `checkpoint/package.go:483-492` 會要求它存在，但 `LoadModelPackageBundle` `checkpoint/bundle_kinds.go:35-56` 的 bundle document 沒有同一個 required walk。

具體 producer 是 `NewModelPackage`／`SaveModelPackageBundle` `checkpoint/package.go:87-113`、`checkpoint/bundle_kinds.go:14-32`，其 `evidence` 可為非空相對路徑。刪除有效 bundle 的 `evidence_registry` 後，strict decode 會得到 nil slice；`canonicalModelPackage` 的 `ownedEvidenceRegistry` `checkpoint/package.go:232-234,317-337` 接受空 slice，`normalizeModelPackage` `checkpoint/package.go:411-433` 再把 nil 轉成空 slice，topology 不會因此改變。這會讓宣告中 publisher stands behind 的 evidence paths 消失。未實跑前只列來源候選，不宣稱讀取已成功。

其他 model required 欄位不能用同樣理由直接列缺口：`units` 缺欄位會在 `validateUnits` `checkpoint/package.go:276-283` 拒絕，`compatible_versions` 缺欄位會在 `validateVersionList` `checkpoint/package.go:286-314` 拒絕，參數或拓樸陣列缺失通常會在 trainer shape／topology check 拒絕。`capacity` 明確是 `omitempty` 的舊格式可省略欄位，不能誤報。

### NEEDS INVESTIGATION：TaskTraining 的 Chemistry.Resources Map false rejection

**來源可達候選，未實跑，候選 P2。** `checkRequiredFields` `checkpoint/required.go:25-27` 只接受 struct 根，`checkpoint/required.go:115-116` 的 default 對 `reflect.Map` 回傳 `unsupported field kind map`，沒有 Map 分支。

`learning.Config.Chemistry` 是合法的 episode training boundary（`learning/network.go:57-60`）。其 `ChemicalPart.Resources` 是 `map[string]float64`（`learning/chemistry.go:59-71`）。`SaveTaskTrainingBundle` `checkpoint/task_bundle.go:24-46` 只要求 `validatedTaskTraining` 成功；`validatedTaskTraining` `:169-182` 透過 `learning.RestoreTrainer`，而 `prepareChemicalEpisode` `learning/chemical_episode.go:29-104` 會驗證 resources 名稱與 finite 值，沒有禁止這個 Map。這是可具體構造的合法 producer。

讀回時，`readBundle` `checkpoint/bundle.go:323-331` 在 array restore 前以 strict decoder 解碼 `TaskTrainingSnapshot`。`TaskTrainingSnapshot.UnmarshalJSON` `checkpoint/task_bundle.go:109-145` 在同一時間對完整 nested `learning.TrainingSnapshot` 呼叫 `checkRequiredFields` `:133-134`。若 `training.config.chemistry` 存在，walker 會走到 `ChemicalEpisode.part.resources`，在 Map kind 失敗。因此來源鏈預測合法的 task-training chemistry bundle 會被自己的 document validator 擋住，但本輪沒有 runtime probe，狀態仍是 NEEDS INVESTIGATION。

這個 Map 問題不應擴張到所有入口：

- standalone episode `Load` 的 `checkEpisodeRequiredFields` `checkpoint/chemical_episode.go:45-105` 先刪除 chemistry 再走 generic walker，並在 `:107-160` 透過 `requireIndividualChemical` 手動檢查 resource object，所以來源上有 Map-safe 路徑。
- `LoadTrainingBundle` 沒有 `TaskTrainingSnapshot.UnmarshalJSON` 這個 generic nested walk，故本項目前只指向 `task_training`。
- `canonicalModelPackage` `checkpoint/package.go:223-228` 明確拒絕 `Config.Chemistry`，所以 model package 的 Map error 只會出現在本來就不合法的 model input，沒有合法 model producer 可達證據。
- `DecodeTaskConfig` 的 generic Map 限制只對 caller 自己提供的 struct 生效；本輪沒有 CoImNet typed task config producer 使它成為已確認儲存缺陷。

### arrays.bin、placeholder 與 legacy 規則

`bundleSkeleton` `checkpoint/bundle.go:455-510` 對長度至少 4096、且元素是支援 numeric dtype 的一維 slice，將原欄位放進 `arrays.bin` manifest entry（`:490-493`），document 欄位改成長度為零的 `[]`。`readBundle` 的順序是先 strict decode document，再由 `restoreBundleArrays` `checkpoint/bundle.go:414-452` 要求 skeleton slice 仍為零長度並填回原始陣列；因此任何可重用的 bundle presence walker 必須只判斷「欄位是否存在及 JSON token 是否合法」，不能在 document 階段要求原始長度或內容，也不能把存在的 `[]` 當成缺欄位。

既有 `checkRequiredFields` `checkpoint/required.go:102-114` 對存在的空 array 會通過，對 JSON `null` slice 也明確保留合法性；這與 standalone episode 的 nil slice wire semantics 一致（`:11-24`）。bundle writer 對 extracted 長陣列實際寫 `[]`，小或 nil 的 legacy slice 仍可能是 `null`，兩者都不可被新 walker 誤拒絕。真正缺少 key 仍要拒絕，因為 `:103-108` 有 presence check。

`SourceStates` 在 `learning/chemistry.go:64-66` 是 `omitempty`，註解明定 absent 代表 legacy fresh boundary。它不是 required field；省略不得誤報。若欄位存在，個體 validator 與 chemical episode validator 仍須檢查每個 source state 的 nested controller shape。這個 legacy 規則與 arrays placeholder 是兩個不同的 presence 語意，不能以「所有 slice 都必須出現」取代。

### 可直接沿用的最小方案（僅建議，不在本輪改碼）

1. 以現有 `presenceObject`／`jsonField`／`requiredObject`（`checkpoint/presence.go`）與 `checkRequiredFields` 為底層 walker，讓 required-field 判斷在 bundle document decode 後、`restoreBundleArrays` 前執行；保留目前對 `[]`、`null` slice、`omitempty` 的語意，絕不做 array length check。
2. `model_package` 可直接沿用 generic walk，因合法 producer 已在 `canonicalModelPackage` 拒絕 Chemistry；`evidence_registry` 會因此回到 standalone 與 bundle 一致的 required presence。
3. `training` 應增加一個 chemical-aware wrapper：把 `training.config.chemistry` 按 `checkEpisodeRequiredFields` 的方式抽出，以 `requireIndividualChemical` 驗證 Map-safe chemical object，再對移除 chemistry 的 `TrainingSnapshot` 走 generic walker。這樣可補 options zero-valued omission，而不會誤擋合法 Chemistry。
4. `task_training` 已有 `TaskTrainingSnapshot.UnmarshalJSON` 作為最小切入點；其 `:133-134` 的 generic call 應重用同一個 chemical-aware wrapper，避免另立 public API。task top-level required fields、array placeholder 與 manifest checks 維持現有位置。

本補充沒有新增 CONFIRMED P1/P2。以上三項均是具體 producer 與 consumer 的 source-reachable NEEDS INVESTIGATION，需由 Root 的既有實際 probe 或等價的受控讀回證據決定是否升級。

## Changed

| 檔案 | 變更摘要 |
|---|---|
| `/Users/timlai/Developer/coimnet/runs/.tmp/chemical-restore-audit-20261014/review/bundle-presence-scope.md` | 新增本補充稽核報告；未修改正式程式、測試、原報告或 Git 狀態。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| codebase-memory graph search | `coimnet-chemistry-ocr` | 已定位四種 bundle、standalone validators、RestoreTrainer、Chemistry 與 presence walker。 |
| source read and SHA-256 | HEAD `00bc7016c9380c23ab2154caac10666d798581a5` | 只讀核對，指紋列於下方；與 `source-before.json` 對應來源一致。 |
| Go、測試、Git 寫入 | CoImNet repo | 未執行。沒有 commit、stash、reset 或正式程式修改。 |

## Verified

- 已核對 `readBundle` 的 manifest/document decode、task-only manifest required walk、array restore 順序與四種 bundle kind 的 producer/consumer。
- 已核對 standalone model、individual、episode training 與 generic payload/config validator 的實際入口。
- 已用 `learning.Options` 的 JSON tags、`validateOptions`、`RestoreTrainer` 及 bundle load 順序判斷 Training candidate 的來源可達性，沒有把存在測試當成通過證據。
- 已核對合法 Chemistry episode 的 `Config.Chemistry`、`ChemicalPart.Resources` Map、task bundle producer 與 task document consumer 的交叉路徑。
- 已核對 `>=4096` numeric slice 的 `[]` skeleton、`arrays.bin` restore 前後順序，以及 `SourceStates` 的 `omitempty` legacy fresh boundary。

### Direct source fingerprints

| 來源 | SHA-256 |
|---|---|
| `checkpoint/bundle.go` | `cf12e264c8101e47f5a65f360d0fa5f98335bdfc3765aa6bd8a987961b19754f` |
| `checkpoint/bundle_kinds.go` | `83be130d07f6d86a75eeccaa8ce35e7c876a507f20b112523ab6d79ba5cbfb52` |
| `checkpoint/task_bundle.go` | `88f93b73dc851e25fceed77c58fa59a3ce22653a021d3f8d72d732593fa45cdf` |
| `checkpoint/required.go` | `83429cc883848a8d1627eede475c44c20bd6e00bf4861a27e9aef31c1ab3d686` |
| `checkpoint/presence.go` | `9c6bbd1df12a96bd720a14649ad4440cdf2e3c20fe440414a92666fc7dd82590` |
| `checkpoint/package.go` | `a6a4140c90c4063af996a4eb8499f535738a2036586f95fcedf28b8326134de5` |
| `checkpoint/checkpoint.go` | `219a5e583361ae21ab8c0b2cc483d27b1736b1c395bbf4dea79089fe3b34c951` |
| `checkpoint/chemical_episode.go` | `0555a9a3573d019a95dc5f5aef3e17e25c759cc32191f0eb088fb2981523b3f8` |
| `checkpoint/individual.go` | `8b26f07b3b8ca656ab4c996d552bc42edbc80abcd278e0317519052471d2ef7a` |
| `learning/trainer.go` | `0a2f2cce3a72e00df4198767553f4a5c58785b3d3eaaae2af1b3731c95928943` |
| `learning/network.go` | `56fe942df2304d60d13fe46221b43b407d8f343e6e7792258d8a7be751d2f2eb` |
| `learning/chemical_episode.go` | `928fb5dfea73a2ace4be49debf6084aad77173c5b6ff390b6f0df40cb4d941b7` |
| `learning/chemistry.go` | `4e6164cbafd843c56c357558e9b8a580598b607daeccffbfc74a714052a52032` |

## Notes

- 本檔是來源契約補充，不取代 `/Users/timlai/Developer/coimnet/runs/.tmp/chemical-restore-audit-20261014/review/restore-review.md`，也沒有修改它。
- `source-before.json` 另含 806 份來源指紋；本檔只列直接讀取並形成上述判斷的檔案。
- Root 的三個 chemical missing-field runtime probes 尚未在本輪讀取結果中採用；即使它們稍後完成，也只會決定各 candidate 是否升級，不改變本檔已列出的來源邊界。

## Next

Root 應以既有 probe 或受控的 document mutation/readback 證明三個 NEEDS INVESTIGATION candidate 是否實際觸發，再決定是否併入主報告；在此之前不要把它們宣稱為 CONFIRMED P1/P2。

Skills：無
