## Diff Inspector

Scope: CLEAN
Reviewed: base `16718541d6adce7cbc15b74308ccfe37c25c461f` 至 source-manifest-v2.json 的 stateful dynamics、learning、PPO 與相關文件。
Adversarial review: RUN — Trigger: 多模組數值核心、保存狀態與提交流程。模型 `gpt-5.6-luna`，reasoning effort `max`，repo 全程唯讀。Root 審查此結果，沒有以它取代自己的測試及完整 diff 審查。

### Findings

No remaining confirmed findings.

原 [P1] [CONFIRMED, FIX VERIFIED] `learning/trainer.go:537` 的候選 fresh-zero 驗證缺口已修正。non-nil state 使用 forwardSegmentState，`learning/stateful.go:73` 傳 &initial；Step／StepFrom 傳 nil，維持原 Predict。原 probe 現在兩次都回 candidate update rejected、Applied=false，weight 保持 0。修正前首次會錯誤提交約 9.9999999e9。

公開 `TestStepFromStateRejectsCandidateThatOnlyRunsFromZero` 的 immediate 與 open_accumulation 均通過。錯誤時 StepResult、參數、最佳化器、更新次數、原累積視窗及保存狀態保持。Root 另以 candidate-red.log 與 feature-final.log 獨立重現前後差異，原 probe 來源保存於 candidate-probe.go.txt。

### 審查覆蓋與實際回報

審查員覆蓋最大 delay／uint64 counter、延遲跨段、4 ULP history、worker ownership、取消與原子性、Backward input／Initial／window、舊 Forward／Backward、第二份非法 InitialNeural preflight、同狀態多 epoch PPO、AdamW 與累積器、新程序恢復。

審查員實際執行：

- `go test -count=1 ./dynamics -run 'FromState|Continuous|State' -v`：通過。
- `go test -count=1 ./learning/... -run 'Stateful|FromState|PPO|Update|Checkpoint|Delayed' -v`：通過。
- 修正後 `go run /tmp/coimnet_step_state_candidate_probe.go`：兩次均拒絕，weight=0。
- 修正後 `go test -count=1 ./learning ./learning/rl -run 'FromState|Stateful|TestUpdate.*Recompute'`：通過。
- 相關檔案 `gofmt -d` 無輸出，`git diff --check` 通過。

這些為審查員的實際回報；Root 自行執行的對應證據在 feature-final.log、related-final.log、candidate-probe-final.log。審查員沒有重跑完整全套 race，也未另行重算來源與凍結 hash；來源完整性與全套檢查由 Root 負責。

### 容量疑慮判定

原 Trace 私有起點列超過單個返回 output 元素上限的疑慮，依 dynamics/state.go:15-17 與 docs/resources.md:171 排除。MaxStateValues 分別約束 State voltage、retained history、每次 Advance output，明定不是程序記憶體上限；Trace 私有起點列不納入同一限制。沒有確認的容量 finding，不建議擴大修改。

未新增其他平台、GPU、完整圖效能、導航學習或生物機制驗收。
