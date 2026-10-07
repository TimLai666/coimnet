# 模型包容量驗證審查

範圍為 ticket 46。修正前，checksum 正確但容量與模型不符的模型包會被接受，保存及建立個體也沒有核對容量。

## 審查結論

Scope: CLEAN。Root 已審查完整程式差異與測試草稿，沒有尚待修正的已確認缺陷。完整一般／race 與提交掃描通過，實作 `d4ffe93` 已推送並核對遠端。交付紀錄見 delivery.json。

共同入口對非 nil Capacity 精確核對六個整數欄位，使用已驗證 Trainer 的報告，把 FreeParameterCount 設為 ParameterCount。模型包不保存訓練遮罩，LIF theta 必須包含在容量內。nil 舊欄位不補寫。沒有額外建立 Network，原驗證順序、公開簽名、格式與依賴保持。

## 驗證範圍

- 三種核心六欄的載入、同格式 migration、保存、bundle 保存／載入與不符模型的個體建立。
- 手算報告：連續 (2,1,1,8,8,3)、LIF (3,3,1,16,16,6)、向量 (2,1,3,26,26,15)。
- 零、負值、正值不符與有效 checksum 控制。回傳零值／nil、不發布部分結果。
- 合法模型往返、純量建立個體、容量指標隔離、nil 舊檔省略欄位與逐位元相同的再保存、既有目的地保持。
- CLI 的 validate、inspect 與 Markdown export 各驗證合法容量、不符容量與舊檔，共九個流程。

向量核心的合法持續個體仍依既有契約拒絕。本票只驗證其不符容量先被共同入口拒絕。JSON export 是原始文件的排版副本，不經模型語意驗證，不在本次載入流程的驗收範圍。

## Root 測試審查修正

草稿使用 capacity 字樣識別錯誤，但暫存路徑也包含該字樣，既有目的地錯誤會誤通過。Root 改為核對容量錯誤的完整片語，並補上既有目的地、migration、bundle 所有權及舊欄位省略。各失敗流程分成獨立案例，避免一次 Fatal 阻止後續欄位與 migration 的驗證。向量的不符容量也加入個體建立驗收，沒有變更其合法模型的既有限制。

最終凍結測試在修正前有三個負向頂層失敗、兩個正向頂層通過。修正後五個頂層全數通過。指紋見 test-freeze.json，修正前後日誌見 root-final-red.log 與 root-green.log。

## OpenCode 派工與限制

宿主為 Codex。三次執行均使用 `opencode run --pure --agent build --model opencode/big-pickle --format json`，分成測試草稿、測試套用重試與獨立實作。每個執行者只能修改指定檔案，禁止其他派工、Git 寫入與測試／實作混寫。Root 親自執行驗證，沒有採用執行者的摘要作為完成證據。

第一次測試寫入被 OpenCode 的 edit 權限擋住。原因是白名單使用絕對路徑，而官方 [write 工具](https://raw.githubusercontent.com/anomalyco/opencode/dev/packages/opencode/src/tool/write.ts) 的權限比對使用 worktree 相對路徑。Root 保留完整草稿與錯誤，停止該程序，修正為同一目標的相對路徑後重試成功。權限始終只允許指定檔案，沒有改動永久設定或擴大 Git 權限。第一次停止的程序 exit -15，後兩次 exit 0。模型紀錄的 cost 為 0。

## 證據與限制

本票是軟體正確性證據。沒有新增任務學習或生物機制證據。完整 Mac 一般測試 57 個套件、build、vet、格式及相依性已通過。完整 race 57 個套件也全部通過。648 份 Go 來源凍結，3,053 份範圍外既有檔案保持。原始交接、資料、模型、範例設定及依賴沒有變更。

全專案需求維持 89／91，TSK-11 與 OPS-05 保持 specified。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| [AGENTS.md](/Users/timlai/Developer/coimnet/AGENTS.md) | 移除已解決的容量提醒，保留派工規則與其他追蹤項目 |
| [ENG.md](/Users/timlai/Developer/coimnet/ENG.md) | 記錄六欄容量、LIF 全參數及 nil 相容的共同契約 |
| [checkpoint/package.go](/Users/timlai/Developer/coimnet/checkpoint/package.go) | 共同入口核對宣告容量，不增加網路建立 |
| [checkpoint/package_capacity_validation_test.go](/Users/timlai/Developer/coimnet/checkpoint/package_capacity_validation_test.go) | 新增五個公開行為回歸與獨立手算控制 |
| [delivery-status.md](/Users/timlai/Developer/coimnet/delivery-status.md) | 同步 ticket 46 的驗收與交付狀態 |
| [docs/INDEX.md](/Users/timlai/Developer/coimnet/docs/INDEX.md) | 工作票範圍更新為 01–46 |
| [docs/requirements-status.json](/Users/timlai/Developer/coimnet/docs/requirements-status.json) | 只替 STA-01 與 COR-06 附加本票證據 |
| [docs/tickets/46-model-package-capacity-validation.md](/Users/timlai/Developer/coimnet/docs/tickets/46-model-package-capacity-validation.md) | 記錄 Root 契約、工作責任及五項固定驗收 |
| [evidence/STA-01/model-capacity-20261007/baseline.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/baseline.json) | 來源、測試或原件 SHA-256 |
| [evidence/STA-01/model-capacity-20261007/build.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/build.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/build.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/build.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-build.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-build.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-invalid-export-stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-invalid-export-stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-invalid-export-stdout.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-invalid-export-stdout.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-invalid-inspect-stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-invalid-inspect-stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-invalid-inspect-stdout.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-invalid-inspect-stdout.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-invalid-validate-stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-invalid-validate-stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-invalid-validate-stdout.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-invalid-validate-stdout.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-invalid.coimpkg](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-invalid.coimpkg) | 來自既有人工 fixture 的 CLI 容量控制副本 |
| [evidence/STA-01/model-capacity-20261007/cli-legacy-export-stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-legacy-export-stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-legacy-export-stdout.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-legacy-export-stdout.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-legacy-inspect-stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-legacy-inspect-stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-legacy-inspect-stdout.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-legacy-inspect-stdout.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-legacy-reading.md](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-legacy-reading.md) | 人工模型的 CLI Markdown 匯出控制 |
| [evidence/STA-01/model-capacity-20261007/cli-legacy-validate-stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-legacy-validate-stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-legacy-validate-stdout.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-legacy-validate-stdout.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-valid-export-stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-valid-export-stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-valid-export-stdout.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-valid-export-stdout.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-valid-inspect-stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-valid-inspect-stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-valid-inspect-stdout.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-valid-inspect-stdout.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-valid-reading.md](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-valid-reading.md) | 人工模型的 CLI Markdown 匯出控制 |
| [evidence/STA-01/model-capacity-20261007/cli-valid-validate-stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-valid-validate-stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-valid-validate-stdout.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-valid-validate-stdout.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/cli-valid.coimpkg](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-valid.coimpkg) | 來自既有人工 fixture 的 CLI 容量控制副本 |
| [evidence/STA-01/model-capacity-20261007/cli-workflow.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/cli-workflow.json) | 九個 CLI 流程與人工輸入指紋 |
| [evidence/STA-01/model-capacity-20261007/delegation.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/delegation.json) | 派工命令、角色、結果及限制 |
| [evidence/STA-01/model-capacity-20261007/diff-check.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/diff-check.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/diff-check.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/diff-check.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/full-race.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/full-race.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/full-race.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/full-race.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/full-test.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/full-test.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/full-test.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/full-test.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/gofmt.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/gofmt.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/gofmt.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/gofmt.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/governance-final.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/governance-final.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/governance-final.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/governance-final.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/governance-repaired.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/governance-repaired.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/governance-repaired.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/governance-repaired.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/implementation-worker-config.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/implementation-worker-config.json) | 該次派工的精確權限設定 |
| [evidence/STA-01/model-capacity-20261007/implementation-worker-prompt.txt](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/implementation-worker-prompt.txt) | 實際派工指示 |
| [evidence/STA-01/model-capacity-20261007/implementation-worker.jsonl](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/implementation-worker.jsonl) | OpenCode 原始事件紀錄 |
| [evidence/STA-01/model-capacity-20261007/implementation-worker.stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/implementation-worker.stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/mod-tidy-diff.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/mod-tidy-diff.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/mod-tidy-diff.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/mod-tidy-diff.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/mod-verify.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/mod-verify.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/mod-verify.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/mod-verify.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/root-final-red.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/root-final-red.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/root-final-red.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/root-final-red.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/root-green.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/root-green.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/root-green.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/root-green.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/root-red.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/root-red.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/root-red.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/root-red.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/root-review.md](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/root-review.md) | Root 審查、派工限制與逐檔異動 |
| [evidence/STA-01/model-capacity-20261007/source-freeze.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/source-freeze.json) | 來源、測試或原件 SHA-256 |
| [evidence/STA-01/model-capacity-20261007/test-freeze.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/test-freeze.json) | 來源、測試或原件 SHA-256 |
| [evidence/STA-01/model-capacity-20261007/test-worker-config.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/test-worker-config.json) | 該次派工的精確權限設定 |
| [evidence/STA-01/model-capacity-20261007/test-worker-prompt.txt](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/test-worker-prompt.txt) | 實際派工指示 |
| [evidence/STA-01/model-capacity-20261007/test-worker-proposal.txt](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/test-worker-proposal.txt) | 第一次派工保留下來的完整測試草稿 |
| [evidence/STA-01/model-capacity-20261007/test-worker-recovery-config.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/test-worker-recovery-config.json) | 該次派工的精確權限設定 |
| [evidence/STA-01/model-capacity-20261007/test-worker-recovery-prompt.txt](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/test-worker-recovery-prompt.txt) | 實際派工指示 |
| [evidence/STA-01/model-capacity-20261007/test-worker-recovery.jsonl](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/test-worker-recovery.jsonl) | OpenCode 原始事件紀錄 |
| [evidence/STA-01/model-capacity-20261007/test-worker-recovery.stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/test-worker-recovery.stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/test-worker.jsonl](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/test-worker.jsonl) | OpenCode 原始事件紀錄 |
| [evidence/STA-01/model-capacity-20261007/test-worker.stderr.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/test-worker.stderr.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/verification.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/verification.json) | 需求證據、命令、環境、輸入指紋與觀察結果 |
| [evidence/STA-01/model-capacity-20261007/vet.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/vet.json) | 實際驗證命令、環境與結束狀態 |
| [evidence/STA-01/model-capacity-20261007/vet.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/vet.log) | 實際命令的輸出或錯誤紀錄 |
| [evidence/STA-01/model-capacity-20261007/commit-scan.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/commit-scan.json) | 提交前掃描命令與結果 |
| [evidence/STA-01/model-capacity-20261007/commit-scan.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/commit-scan.log) | 提交前掃描實際輸出 |
| [evidence/STA-01/model-capacity-20261007/delivery.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/delivery.json) | 實作提交、推送與遠端核對的交付證據 |
| [evidence/STA-01/model-capacity-20261007/governance-delivery.json](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/governance-delivery.json) | 交付文件的格式與治理檢查紀錄 |
| [evidence/STA-01/model-capacity-20261007/governance-delivery.log](/Users/timlai/Developer/coimnet/evidence/STA-01/model-capacity-20261007/governance-delivery.log) | 交付文件的八個格式與治理頂層檢查輸出 |
