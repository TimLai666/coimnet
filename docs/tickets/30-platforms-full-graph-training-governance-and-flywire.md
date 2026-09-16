# 30 — 使用者可以在目標平台用 CPU 參考路徑、在全圖上做短訓練與保存恢復、重現效能報告、從乾淨環境重跑一切；維護者可以核對需求、決策、能力界線與授權；研究者可以匯入獨立的 FlyWire 資料

Epic：介面、平台與規模；定位、依賴與交接；接線資料與匯入

User Story：使用者可以在各平台分別看到「編譯通過」與「實際執行通過」的標記，用單執行緒參考模式或
受控並行模式跑 CPU 路徑而結果逐位相同；可以在完整主要圖上做一次涵蓋宣告可訓練參數的反向更新、開啟
可塑性與調節、保存並完整恢復，並拿到峰值記憶體證據；可以重現把初始化、傳輸、暖機與穩態分開的效能與
活動量報告；接手者可以在乾淨環境依同一套設定重跑所有已宣稱的能力，缺少的項目明示受阻。維護者可以核對
需求追蹤與決策紀錄、讀到把初始化／人工圖／真實圖／工具輔助／核心能力分開的能力說明、查到授權盤點；
研究者可以把 FlyWire 資料匯成獨立命名空間的圖，不與 MaleCNS 無證據拼接。

Blocked by：24（組態、`benchmark`、`report`、`doctor`）、21／18（全圖上的調節與可塑性）、16（模型包）、
OPS-05 的裝置後端：**受阻於使用者決定後端技術與提供裝置存取**；DAT-06 的真實資料：**受阻於使用者
取得 FlyWire 授權資料**

Status：第一階段已驗證（2026-09-17）；第二～五階段待派工／受阻

對應需求：OPS-04（各平台編譯與實際執行分開標記；至少有參考環境的完整測試）、OPS-05（與 CPU 比對、裝置
更新／恢復測試及實際量測；未測不標通過）、OPS-07（真實資料統計、全圖前向／反向、可塑性／調節及峰值記憶體
有證據）、OPS-08（初始化、傳輸、暖機及穩態分開；無功率量測不宣稱能耗）、OPS-10（命令、設定、來源、硬體
與證據齊全；缺少項目明示受阻）、GOV-01（有需求追蹤與決策紀錄；沒有未授權刪除、遠端寫入或範圍縮減）、
GOV-04（初始化、人工圖、真實圖、工具輔助與核心能力清楚分開）、GOV-05（授權盤點可查；發布授權未確認時
不公開發布）、GOV-06（README、API、例子、格式、來源、風險、需求證據齊備，不依賴原對話）、DAT-06
（版本、ID 空間與映射獨立；禁止與 MaleCNS 無證據拼接）。主規格 1.3、1.4、2.1–2.3、3.2、4.3、4.4、5.1、
5.2、5.5、17.1–17.5、18.5、18.6、19.2、19.4、22.1–22.4。

## Root 決策（2026-09-15）

### 第一階段：CPU 參考路徑與治理文件（OPS-04、GOV-01、GOV-04、GOV-05、GOV-06）

1. **受控並行模式**：`dynamics.Config.Workers int`（0 或 1 = 單執行緒參考；> 1 = 依目標節點分區的
   worker pool，每個目標節點的加總順序固定為邊的宣告順序，與 worker 數無關），連續與 LIF 都支援；
   測試：`Workers ∈ {1, 2, 7}` 的 `Forward`／`Advance` 逐位相同；熱路徑不逐步建 map／字串（`go test
   -bench` 的 `allocs/op` 釘住為每步常數，寫進證據）；沒有每個神經元一個 goroutine（審查清單）。任何
   改變數值語意的加速（例如非固定加總順序）都要另設模式並附誤差報告；本票不引入這種模式。
2. **平台矩陣**：`scripts/platform-matrix.sh`：對 `linux/amd64`、`linux/arm64`、`darwin/arm64`、
   `windows/amd64` 做 `GOOS/GOARCH` 交叉編譯（`go build ./...` 與 `go vet`），對目前主機做完整
   `go test ./...`、`-race`、`go vet`、`go mod verify` 與可重現基準；輸出
   `evidence/OPS-04/platform-matrix.json`，每個平台一列 `{compiled: bool, executed: bool, go_version,
   uptime}`，`executed` 只有真的在該平台跑過測試才為 true（目前：macOS arm64 與 Ubuntu 的既有紀錄
   v25／v26，Windows 只有 compiled）。核心運行不依賴 Python（`go list -deps` 測試證明沒有 cgo 或
   外部程序依賴）。
3. **需求追蹤與決策紀錄（GOV-01）**：根套件加 `governance_test.go`：`docs/requirements-status.json`
   的 id 集合等於 `docs/handoff/requirements.json` + `docs/requirements-addendum.json`；每個 status 屬於
   主規格 18.6 的七個值；每個 `passed` 的 evidence 路徑存在且 JSON 含 `requirement`、`reproduction_command`、
   `environment`、`observed_result`、`test_log`；每張 ticket 的「Root 決策」段有日期。決策紀錄維持在
   `delivery-status.md` 的「決策紀錄」段，補上 2026-09-15 以後的決策（做完整規格、票 15–30 的 root
   決策指向各票）。「沒有未授權刪除、遠端寫入或範圍縮減」以 `AGENTS.md` 的授權段與 git 歷史為證，
   證據紀錄列出本專案從未執行的操作（建立遠端、發布、刪原件）。
4. **能力界線（GOV-04）**：README 的能力表改成四欄：`初始化模型／已訓練`、`fixture／real-subgraph／
   male-full`、`工具輔助／核心能力`、`證據路徑`；科學界線段落逐條對應主規格 3.2（接線圖不是已訓練模型、
   傳導物質預測 ≠ 作用正負、未知受體 ≠ 沒有受體、突觸數 ≠ 參數數、獎懲／損失／調節物質／荷爾蒙是不同
   概念、固定編碼器 ≠ 歸因完成、專案名稱不構成新學術分類）。`checkpoint.ModelPackage` 加 `Scale string
   ∈ fixture|real-subgraph|male-full` 與 `Trained bool`（必填；`Trained = false` 的包名稱必須以
   `initialized-` 開頭，載入時檢查），模型卡與實驗卡範本 `docs/model-card-template.md`、
   `docs/experiment-card-template.md`（欄位：名稱、規模、是否訓練、資料指紋、外圍模型、工具角色、
   證據等級、限制）。
5. **授權盤點（GOV-05）**：`docs/licenses.md`：`go list -m -json all` 產生的相依授權表（腳本
   `scripts/license-inventory.sh`，結果 commit 進 repo）、資料來源授權（MaleCNS 發布條款、FlyWire 條款
   的連結與取得日期）、移植或改寫的程式碼出處（若有）、字型與媒體素材（目前無）。**發布**：主規格 2.3
   建議 MIT 但不得代替使用者做法律選擇；本票不建立 `LICENSE`、不建立遠端、不打版本標記；
   `docs/release-checklist.md` 列出 22.4 的發布前確認項，狀態 `blocked_permission`（等使用者選定授權與
   發布授權）。
6. **單一主文件（GOV-06）**：`docs/INDEX.md` 作入口：README、ENG、`docs/handoff/`（主規格與需求副本）、
   API（`go doc` 指引與各套件 doc.go）、例子清單、檔案格式清單（每個 schema 版本一列）、來源與授權、風險
   與限制、需求證據（`docs/requirements-status.json`）、票與決策；`scripts/check-links.sh` 檢查所有
   Markdown 相對連結存在（進 `verify.sh`）。副本與主文不一致時以主文為準的規則寫進 INDEX。
7. 證據：`evidence/OPS-04/`、`evidence/GOV-01/`、`evidence/GOV-04/`、`evidence/GOV-05/`、`evidence/GOV-06/`。

### 第二階段：FlyWire adapter（DAT-06）

8. **獨立命名空間**：`connectome` 的資料集識別加 `flywire/<version>`；FlyWire 公開釋出的欄位表先以官方
   欄位建 adapter 欄位表（`docs/flywire-source-audit.md`：`connections`、`neurons`／`classification` 的
   官方欄位名、型別、取得日期、條款連結），再映射到本專案標準欄位；外部 ID 以 uint64 保留、JSON 用字串；
   內部連續索引與可逆對照保存；索引寬度由數量決定，不得無聲截成 32 位元（測試：超過 2^53 的 ID 往返、
   溢位失敗）。`data import --dataset flywire --manifest …` 產生獨立的 GraphStore 與報告，`DatasetManifest`
   含 schema 版本、資料集識別、來源版本、檔案角色與校驗、授權來源、取得時間、欄位映射、篩選條件、座標
   單位與轉換歷史；`NeuronRecord` 含命名空間、外部 ID、內部索引、類型、區域、來源與品質註記，未知保留
   為未知。
9. **禁止拼接**：任何把兩個資料集的圖、節點或參數合併的 API 都要求 `MappingEvidence{Source, Method,
   Coverage, Version}`；沒有就拒絕（測試：MaleCNS 圖與 FlyWire 圖的 `Merge`／`Compare` 在無證據時回錯；
   `simulate compare` 拒絕跨資料集的 named set）。本票不提供任何映射，因為沒有正式映射證據。
10. 真實資料：FlyWire 需要帳號與條款同意，狀態 `blocked_data`（等使用者提供下載後的檔案路徑與授權欄位）；
    fixture 走完整匯入、統計、保存、讀回。證據 `evidence/DAT-06/`。

### 第三階段：完整主要圖的短訓練與保存恢復（OPS-07）

11. **全圖流程**（macOS arm64，`data/` 的既有全圖 165,122 節點、25,563,197 邊）：`examples run
    full-graph-short-training`：以 16 的模型包（`male-full`、`initialized-…`）建立 `Individual`，
    `derived_release/v1` 參數集（13）給符號與權重（17 的 `SignsFromParameterSet`，unknown 政策明示），
    `InputNodes` 與 `ReadoutNodes` 用 14 的具名集合，短序列（例如 32 步、`Truncation = 8`），做**一次**
    涵蓋所有宣告可訓練參數（weights、bias、log_tau、theta_raw、encoder、readout）的反向更新（17 的
    遮罩全開），開啟 18 的可塑性（子集邊或全部，依記憶體預估）與 21 的化學狀態（1 區域 1 通道），
    原子保存個體快照（模型包 + 個體 + 訓練快照三種都寫），新程序完整恢復並續跑 4 步與不中斷執行逐項
    相等。全部節點都參與（不得只算一小部分節點卻稱全圖）。
12. **資源**：先用 24 的 `resources.Estimate` 預估（含邊參數、AdamW、梯度、可塑性邊狀態、反向歷史、
    化學狀態、緩衝），對 `resources.max_memory_mib` 檢查；不足就拒絕並記錄估算（不裁圖）。實測記錄
    峰值 RSS、各階段耗時、`uptime`；`evidence/OPS-07/verification.json` 列出真實資料統計（08 的報告）、
    全圖前向（12）、本票的反向更新、可塑性／調節啟用、快照與恢復、資源報告。硬體不足時標
    `blocked_hardware` 並保留精確指令、所需資源與預估來源。全腦多步長訓練與五類任務精度是另列的研究
    結果，不在本票宣稱。

### 第四階段：效能報告與乾淨環境重跑（OPS-08、OPS-10）

13. **效能與活動量報告**：24 的 `benchmark` 輸出 `coimnet-benchmark/v1`：每個階段（匯入、前向、反向、
    局部學習、調節、快照）分開記錄 `init`、`transfer`（CPU 為 0，欄位保留）、`warmup`、`steady` 的耗時
    與 RSS，活動量（每步放電數、平均速率、非零輸出比例），`energy: {measured: false, note: "no power
    measurement"}` 固定寫出（沒有功率量測就不宣稱能耗）；同 seed 同設定重跑兩次的耗時差異與活動量逐位
    相同寫進報告；fixture 與全圖各一份，全圖附 `uptime`。證據 `evidence/OPS-08/`。
14. **乾淨環境重跑**：`scripts/clean-env-verify.sh`：把 repo 匯出到暫存目錄（`git archive`），
    `go mod download` 與 `go mod verify`（鎖定依賴），依序執行 `doctor`、`go test ./...`（單元與數值）、
    人工延遲關聯、閾值訓練、調節干預（22）、離線教師（27）、快照恢復；有官方資料時再跑真實子圖與全圖
    檢查、五類任務與共同核心流程（28／29）、對照、效能與完成報告；每一步寫入 `evidence/OPS-10/run.json`
    `{step, command, config_hash, status ∈ passed|failed|blocked_data|blocked_hardware|blocked_permission,
    log}`；缺少資料或硬體的步驟標對應的 `blocked_*`，離線小測試通過不會把全圖或真實資料項目標通過。
    24 的 `report` 產生 22.3 的最終報告（repo 提交、依賴鎖定、需求逐項狀態、實際命令、完整／失敗／受阻
    測試、資料與模型指紋、硬體、數值結果、五類任務、持續學習矩陣、對照、生物機制證據等級、授權、限制），
    每個 `passed` 有證據路徑、每個受阻有原因、範圍與解除條件。證據 `evidence/OPS-10/`。

### 第五階段：裝置後端（OPS-05，blocked）

15. **能力偵測先做**：`backend` 套件：`Capabilities{ContinuousForward, SpikingForward, SparseBackward,
    VariableWeights, TeacherLoss, DeviceStateSave, Deterministic bool}`；`CPU` 後端全宣告 true；組態要求
    未支援的組合在執行前就回具體錯誤；CPU 回退必須明示（`FallbackReport`），且永遠不會在裝置失敗後重跑
    半次學習更新（更新交易序號測試）。這部分不需要 GPU，屬本票第一階段可做。
16. **裝置實作需要使用者決定**：後端技術（CUDA via cgo、Vulkan compute、WebGPU／wgpu-native、OpenCL）、
    目標機器（Ubuntu 1 的 RTX 4070 12 GB 已確認存在，但需要可用的遠端執行工具）、與 Go／Insyra 生態的
    整合方式。決定前狀態 `blocked_hardware`（CI 無 GPU）+ `blocked_permission`（未決定技術與存取）；
    決定後的驗收：稀疏前向／反向與 CPU 對照（1e-6 內或宣告誤差）、裝置狀態保存與恢復、傳輸／編譯／
    暖機／穩態量測、裝置記憶體執行前檢查，不把一般矩陣加速當成果蠅核心已加速。

## 契約摘要

```go
package dynamics   // Config.Workers int（0/1 = 單執行緒參考；>1 = 目標節點分區，加總順序固定）
package backend
type Capabilities struct { ContinuousForward, SpikingForward, SparseBackward, VariableWeights, TeacherLoss, DeviceStateSave, Deterministic bool }
type Backend interface { Name() string; Capabilities() Capabilities }
func Require(b Backend, need Capabilities) error   // 執行前拒絕不支援的組合
package checkpoint // ModelPackage.Scale string; ModelPackage.Trained bool
package connectome // MappingEvidence 與跨資料集拒絕
```

## 驗收

- [x] 第一階段：`Workers` 逐位相同、`allocs/op` 常數、平台矩陣 JSON（compiled／executed 分開）、無 Python
  依賴測試、治理測試（id 集合、狀態值、證據存在、決策日期）、README 四欄能力表與界線、模型包
  `Scale`／`Trained`／`initialized-` 規則、授權盤點、發布清單 blocked_permission、INDEX 與連結檢查；
  `go test`、race、vet；`evidence/OPS-04/`、`evidence/GOV-01/`、`GOV-04/`、`GOV-05/`、`GOV-06/`。
- [ ] 第二階段：FlyWire fixture 匯入、大 ID 往返與溢位、manifest 欄位、無證據拼接拒絕；`evidence/DAT-06/`
  （真實資料 blocked_data）。
- [ ] 第三階段：全圖預估與檢查、一次全參數反向、可塑性與調節啟用、三種保存物與新程序恢復逐項相等、
  峰值 RSS 與耗時；`evidence/OPS-07/`（或 blocked_hardware 與精確指令）。
- [ ] 第四階段：`benchmark` 四段分開與 `energy.measured = false`、同 seed 重跑；`clean-env-verify.sh`
  全步驟與 `run.json`、`report` 的 22.3 清單；`evidence/OPS-08/`、`evidence/OPS-10/`。
- [ ] 第五階段：`backend` 能力偵測與明示回退（可做）；裝置實作 blocked，等使用者決定技術與存取。

## 第一階段證據（2026-09-17）

- **治理測試（GOV-01）**：`governance_test.go` 四項治理測試（`TestGovernanceRequirementIDsMatchHandoff`、`TestGovernanceStatusValuesAreDeclared`、`TestGovernancePassedRequirementsHaveEvidence`、`TestGovernanceTicketsHaveDatedRootDecisions`）全數 PASS。決策紀錄維持在 `delivery-status.md` 第 101 行「## 決策紀錄」段落。確認本專案從未執行建立遠端、發布或刪除原件之操作。
  - 證據路徑：[`evidence/GOV-01/verification.json`](../../evidence/GOV-01/verification.json)、[`evidence/GOV-01/test.log`](../../evidence/GOV-01/test.log)
  - Limitations：governance_test.go 驗證靜態結構與宣告，不重新執行全部測試；未執行操作以本地 git log 與 release-checklist 狀態為證。
- **平台矩陣腳本（OPS-04）**：`scripts/platform-matrix.sh` 完成 `linux/amd64`、`linux/arm64`、`darwin/arm64`、`windows/amd64` 之交叉編譯與 `go vet`（compiled=4/4），本機 `darwin/arm64` 完整通過 unit（9.677s）、race（3.653s）、vet（0.225s）、mod_verify（2.527s）。輸出 `evidence/OPS-04/platform-matrix.json` 分開標記 compiled 與 executed。
  - 證據路徑：[`evidence/OPS-04/verification.json`](../../evidence/OPS-04/verification.json)、[`evidence/OPS-04/platform-matrix.json`](../../evidence/OPS-04/platform-matrix.json)、[`evidence/OPS-04/platform-matrix.log`](../../evidence/OPS-04/platform-matrix.log)
  - Limitations：非 host 平台僅交叉編譯與 vet，無實際執行紀錄；早期 Ubuntu 完整測試紀錄見 `evidence/cpu-reference-20260914/`。
- **Workers 欄位與分區、連續與 LIF 核心並行（OPS-04）**：`dynamics.Config.Workers int`（0/1 為單執行緒參考；>1 為按 target 節點分區之 worker pool，固定為邊之宣告加總順序）。連續核心（`TestContinuousWorkersAreBitIdentical`、`TestContinuousWorkersGradientBitIdentical`）與 LIF 放電核心（`TestLIFWorkersAreBitIdentical`、`TestLIFWorkersGradientBitIdentical`）均通過 Workers ∈ {0, 1, 2, 7} 前向與梯度之位元完全相同測試。基準測試（`BenchmarkContinuousForwardWorkers1` 107 allocs/op vs `Workers4` 491 allocs/op；`BenchmarkLIFForwardWorkers1` 443 allocs/op vs `Workers4` 795 allocs/op）記錄每步配置量；每步配置量不隨步數增加由 `TestContinuousForwardAllocsAreConstant` 與 `TestLIFForwardAllocsAreConstant` 證明，基準只記錄數值。
  - 證據路徑：[`evidence/OPS-04/workers-bench.log`](../../evidence/OPS-04/workers-bench.log)
  - Limitations：Workers 並行目前僅在 fixture 上驗證；熱路徑配置以 allocs/op 記錄而非 profile。
- **能力表與科學界線、模型卡範本（GOV-04）**：`README.md` 能力狀態表包含四欄（模型狀態、資料規模、角色、證據）共 16 列宣告；科學界線 8 條逐條對應主規格 3.2；模型卡與實驗卡範本（`docs/model-card-template.md`、`docs/experiment-card-template.md`）確立 `initialized-`、`fixture-`、`real-subgraph-`、`male-full-` 命名規範。
  - 證據路徑：[`evidence/GOV-04/verification.json`](../../evidence/GOV-04/verification.json)
  - Limitations：能力表由人工維護，沒有自動同步測試。
- **授權盤點與發布清單（GOV-05）**：`docs/licenses.md` 盤點 182 個相依模組授權（68 個未知含未下載）與資料/素材段三列說明；`docs/release-checklist.md` 狀態明示為 `blocked_permission`（等待使用者選定開源授權與發布授權）。
  - 證據路徑：[`evidence/GOV-05/verification.json`](../../evidence/GOV-05/verification.json)
  - Limitations：授權判定是固定文字規則、未知項需人工核對、發布授權等使用者。
- **單一主文件與索引（GOV-06）**：`docs/INDEX.md` 作為單一入口整合 9 大段落，39 個 Markdown 相對連結全數通過存在性檢查；檔案格式表格列出 52 個 schema 版本字串（Go 全庫掃描為 54 個唯一字串）。
  - 證據路徑：[`evidence/GOV-06/verification.json`](../../evidence/GOV-06/verification.json)
  - Limitations：連結檢查是一次性指令，沒有進 verify.sh。

## 依據

- 主規格 1.3（三種結果分別報告、受阻不得以小型成功替代）、1.4（權威順序、不得縮減範圍）、2.1
  （CPU 參考、GPU 獨立能力、FlyWire 供比較）、2.2、2.3（必須回問：發布、授權、遠端）、3.2（README
  科學界線）、4.3（不依賴 Python、封裝器為選用）、4.4（能力偵測、明示回退）、5.1（FlyWire 獨立、禁止
  拼接）、5.2（原始檔只讀、ID 精度、索引寬度）、5.5（manifest 與 NeuronRecord 欄位）、17.1–17.5（資源
  預估、三種規模、CPU 路徑、GPU 路徑、全腦驗證最低內容）、18.5（自動檢查）、18.6（完成狀態七值）、
  19.2（必要文件、授權出處）、19.4（模型卡、初始化命名）、22.1–22.4（接手、乾淨環境、最終報告、發布前
  確認）。
