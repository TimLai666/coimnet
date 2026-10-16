# 介入報告有限範數與失敗原狀的 Root 審查

Scope CLEAN。正式實作只有 learning/intervene.go 與 learning/individual.go，三份新增測試由 Root 凍結，實作者沒有更動。Root 完整閱讀兩檔 diff、三份測試、四個差異使用者、advanceRows／advanceStepwise 的候選提交、來源 clone 與所有 defer，並自行執行驗收。最終來源見 formal-source-v2.json（844 份），檢查見 frozen-checks-v2.json（三份）。

## 已確認與採用

原差異先平方，會把可表示的 5e307 算成 +Inf、2.5e-201 算成零。正常索引順序平方和位元保持，只有實質上下溢位使用 scaled compensated fallback，最高端用精確平方和判斷最近偶數捨入界線。四個使用者與等形有限輸入前提保持，數值 helper 見 learning/intervene.go:659。真正不可表示時回傳空輸出、空紀錄與錯誤。

learning/individual.go:620-628 的 advanceRowsLocked 只在完整成功後替換 neural、plastic.state、chemical.state/report/sources。舊 ownership 引用可以還原報告拒絕，且保留快照未保存的來源計算快取。第一版多餘的 committed 旗標、整個 plasticRuntime 與 report 深拷貝已移除，僅留一個 callOK 與必要舊欄位。沒有變更數值判準。

## v1 審查阻擋與 v2 修正

v1 的 92 項專項與完整工程曾通過，但 Root 追加並行 Snapshot／ChemistryReport 後，確認 rollback 寫入未持鎖，讀者會讀到最後被拒絕的候選。原始來源、日誌與 snapshot-review-block.json 保留，沒有將 v1 提交。

凍結成正式第三份測試後，一般六 fail／一 pass，競態七 fail、49 次競態警告。較早的追加 overlay 在兩種建置各七 fail。這是排程觀察差異，測試斷言沒有放寬，見 red-readers-verification.json。

v2 保持 preflight 次序，之後在 learning/intervene.go:95-98 取得可取消的個體鎖，直到快照、twin、subject、報告與 cleanup 完成才解鎖。公開 Snapshot 的原複製內容精確抽至 learning/individual.go:412 的 snapshotLocked，subject 改用 advanceRowsLocked。defer 逆序為 chemical hook、row hook、shuffle network/hash 還原、runtime 還原、解鎖，讀者不會看到暫時網路或被拒絕的狀態。twin 有獨立鎖，沒有重入相同鎖。並行修改仍是既有不支援契約。

## 最終專項與獨立控制

有效數值 RED 為 14 pass／78 fail，92 個事件全部成功編譯執行，初稿過期 API 呼叫錯誤另存而未計為缺陷。v2 一般／競態各 99 pass，相關各 223 pass，包含原 119 個事件。公開大／小 witness 回報 5e307／2.5e-201，JSON 匯出成功。18 種 preflight 在 v1／v2 及一般／競態的輸出、錯誤、狀態指紋完全相同。

四種隔離錯誤版本兩種建置都成功編譯、執行並被斷言拒絕：逐項平方各 25 fail、漏還原各 60 fail、逐項 Hypot 各四 fail、未保護讀取一般六 fail／競態七 fail。數值參考使用獨立 8192-bit 計算，正常平方和逐位比較。永久 upper_finite 用相對誤差，另有精確 MaxFloat64 overlay 兩種建置各兩 pass。

觀察者 stress 確認實際交錯發生，但沒有強制每次迭代都交錯。六案例與錯誤版本控制、race 及 Root 的鎖順序審查共同構成證據，不宣稱任意排程形式驗證。大型 STDP aggregate 未逐欄對照所有 state，但另有原公開 eligibility witness 與完整失敗原狀比較。獨立 Luna max 唯讀對抗審查沒有提出已確認的新阻擋缺陷。

844 份來源的完整一般／競態各 58 套件通過，8,104／8,103 pass、23／24 skip，原有各 8,028 個終端事件保持，只新增各 99 pass。格式、build、vet、固定 Insyra v0.3.4、模組、31 項治理與最後 metadata 的 31 項治理通過。最終來源 844 份保持凍結，沒有在完整測試途中修改 Go 檔案。

## 另列的確認問題

learning/consolidation.go:152、161-162、244-249 的 BaseL2／SlowL2／PlasticL2 在最高端使用逐項 Hypot，會把真正不可表示的幅度回報為 MaxFloat64，並寫入 Used。一般／競態各九個公開案例已確認，見 next-audit-verification.json 與 ticket129。這是既有另一條路徑，未在本票擴大修改，MOD-06 保持 specified。沒有新的真實任務、生物校準或整體零 bug 證據。

## 派工實況與減法審查

宿主 Codex。本輪 opencode models 實際失敗：Unknown: FileSystem.open (/Users/timlai/.local/share/opencode/log/opencode.log)。依專案第二選擇使用原生 gpt-5.6-luna、reasoning effort=max，負責數值／ownership 契約、兩個實作格子及唯讀審查。Root 保有判準、凍結測試、實際驗證、完整 diff 與交付審查。

移除第一候選多餘的深拷貝與重複提交旗標。v2 僅抽原快照複製內容並沿用已持鎖的 advance 入口，不新增公開鎖 API、重複驗證或新的狀態格式。Consolidate 另票處理，避免同時修改另一條狀態提交路徑。

本輪自有暫存已清除，28 個必要輸入、診斷與隔離回退來源已封存並重新比對。18 個原模型、六份必要資料、15,046 個原始模組檔、32 個其他工作檔案與兩個既有暫存根經清理前後核對保持。實際移除量見 cleanup.json，提交與遠端讀回以 delivery.json 為準。
