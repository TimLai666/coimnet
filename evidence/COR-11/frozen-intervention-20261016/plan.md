# 凍結介入對照與失敗原子性

Root 決策｜2026-10-16。上一輪有進展。當前公開 probe 已重現兩個同根因表現：未生效 plan 的 ActivityDelta=0.02158486843109131；fresh neural source 在化學凍結下可以前進，但 Intervene 內部解凍的 twin 回錯，此時 subject 已前進一步。原始證據 red-public-probe.log 保留。

同一介入的因果對照須保留兩種 runtime freeze 狀態。外部 RestoreIndividual 繼續預設解凍，不增加 schema 欄位。Intervene 先驗證 plan／shuffle，再建立並執行無介入 twin，成功後才執行 subject，確保 twin 回錯或取消時 subject 尚未提交。這是既有介入差異與失敗安全契約的修正，沒有變更正常神經／化學／可塑性演算法。

唯一正式實作檔案 learning/intervene.go 的 Intervene：搬動原 twin 區塊至任何 subject 可變狀態之前，複製 chemical.frozen／plastic.frozen，更新此函式說明。Root 擁有 learning/intervene_runtime_test.go、公開 probe、判準與證據，實作者不得改測試。先執行 RED 再派工。其餘檔案不順手重構，OfferFeedback 舊註解只保留後續線索。

固定 85 個公開行為案例：36 個有無 plasticity、Continuous/LIF/Mixed、timeline/neural 及全部合法 freeze 組合的未生效介入；9 個 fresh neural 化學凍結；18 個 active block 對未調節控制的正對照；18 個在第一個完整 advance 後取消的原子性；4 個無 chemistry 的 Continuous/LIF plastic freeze。另有五個父測試，合計 90 個 terminal pass。每個設定都從公開 snapshot 建立明確重套 flags 的手動 control。

未生效介入要求逐位相同輸出與 snapshot，以及全部 delta=0、AppliedRows=0。active block 的輸出與 DisableChemistry 控制逐位相同，ActivityDelta／PlasticDelta 以控制輸出和快照的獨立 L2 算式核對，容差 1e-12*(1+abs(want))、拒絕非有限，AppliedRows=列數、無直接濃度變化。取消使用遵守 context.Context 介面的可控 Err 計數器，在一個完整 Advance 的 Err 呼叫數加一時取消，重現舊路徑 subject 已提交再 twin 失敗，修正後 subject 尚未執行。拒絕後 snapshot、上一份化學報告及 flags 保持，再合法接續正確。

Root 審查完整差異，親自執行專項一般／競態與公開 probe、至少兩個隔離回退對照（遺漏 flags、先提交 subject 再執行 twin），並檢查原有介入／freeze 覆蓋。正式來源改動後執行格式、全框架 build、完整 unit、完整 race、vet、固定 Insyra v0.3.4、模組及 31 項治理。沿用舊終端事件與七份 legacy 數值紀錄，不能放寬舊測試。保留必要證據後，只清理本輪 runs/.tmp，核對模型、資料、原始模組及他人工作，再依已授權正常提交／推送／讀回。

減法審查：不新增公開開關、不把 runtime flag 寫入 snapshot、不替未明示的 frozen fix/remove/swap 選新優先序。凍結列目前不進 pre/post-kinetics，相關 AppliedRows=0 保持；若要改這項規則另行釐清。整體零 bug 目標不能由此單票宣稱達成。

Root 在隔離回退完成後確認：兩個獨立根因都屬本票範圍。遺漏 freeze flags 的版本有 45 個 terminal failure；保留 flags 但先提交 subject 的版本有 19 個 failure，全部是取消原子性。後者包含兩種 freeze 都關閉的正常個體，因此不能只歸因於解凍。最終以兩個已確認缺陷分別記錄。

驗證期間另由合法公開 probe 確認 P2：介入差異範數先平方，將可表示的 5e307 算成 +Inf，造成 JSON 匯出失敗。這個 helper 本票未修改，另由 ticket128 處理，不更動正在測試的 841 份來源。COR-11 保持 specified，工程追蹤暫為 90／91，本票的兩項修正仍須完成原定完整驗證和交付。
