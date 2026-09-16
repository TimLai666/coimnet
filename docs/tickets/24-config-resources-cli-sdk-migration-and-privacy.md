# 24 — 使用者可以用嚴格展開的組態、啟動前的資源預估、完整的 CLI 與 SDK，並安全地遷移格式與保護敏感資料

Epic：介面、平台與規模；保存與安全

User Story：使用者可以寫一份嚴格驗證、全部展開、覆寫來源可查的組態，啟動前看到包含邊狀態歷史與外圍
模型的記憶體預估並在不足時被拒絕而不是被暗中裁圖；可以用 SDK 做核心工作並取消長任務而不洩漏資源；
可以用 CLI 完成資料、模型、訓練、恢復、評估、對照、效能、匯出與報告，每個命令都有端到端與失敗測試；
維護者可以把舊格式遷移成新檔而不動原件並拿到資訊損失報告；日誌自動遮罩、金鑰只存參照、提交前掃描、
預設不外傳。

Blocked by：16 模型包（`model inspect`）、17 最佳化器（組態的學習規則區塊）、20／21（組態的調節器區塊，
可先以「未宣告」通過）、23（`evaluate` 子命令由該票接上）、22（`ablate` 子命令由該票接上）

Status：第一階段已完成並驗證（2026-09-16 完成，2026-09-17 重跑驗證，見「第一階段證據」）；第二、三階段未開始。契約已於
2026-09-15 定案，分三階段派工，第一階段不依賴 20 以後的票。

對應需求：OPS-03（未知欄位拒絕，覆寫順序與來源可查，dry-run 不學習不呼叫教師）、OPS-06（計算包含邊
狀態歷史與外圍模型；不足時拒絕，不自動縮減）、OPS-01（API 例子可執行，錯誤不 panic，取消後釋放資源）、
OPS-02（每個命令有端到端與失敗測試；沒有固定假輸出）、STA-05（不覆寫原檔，有相容性檢查與資訊損失
報告）、STA-06（日誌遮罩、資料路徑及提交掃描，預設無外傳）。主規格 15.2、15.5、15.6、16.1–16.4、
17.1、17.2、18.4、20.1、21、22.4。

## Root 決策（2026-09-15）

### 第一階段：組態與資源預估（OPS-03、OPS-06）

1. **組態（新套件 `config`）**：`coimnet-config/v1` JSON 文件，經 `internal/strictjson` 讀取（未知欄位、
   重複鍵、null 必填都拒絕）。必備區塊照主規格 16.3：`data{version, filter}`、`model{kind ∈ continuous|lif,
   package 或 graph 路徑}`、`time{unit, step}`、`sharing{parameter_sharing}`、`trainable{trainable, masks}`、
   `signals{encoding, mapping}`、`task{name, loss}`、`learning{rules: gradient|hebbian_rate|stdp_pair,
   options}`、`modulation{sources, chemistry, receptors, effects}`（可為 `null` = 未啟用）、`reset{...}`、
   `teacher{...}`（可為 `null`）、`seed`、`device{kind ∈ cpu}`、`resources{max_memory_mib, max_temp_mib,
   max_runs}`、`splits{train, validation, test}`、`output{dir}`。每個 `name` 欄位都必須對應有版本的
   實作（例如 `generator: delayed-correlation/v1`），不存在的名稱直接拒絕，不得替代。
   **展開與來源**：`Load(ctx, path string, env []string, cli []string) (Resolved, error)`；`Resolved{Config;
   Provenance map[string]string /*json pointer → default|file|env|cli*/; Secrets map[string]SecretRef}`；
   優先序固定 `cli > env > file > default`；環境變數形如 `COIMNET_<SECTION>_<FIELD>`；CLI 覆寫形如
   `--set section.field=value`；展開後的設定以 `Resolved.Marshal()` 輸出（含 `schema_version`），
   金鑰欄位型別為 `SecretRef{Env string}`，輸出永遠是 `{"ref": "env:NAME"}`，測試證明值不出現。
2. **資源預估（新套件 `resources`）**：`Plan{Nodes, Edges, StateDim, Individuals, HistorySteps int;
   Precision string ∈ f32|f64; Optimizer string ∈ none|adamw; PlasticEdges, EligibilityEdges int;
   MaxDelay int; EncoderValues, ReadoutValues int; ModulationRegions, ModulationChannels, Receptors int;
   ReplayItems, ReplayItemBytes int; BufferFactor float64}` → `Estimate(p Plan) (Report, error)`，
   `Report{Items []Item{Name string; Bytes uint64; Formula string}; TotalBytes uint64}`；項目至少涵蓋
   主規格 17.1 列出的：圖索引（`(N+1+E)×8`）、基礎參數（`E×w`）、最佳化器（AdamW `3×E×w`）、目前
   神經狀態（`N×C×w×B`）、快速連線（`PlasticEdges×w×B`）、近期參與紀錄（`EligibilityEdges×w×B`）、
   延遲（`N×(MaxDelay+1)×w×B`）、反向歷史（`2×E×w×T×B`，即 17.1 的 `8BE×T` 對 f32）、編碼器、讀出、
   調節器（`Regions×Channels×w` + 受體）、重播、暫存（`BufferFactor × 前面總和`）。每個項目附公式字串。
   `Check(r Report, limitBytes uint64) error`：超過就回錯，錯誤訊息含完整清單；**不得**修改 Plan。
   算術示例測試：主規格 17.1 的 `E = 15,000,000`、f32、AdamW → 最佳化器＋參數 `16E` ≈ 240 MB；
   加 `T = 128` 的雙邊狀態歷史 → 約 15.36 GB（測試釘住這兩個數）。真實圖：`E = 25,563,197,
   N = 165,122, T = 0, B = 1` 的預估寫進證據並與 NAT-01 實測 RSS 4.38 GB 並列（只陳述，不宣稱吻合）。
3. **`--dry-run`**：CLI `run --config X --dry-run` 讀組態、展開、檢查依賴（模型包或圖檔存在、
   generator 名稱有實作）、資料存在、資源預估對 `resources.max_memory_mib`；輸出 JSON（`schema_version:
   coimnet-dry-run/v1`）含展開設定、來源、預估與檢查結果；**零副作用**：不寫參數、不動快照、不呼叫教師
   （測試：以計數的假教師介面證明呼叫次數為 0；輸出目錄沒有新檔）。
4. 證據：`evidence/OPS-03/`、`evidence/OPS-06/`（fixture；OPS-06 另附真實圖預估）。

### 第二階段：SDK 與 CLI（OPS-01、OPS-02）

5. **SDK**：根套件 `coimnet` 新增可執行範例 `example_sdk_test.go`（`Example*` 函式，`// Output:`
   驗證）覆蓋主規格 16.1 列的能力：載入圖、建立核心、建立個體、註冊自訂編碼／動態／學習／調節／任務
   （以既有介面：`signal` adapter、`modulation.Source`、`plasticity.Rule`、`experiment` 的任務 generator
   註冊表 `experiment.RegisterGenerator(name, version, fn)`）、逐步觀察與輸出、回傳事後結果、訓練、
   評估、干預（22 未完成時範例標明）、觀察局部狀態、保存與恢復。**不 panic**：對每個公開建構子與方法寫
   表格式非法輸入測試（nil、負長度、NaN、空序列、錯形狀），全部回 `error`；**取消釋放**：對
   `simulate.Run`、`Trainer.Step`、`Individual.Advance`、`download.Fetch`、`connectome.Build` 各一測：
   啟動後取消，等待返回，`runtime.NumGoroutine()` 回到基線 ± 0，`runtime.ReadMemStats` 經兩次 GC 後
   `HeapInuse` 回到基線的 1.1 倍內；不公開裸指標（審查清單：所有回傳的 slice 都是副本，測試改回傳值
   不影響內部）。
6. **CLI**：補齊主規格 16.2 的指令群並統一 JSON 輸出 `schema_version`：`doctor`（既有）、`data fetch /
   inspect / import / validate / derive`（既有）、`model inspect`（讀模型包或個體快照：拓撲、參數統計、
   模式、證據清單、映射、資源預估）、`model validate`（重算指紋與校驗、拒絕不相容）、`run`（依組態
   執行 train／infer／evaluate，支援 `--dry-run` 與取消：SIGINT 後返回非零並寫出部分報告）、
   `resume`（既有，改吃組態並拒絕「新預設改變原實驗」：快照的 options 與組態不一致時拒絕，除非
   `--allow-option-change`）、`evaluate`（23 接上；本票先掛骨架並在未實作時明確拒絕）、`ablate`
   （22 接上，同上）、`benchmark`（量測匯入、前向、反向、局部學習、調節與快照的耗時與 RSS，輸出
   JSON，附 `uptime`）、`export`（匯出模型包或報告為 JSON；不支援的格式明確拒絕，列出支援清單）、
   `examples list / run`（既有，補顯示資料與外部依賴）、`report`（把設定、成績、限制、來源、完成
   狀態組成一份 JSON + Markdown）。每個命令：一個端到端測試（真的執行並檢查輸出內容，不是檢查字串
   「ok」）與至少一個失敗測試（錯參數、缺檔、壞校驗）。`--help` 對每個命令列參數、預設、範例與錯誤。
7. 證據：`evidence/OPS-01/`、`evidence/OPS-02/`。

### 第三階段：遷移與敏感資料（STA-05、STA-06）

8. **遷移（`checkpoint.Migrate`）**：`Migrate(ctx, src, dst string, target string) (MigrationReport, error)`：
   `dst` 不得等於 `src`、不得已存在（不覆寫任何檔）；讀 `src` 用嚴格解碼；相容性檢查以 schema 的
   主次版本（`name/vMAJOR[.MINOR]`）：未知必要欄位、未知演算法版本、未註冊擴充都報錯並指名；寫出新檔
   後回報 `MigrationReport{SourceSHA256, TargetSHA256, SourceSchema, TargetSchema; FieldChanges []
   {Path, Kind ∈ added|renamed|dropped|defaulted|retyped, Note}; PrecisionMapping []{Path, From, To};
   InformationLoss []string}`。第一個真實遷移：聯集出現前的連續個體檔（`neural` 直接是 `State`）→
   `coimnet-individual-checkpoint/v1` 聯集形（`upgradeIndividualNeural` 的規則），報告 `FieldChanges`
   一條 `renamed neural → neural.continuous, added neural.core`；同版本遷移是逐位複製並回報
   「無資訊損失」。CLI `checkpoint migrate`。測試：遷移後 `src` 的 SHA-256 與內容逐位不變。
9. **敏感資料（新套件 `internal/redact` + 腳本）**：`redact.Writer`（包住 log 輸出，遮罩 `sk-…`、
   `Bearer …`、`token=…`、`api_key`、以及 `raw_text`／`audio`／`image` 欄位的內容，只留長度與 SHA-256
   前 8 碼）；所有 CLI 日誌與 `report` 經過它；`config.SecretRef` 只存參照（第一階段已做）；
   `scripts/scan-commit.sh`：掃 `git diff --cached` 的新增行是否含金鑰樣式、掃新增檔是否 > 5 MiB 或位於
   `data/` 原件目錄、掃是否含 `teacher_response` 原文；提供 `.githooks/pre-commit` 範本（不自動安裝，
   README 說明 `git config core.hooksPath`）。**預設無外傳**：測試以 `go list -deps` 證明只有
   `download` 與（未來的）`teacher` 套件 import `net/http`，其他套件都不；框架沒有遙測（`grep` 測試）。
   快照讀取（既有 `internal/fileio.ReadRegular` 的大小上限與路徑檢查）補一測：symlink 跳脫與超大檔拒絕。
10. 證據：`evidence/STA-05/`、`evidence/STA-06/`；文件：README（CLI 表、鉤子安裝）、ENG（config、
    resources、redact 段）、`docs/individual-state.md`（遷移段）。

## 契約摘要

```go
package config
type SecretRef struct { Env string }
type Resolved struct { Config Config; Provenance map[string]string; Secrets map[string]SecretRef }
func Load(ctx context.Context, path string, env []string, cli []string) (Resolved, error)
func (r Resolved) Marshal() ([]byte, error)

package resources
type Plan struct { ... }; type Report struct { Items []Item; TotalBytes uint64 }
func Estimate(p Plan) (Report, error); func Check(r Report, limitBytes uint64) error

package checkpoint
func Migrate(ctx context.Context, src, dst, target string) (MigrationReport, error)

package redact
func NewWriter(w io.Writer) io.Writer
```

## 驗收

- [x] 第一階段：未知欄位拒絕、來源追蹤四層、金鑰只存參照、不存在的 generator 拒絕；資源預估兩個算術示例
  釘住、真實圖預估記錄、超限拒絕且 Plan 不變；`run --dry-run` 零副作用與教師呼叫次數 0；
  `go test`、race、vet；`evidence/OPS-03/`、`evidence/OPS-06/`。
- [ ] 第二階段：SDK 範例可執行、非法輸入不 panic、五個取消釋放測試；每個 CLI 命令端到端與失敗測試、
  `--help` 完整；`evidence/OPS-01/`、`evidence/OPS-02/`。
- [ ] 第三階段：遷移不覆寫原件、報告含前後指紋與資訊損失、一個真實遷移；遮罩、掃描腳本、無遙測與
  `net/http` 範圍測試、symlink 與超大檔拒絕；`evidence/STA-05/`、`evidence/STA-06/`；文件。

## 第一階段證據（2026-09-16 完成，2026-09-17 重跑全部驗證）

`config`、`resources` 與 `internal/cli/config_run.go` 完成，先寫失敗測試再實作，紅燈輸出留在
`evidence/OPS-03/red-config.log`、`evidence/OPS-03/red-dry-run.log` 與 `evidence/OPS-06/red-resources.log`。

- `evidence/OPS-03/`：`verification.json`、`test.log`、`cli-exit-status.log`，以及可重跑的
  `fixture/`（三神經元模型包、組態、成功與被拒絕的兩份 `coimnet-dry-run/v1` 報告）。
- `evidence/OPS-06/`：`verification.json`、`test.log`、`male-full-estimate.json`。
- 文件：`docs/config-schema.md`，`config/config_test.go` 的 `TestSchemaDocumentListsEveryTopLevelKey`
  會把 `Defaults()` 的每個頂層鍵與每個任務產生器名稱對照該文件。
- 驗證：`gofmt -l .` 無輸出、`go vet ./...`、`go test -count=1 ./...`、
  `go test -race -count=1 ./config/ ./resources/ ./internal/cli/` 全部通過，
  `go run ./cmd/coimnet run --help` 列出三個旗標、預設值與範例。
- 兩個釘住的算術示例：`E = 15,000,000`、f32、AdamW 的參數加最佳化器為 240,000,000 bytes（`16E`）；
  加上 `T = 128`、`B = 1` 的反向歷史為 15,360,000,000 bytes。全圖預估（165,122 節點、25,563,197 邊、
  f64、AdamW、`T = 0`）為 1,026,490,952 bytes，與 NAT-01 實測 4,378,574,848 bytes 最大 RSS 並列記錄，
  不宣稱兩者吻合。

### 與票面的差異

1. **`learning.options` 不含 `trainable` 與 `masks`。** 票面同時列出 `trainable{trainable, masks}` 區塊與
   「`learning.Options` 形狀」的 `options`，兩者會讓同一個開關在文件裡出現兩次，其中一份必然被忽略。
   本階段改用 `config.LearningOptions`：欄位名稱與 `learning.Options` 相同，但拿掉 `trainable` 與
   `masks`，那兩個欄位只在 `trainable` 區塊宣告。
2. **`task` 分成 `name` 與 `version` 兩個欄位**，註冊表的鍵是 `name/version`，例如
   `delayed-correlation/v1`。
3. **秘密指到的 `COIMNET_` 變數不當成覆寫。** 環境覆寫的命名規則是 `COIMNET_` 加欄位路徑，而金鑰參照
   也常指向 `COIMNET_` 變數，兩者會撞名。被 `secret` 指名的變數一律跳過；如果它同時對應到某個組態欄位，
   載入直接回錯誤要求改名，避免同一個變數有兩種意思。
4. **`cmd/coimnet/main.go` 改用 `cli.ExitCode(err)`**（原本固定 `os.Exit(1)`），否則票面要求的
   「0／2／1」三種退出狀態無法真的出現在行程上。其他子命令的錯誤仍然是 1，行為不變。
5. **`teacher.kind` 註冊表只有 `http/v1`，而且沒有任何客戶端實作它。** 這是為了讓組態能宣告教師並驗證
   「呼叫次數為 0」。`run --dry-run` 對已宣告的教師一律回報 `skipped`。
6. **`resources.Plan.BufferFactor` 在 dry-run 固定為 0**，組態沒有對應欄位。報告的檢查說明與證據都寫明
   沒有估算暫存額度。
7. **`Nodes == 0` 時 `graph_index` 為 0**，而不是 `(0+1+0)*8 = 8`，因為沒有節點就沒有那個索引陣列；
   `Edges > 0` 而 `Nodes == 0` 的計畫直接拒絕。這是「零計畫總和為零」那條測試成立的前提。
8. **`splits` 是三個相加為 1 的比例**，容許 1e-9 浮點誤差。票面只寫了 `{train, validation, test}`。
9. **`resources.max_memory_mib` 允許 0**，表示沒有任何計畫可接受。fixture 的預估只有 192 bytes，
   任何整數 MiB 都不會觸發拒絕，需要 0 才能驗證超限路徑。

## 依據

- 主規格 15.2（恢復檢查項目）、15.5（敏感資料）、15.6（相容性與遷移）、16.1（SDK 能力與規則）、
  16.2（CLI 契約表、dry-run）、16.3（組態必備內容、覆寫優先序、金鑰參照）、16.4（名稱必須對應
  實作）、17.1（資源預估項目與算術示例）、17.2（不足時拒絕）、18.4、20.1（沒有固定假輸出）、21、22.4。
