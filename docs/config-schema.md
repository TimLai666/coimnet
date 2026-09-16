# 執行組態 `coimnet-config/v1`

一份組態描述一次執行要用哪個模型、哪個任務、哪些學習規則，以及可以花多少記憶體。`config.Load` 讀進檔案後會展開成完整文件，並記下每一個值是由哪一層設定的。`coimnet run --config FILE --dry-run` 會印出展開後的文件、來源表、檢查結果與啟動前的記憶體預估。

讀取規則是嚴格的。未知欄位、大小寫別名重複的鍵、必填欄位寫 `null`、註冊表裡沒有的名稱，都會回錯誤並指出是哪一個欄位。框架不會改用相近的名稱，也不會自動縮小任何數量。

檔案上限為 1 MiB，經 `internal/strictjson` 解碼，巢狀深度上限 64 層，尾隨資料會被拒絕。

## 四層覆寫

同一個欄位可以被四層設定，後面的蓋掉前面的：

1. `config.Defaults()` 的預設值。
2. 組態檔。檔案只需要寫要改的欄位，沒寫的沿用預設。
3. 環境變數。名稱是 `COIMNET_` 加上 JSON pointer，斜線換成底線並全部大寫。`resources.max_memory_mib` 對應 `COIMNET_RESOURCES_MAX_MEMORY_MIB`，`seed` 對應 `COIMNET_SEED`。
4. `--set section.field=value`，可以重複使用，同一個欄位以最後一次為準。

環境變數與 `--set` 的值會依目標欄位的型別解析。`COIMNET_TIME_STEP=oops` 會回錯誤，不會變成零。帶 `COIMNET_` 前綴但對不到任何欄位的變數也是錯誤，不會被忽略。

字串陣列（目前只有 `learning.rules`）可以用逗號分隔整批換掉，例如 `--set learning.rules=gradient,hebbian_rate`。其他陣列與物件只能在檔案裡宣告。

`modulation` 與 `teacher` 沒有宣告時是 `null`。這兩段是整段開或整段關的結構決定，環境變數與 `--set` 不能把關著的那一段打開。

## 來源表

展開後的每一個葉節點都有一筆來源紀錄，鍵是 JSON pointer，值是 `default`、`file`、`env` 或 `cli` 其中一個。

```
"/output/dir": "cli",
"/seed": "env",
"/model/package": "file",
"/time/unit": "default"
```

葉節點的定義和 JSON 輸出一致：純量、秘密參照、空陣列，以及沒有宣告的 `modulation` 與 `teacher`。陣列由某一層整個設定時，該層擁有裡面每一個元素。

`Resolved.Marshal()` 會把來源表寫成文件裡的 `provenance` 欄位，展開後的文件因此可以直接存檔並重新讀入。重新讀入時 `provenance` 會被接受但不採用：來源是那一次載入的結果，每次載入都重新計算，所以重讀展開文件得到的每一筆來源都是 `file`。

## 秘密只存參照

需要憑證的欄位型別是 `SecretRef`，在文件裡只有一種寫法：

```json
"secret": {"ref": "env:COIMNET_TEACHER_TOKEN"}
```

`config` 不會去讀那個環境變數的值，也不會把值放進展開後的文件。直接寫字串會被拒絕，錯誤訊息指出是哪一個欄位，而且不會把被拒絕的內容重印一次。被秘密指名的 `COIMNET_` 變數不會當成覆寫使用；如果它剛好又對應到某個組態欄位，載入會直接回錯誤，要求換一個變數名稱，避免同一個變數有兩種意思。

## 各段欄位

預設值取自 `config.Defaults()`。`model.package`、`model.graph` 與 `output.dir` 沒有預設，因為沒有任何預設知道要讀哪個模型檔、要寫到哪裡，必須由檔案、環境變數或 `--set` 補上。

### 頂層

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `schema_version` | 字串 | `coimnet-config/v1` | 必填。其他值會被拒絕。 |
| `seed` | 非負整數 | `0` | 執行的亂數種子。 |

### `data`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `version` | 字串 | `malecns-v1.0` | 資料發布版本，寫進報告用。 |
| `filter` | 字串 | `annotated_neurons` | 選出這張圖的過濾條件。 |

這兩個欄位都是宣告，`config` 不會開啟任何資料檔。

### `model`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `kind` | 字串 | `continuous` | `continuous` 或 `lif`。 |
| `package` | 路徑字串 | 無 | `checkpoint.LoadModelPackage` 讀得到的模型包。 |
| `graph` | 路徑字串 | 無 | `connectome.Load` 讀得到的圖檔。 |

`package` 與 `graph` 必須剛好宣告一個。兩個都寫或兩個都不寫都會被拒絕，框架不會替使用者挑一個。

### `time`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `unit` | 字串 | `model_step` | 目前只有 `model_step`。這是宣告，沒有任何程式把它換算成毫秒。 |
| `step` | 有限正數 | `1` | 一步的長度。 |

### `sharing`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `parameter_sharing` | 字串 | `per_edge` | `per_edge` 或 `per_type`。 |

### `trainable`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `trainable` | 物件 | 連續核心的六個群組旗標，`theta` 以外都是 true | 對應 `learning.Trainable`：`encoder`、`weights`、`bias`、`tau`、`theta`、`readout`。 |
| `masks` | 物件或 `null` | `null` | 對應 `learning.UpdateMasks`，用 `edges` 與 `nodes` 兩個布林陣列把某個群組縮到個別邊與節點。 |

參數群組旗標與遮罩只在這一段宣告。`learning.options` 裡沒有這兩個欄位，同一個開關不會在文件裡出現兩次，也就不會有一份被默默忽略。

### `signals`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `encoding` | 字串 | `insyra-linear/v1` | 觀測值的編碼。 |
| `mapping` | 字串 | `identity/v1` | 輸出的映射。 |

### `task`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `name` | 字串 | `delayed-correlation` | 與 `version` 合起來是 `name/version`，必須在任務產生器註冊表裡。 |
| `version` | 字串 | `v1` | 同上。 |
| `loss` | 字串 | `mse/v1` | 損失函數。 |

### `learning`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `rules` | 字串陣列 | `["gradient"]` | 至少一項，不可重複，每一項都要在學習規則註冊表裡。要凍結模型請用 `trainable`，不是清空這個陣列。 |
| `options` | 物件 | 見下表 | AdamW 與反向視窗設定，欄位名稱與 `learning.Options` 相同。 |

`learning.options` 的欄位：

| 欄位 | 型別 | 預設 | 限制 |
| --- | --- | --- | --- |
| `learning_rate` | 數字 | `0.01` | 有限且大於零。 |
| `beta1` | 數字 | `0.9` | `[0,1)`。 |
| `beta2` | 數字 | `0.999` | `[0,1)`。 |
| `epsilon` | 數字 | `1e-8` | 有限且大於零。 |
| `weight_decay` | 數字 | `0` | 有限且非負。 |
| `clip_norm` | 數字 | `1` | 有限且非負。 |
| `truncation` | 整數 | `0` | 非負。反向視窗，也是資源預估的 `T`。 |
| `loss_scale` | 數字 | `0` | 有限且非負，零表示一。 |
| `accumulate_steps` | 整數 | `0` | 非負，零與一都表示每一步都更新。 |
| `ranges` | 物件或 `null` | `null` | 對應 `learning.ParameterRanges`。 |
| `schedule` | 物件或 `null` | `null` | 對應 `learning.Schedule`。 |

### `modulation`

整段可以是 `null`，表示這次執行不開化學調節。宣告成物件時：

| 欄位 | 型別 | 說明 |
| --- | --- | --- |
| `sources` | 陣列 | 至少一項，每項是 `{kind, channels}`，`channels` 至少是 1。 |
| `chemistry` | 物件 | `{regions, channels}`，兩者至少是 1，決定濃度場的大小。 |
| `receptors` | 非負整數 | 受體數量。 |
| `effects` | 物件 | `{reward_kind}`，把原始獎勵轉成釋放量的規則。 |

### `reset`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `neural_at_episode_start` | 布林 | `true` | 每段開始時清掉神經狀態。 |
| `plastic_at_episode_start` | 布林 | `true` | 每段開始時清掉快速變化。 |
| `chemical_at_episode_start` | 布林 | `true` | 每段開始時清掉化學狀態。 |

### `teacher`

整段可以是 `null`，表示這次執行沒有外部教師。宣告成物件時：

| 欄位 | 型別 | 說明 |
| --- | --- | --- |
| `kind` | 字串 | 目前註冊表只有 `http/v1`。 |
| `endpoint` | 字串 | 呼叫會送到的位址，不可留空。 |
| `budget` | 物件 | `{max_requests, max_cost}`，兩者非負且有限。 |
| `secret` | 秘密參照 | `{"ref": "env:NAME"}`。 |

`http/v1` 目前只是可以被宣告的名稱，沒有任何客戶端實作它，教師套件是 ticket 27。`run --dry-run` 對已宣告的教師一律回報 `skipped`，並在 detail 裡寫明這次沒有呼叫過。

### `device`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `kind` | 字串 | `cpu` | 目前只有 `cpu`。 |

### `resources`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `max_memory_mib` | 非負整數 | `8192` | 啟動前預估的上限。超過就拒絕執行，不會縮圖也不會減少步數。填 0 表示沒有任何計畫可接受，會讓 `--dry-run` 印出預估後直接拒絕。 |
| `max_temp_mib` | 非負整數 | `40960` | 暫存空間上限。 |
| `max_runs` | 整數 | `1` | 至少 1。 |

`max_memory_mib` 是帳面上限，只計入宣告出來的陣列。Go 配置餘量、解碼後的 JSON 與執行環境都不在裡面，因此它不是程序 RSS 的上限。

### `splits`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `train` | 0 到 1 的數字 | `0.8` | 三個比例相加必須是 1，容許 1e-9 的浮點誤差。 |
| `validation` | 0 到 1 的數字 | `0.1` | 同上。 |
| `test` | 0 到 1 的數字 | `0.1` | 同上。 |

### `output`

| 欄位 | 型別 | 預設 | 說明 |
| --- | --- | --- | --- |
| `dir` | 路徑字串 | 無 | 實際執行時寫入的目錄。`--dry-run` 不會建立它，也不會在裡面留下任何檔案。 |

## 名稱註冊表

每一個 `name` 欄位都要對應到目前有實作的名稱。不在表裡的名稱會被拒絕，錯誤訊息會寫出欄位、查的是哪一個註冊表，以及該註冊表現有的全部名稱。

| 註冊表 | 欄位 | 目前的名稱 |
| --- | --- | --- |
| task generators | `task.name` 加 `task.version` | `delayed-correlation/v1`、`lif-threshold/v1` |
| losses | `task.loss` | `mse/v1` |
| encodings | `signals.encoding` | `insyra-linear/v1` |
| mappings | `signals.mapping` | `identity/v1` |
| learning rules | `learning.rules[]` | `gradient`、`hebbian_rate`、`stdp_pair` |
| modulation source kinds | `modulation.sources[].kind` | `external_timeline`、`neural_activity`、`internal_resource`、`replay` |
| reward kinds | `modulation.effects.reward_kind` | `relu`、`split` |
| model kinds | `model.kind` | `continuous`、`lif` |
| devices | `device.kind` | `cpu` |
| parameter sharing modes | `sharing.parameter_sharing` | `per_edge`、`per_type` |
| time units | `time.unit` | `model_step` |
| teacher kinds | `teacher.kind` | `http/v1` |

兩個任務產生器分別對應 `experiment.RunDelayed` 與 `experiment.RunLIFThreshold`。第一階段只解析名稱，不會執行它們。

## 最小可用的檔案

```json
{
  "schema_version": "coimnet-config/v1",
  "model": {"kind": "continuous", "package": "models/tiny.coimpkg"},
  "output": {"dir": "runs/fixture"}
}
```

其餘欄位全部取預設值。用這份檔案跑 `coimnet run --config FILE --dry-run`，來源表裡 `/model/package` 是 `file`，`/time/unit` 是 `default`。

## 和程式碼保持一致

`config/config_test.go` 的 `TestSchemaDocumentListsEveryTopLevelKey` 會把 `Defaults()` 序列化，逐一檢查每個頂層鍵都出現在本文件裡。新增一段設定時，同一次修改要把它寫進上面的表格，否則測試會失敗。
