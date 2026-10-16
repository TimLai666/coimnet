# 唯讀觀察者的最小修正格子

Root 決策｜2026-10-16。第一候選的數值及獨立失敗原狀檢查通過，整套工程也通過，但追加 Snapshot 讀取已重現 rollback 競態與被拒絕候選外露，不可提交。原 843 份來源的完整工程結束前沒有修改正式 Go。

六個新葉案例跨連續、LIF、混合核心與 shuffle，會在 100 次真正不可表示的介入中並行讀 Snapshot 與 ChemistryReport。每次都應拒絕，唯讀者只可看見呼叫前已完成的狀態與報告。一般與競態各七個事件全部失敗。Root 擁有三份測試及期望，實作者不可修改。

正式格子只含 `learning/intervene.go` 與 `learning/individual.go`。保留既有 plan.Validate、checkNodeInterventions、empty-input 的先後，這些 preflight 完成後才以 `i.mu.LockContext(ctx)` 取得整個執行區的鎖，最後一個 defer 才解鎖。因仍不支援與其他 mutating calls 並行，preflight 的 shape/config 不必增加新 helper 或重複驗證。唯讀 getters 使用同一把鎖，必須等報告驗證、還原、shuffle 與 hooks 清理完成。

`Individual.Snapshot` 的現有持鎖複製內容抽為私有 `snapshotLocked()`，公開 wrapper 的 nil 行為與複製位元保持。Intervene 在持鎖後改呼叫此 helper，subject 改用已存在的 `advanceRowsLocked`。twin 是獨立物件，仍走原來的 public/locking 路徑。不可把外層鎖和公開 `i.Snapshot()`、`i.advanceRows()` 混用造成自鎖。

數值 helper、原 92 個驗收、凍結旗標、模型格式、參數與正常更新規則保持。只修正 rowOverride 的過時「鎖外設定」註解及唯讀者可以觀察的邊界，不新增並行 mutating API 保證，也不改未知問題的錯誤政策。

Root 先跑七個新事件的普通／競態，再重跑 92 個原新專項、216 個既有／相關事件、公開大／小及數值 mutation。另重驗 nil/canceled context 與非法 plan/input 的現有優先序、取消原子性與來源快取。凍結最終 844 份來源後，重新執行格式、build、完整一般／競態、vet、固定依賴、模組與治理。第一套工程結果只保留為第一候選證據，不當成最終驗收。
