# 44 — doctor 能準確區分 GPU 支援條件與本機探測

**Epic:** GOV-04／OPS-02 能力診斷，OPS-05 限制維持
**User Story:** 使用者可以從 doctor 判斷目前可用的 GPU 程式路徑、必要條件，以及哪些能力尚未在本機執行驗證。
**Blocked by:** 30 的受限 WebGPU episode trainer（已交付）
**Status:** in_progress

## 問題與交付契約

`coreCapabilities()` 把 GPU 六項能力固定為 `not_implemented`，與現有 WebGPU 稀疏原語、純量零延遲連續核心及 episode trainer 不一致。修正六個既有字串為 `implemented_with_constraints`，新增 `core.gpu.details`。CPU 的六項字串、既有 GPU 硬體探測、Insyra probe、API 簽名及 `coimnet-doctor/v1` 保持。JSON 只新增 GPU details，CPU 不輸出 details。

支援條件為 `backend=webgpu`、`model=continuous`、`state_dimension=1`、`edge_shape=scalar`、`max_delay_steps=0`、`training_scope=independent_episode`、`recompute=not_supported`、`sparse_precision=float32`。`neural_state=cpu_float64`、`activation=cpu`、`optimizer=cpu_adamw`。`device_parameter_update` 與 `device_state_save` 為 `not_implemented`，`full_graph=unverified`。`execution` 與 `encoder_readout_execution` 都是 `not_probed`。

頂層 `gpu` 仍只回報這次作業系統工具的硬體結果，裝置存在、工具缺失、逾時或解析失敗都不能變成 WebGPU 已執行。Doctor 不新增 GPU 初始化、訓練或參數。歷史 Metal 證據不能充作本次 probe，OPS-05 仍為 specified。

## Root 骨架與檔案責任

- Root：新增 `BackendCapabilities.Details` 與 `GPUBackendDetails` 可編譯資料形狀。本票、ENG、交付狀態、需求證據與整合文件由 Root 更新。
- 測試 agent：只改 `internal/cli/doctor_test.go`，新增六欄、完整限制、JSON 相容性、探測分離、CLI help／錯誤流程驗收，並修正原本錯誤的 GPU 未實作期望。不得改實作。
- 實作 agent：只改 `internal/cli/doctor.go` 與 `internal/cli/run.go`，填入以上結果並同步 help／總覽。不得修改測試檔。

## 完整使用流程與邊界

1. `coimnet doctor` → 合法 v1 JSON → 真實硬體與 Insyra probe，GPU 六項受限實作及完整限制，核心執行仍 not_probed。
2. `coimnet doctor --help`／總覽 → 可發現支援條件、硬體探測範圍、核心未探測狀態及錯誤。
3. nil／取消 context、非法參數或輸出失敗 → 保留原錯誤行為。缺少可選探測工具 → unknown，不影響靜態實作宣告。

每份 `coreCapabilities()` 回傳獨立的 details，修改一份報告不能污染下一份。無資料遷移、依賴或 GPU 核心運算變更。

| 操作／條件 | 處理與使用者可見結果 | 驗收責任 |
| --- | --- | --- |
| Doctor 的 context 是 nil 或已取消 | Doctor 回錯，CLI 不輸出成功 JSON | 本票公開入口回歸 |
| CLI 空參數／零延遲值 | 空參數沿用總覽。合法 details 保留零延遲數值 | 本票 help／JSON 回歸 |
| 未知旗標或多餘位置參數 | CLI 回錯，不輸出報告 | 本票 CLI 回歸 |
| 可選工具缺失、逾時、解析失敗或零裝置 | probe 分別回 unknown／unavailable，支援宣告與 not_probed 不變 | 沿用 probe 回歸，加本票缺失工具使用流程 |
| mandatory Insyra probe 失敗或 stdout 寫入失敗 | 沿用原錯誤傳遞，不用靜態能力覆蓋失敗 | 既有 Insyra 數值檢查與本票 writer 回歸 |
| 重複或並行建立報告 | 無新增共享可變狀態，每份 details 獨立 | 本票隔離回歸與完整 race |

## 驗收

- [x] 失敗測試在骨架／原能力值重現，Root 審核並保存 red 與來源指紋（`evidence/OPS-02/doctor-gpu-capabilities-20261007/focused-final-red.log`）。
- [x] GPU 六欄與支援限制、CPU／Insyra／硬體探測分離及 JSON 相容性回歸通過（Root 的 `focused-green.log`，13 個頂層測試）。
- [x] 真實 CLI 三條流程、help／文件及錯誤行為通過（`cli-workflow.log`，含正常／缺失工具、help／總覽及兩項非法參數）。
- [x] 完整 gofmt、build、test、race、vet、依賴與歷史原件檢查、Root diff 審查通過。
- [ ] 保存命令、環境、輸入指紋、結果及日誌。更新追蹤、提交、推送並核對遠端。

減法審查：沿用原有硬體探測，不增加 GPU 初始化、訓練選項或模型。只修正錯誤能力宣告及必要限制。
