# CoImNet 開發進度

## 目前階段

[ticket137](docs/tickets/137-stream-partition-state.md) 已通過完整工程驗收，一般／競態各 58 套件通過，8,628／8,627 pass、23／24 skip，原有各 8,597 個終端事件保持，只新增各 54 pass。新增 52 個語音分段／恢復子案例及 2 個父測試、兩種隔離刻意錯誤控制均驗收，格式、build、vet、固定 Insyra v0.3.4、模組驗證及 31 項治理通過，見[驗收](evidence/TSK-02/partition-state-20261017/verification.json)。855 份既有 Go／模組來源保持，只新增公開 SDK 性質回歸，目前沒有確認產品缺陷。早先沙箱禁止本機測試連接埠的失敗已保存[原因](evidence/TSK-02/partition-state-20261017/sandbox-failure.json)，最終完整驗收已在允許本機連線的環境通過。最後 31 項治理通過，1,551,595,591 bytes 本票暫存已清除，856 份來源、18 份原模型、6 份既有資料、15,046 份原始模組檔及其他工作保持，見[清理](evidence/TSK-02/partition-state-20261017/cleanup.json)。主要提交 `23fdec0` 已正常推送且遠端讀回完全相同，見[交付](evidence/TSK-02/partition-state-20261017/delivery.json)。本票 4／4，工程追蹤保持 91／91，整體零 bug 目標持續進行。

線上取消與回饋識別查核已完成本機驗證，10 個受控情境的一般／競態各 24 pass，重試後完整模型快照及明列的佇列／間隔狀態與正常孿生相同，沒有確認新產品缺陷，見[驗收](evidence/LRN-06/online-cancel-20261017/verification.json)及[Root 核對](evidence/LRN-06/online-cancel-20261017/root-review.md)。正式 856 份來源保持，沿用同源完整工程驗收。1,002,456,363 bytes 本輪暫存已清除，原模型、資料與其他工作保留，見[清理](evidence/LRN-06/online-cancel-20261017/cleanup.json)。本輪 4／4，主要提交 `c040d16` 已推送且遠端讀回一致，見[交付](evidence/LRN-06/online-cancel-20261017/delivery.json)。工程追蹤保持 91／91，整體排查持續。

[ticket138](docs/tickets/138-online-multisource-retry.md) 已通過九個完整工程步驟，一般 8,633 pass／23 skip、競態 8,632 pass／24 skip，各 58 套件通過，原有各 8,651 個終端事件保持，只新增各 5 pass。Root 已重新解析日誌及逐筆比對，見[驗收](evidence/LRN-06/multisource-retry-20261017/verification.json)。只新增同一行動的多來源階段公開回歸，沒有確認線上學習產品缺陷。最後 31 項治理通過，1,740,839,595 bytes 本票暫存已清除，857 份來源、18 份模型、6 份必要資料、32 份其他工作與 15,046 份原始模組保持，見[清理](evidence/LRN-06/multisource-retry-20261017/cleanup.json)。主要提交 `4ccab07` 已正常推送且遠端讀回相同，見[交付](evidence/LRN-06/multisource-retry-20261017/delivery.json)。本票 4／4。

[ticket139](docs/tickets/139-media-report-config-ownership.md) 已確認媒體報告的設定共用缺陷：返回後修改呼叫端種子與保留條件，舊報告內容跟著變動，卻不重算 ConfigHash。四條公開流程在原版一般／競態均重現，六行隔離候選的一般／競態各 5 pass，四份正常完整報告的 SHA-256 在各建置內保持，見[驗收](evidence/TSK-04/report-config-ownership-20261017/verification.json)。這會影響實驗紀錄，尚無模型權重或品質改變證據。探測 68,295 bytes 暫存已清除，三處正式修正與凍結回歸已採用。本票 5／5，與 ticket140 共用最終來源完整驗證，見[採用紀錄](evidence/OPS-03/report-ownership-adoption-20261017/adoption.json)。

[ticket140](docs/tickets/140-experiment-report-ownership.md) 已公開重現另外四個實驗報告 producer 的清單共用，原版一般／競態各 7 fail、隔離候選各 7 pass，五份正常報告指紋保持。65,120 bytes 探測暫存已清除，四處正式修正與凍結回歸已採用，本票 5／5，與 ticket139 共用最終工程驗證，見[證據](evidence/OPS-03/experiment-report-ownership-20261017/verification.json)。

兩票共用的完整工程驗收已通過，一般 8,645 pass／23 skip、競態 8,644 pass／24 skip，各 58 套件通過，原有各 8,656 個終端事件保持，只新增各 12 pass。九份正常報告與已核准中性增益各 21 項控制保持，九項工程步驟與 31 項治理通過，見[最終驗收](evidence/OPS-03/report-ownership-adoption-20261017/verification.json)。最後 31 項治理通過，1,556,834,694 bytes 本輪暫存已清除，原模型、資料、模組及其他工作保持，見[清理](evidence/OPS-03/report-ownership-adoption-20261017/cleanup.json)。修正 `6eebf25` 與主要證據 `a685b80` 已正常推送且遠端讀回完全相同，859 份提交來源均與驗收指紋相符，見[交付](evidence/OPS-03/report-ownership-adoption-20261017/delivery.json)。兩票各 5／5。下一項正式採用 ticket141 已通過一般／競態的合成尺寸候選，再處理 ticket142 WAV 欄位截短。

[ticket141](docs/tickets/141-asr-synthesis-dimensions.md) 已確認合成音訊的間隔乘法溢位。公開兩字元探測會錯誤回傳 1 樣本或越界 panic，正常 4 樣本控制通過。2,993 bytes 初次探測暫存已清除。新增凍結公開回歸在原版一般／競態各 10 fail／3 pass，正常控制通過，隔離候選一般／競態各 13 pass，正常音訊 payload 指紋保持，17,082 bytes 隔離暫存已清除，見[候選驗收](evidence/TSK-02/synth-shape-audit-20261017/shape-candidate-verification.json)。報告階段已交付，原候選與凍結回歸已原樣採用，以 860 份來源完成完整工程驗證，見[採用紀錄](evidence/TSK-02/synth-shape-adoption-20261018/adoption.json)。正式一般／競態各 13 pass，正常音訊 payload 保持，見[專項驗收](evidence/TSK-02/synth-shape-adoption-20261018/focused-verification.json)。格式、build 與完整一般 8,658 pass／23 skip、58 套件已通過，原有 8,668 個終端事件保持，見[Root 核對](evidence/TSK-02/synth-shape-adoption-20261018/unit-root-review.json)。完整競態 8,657 pass／24 skip、58 套件與九項工程步驟均已通過，正常音訊、九份正常報告與中性增益控制保持，見[正式驗收](evidence/TSK-02/synth-shape-adoption-20261018/verification.json)。最後 31 項治理通過，1,576,147,099 bytes 本票暫存已清除，原模型、資料、模組及其他工作保持，見[清理](evidence/TSK-02/synth-shape-adoption-20261018/cleanup.json)。修正 `c875fbf` 與主要證據 `5f0e30a` 已正常推送且遠端讀回相同，860 份提交來源與驗收相符，見[交付](evidence/TSK-02/synth-shape-adoption-20261018/delivery.json)。本票 5／5。

[ticket142](docs/tickets/142-wav-metadata-representability.md) 已用公開 WAV 寫入／讀回重現取樣率、聲道數與 byte rate 被截短，正常 8000 Hz 單聲道控制通過，見[Root 核對](evidence/TSK-02/wav-metadata-audit-20261017/verification.json)。五份必要探測檔已封存，134,302 bytes 本次暫存已清除，859 份正式來源、原模型、資料與其他工作保持，見[清理](evidence/TSK-02/wav-metadata-audit-20261017/cleanup.json)。公開原版的一般／競態各 6 fail／5 pass，五種欄位缺陷重現，正常 golden／既有路徑控制通過。另完成不配置音訊的私有算術骨架尺寸回歸，見[失敗驗收](evidence/TSK-02/wav-metadata-audit-20261017/metadata-all-red-verification.json)。隔離候選一般／競態各 22 pass，正常完整 bytes 與既有路徑控制保持，11,606 bytes 暫存已清除，見[候選驗收](evidence/TSK-02/wav-metadata-audit-20261017/metadata-candidate-verification.json)。本票 2／5，正式 WAV 尚未改動，正式採用待 ticket141 交付後接續。

[ticket143](docs/tickets/143-asr-generation-capacity.md) 已用公開 Generate 重現樣本容器與 rune 緩衝的兩種配置 panic，正常單字元與零樣本的未使用極端字長控制通過，見[驗收](evidence/TSK-02/generate-capacity-audit-20261018/verification.json)。2,869 bytes 探測暫存已清除，860 份正式來源、模型、資料與其他工作保持。公開原版一般／競態各 3 fail／3 pass，隔離候選各 6 pass，12,898 bytes 專項暫存已清除，見[候選驗收](evidence/TSK-02/generate-capacity-audit-20261018/candidate-verification.json)。本票 2／5，正式修正與 ticket142 共用同一份最終來源的完整工程驗證，待 ticket141 交付後接續。

線上回饋消費、取消與重試的補充唯讀稽核已完成，未確認產品缺陷，見[Root 核對](evidence/TSK-02/partition-state-20261017/online-audit-review.json)。Root 另核對同一凍結來源完整一般日誌中的 7 個父測試、13 個 pass 事件，這份補充稽核沒有執行新 Go。取得 learner 鎖後、無 target 或合法間隔 skip 時取消的行為尚無受控動態反例，且未確認有更強的逐筆取消契約，不據此修改學習機制。

先前的語音串流與 OCR 頁面的一致性唯讀稽核已完成，沒有確認新缺陷，見[稽核結果](evidence/TSK-02/stream-consistency-20261017/verification.json)。Root 核對 855 份來源保持，重新讀取最近完整一般／競態日誌，ASR 各 12 個、OCR 各 19 個相關 pass 事件及已核准中性增益各 21 個 pass 事件相符。該次沒有執行新 Go 測試或訓練，工程追蹤保持 91／91。OpenCode 稽核被自動核准審查拒絕，實際分工使用 Codex 內建子代理；該次未使用的派工與工具探索暫存共 9,995 bytes 已清除，原有暫存目錄保持。證據提交 `ce75a69` 已推送且遠端讀回相同，見[交付](evidence/TSK-02/stream-consistency-20261017/delivery.json)。任意隨機 Feed 分段的完整快照比對已由 ticket137 補驗，整體零 bug 目標持續進行。

[ticket136](docs/tickets/136-aggregate-cell-representability.md) 已通過完整工程驗收。855 份來源的一般／競態各 58 套件通過，8,574／8,573 pass、23／24 skip，原有各 8,575 個終端事件保持，只新增各 22 pass。正式原版各 15 fail／7 pass，修正後相關各 114 pass，各 64 份正常統計及合計 12 份舊模型接續逐位元組保持。格式、build、vet、固定 Insyra v0.3.4、模組及 31 項治理通過，已核准的中性增益控制各 21 pass，見[驗收](evidence/LRN-08/aggregate-range-20261017/verification.json)。LRN-08 恢復 passed，工程追蹤 91／91。最後 31 項治理通過，1,773,523,834 bytes 本輪暫存已清除，原模型、必要資料、原始模組及其他工作保持，見[清理](evidence/LRN-08/aggregate-range-20261017/cleanup.json)。本票 4／4，主要提交 `01e774e` 已推送且遠端讀回相同，見[交付](evidence/LRN-08/aggregate-range-20261017/delivery.json)。三條化學路徑的唯讀補充稽核未確認新缺陷，既有首步差異符合明文生命週期，見[補充稽核](evidence/LRN-08/aggregate-range-20261017/chemical-path-audit-review.json)。整體零 bug 目標保持進行中。

[ticket135](docs/tickets/135-chemical-evaluation-reset.md)已通過完整工程驗收。854份來源的一般／競態各58套件通過，8,552／8,551 pass、23／24 skip，原有各8,556個終端事件保持，只新增各19 pass。相關各136 pass、64份正常報告及十二份舊化學保存接續保持，原版與略過重設的對照均未通過回歸。格式、build、vet、固定依賴及31項治理通過。LRN-10恢復passed，本票4／4，最後31項治理通過，2,156,022,451 bytes本輪暫存已清除，原模型、資料及其他工作保持。主要提交`2c95625`已推送且遠端讀回相同，見[交付](evidence/LRN-10/chemical-evaluation-reset-20261017/delivery.json)。已確認的[ticket136](docs/tickets/136-aggregate-cell-representability.md)統計溢位維持specified，工程追蹤90／91。

[ticket134](docs/tickets/134-evaluation-mse-representability.md)已完成正式修正與完整工程驗收。853份來源的一般／競態各58套件通過，8,533／8,532 pass、23／24 skip，原有各8,540個終端事件保持，只新增各16 pass。相關各244 pass、各64份正常報告及十二份舊化學模型接續保持。格式、build、vet、固定依賴及最後31項治理通過。LRN-10恢復passed，工程追蹤91／91。24份必要輸入已保存，1,802,429,189 bytes本輪暫存已清除，原模型、資料及其他工作保持。本票4／4，主要提交d7defe7已同步且遠端讀回相同，見[交付](evidence/LRN-10/mse-representability-20261016/delivery.json)。使用者核准的中性增益21個控制在本輪完整一般／競態也均通過，見[驗收](evidence/LRN-10/mse-representability-20261016/verification.json)。

[ticket133](docs/tickets/133-replay-restore-state-validation.md)已正式採用重播恢復的必要驗證。851份正式來源的完整一般／競態各58套件通過，8,517／8,516 pass、23／24 skip，原有各8,466個終端事件保持，只新增各74 pass。48份修改前重播存檔在新程序接續逐位元組相同，八列公開非法狀態均被拒絕。格式、build、vet、固定Insyra v0.3.4、模組與31項治理通過。已核准中性增益的21個控制事件在兩種建置均通過。本票4／4，最後31項治理通過，60份必要輸入封存，1,739,953,161 bytes本輪暫存已清除，原模型、資料、模組及其他工作保持。主要提交`d6a9678`已推送且遠端讀回相同，見[交付](evidence/LRN-07/restore-validation-20261016/delivery.json)。LRN-07恢復passed，LRN-10由ticket134接續，工程追蹤90／91。見[驗收](evidence/LRN-07/restore-validation-20261016/verification.json)。

[ticket132](docs/tickets/132-adaptation-feedback-permission.md) 的回饋權限修正已正式採用。一般／競態各39項相關測試、35項固定評分控制與12份舊化學快照接續通過，10列公開控制中已允許score保持，未允許答案不再影響學習。本票4／4，主要提交`9ff5aa2`已推送且遠端讀回相同，見[交付](evidence/LRN-10/feedback-permission-20261016/delivery.json)。下一個可驗證成果是ticket133的完整失敗回歸與修正。848份正式來源的完整一般／競態各58套件通過，8,443／8,442 pass、23／24 skip，原有各8,459個終端事件保持，只新增各7 pass。格式、build、vet、固定Insyra v0.3.4、模組及31項治理通過。最後31項治理通過，33份必要輸入封存，1,786,439,615 bytes本輪暫存已清除，原模型、資料及其他工作保持。本票交付完成。LRN-10另有[ticket134](docs/tickets/134-evaluation-mse-representability.md)的極端有限誤差溢位，維持specified，LRN-07由ticket133接續，目前89／91。見[驗收](evidence/LRN-10/feedback-permission-20261016/verification.json)。

[ticket131](docs/tickets/131-fixed-scoring-state.md) 的固定評分修正通過完整工程驗收。847份正式來源的一般／競態各58套件通過，8,436／8,435 pass、23／24 skip，原有各8,424個終端事件保持，只新增各35 pass。格式、build、vet、固定Insyra v0.3.4、模組及31項治理通過。12份舊化學快照接續保持。本票4／4，31項最後治理通過，1,718,722,369 bytes本輪暫存已清除，原有模型、資料與其他工作保持。主要提交`2387ca9`已推送且遠端讀回一致，見[交付](evidence/LRN-10/policy-composition-20261016/delivery.json)。見[驗收](evidence/LRN-10/policy-composition-20261016/verification.json)與[清理](evidence/LRN-10/policy-composition-20261016/cleanup.json)。[ticket132](docs/tickets/132-adaptation-feedback-permission.md)的候選修正僅在暫存overlay通過一般／競態各39項相關測試及10列公開控制，正式程式尚未採用。[ticket133](docs/tickets/133-replay-restore-state-validation.md)的重播恢復缺口仍待修正。LRN-10及LRN-07保持specified，工程追蹤89／91。

[ticket130](docs/tickets/130-online-receptor-gate-ownership.md) 已修正線上作答提前學習與回饋閘門衝突。一般／競態各168個新專項、四個相關套件、32個公開控制與12份舊化學模型接續通過，兩種隔離錯誤均被拒絕。846份正式來源的完整一般／競態各58套件通過，8,401／8,400 pass、23／24 skip，原有各8,256個終端事件保持，只新增各168 pass。格式、build、vet、固定Insyra v0.3.4、模組與31項治理通過。LRN-06恢復passed，工程追蹤91／91。本票4／4。最後31項治理通過，1,873,901,825 bytes自有暫存已清除，原模型、資料、原始模組及其他工作保持。主要提交`119147a`已推送且遠端讀回相同，見[清理](evidence/LRN-06/online-receptor-policy-20261016/cleanup.json)與[交付](evidence/LRN-06/online-receptor-policy-20261016/delivery.json)。見[驗收](evidence/LRN-06/online-receptor-policy-20261016/verification.json)。

[ticket 129](docs/tickets/129-consolidation-norm-boundary.md) 已修正三種記憶報告的最高端誤接受與誤拒絕。一般／競態各129個新專項、224個相關事件與33個公開控制通過，兩種隔離錯誤均被拒絕。845份來源的完整一般／競態各58套件通過，8,233／8,232 pass、23／24 skip，原有各8,127個終端事件保持，只新增各129 pass。格式、build、vet、固定Insyra v0.3.4、模組及治理通過，MOD-06恢復passed，工程追蹤90／91，LRN-06由ticket130接續。本票4／4。最後31項治理通過，1,882,889,920 bytes自有暫存清除，原模型、資料、原始模組及其他工作保持。主要提交`0e08198`已正常推送且遠端讀回相同，見[清理](evidence/MOD-06/consolidation-norm-20261016/cleanup.json)與[交付](evidence/MOD-06/consolidation-norm-20261016/delivery.json)。見[驗收](evidence/MOD-06/consolidation-norm-20261016/verification.json)。

[ticket 128](docs/tickets/128-intervention-finite-norm.md) 已修正介入報告的中間平方上溢／下溢，真正不可表示的差異會回傳錯誤並保留原狀態、報告與來源快取，並行讀取只取得完整操作的狀態。一般／競態各 99 個新專項、223 個相關事件與兩個原公開 witness 通過，四種隔離回退在兩種建置均被拒絕。844 份來源的完整一般／競態各 58 套件通過，8,104／8,103 pass、23／24 skip，原有各 8,028 個終端事件保持，只新增各 99 pass。格式、build、vet、固定依賴與治理通過，COR-11 恢復 passed，見[驗收](evidence/COR-11/finite-intervention-norm-20261016/verification.json)。本票 4／4，最後 31 項治理通過，2,632,490,749 bytes 自有暫存已清除，原模型、資料、原始模組與其他工作保持，見[清理](evidence/COR-11/finite-intervention-norm-20261016/cleanup.json)。主要提交 `2b9c959` 已推送且遠端讀回相同，844 份提交來源核對完成，見[交付](evidence/COR-11/finite-intervention-norm-20261016/delivery.json)。MOD-06 的三種記憶報告在最高端錯誤接受寫入，保持 specified，由 [ticket 129](docs/tickets/129-consolidation-norm-boundary.md) 接續，目前工程追蹤 90／91。

[ticket 127](docs/tickets/127-frozen-intervention.md) 已修正內部對照丟失凍結旗標，以及對照失敗前 subject 已提交的兩個缺陷。兩個公開 probe、一般／競態各 90 項專項、既有與新增各 119 項介入／凍結測試通過，兩種隔離回退被拒絕。841 份來源的完整一般／競態各 58 套件通過，8,005／8,004 pass、23／24 skip，原有各 7,938 個終端事件保持，只新增各 90 pass。格式、build、vet、固定依賴及治理通過，COR-11 因另已重現的範數溢位保持 specified，工程追蹤 90／91，下一項由 ticket128 修正，見[驗收](evidence/COR-11/frozen-intervention-20261016/verification.json)。本票 4／4，最後 31 項治理通過，1,740,939,604 bytes 自有暫存已清除，原模型、資料、原始模組與其他工作保持，見[清理](evidence/COR-11/frozen-intervention-20261016/cleanup.json)。主要提交 `0af230b` 已推送且遠端讀回相同，841 份提交來源核對完成，見[交付](evidence/COR-11/frozen-intervention-20261016/delivery.json)。模型格式、正常載入及數值規則保持，整體機制稽核持續。

[ticket 126](docs/tickets/126-persistent-chemical-clock.md) 已通過持續 timeline 與延後回饋的絕對時間驗收。一般／競態各 48 案例、1,920 條不同路徑、48 次新程序接續及 48 次原子性拒絕通過，controller 視窗另與公開神經歷史獨立比對。兩種刻意時鐘錯誤於兩個建置均被拒絕。840 份正式來源與已交付 ticket125 逐位相同，沿用前先重新雜湊完整一般／競態日誌並重算事件。本輪診斷、全框架 build／vet、依賴及治理重新執行，工程追蹤保持 91／91，見[驗收](evidence/MOD-02/persistent-source-clock-20261016/verification.json)。本票 4／4，最後 31 項治理通過，1,301,795,158 bytes 自有暫存已清除，原模型、資料、原始模組與其他工作保持，見[清理](evidence/MOD-02/persistent-source-clock-20261016/cleanup.json)。主要提交 `a2015a5` 已推送且遠端讀回相同，840 份提交來源核對完成，見[交付](evidence/MOD-02/persistent-source-clock-20261016/delivery.json)。沒有確認新的正式運算缺陷。下一項先驗證化學凍結與 Intervene 對照並用的旗標差異，不先改模型規則。

[ticket 125](docs/tickets/125-chemical-homeostasis-state.md) 已通過活動化學與慢速穩定並用的獨立持續狀態驗收。一般／競態各 640 條分段/恢復路徑、16 次新程序及 40 次 fresh neural 拒絕通過，兩種刻意錯誤被拒絕。840 份來源的完整一般／競態各 58 套件通過，7,915／7,914 pass、23／24 skip，原有各 7,165 個終端事件保持，新增各 772 個通過及一個明示的父程序入口 skip。格式、build、vet、固定依賴與 31 項治理通過，工程追蹤保持 91／91，見[驗收](evidence/MOD-02/homeostasis-chemical-state-20261016/verification.json)。本票 4／4，最後 31 項治理通過，1,685,320,683 bytes 自有暫存已清除，原模型、資料、原始模組與其他工作保持，見[清理](evidence/MOD-02/homeostasis-chemical-state-20261016/cleanup.json)。主要提交 `b8b7686` 已推送且遠端讀回相同，840 份提交來源核對完成，見[交付](evidence/MOD-02/homeostasis-chemical-state-20261016/delivery.json)。沒有確認新正式運算缺陷，整體機制稽核持續。下一個可驗證候選為[持續化學來源的絕對時間對照](evidence/MOD-02/homeostasis-chemical-state-20261016/next-audit.md)，尚未確認缺陷。

[ticket 124](docs/tickets/124-homeostasis-chemical-reference.md) 已通過活動化學與慢速穩定並用的獨立梯度及真實更新驗收。一般／競態各 80 組完整梯度、160 條訓練路徑及 253 個專項通過，錯傳 r/h 導數與漏更新休止步的對照均被拒絕。839 份來源的完整一般／競態各 58 套件通過，7,143／7,142 pass、22／23 skip，原有各 6,912 個終端事件保持，只新增各 253 個通過。格式、build、vet、固定依賴與 31 項治理通過，工程追蹤保持 91／91，見[驗收](evidence/MOD-02/homeostasis-chemical-gradient-20261016/verification.json)。本票 4／4，最後 31 項治理通過，1,584,963,058 bytes 自有暫存已清除，原模型、資料、原始模組與其他工作保持，見[清理](evidence/MOD-02/homeostasis-chemical-gradient-20261016/cleanup.json)。主要提交 `94de767` 已推送且遠端讀回相同，839 份提交來源核對完成，見[交付](evidence/MOD-02/homeostasis-chemical-gradient-20261016/delivery.json)。沒有確認新正式運算缺陷，整體機制稽核持續。下一個可驗證候選為[化學與慢速穩定並用的持續狀態讀回](evidence/MOD-02/homeostasis-chemical-gradient-20261016/next-audit.md)，尚未確認缺陷。

[ticket 123](docs/tickets/123-homeostasis-gradient-reference.md) 已通過慢速活動穩定的獨立梯度驗收。一般／競態各 120 條公開路徑、163 個專項通過，錯把 r/h 傳回的對照被手算拒絕。838 份來源的完整一般／競態各 58 套件通過，6,890／6,889 pass、22／23 skip，原有各 6,749 個終端事件保持，只新增各 163 個通過。格式、build、vet、固定依賴與 31 項治理通過，工程追蹤保持 91／91，見[驗收](evidence/COR-04/homeostasis-gradient-20261016/verification.json)。本票 4／4，最後 31 項治理通過，1,578,450,997 bytes 自有暫存已清除，原模型、資料、原始模組與其他工作保持，見[清理](evidence/COR-04/homeostasis-gradient-20261016/cleanup.json)。主要提交 `d1b7723` 已推送且遠端讀回相同，838 份提交來源核對完成，見[交付](evidence/COR-04/homeostasis-gradient-20261016/delivery.json)。沒有確認新正式運算缺陷，整體機制稽核持續。下一個可驗證候選為[活動化學與慢速穩定並用的獨立梯度](evidence/COR-04/homeostasis-gradient-20261016/next-audit.md)，尚未確認缺陷。

[ticket 122](docs/tickets/122-homeostasis-rate.md) 已完成 4／4。修正慢速活動率的中間溢位誤拒絕，正常公式與合法域保持。837 份來源的一般／競態各 58 套件通過，6,727／6,726 pass、22／23 skip，原有各 6,700 個終端事件保持，只新增各 49 個通過。build、vet、固定依賴與最後 31 項治理通過，COR-04 恢復 passed，工程追蹤 91／91。49 個專項、Rosetta amd64 的 98 個檢查，以及正常／舊保存對照通過，見[驗收](evidence/COR-04/homeostasis-rate-20261016/verification.json)。1,965,684,907 bytes 本輪暫存已清除，原模型、資料、原始模組與其他工作保持，見[清理](evidence/COR-04/homeostasis-rate-20261016/cleanup.json)。修正 `72bfcc1` 與主要證據 `c721643` 已正常推送且遠端讀回相同，837 份提交來源核對完成，見[交付](evidence/COR-04/homeostasis-rate-20261016/delivery.json)。整體機制稽核持續，下一個候選是[慢速穩定的獨立梯度](evidence/COR-04/homeostasis-rate-20261016/next-audit.md)。

[ticket 121](docs/tickets/121-lif-max-reference.md) 已完成 4／4。一般／競態各 72 組公開完整梯度、72 組分段待更新梯度與 11 個手算對照通過，七個原案例在各建置內逐位保持。兩個刻意算錯的正式運算對照都被新測試拒絕。836 份來源的完整一般／競態各 58 套件通過，6,678／6,677 pass、22／23 skip，原有各 6,542 個終端事件保持，只新增各 158 個通過。格式、build、vet、固定 Insyra v0.3.4、模組與 31 項治理通過，工程狀態保持 91／91，見[驗收](evidence/MOD-04/lif-max-reference-20261016/verification.json)。1,588,449,051 bytes 自有暫存已清除，原模型、資料、原始模組及其他工作保持，見[清理](evidence/MOD-04/lif-max-reference-20261016/cleanup.json)。主要提交 `c8c15f1` 已正常推送且遠端讀回相同，836 份來源與 82 個交付檔案的提交內容相符，見[交付](evidence/MOD-04/lif-max-reference-20261016/delivery.json)。沒有確認新正式程式缺陷，整體機制稽核持續，下一個候選為[慢速活動穩定的獨立梯度](evidence/MOD-04/lif-max-reference-20261016/next-audit.md)。

[ticket 120](docs/tickets/120-max-clamp-gradient.md) 的最大受體作用／負係數／夾限組合與正式回歸已通過驗收，沒有確認新程式缺陷。一般／競態各 336 組完整／分段訓練、36 個對照、56 個初始化學步、432 個獨立參考與 28 次新程序接續通過。835 份來源的完整一般／競態各 58 套件通過，6,520／6,519 pass、22／23 skip、零失敗，原有各 6,477 個終端事件保持，只新增各 65 個通過。格式、build、vet、固定 Insyra v0.3.4、模組與工程治理 31 項通過，見[驗收](evidence/MOD-04/max-clamp-gradient-20261016/verification.json)。ticket21 的負係數說明已修正，原有正式實作及模型格式保持。工程狀態保持 91／91，本票完成 4／4，最後治理 31 項通過，1,668,319,814 bytes 自有暫存已清除，原模型、資料、模組與其他工作保持，見[清理](evidence/MOD-04/max-clamp-gradient-20261016/cleanup.json)。主要提交 `4d1b89a` 已推送且遠端讀回相同，835 份來源及 229 個交付檔案的提交內容相符，見[交付](evidence/MOD-04/max-clamp-gradient-20261016/delivery.json)。下一個可驗證成果是[放電模型最大作用的完整獨立梯度對照](evidence/MOD-04/max-clamp-gradient-20261016/next-audit.md)，尚未確認新缺陷。整體缺陷檢查持續，真實品質沒有新結論。

[ticket 119](docs/tickets/119-partial-chemical-recompute.md) 已通過公開組合稽核。一般／競態各 192 組完整／分段結果一致，22 個機制關閉對照有效、54 個獨立參考及 16 次新程序接續通過，沒有確認新缺陷。834 份正式來源與上一輪驗收相同，沿用完整工程結果，另通過本輪診斷 vet、模組與 31 項治理。91／91 工程需求保持 passed，見[驗收](evidence/MOD-02/partial-source-recompute-20261016/verification.json)。最後 31 項治理通過，本輪 1,058,888,032 bytes 暫存已清除，18 個原模型、六份必要資料、原始模組與其他工作保持，見[清理](evidence/MOD-02/partial-source-recompute-20261016/cleanup.json)。主要證據提交 `3b2e15f` 已推送且遠端讀回相同，834 份來源與 117 個交付檔案的提交內容相符，見[交付](evidence/MOD-02/partial-source-recompute-20261016/delivery.json)。本票完成 4／4。下一個可驗證成果是[最大受體作用及夾限交界的梯度對照](evidence/MOD-02/partial-source-recompute-20261016/next-audit.md)，尚未確認缺陷。整體零 bug 目標持續，真實模型品質沒有新結論。

[ticket 118](docs/tickets/118-signed-gradient-finite.md) 的共用固定符號梯度有限值修正已通過完整工程驗收。834 份來源的一般／競態各 58 套件通過，6,455／6,454 pass、22／23 skip、零失敗，原有各 6,405 個終端事件保持，只新增 72 個通過。格式、build、vet、固定 Insyra v0.3.4、模組校驗及 31 項治理通過，正常核心、舊神經狀態與舊化學模型的 32／32／12 組接續結果保持。COR-08 恢復 passed，最新 91／91 工程需求 passed。見[驗收](evidence/COR-08/signed-gradient-overflow-20261016/verification.json)。最後 31 項治理通過，本輪 1,726,991,821 bytes 暫存已清除，原模型、資料、原始模組與其他工作保持，見[清理](evidence/COR-08/signed-gradient-overflow-20261016/cleanup.json)。修正 `29d611f` 與回歸／證據 `90cceb0` 已正常推送且遠端讀回相同，834 份來源及 96 個交付檔案的提交內容相符，見[交付](evidence/COR-08/signed-gradient-overflow-20261016/delivery.json)。本票完成 4／4。下一個可驗證成果是來源視窗尚未填滿時，多通道化學與分段重算的梯度對照，尚未確認新缺陷。整體零 bug 目標持續，真實模型品質沒有新結論。

[ticket 117](docs/tickets/117-state-parity.md) 已完成 4／4。一般／競態各 96 組混合神經元案例、3,744 個接續點與 80 次節點改編號對照相同，各機制關閉後都改變輸出，沒有確認新的核心缺陷。833 份正式來源相同，沿用既有完整驗收，這輪另通過診斷 vet 與 31 項治理。91／91 工程需求保持 passed，845,159,670 bytes 暫存已清除，原模型、資料、原始模組與其他工作保持。提交 `f12db13` 已推送且遠端讀回相同，833 份來源與 46 個交付檔案的提交內容相符，見[驗收](evidence/COR-04/state-parity-20261016/verification.json)、[清理](evidence/COR-04/state-parity-20261016/cleanup.json)及[交付](evidence/COR-04/state-parity-20261016/delivery.json)。下一個可驗證成果是非零化學與來源歷史下，一般訓練和分段重算的完整更新對照，尚未確認新缺陷。整體零 bug 目標保持進行中。

[ticket 116](docs/tickets/116-threshold-tail.md) 已完成 5／5。833 份來源的一般／競態各 58 套件通過，6,383／6,382 個事件成功、22／23 個略過、零失敗，原有各 6,136 個終端事件保持，新增各 269 個通過。格式、build、vet、固定 Insyra v0.3.4、模組校驗及最後 31 項治理通過，見[驗收](evidence/COR-04/threshold-tail-20261016/verification.json)。32 份正常核心、32 次舊神經狀態與 12 次舊化學模型接續相同。COR-04／09 恢復 passed，目前 91／91 工程需求 passed。必要 25 個工作檔已封存，本輪 1,702,725,267 bytes 暫存已清除，原模型、資料、原始模組及其他工作保持，見[清理](evidence/COR-04/threshold-tail-20261016/cleanup.json)。修正 `475cc75` 與回歸／證據 `1f644f7` 已推送且遠端讀回相同，833 份提交來源及 963 個唯一提交內容指紋相符，見[交付](evidence/COR-04/threshold-tail-20261016/delivery.json)。整體零 bug 目標保持進行中。

[ticket 115](docs/tickets/115-surrogate-extreme-audit.md) 已通過完整工程驗收。830 份來源的一般／競態各 58 套件通過，6,114／6,113 個測試事件成功、22／23 個略過、零失敗，原有各 6,085 個終端事件保持，只新增 51 個通過。格式、build、vet、固定 Insyra v0.3.4、相依性及最後 31 項治理通過，見[驗收](evidence/COR-09/surrogate-square-20261016/verification.json)。32 份正常核心紀錄、32 次舊神經狀態與 12 次舊化學模型接續相同。COR-04／09 恢復 passed，目前 91／91 工程需求 passed。本輪 1,661,870,007 bytes 暫存已清除，原模型、必要資料、原始模組及兩個舊工作目錄保持，見[清理](evidence/COR-09/surrogate-square-20261016/cleanup.json)。修正 `25685dc` 與回歸／證據 `4ff6619` 已正常推送且遠端讀回相同，830 份已提交來源與驗收指紋相符，見[交付](evidence/COR-09/surrogate-square-20261016/delivery.json)。本票完成 5／5。下一項是[閾值尾端斜率的待查假設](evidence/COR-09/surrogate-square-20261016/next-audit.md)，尚未公開重現。整體零 bug 目標保持進行中。

[ticket 114](docs/tickets/114-lif-event-adjoint.md) 已修正五條 LIF／混合反向路徑的事件乘加中間溢位。828 份來源的一般／競態各 58 套件通過，6,063／6,062 個測試事件成功、22／23 個略過、零失敗，原有各 6,062 個終端事件保持，只新增 23 個通過。格式、build、vet、固定 Insyra v0.3.4、相依性及最後 31 項治理通過，見[驗收](evidence/COR-09/event-adjoint-20261016/verification.json)。32 份正常核心紀錄、32 次舊神經狀態與 12 次舊化學模型接續逐位相同。COR-04／09 恢復 passed，目前 91／91 工程需求 passed。本輪 1,926,900,129 bytes 暫存已清除，原模型、必要資料、原始模組與兩個舊工作目錄保持，見[清理](evidence/COR-09/event-adjoint-20261016/cleanup.json)。修正 0fbdb89 與主要回歸／證據 6092f93 已正常推送且遠端讀回相同，828 份已提交來源相符，見[交付](evidence/COR-09/event-adjoint-20261016/delivery.json)。本票完成 5／5。下一個可驗證成果見 [ticket 115](docs/tickets/115-surrogate-extreme-audit.md)，尚未執行公開重現或修改程式。整體零 bug 目標保持進行中。

[ticket 113](docs/tickets/113-membrane-tau-derivative.md) 已修正十處膜時間導數的中間溢位。826 份來源的一般／競態各 58 套件通過，6,040／6,039 個測試事件成功、22／23 個略過、零失敗，原有各 6,035 個終端事件保持，只新增 27 個通過。格式、build、vet、固定 Insyra v0.3.4、相依性與 31 項治理通過，見[驗收](evidence/COR-04/zero-tau-adjoint-20261015/verification.json)。40 組正常核心、32 次舊神經狀態及 12 次舊化學模型的新程序接續逐位相同，見[候選驗收](evidence/COR-04/zero-tau-adjoint-20261015/candidate-verification.json)。COR-02／04／06／08／09 恢復 passed，目前 91／91 工程需求 passed。最後 31 項治理通過，本輪 1,915,525,802 bytes 暫存已清除，原模型、必要資料、原始模組檔與兩個舊工作目錄保持，見[清理](evidence/COR-04/zero-tau-adjoint-20261015/cleanup.json)。修正 a45bae8 與主要回歸／證據 c7a4d7a 已正常推送且遠端讀回相同，826 份已提交來源相符，見[交付](evidence/COR-04/zero-tau-adjoint-20261015/delivery.json)。本票完成 5／5。下一個可驗證成果是核對適應項在不應期內的反向傳遞，尚未確認新缺陷。整體零 bug 目標保持進行中。

[ticket 112](docs/tickets/112-chemical-threshold-sum.md) 已通過完整工程驗收。824 份來源的一般／競態各 58 套件通過，6,013／6,012 個事件成功、22／23 個略過、零失敗，原有各 5,951 個終端事件保持，只新增 84 個通過。格式、build、vet、固定依賴與 31 項治理通過，見[驗收](evidence/MOD-04/lif-threshold-sum-20261015/verification.json)。32 組正常核心及 12 次舊化學模型的新程序接續逐位相同，額外邊界控制一般／競態各 33 個事件通過。COR-04／MOD-04 恢復 passed，目前 91／91 工程需求 passed。最後 31 項治理通過，本輪 1,814,305,716 bytes 暫存已清除，原模型與資料保持，見[清理](evidence/MOD-04/lif-threshold-sum-20261015/cleanup.json)。修正 30ec039 與回歸／證據 ff67526 已正常推送且遠端讀回相同，824 份已提交來源相符，見[交付](evidence/MOD-04/lif-threshold-sum-20261015/delivery.json)。本票完成 5／5。下一個可驗證成果是核對不應期、化學門檻與持續狀態的模擬及訓練一致性，尚未判定有新缺陷。整體零 bug 目標保持進行中。

[ticket 111](docs/tickets/111-plasticity-time-definition.md) 已釐清局部可塑性的時間符號：原始規格沒有活動索引，ticket 18 的索引式誤稱為原式，第二階段完成狀態也過時。已修正文字並集中記錄既有的核心後同列訊號，不改模型計算或保存格式。七項文件檢查通過，823 份 Go／module 來源、18 個原模型與六份必要資料保持相同；同份來源的既有一般／競態原始日誌各核對八項相關通過，這輪沒有重新執行 Go，見[驗收](evidence/LRN-05/plasticity-time-definition-20261015/verification.json)。本票完成 4／4，提交 `4f6fa2c` 已推送且遠端讀回相同，見[交付](evidence/LRN-05/plasticity-time-definition-20261015/delivery.json)。沒有建立本票暫存。工程狀態保持 91／91 passed，生物時點與任務效益沒有新結論。下一個可驗證成果是核對適應、穩定項與化學偏移相加時，是否會過早拒絕可表示的門檻；這是待查假設，尚非確認缺陷。整體零 bug 目標保持進行中。

[ticket 110](docs/tickets/110-chemical-threshold-floor.md) 已通過完整工程驗收。823 份來源的一般／競態各 58 套件通過，5,929／5,928 個事件成功、22／23 個略過、零失敗；原有各 5,937 個事件保持，只新增本票 14 個通過。格式、build、vet、固定依賴與 31 項治理通過，見[驗收](evidence/MOD-04/lif-threshold-floor-20261015/verification.json)。32 組正常核心及 12 次舊化學模型新程序接續逐位相同。MOD-04 恢復 passed，最新 91／91 工程需求 passed。最後 31 項治理通過，本輪 1,789,486,258 bytes 暫存已清除，18 個原模型、6 個必要資料與兩個舊目錄保持，見[清理](evidence/MOD-04/lif-threshold-floor-20261015/cleanup.json)。修正 007d4b3 與回歸／證據 76689bf 已正常推送且遠端讀回相同，823 份已提交來源相符，見[交付](evidence/MOD-04/lif-threshold-floor-20261015/delivery.json)。本票完成 5／5。下一個可驗證成果是局部可塑性時間符號與既有手算、呼叫端的契約核對，不先改正常模型。整體零 bug 目標保持進行中。

[ticket 109](docs/tickets/109-textgen-vocabulary-dimensions.md) 的文字模型詞表尺寸修正已通過完整工程驗收。822 份來源的一般／競態各 58 套件通過，5,915／5,914 個成功事件、22／23 個略過、零失敗，格式、build、vet 及固定依賴通過，見[驗收](evidence/TSK-03/textgen-dimensions-20261015/verification.json)。8 組正常模型與 16 次舊保存新程序接續逐位相同，TSK-03／OPS-01 恢復 passed，目前 91／91 工程需求 passed。最後 31 項治理通過，本輪 1,848,615,536 bytes 暫存已清除，原模型、必要資料與兩個舊暫存目錄保持，見[清理](evidence/TSK-03/textgen-dimensions-20261015/cleanup.json)。本票已完成 5／5，修正 `71ad3af` 與回歸／證據 `bdb1fea` 已正常推送且遠端讀回相同，822 份已提交來源相符，見[交付](evidence/TSK-03/textgen-dimensions-20261015/delivery.json)。下一個可驗證成果是釐清既有 LIF 閾值位移兩條路徑的拒絕語意，先核對契約，不先改正常模型。整體零 bug 目標持續進行。

[ticket 108](docs/tickets/108-asr-constructor-dimensions.md) 已完成 5／5。ASR 會在配置前拒絕無法表示的節點、邊與陣列尺寸。最終 820 份 Go／module 的一般／競態各 58 套件通過，5,891／5,890 個事件成功、22／23 個略過、零失敗，build、vet、格式及固定相依性通過，見[驗收](evidence/TSK-02/asr-dimensions-20261015/verification.json)。四組合法模型及 16 次跨版本新程序接續相同。最後 31 項治理通過，自有暫存清除，修正 `5be55c2` 與回歸／證據 `b9b465b` 已正常推送且遠端讀回相同，見[交付](evidence/TSK-02/asr-dimensions-20261015/delivery.json)。OPS-01／TSK-02 已恢復 passed，最新為 91／91 工程需求 passed，整體零 bug 目標持續進行。

[ticket 107](docs/tickets/107-recognizer-unicode-construction.md) 已完成 5／5。OCR／ASR 在建立模型時拒絕原本無法保存的非法字碼，正常模型、舊保存及串流防護保持。專項兩種建置各 353 個事件通過，接續 108 的完整一般／競態與最後 31 項治理通過，見[驗收](evidence/TSK-01/recognizer-unicode-20261015/verification.json)。64 個必要工作檔已保存，自有暫存清除。修正 `c877ab2` 與回歸／證據 `b9b465b` 已正常推送且遠端讀回相同，見[交付](evidence/TSK-01/recognizer-unicode-20261015/delivery.json)。TSK-01／TSK-02 已恢復 passed。

[ticket 106](docs/tickets/106-operational-project-temp.md) 已完成操作腳本與教學的專案內保存，乾淨匯出建置也不再帶入外層工作區版本資訊。永久回歸一般／競態各 14 個事件通過，完整一般／競態各 58 套件通過，5,840／5,839 個事件成功、22／23 個略過、零失敗。build、vet、格式、固定相依性及最後 11 項治理檢查通過，見[驗收](evidence/OPS-03/operational-temp-paths-20261015/verification.json)。README 的訓練、接續與推論已實跑，必要快照及輸入原文已保存。自有暫存已清除，其他五個工作目錄、815 份 Go／module、18 個模型及固定訓練輸入保持，見[清理](evidence/OPS-03/operational-temp-paths-20261015/cleanup.json)。本票完成 4／4。修正 `30713b6` 與回歸／證據 `d62f677` 已正常推送且遠端讀回相同，815 份提交來源指紋一致，見[交付](evidence/OPS-03/operational-temp-paths-20261015/delivery.json)。

[ticket 105](docs/tickets/105-recompute-project-temp.md) 已修正重算工具與文件範例的暫存路徑，工作留在 `runs/.tmp/`，只清除本次子目錄，共用目錄與原結果保留。一般／競態專項各 8 個事件通過，四個真實小圖 helper 與三次文件範例完成，必要快照原檔已保存。完整一般／競態各 58 套件通過，5,826／5,825 個事件成功、22／23 個略過、零失敗，build、vet、格式、相依性及最後 11 項治理／31 個事件通過，見[驗收](evidence/OPS-03/recompute-temp-path-20261015/verification.json)。本票完成 4／4。修正 `eedb19a` 與回歸／證據 `668fd08` 已正常推送且遠端讀回相同，見[交付](evidence/OPS-03/recompute-temp-path-20261015/delivery.json)。自有暫存與教學 fixture 共 36,423,885 bytes 已清除。原有 812 份 Go／module、18 個模型及 ticket 104 固定輸入保持，新增工具測試不參與既有訓練程式。沒有本票交付阻礙，下一個可驗證成果是 ticket 106 的永久回歸整合。整體零 bug 目標持續進行。

[ticket 104](docs/tickets/104-lif-real-task-build-quality.md) 已完成 5／5。12 組固定 LIF 訓練、新程序接續、288 份評分及六組數值陣列通過獨立核對，見[驗收](evidence/TSK-11/lif-build-quality-20261015/verification.json)。主要辨識率 68.30%～71.00%，沒有穩定化學助益證據。六組一般／競態最終狀態不同，各自建置內的保存接續相同，見[結果](evidence/TSK-11/lif-build-quality-20261015/README.md)。六份必要資料保存在專案 data/，16 份恢復封存已保存，自有 1,769,612,968 bytes 暫存已清除。最後 31 項治理通過，來源／證據提交 `b9b465b` 已正常推送且遠端讀回相同，見[交付](evidence/TSK-11/lif-build-quality-20261015/delivery.json)。

[ticket 103](docs/tickets/103-lif-threshold-range.md) 已修正合法閾值跨度溢位及最終閾值溢位未拒絕的缺陷。一般／競態各 40 個永久回歸通過，32 組正常訓練及 64 次舊快照新程序接續相同。完整一般／競態各 58 套件通過，5,818／5,817 個事件成功、22／23 個略過、0 失敗，build、vet、格式、固定相依性與最後 11 項治理／31 事件通過，見[驗收](evidence/COR-04/threshold-range-audit-20261015/verification.json)。812 份提交來源與驗證指紋相同，最新 91 項工程 passed。本輪兩個暫存共 1,743,722,445 bytes 已清除，原有目錄及 18 個原模型檔保持。修正 `29dffdc` 與回歸／證據 `b433d26` 已正常推送且遠端讀回相同，見[交付](evidence/COR-04/threshold-range-audit-20261015/delivery.json)。本票完成 5／5，沒有目前交付阻礙。下一個可驗證成果是固定真實單字元資料、種子與預算的 LIF 長期辨識品質量測，下一票尚未建立。整體零 bug 目標保持進行中。負向調變取下界的既有契約差異見 AGENTS.md Follow-ups。

[ticket 102](docs/tickets/102-ocr-build-quality.md) 已完成 12 組固定 KMNIST 單字元訓練，各 20 輪／120,000 更新及新程序載入。六個普通／競態配對的完整參數、最佳化器、逐輪品質、圖片預測及下一次更新逐位相同。化學 ON 的主要測試辨識率差為 −0.10／+0.80／+0.15 個百分點，三組主要分類損失都變差，沒有穩定助益證據。Root 獨立核對 288 份評分及全部保存數值，見[結果](evidence/TSK-11/build-quality-audit-20261015/README.md)與[驗收](evidence/TSK-11/build-quality-audit-20261015/verification.json)。完整一般／競態各 58 套件通過，5,778／5,777 個測試事件成功、22／23 個略過、0 個失敗，build、vet、格式及相依性通過。Root 的驗收與數值報告缺陷已補正，正式 Go、driver、協定、原模型及訓練預算保持。最後 11 項治理／31 個事件通過，主暫存 1,279,139,577 bytes 與重現暫存 653,211,566 bytes 已清除，原資料與 18 個原模型檔保持。本票完成 5／5，稽核提交 9ff738f 已正常推送且遠端讀回相同，見[交付](evidence/TSK-11/build-quality-audit-20261015/delivery.json)。必要日誌壓縮保留。本任務未執行 LIF candidate，LIF 長期品質與整體零 bug 目標保持未知／進行中，工程需求保持 91 項 passed。

[ticket 101](docs/tickets/101-lif-build-numerics.md) 已確認 LIF 的跨建置差異來自 candidate 融合不同乘法項，是 Go 允許的取整結果。普通／race 各自的公開 Network／LIF、唯讀 trace、獨立完整前向／反向及持續狀態整批／單步對照逐位相同，4 次新程序重跑保持，正常公式、模型格式與依賴不變。完整一般／race 各 58 套件通過，5,778／5,777 pass、22／23 skip、0 fail，格式、build、vet、相依性及最後 11 項治理／31 個事件通過，見[驗收](evidence/LRN-01/lif-float-audit-20261015/verification.json)。810 份來源保持，本輪 1,319,867,403 bytes 暫存已清除，18 個原模型檔、原有目錄及 41,797 個連結來源保持，見[清理](evidence/LRN-01/lif-float-audit-20261015/cleanup.json)。本票完成 4／4，稽核提交 `6b7bea6` 已正常推送且遠端讀回相同，見[交付](evidence/LRN-01/lif-float-audit-20261015/delivery.json)。目前沒有阻礙。91 項工程 passed 與整體零 bug 目標分開追蹤。下一個可驗證成果是固定單字辨識任務的跨建置成對訓練品質量測，下一票尚未建立，先固定資料、種子與預算。

[ticket 100](docs/tickets/100-gpu-cancel-preservation.md) 的真實 Apple M3 Metal 更新中取消與恢復已通過，一般／race 各 1,916 個可達點，50 次新程序接續逐位元組相同。兩種建置另各 1,624 個重複 Trainer 點讀回實際選中 GPU bank，取消狀態與原物件重試一致，沒有新確認取消缺陷。完整一般／race 各 58 套件、格式、build、vet、固定相依性及最後 31 個治理事件通過，810 份來源保持，見[驗收](evidence/OPS-05/gpu-cancel-audit-20261015/verification.json)。本輪 1,497,331,491 bytes 暫存已清除，18 個原模型檔、原有暫存目錄及模組來源保持，見[清理](evidence/OPS-05/gpu-cancel-audit-20261015/cleanup.json)。稽核提交 02a13ea 已正常推送且遠端讀回相同，810 份提交來源相符，見[交付](evidence/OPS-05/gpu-cancel-audit-20261015/delivery.json)。本票完成 4／4，沒有目前阻礙。浮點差異已推到第一次更新的 Core.Inputs[4][2]，早於 Adam，精確運算與品質影響未確認，見[診斷](evidence/OPS-05/gpu-cancel-audit-20261015/core-input-gradient-comparison.json)。下一個可驗證成果是以相同編碼與讀出邊界值隔離 LIF 前向及反向，確認第一個浮點差異的來源。下一票尚未建立。91 項工程 passed 與整體零 bug 目標分開追蹤。

[ticket 99](docs/tickets/99-training-cancel-preservation.md) 已完成 40 種合法設定，另補驗其中 20 種視窗 3 的滿窗更新。一般／race 各 7,408 個取消點保持完整快照與化學報告，原個體及恢復重試與對照相同。120 次新程序接續在各自相同建置下逐位一致，相關控制一般／race 各 42 個事件通過。沒有新確認取消缺陷，810 份來源保持。build、vet、相依性、格式及最後 31 個治理事件通過，完整一般／race 沿用已核對相同來源的上一輪證據，見[驗收](evidence/STA-03/training-cancel-audit-20261015/verification.json)。一般與競態建置的一個 Adam 第二動量有一個 float64 間距差異，兩者實際機器碼都使用融合乘加，精確來源待查，沒有改正常演算法。GPU 候選逐階段取消尚未實測。兩個本輪暫存目錄共 1,629,450,457 bytes 已清除，原有目錄及 18 個原模型檔保持，見[初次清理](evidence/STA-03/training-cancel-audit-20261015/cleanup.json)及[補驗清理](evidence/STA-03/training-cancel-audit-20261015/cleanup-full-window.json)。稽核提交 `efc1df5` 已正常推送且遠端讀回相同，810 份提交來源相符，見[交付](evidence/STA-03/training-cancel-audit-20261015/delivery.json)。本票完成 4／4，最新為 91 項 passed，整體零 bug 目標保持進行中。目前沒有交付阻礙。下一個可驗證成果是 GPU 候選各階段取消與跨建置浮點第一個分歧值的獨立診斷。下一票尚未建立，先固定驗收契約，不改正常模型。

[ticket 98](docs/tickets/98-training-failure-composition.md) 已修正有限梯度累積溢位仍回成功、使模型無法保存或恢復的缺陷。28 組正負／視窗／公開入口及凍結控制，Root 一般／race 各 30 個事件通過。32 種正常設定的修改前後結果及 64 次新程序恢復逐位相同，六個既有 ON／OFF 模型的推論與下一次更新保持。完整一般／race 各 58 套件通過，5,778／5,777 pass、22／23 skip、0 fail，格式、build、vet、相依性與 CLI 接續也通過，見[驗證](evidence/LRN-06/training-failure-audit-20261015/verification.json)。810 份來源及凍結回歸指紋保持。最新為 91 項 passed。最後 11 項治理、31 個事件通過，本輪 1,396,734,821 bytes 暫存已清除，原有目錄與 18 個模型原檔保持，見[清理](evidence/LRN-06/training-failure-audit-20261015/cleanup.json)。修正 `5e26db8` 與回歸／證據 `0393ad8` 已正常推送且遠端讀回相同，810 份提交來源相符，見[交付](evidence/LRN-06/training-failure-audit-20261015/delivery.json)。本票完成 4／4，沒有目前阻礙。下一個可驗證成果是訓練取消時的個體歷史與保存一致性。下一票尚未建立，先核對公開契約與可重現案例。原始零 bug 目標保持進行中，真實任務品質提升尚未證明。

[ticket 97](docs/tickets/97-chemical-restore-composition.md) 的存檔欄位與化學載入修正已通過完整工程驗證。46 個公開回歸、48 份舊個體新程序接續、三種舊保存物、八個原始拒絕及 14 個額外非法集合案例通過。六個既有 ON／OFF 手寫辨識模型的 SDK 推論與下一次更新相同，18 個模型原檔保持。完整一般／race 各 58 套件通過，5,748／5,747 pass、22／23 skip、0 fail，格式、build、vet、相依性、CLI 接續及 809 份來源指紋核對通過，見[驗證](evidence/STA-03/chemical-restore-audit-20261014/verification.json)。STA-02 恢復 passed，最新為 91 項 passed。最後 11 項治理通過，本輪 2,122,375,284 bytes 暫存已清除，原有資料、模型及暫存目錄保持，見[清理](evidence/STA-03/chemical-restore-audit-20261014/cleanup.json)。修正 `0f6c87d` 與回歸／證據 `6e811e0` 已正常推送且遠端讀回相同，809 份提交來源相符，見[交付](evidence/STA-03/chemical-restore-audit-20261014/delivery.json)。本票完成 4／4。下一個可驗證成果是訓練更新失敗時的參數與狀態保留稽核。完整零 bug 與真實任務品質提升尚未證明。

[ticket 96](docs/tickets/96-expression-gradient-composition.md) 的有限範圍梯度稽核已通過：160 組一般／race 案例一致，80 組觀察到不同區域受體反應，四種錯誤參考均抓到數值差異，原有化學回歸各 100 個事件通過。正式 806 份來源及既有測試不變，沒有新確認缺陷。build、vet、固定相依性及治理 31 個事件通過，本輪 891,799,195 bytes 暫存已清除，原有資料與暫存保留，見[驗證](evidence/MOD-06/expression-gradient-audit-20261014/verification.json)及[清理](evidence/MOD-06/expression-gradient-audit-20261014/cleanup.json)。本票完成 4／4，提交 `4b1e209` 已正常推送且遠端讀回相同，806 份提交來源指紋相符，見[交付](evidence/MOD-06/expression-gradient-audit-20261014/delivery.json)。工程需求保持 91 項 passed，完整零 bug 與真實品質提升尚未證明。

[ticket 89](docs/tickets/89-chemical-composition-neutrality.md) 的正式中性增益修正已通過工程驗收。20 個原始失敗事件改為 21 個通過事件，15 組合法舊個體及 10 組合法舊訓練存檔／模型包的後續結果完全相同，3 組舊個體及 10 組舊訓練存檔／模型包明確拒絕且原檔不變。完整一般／race 各 58 套件通過，5702／5701 pass，22／23 skip，沒有 fail，其餘工程與 CLI 接續通過。最新工程需求為 91 項 passed，見[本輪驗收](evidence/MOD-04/neutral-gain-adoption-20261014/verification.json)。最後 11 項治理通過，本輪 1,304,068,945 bytes 暫存已清除，原有暫存目錄保留，見[清理](evidence/MOD-04/neutral-gain-adoption-20261014/cleanup.json)。修正 `90914c3` 與回歸／證據 `831158f` 已正常推送，遠端讀回相同，806 份提交來源相符，見[交付](evidence/MOD-04/neutral-gain-adoption-20261014/delivery.json)。本票已完成 4／4。整體零 bug 與真實品質提升尚未證明。

以下為歷史階段紀錄，目前狀態以首段及需求追蹤檔為準。

[ticket 95](docs/tickets/95-ocr-generate-representability.md) 已完成 5／5。OCR 不可表示的樣本／字長宣告已在訓練前回傳錯誤，原始 11 個失敗事件與修正後 46 個回歸事件有實跑證據。28 組正常完整 JSON 及三種舊 OCR 模式的新程序載入、辨識與下一次參數更新相同。完整一般／race 各 58 套件通過，5,681／5,680 pass，22／23 skip，沒有 fail，build、vet、固定相依性、公開 API 文件及 CLI 接續通過，見[驗收](evidence/TSK-01/glyph-contract-20261014/verification.json)。最後 11 項治理通過，1,225,230,613 bytes 本輪暫存已清除，原有目錄保留，見[清理](evidence/TSK-01/glyph-contract-20261014/cleanup.json)。修正 `0ac2565` 與回歸／證據 `e492041` 已正常推送，遠端讀回相同，804 份提交來源相符，見[交付](evidence/TSK-01/glyph-contract-20261014/delivery.json)。下一個需要決策的成果是 ticket 89 的中性化學增益修正；未獲相容性決策前不套用候選。ticket 89 的相容性決策 pending，反相字距語意尚待釐清，最新 90 項 passed、1 項 failed（MOD-04），沒有整體零 bug 或真實辨識率提升結論。

[ticket 94](docs/tickets/94-native-mean-representability.md) 已完成 5／5。有限平均與量測總和發布缺陷已修正，13 個永久回歸事件及 LIF 補驗一般／race 各 3 個事件通過。16 組正常模型的全部 Go JSON 位元組與舊狀態新程序接續相同。完整一般／race 各 58 套件、其餘工程、CLI 接續及最後 11 項治理通過。1,211,375,287 bytes 本輪暫存已清除，原有目錄保留。修正 37cca36 與回歸／證據 ea5d0ec 已推送並讀回一致，802 份提交來源相符，見[驗收](evidence/NAT-01/native-mean-20261014/verification.json)與[交付](evidence/NAT-01/native-mean-20261014/delivery.json)。NAT-01 恢復 passed，最新 90 項 passed、1 項 failed（MOD-04）。ticket 89 的相容性決策 pending，整體零 bug 與真實品質提升尚未證明。

[ticket 93](docs/tickets/93-native-plastic-norm-representability.md) 已完成 5／5。68 個舊來源失敗事件、97 個修正後通過事件，20 組合法配置在舊／新來源與新程序重跑完全相同。完整一般／race 各 58 套件、其餘工程、CLI 新程序保存接續與最後 11 項治理通過。NAT-06 恢復 passed，1,203,188,028 bytes 本輪暫存已清除，原有目錄保留。修正 7240ad3 與回歸／證據 d5a1a51 已推送並讀回一致，801 份提交來源相符，見[驗收](evidence/NAT-06/native-plastic-norm-20261014/verification.json)與[交付](evidence/NAT-06/native-plastic-norm-20261014/delivery.json)。另確認 NAT-01 合法平均值被錯誤拒絕，接續由 [ticket 94](docs/tickets/94-native-mean-representability.md) 修正。最新狀態為 89 項 passed、2 項 failed（MOD-04／NAT-01）。ticket 89 的相容性決策維持 pending，整體零 bug 與真實品質提升尚未證明。

[ticket 92](docs/tickets/92-native-receptor-declaration-refusal.md) 已完成 5／5。144 個受體宣告案例在舊來源失敗，修正後 168 個測試事件通過。14 組合法模型在舊來源、修正來源與新程序完全相同。完整一般／race 各 58 套件、build、vet、相依性、CLI 接續與最後 11 項治理通過，MOD-05 恢復 passed。本輪 1,187,300,480 bytes 暫存已清除，原有目錄保留。修正 1fb539b 與回歸／證據 5c95f11 已推送並核對遠端，799 份提交來源相符，見[驗收](evidence/MOD-05/native-receptor-refusal-20261014/verification.json)與[交付](evidence/MOD-05/native-receptor-refusal-20261014/delivery.json)。另確認 NAT-06 原生幅度報告的中間平方溢位／下溢，下一個可驗證成果是 [ticket 93](docs/tickets/93-native-plastic-norm-representability.md) 的公開失敗回歸與最小數值修正。最新狀態為 89 項 passed、2 項 failed（MOD-04／NAT-06）。ticket 89 的相容性決策 pending，整體零 bug 與真實品質提升尚未證明。

[ticket 91](docs/tickets/91-receptor-rule-ownership-and-restore.md) 已完成 5／5。69 個 RED 失敗事件確認受體指標別名及恢復交叉驗證缺口，原有缺省控制 8 個事件通過。Root 先凍結測試再補獨立複製與共用綁定檢查，修正後 77 個事件通過。32 組合法模型與新程序接續相同，完整一般／race 各 58 套件及其餘工程／CLI 驗證通過，最後 11 項治理通過，本輪 1,251,214,520 bytes 暫存已清除，原有目錄保留，修正 4a3edcf 與回歸／證據 21441c0 已推送並核對遠端，798 份提交來源相符，見[本輪驗證](evidence/MOD-05/receptor-rule-ownership-20261014/verification.json)及[交付](evidence/MOD-05/receptor-rule-ownership-20261014/delivery.json)。全樹追蹤另以 6 個公開案例確認原生模擬忽略受體宣告，MOD-05 標 failed，最新需求狀態為 89 項 passed、2 項 failed，下一個可驗證成果是[ticket 92](docs/tickets/92-native-receptor-declaration-refusal.md)拒絕原生入口未綁定的受體宣告，並保留正常模型。ticket 89 的相容性決策維持 pending，整體零 bug 與真實品質提升尚未證明。

[ticket 90](docs/tickets/90-slow-memory-composition.md) 已完成 5／5。已重現各記憶層有限、鞏固成功後共同有效權重才溢位的 P2 缺陷。正式修正與永久回歸已完成，正常 54 組輸出／記憶／報告在舊來源、修正來源及新程序逐位一致。完整一般／race 各 58 套件與 CLI 接續通過，MOD-06 恢復 passed，最後 11 項治理通過，本輪 1,331,266,511 bytes 暫存已清除，原有目錄保留。修正 ccab7e0、回歸／證據 10ca882 已推送並讀回相同，796 份提交來源相符。另有受體指標所有權 P2 已由四個公開案例確認，列入 Follow-ups，不混入本票來源。ticket 89 的相容性決策保持 pending，尚無整體零 bug 或真實品質提升結論。

[ticket 89](docs/tickets/89-chemical-composition-neutrality.md) 目前完成 2／4。公開入口 15 個零作用案例確認 9 個偏離中性增益，MOD-04 標為 failed，最新工程需求狀態為 90 項 passed、1 項 failed。120 個歷史／分段組合、來源 Replay 時鐘及 96 個初始梯度斷言通過一般與 race 檢查。候選化學模組的一般／race 各 375 個事件通過，但正式程式沒有套用。要求增益範圍包含 1 會拒絕部分舊模型，使用者相容性決策 pending。見[驗證](evidence/MOD-02/chemical-composition-20261014/verification.json)與[Root 審查](evidence/MOD-02/chemical-composition-20261014/root-review.md)。本輪 872,621,783 bytes 暫存已清除，793 份提交來源相符，主要稽核 d8b013e 已推送並讀回一致，見[交付](evidence/MOD-02/chemical-composition-20261014/delivery.json)。本票尚未完成修正與完整工程驗收，沒有整體零 bug 或真實品質提升結論。

[ticket 88](docs/tickets/88-mixed-chemical-gradient-reference.md) 已完成 4／4。未明示 InitialActivity 時首步來源的 Core.Initial 漏算已修正，五組舊來源失敗案例通過。八組混合參考的前向差為零、梯度最大絕對差 5.55111512e-17，四種刻意錯誤均被抓到。完整一般與 race 各 58 套件、格式、build、vet、相依性及最後 11 項治理通過，793 份提交來源與 457 份凍結測試相符。修正 f4dc57f 與主要回歸／證據 5a85737 已推送且遠端相同。本輪 1,942,892,872 bytes 暫存與 10,177 個快取連結已清除，原有暫存保留。見[驗證](evidence/MOD-02/mixed-chemical-gradient-20261014/verification.json)、[Root 審查](evidence/MOD-02/mixed-chemical-gradient-20261014/root-review.md)、[清理](evidence/MOD-02/mixed-chemical-gradient-20261014/cleanup.json)與[交付](evidence/MOD-02/mixed-chemical-gradient-20261014/delivery.json)。整體零 bug 與真實辨識率提升尚未證明。

ticket 87 已完成 5／5 驗收與交付：四類不可表示配置已回錯，34 個回歸事件與 15 個非法公開診斷案例通過。完整一般／race 各 58 套件、格式、build、vet、固定相依性、11 項治理、32 組合法模型逐位比較、8 組活動生命週期及 CLI 新程序接續通過。791 份來源與 455 份凍結測試相符，主要提交內的 791 份來源也逐一核對。修正 `9568fa8` 與回歸／證據 `1ea4e50` 已推送且遠端相同。本輪 9,426 個檔案與 20,501 個快取連結、1,404,334,193 bytes 已清除，原有暫存目錄保留。見[本票](docs/tickets/87-chemical-capacity-validation.md)、[工程驗收](evidence/MOD-02/chemical-capacity-20261014/verification.json)、[清理](evidence/MOD-02/chemical-capacity-20261014/cleanup.json)與[交付](evidence/MOD-02/chemical-capacity-20261014/delivery.json)。正常化學設計未確認矛盾，混合連續／脈衝完整反向的獨立參考缺口已列 Follow-ups，尚無零 bug 或真實辨識率提升結論。

ticket 86 已完成 5／5 驗收與交付。兩個共享狀態缺陷已修正，5 個頂層回歸、31 個測試事件及 20 次 race 共 620 個事件通過。完整一般／race 各 58 套件、build、vet、格式、相依性、11 個治理檢查、32 組合法逐位比較及 CLI 新程序接續通過，788 份來源相符。本輪 1,713,255,539 bytes 暫存已清除，修正 `a008c1e` 與回歸／證據 `d73dc5e` 已推送且遠端相同。模型格式、原算式與混合部分成功契約保持。見[工程驗收](evidence/LRN-06/online-individual-boundary-20261014/verification.json)、[清理](evidence/LRN-06/online-individual-boundary-20261014/cleanup.json)與[交付](evidence/LRN-06/online-individual-boundary-20261014/delivery.json)。尚未證明整體零 bug 或真實辨識率提升。

ticket 85 的混合重試修正已完成 5／5 驗收與交付：10 個新增頂層回歸、32 組正常逐位相容、完整一般／race 各 58 套件、build、vet、相依性與最後 11 項治理通過，785 份來源指紋相符。本輪 2,345,848,209 bytes 暫存已清除，修正 5d8d506、回歸／證據 89fa5a0 及原件保存校正 fa5ac1f 已推送且遠端相同。部分成功不回滾。獨立診斷另確認作答版本錯接及零分回饋非零梯度，已列[ticket 86](docs/tickets/86-online-individual-boundary.md)待修，尚未證明整體零 bug 或真實辨識率提升。見[工程驗收](evidence/LRN-06/online-boundaries-20261014/verification.json)、[清理](evidence/LRN-06/online-boundaries-20261014/cleanup.json)與[交付](evidence/LRN-06/online-boundaries-20261014/delivery.json)。

tickets 78–84的七類缺陷已修復並完成5／5驗收。22個新增回歸、完整一般與race各58套件、build、vet、格式、相依性及最後11項治理通過，782份來源指紋一致，合法公開結果與CLI新程序接續保持。本輪11,212檔與5,597個快取連結共1,878,186,656bytes已清除，提交e7fcc71／f0f0d01已推送並讀回相符。見[工程驗收](evidence/MOD-02/chemical-boundaries-20261013/verification.json)、[清理](evidence/MOD-02/chemical-boundaries-20261013/cleanup.json)與[交付](evidence/MOD-02/chemical-boundaries-20261013/delivery.json)。尚未證明零bug或辨識率提升。

tickets 74–77 的四類公開缺陷已修復，build、完整一般與 race 各 58 套件、vet、相依性與格式通過。16 個新增頂層回歸均通過，一般／race 分別 5,006／5,005 個測試事件，差異是既有大型模型的 race 記憶體跳過。18 組合法渲染、9 組 OCR 小型報告及 60 列控制器正常更新與修正前逐位相同，775 份來源指紋核對相符。最後 11 項治理及暫存清理已完成，9,683 檔共 1,419,291,751 bytes 的本輪暫存已清除，修正 `23582a2` 與回歸／證據 `dd185a3` 已推送，遠端讀回相符。沒有品質提升或零 bug 結論。見[最終工程驗收](evidence/MOD-06/refusal-input-audit-20261013/verification.json)。

機制與設計稽核的 tickets 67–73 已完成驗收與交付，共修正七類缺陷。build、完整一般與 race 各 58 套件、vet、相依性及最後 11 項治理全部通過，770 份來源指紋相符。合法 OCR／音訊的指定對照與基準逐位相同，模型保存格式保持。提交 `768ab10`／`354b2be` 已推送至 `codex/receptor-chemistry-training`，遠端讀回相符。本輪 9,859 個暫存檔、1,469,925,640 bytes 已清除，原有暫存目錄、資料及模型保留。鞏固拒絕報告的計數缺口記為 P2，未確認模型品質受到影響。受體部位不建立多區室膜模型，沒有生物定量正確性、辨識率改善或絕對零 bug 的結論。見[機制稽核](evidence/MOD-02/mechanism-audit-20261013/README.md)、[最終驗收](evidence/MOD-02/mechanism-audit-20261013/verification.json)、[清理](evidence/MOD-02/mechanism-audit-20261013/cleanup.json)與[交付回條](evidence/MOD-02/mechanism-audit-20261013/delivery.json)。

[ticket 66](docs/tickets/66-real-ocr-chemical-comparison.md) 完成 5／5 驗收與交付。三組 KMNIST 化學配對的主要平均辨識率由 72.07% 到 72.35%，增加 0.28 個百分點。確認分割增加 0.22 個百分點，兩批都有一個種子退步，六組都低於 80% 品質門檻。六個模型新程序恢復、三個 OFF 舊快照重現、五項 driver 與 race、十張真實圖片 SDK 比對通過。完整一般與 race 各 58 套件、格式、build、vet、相依性、CLI 接續及治理 11 項通過，772 份來源及兩份 evidence Go 指紋一致。比較工具已補強來源與歷史評分算式核對，沒有修改化學數值核心。主要證據 `a37d569` 已推送並讀回一致，本輪 28,011 個暫存檔共 1.69 GB 邏輯大小已清除，33 個原始資料及模型檔案保留。見[最終驗收](evidence/TSK-11/chemical-quality-20261013/verification.json)、[清理](evidence/TSK-11/chemical-quality-20261013/cleanup.json)與[交付回條](evidence/TSK-11/chemical-quality-20261013/delivery.json)。

[ticket 65](docs/tickets/65-receptor-location-and-chemical-training.md) 已完成 5／5 驗收與交付。有來源的受體位置、一般化學回授訓練、新程序接續、失敗原子性及分段重算／關閉對照通過。完整一般與 race 各 58 套件、格式、build、vet、相依性、CLI 新程序接續與最後治理 11 項通過，772 份來源及兩份 evidence Go 程式指紋相符。本輪 62,032 個暫存檔共 6.15 GB 與 1,323 個連結已清除。實作 `9ed31a5` 推送至 `codex/receptor-chemistry-training` 並讀回一致，見[最終驗收](evidence/MOD-02/receptor-chemistry-20261012/verification.json)、[清理](evidence/MOD-02/receptor-chemistry-20261012/cleanup.json)與[交付回條](evidence/MOD-02/receptor-chemistry-20261012/delivery.json)。實測定位只有細胞類型／樹突觀察，細胞內位置不建立獨立膜電位，反應係數採工程設定。生物定量正確性與 OCR 品質改善沒有由本票驗證。

[ticket 64](docs/tickets/64-real-task-quality.md) 的單字元模式修正、品質驗證與交付已完成。新模式、169 個相關測試、實際 CLI、舊模型逐位元組相容、完整一般／race 各 57 套件、格式、build、vet、相依性及 13 個治理專項通過。三個種子的正式測試為 73.25%、71.45%、71.50%，低於 80% 門檻及四個最近鄰的 81.85%，模型品質沒有達標。已清除 57,028 個暫存檔與 1,585 個快取連結，保留三個模型及六份原始資料。原 91／91 工程驗收保持，詳見[品質證據](evidence/TSK-11/quality-20261012/verification.json)。實作 `3cf5bd6` 已推送並同步原專案，詳見[交付回條](evidence/TSK-11/quality-20261012/delivery.json)。

### 已完成的工程驗收

原始 85 項與 NAT 六項工程需求已完成 91／91 驗收，最終一般、race、格式、build、vet、相依性與治理檢查通過，708 份程式／模組指紋與主專案相同。tickets 59–63 的本機與 SMB 任務保存、實際 Metal 更新／新程序接續、OCR 圖片／頁面／編碼器對照、真實影音模型還原都有[最終證據](evidence/goal-91-20261012/verification.json)。模型品質負面結果與 GPU 全圖／其他平台未測範圍保留。本輪 26,282 份暫存／副本共 3.45 GB 及 913 個快取連結已清除，25 份 SMB 資料原件保持，見[清理回條](evidence/goal-91-20261012/cleanup.json)。主要提交 `a28ce23` 已推送核對並同步原專案，見[交付回條](evidence/goal-91-20261012/delivery.json)。

### 較早階段交付紀錄

以下狀態與數字保留各階段交付當時的範圍，最新累計以本頁首段及需求追蹤檔為準。

[ticket 58](docs/tickets/58-fullgraph-recompute-efficiency.md) 已完成 4／4 驗收與主要證據交付。真實全圖八組交錯配對已量測，正式七組的完整更新耗時由中位數 18.30 降到 8.46 秒，配對減少 52.56%；十六次全部輸出、梯度、更新參數與 Adam 狀態相同。一般與 race 各 57 套件、11 個治理專項及全部原生檢查已通過。主要證據已推送並核對遠端 `20a0896`，本輪 14,742 份副本與暫存共 2.62 GB 已清除，原始全圖與參數指紋保持。[量測報告](evidence/LRN-02/fullgraph-activation-reuse-20261011/README.md)、[交付回條](evidence/LRN-02/fullgraph-activation-reuse-20261011/delivery.json)與[清理回條](evidence/LRN-02/fullgraph-activation-reuse-20261011/cleanup.json)。

[ticket 57](docs/tickets/57-recompute-activation-reuse.md) 已完成 5／5 驗收與交付。34 個重算專項、八組效能門檻、完整一般與 race 各 57 套件、格式、build、vet、相依性、11 個治理專項及 CLI 新程序恢復比對通過。固定人工密集圖耗時降低 58.88–91.41%，記憶體成本與限制見[資源說明](docs/resources.md#分段活化共用量測2026-10-11)。來源與證據已推送並核對遠端 `9414b9d`，669 份來源／模組的提交內容與驗收指紋相同。本輪驗收副本及 2.07 GB 暫存已清除，見[驗收證據](evidence/LRN-02/activation-reuse-20261011/verification.json)、[交付回條](evidence/LRN-02/activation-reuse-20261011/delivery.json)與[清理回條](evidence/LRN-02/activation-reuse-20261011/cleanup.json)。

[ticket 56](docs/tickets/56-recompute-sequence-capacity.md) 已完成 5／5 驗收與交付。兩種核心的有限序列重算已避開持續狀態容量限制，公開 Step／StepFrom、參數及最佳化器逐位相同。完整一般與 race 各 57 套件、28 個重算及 11 個治理專項、格式、build、vet、相依性與 CLI 恢復比對通過。程式與證據已推送並核對遠端 `0aab316`，665 份 Go 來源及模組的提交內容與驗收指紋一致。本輪驗收副本及 1.21 GB 暫存已清除，見[驗收證據](evidence/LRN-02/sequence-capacity-20261011/verification.json)與[交付回條](evidence/LRN-02/sequence-capacity-20261011/delivery.json)。

tickets 54、55 已完成本機原生磁碟完整驗收。已提交來源 `cf00886` 的 662 份 Go 來源與模組指紋一致，格式、build、完整一般與 race 各 57 套件、vet、相依性及 11 個治理／索引／隱私專項通過。14 份合法報告與修正前及兩個新程序逐位元組相同。正式證據存回專案，本輪本機副本與快取、先前 39 份中斷映像暫存均已清除，失敗紀錄保留。見 [完整驗收](evidence/MOD-09/protocol-ownership-20261010/local-check-20261011/requirement-verification.json)與 [清理紀錄](evidence/MOD-09/protocol-ownership-20261010/local-check-20261011/cleanup.json)。

[ticket 55](docs/tickets/55-network-model-bundles.md) 完成 5／5 驗收。三種 SDK 資料夾模型已在實際 SMB 分享上保存、讀回並由新程序接續。一般專項 23 個頂層測試、race 22 個通過，既有大容量案例只在 race 略過。兩個獨立程序保存只有一個成功。原實作 `79339be`、回歸 `842bd1d` 已推送核對，最後來源的完整原生磁碟套件現已通過。舊單檔格式在本 SMB 分享上的限制與 NAS 故障耐久性未驗證的範圍保留。

[ticket 54](docs/tickets/54-bioinspired-config-ownership.md) 完成 5／5 驗收。設定的雙向、報告間及取消後部分報告隔離通過，14 份合法報告在本次兩個新程序中與修正前相同。最後來源完整一般與 race 各 57 套件通過。基準 HEAD 為 `9c23868`，主要專案保留於 `/Volumes/SSD/Developer/coimnet`。

[ticket 52](docs/tickets/52-bioinspired-finite-validation.md) 與 [ticket 53](docs/tickets/53-project-local-data-and-runs.md) 各完成 5／5 驗收與交付。非有限值在執行前回錯，七組合法報告逐位元組相同。207 份資料及結果已搬回專案，清理後保留的 479 份原件逐檔核對通過，9,151 份暫存共 714 MB 已清除。完整一般與 race 各 57 套件、格式、build、vet、相依性與八個治理／索引專項通過。實作 `d3aa982` 已推送並核對遠端，見[數值交付](evidence/MOD-09/finite-config-20261010/delivery.json)與[保存交付](evidence/OPS-03/local-storage-20261010/delivery.json)。

[ticket 51](docs/tickets/51-continual-protocol-ownership.md) 已完成 5／5 驗收與交付。目標是讓連續任務的執行、雜湊與報告各自保留呼叫時的完整設定。八組回歸、30 個相關頂層測試、七份完整報告與雙程序一致、八個 CLI 流程、完整一般與 race 各 57 套件及格式、build、vet、相依性通過。656 份 Go 來源與 3,514 份原件保持，八個治理／索引專項通過，實作 `3546b88` 已推送並核對遠端，見[工程驗證](evidence/LRN-08/protocol-ownership-20261009/verification.json)與[交付紀錄](evidence/LRN-08/protocol-ownership-20261009/delivery.json)。

[ticket 50](docs/tickets/50-comparison-finite-validation.md) 已完成 5／5 驗收與交付，實作 933ba4c 已推送且遠端一致。三個新增回歸、37 個相關頂層測試、7 份原提交與雙程序合法報告、22 個既有錯誤及 13 個 CLI 流程通過。完整一般／race 各 57 套件、格式、build、vet、相依性、治理及 3,401 份原件核對通過，655 份模組 Go 來源保持凍結。

[ticket 49](docs/tickets/49-nav2d-report-config-ownership.md) 已完成 5／5 驗收與交付。三個先失敗回歸、20 個相關頂層測試、24 筆合法結果在兩個新程序中逐位元組相同、八個 CLI 流程通過。完整一般與 race 各 57 套件、build、vet、格式、相依性與八個治理／索引專項通過。654 份凍結 Go 來源及 3,321 份原件核對通過。Root 審查完成，實作 1b088e4 已推送且遠端一致，見[工程驗證](evidence/OPS-03/nav2d-policy-ownership-20261008/verification.json)與[交付紀錄](evidence/OPS-03/nav2d-policy-ownership-20261008/delivery.json)。

[ticket 48](docs/tickets/48-nav2d-finite-config-validation.md) 已完成 5／5 驗收與交付。四個新增回歸與 11 個選取的頂層測試通過，12 組非法環境設定在全部公開入口回錯。24 筆合法結果在兩個新程序中與修正前逐位元組相同，八個 CLI 流程通過。完整一般與 race 各 57 套件、build、vet、格式、相依性及八個治理／索引專項通過。3,246 份既有檔案與凍結來源保持，Root 審查通過，實作 6f2aa38 已推送且遠端一致。見[工程驗證](evidence/OPS-03/nav2d-finite-config-20261007/verification.json)與[交付紀錄](evidence/OPS-03/nav2d-finite-config-20261007/delivery.json)。

[ticket 47](docs/tickets/47-nav2d-effective-config-reports.md) 已完成 5／5 驗收與交付。六個新增回歸、八個真實 CLI 流程、24 筆修正前後報告記錄與兩個新程序重現通過，完整一般測試、build、vet、格式、相依性與治理也通過。完整 race 57 套件通過，Root 與 Luna max 獨立審查完成，實作 685a090 已推送且遠端一致，見[交付紀錄](evidence/OPS-03/nav2d-effective-config-20261007/delivery.json)，訓練結果及歷史原件保持。導航設定的非有限值驗證由 ticket 48 接續。

[ticket 46](docs/tickets/46-model-package-capacity-validation.md) 已完成 5／5 驗收與交付。共同入口核對六欄容量，沒有容量欄位的舊檔保持。五個回歸、九個 CLI 流程、完整一般與 race 各 57 個套件、build、vet、格式、相依性與治理均通過。實作 `d4ffe93` 已推送並核對遠端，見[工程驗證](evidence/STA-01/model-capacity-20261007/verification.json)、[Root 審查](evidence/STA-01/model-capacity-20261007/root-review.md)與[交付紀錄](evidence/STA-01/model-capacity-20261007/delivery.json)。本票沒有阻礙。

[ticket 45](docs/tickets/45-format-index-consistency.md) 已完成 5／5 驗收與交付。124 列、173 個本機連結、8 個格式／治理專項、無 Git 匯出，以及完整一般／race 各 57 個套件、建置、vet、格式與相依性檢查通過。3,022 份既有檔案與 647 份凍結 Go 來源保持。實作 `baf7b91` 已推送並核對遠端，見[工程驗證](evidence/GOV-01/format-index-20261007/verification.json)與[交付紀錄](evidence/GOV-01/format-index-20261007/delivery.json)。本票沒有阻礙。

[ticket 44](docs/tickets/44-doctor-gpu-capabilities.md) 已完成 5／5 驗收與交付。doctor 明列受限的 GPU 訓練支援條件，硬體探測與核心執行分開報告。13 個頂層專項、真實 CLI、Mac 完整一般與 race 各 57 個套件、建置、vet、格式及相依性檢查通過。2,953 份既有檔案與凍結測試保持。實作 `d7c8963` 已推送並核對遠端，見[工程驗證](evidence/OPS-02/doctor-gpu-capabilities-20261007/verification.json)與[交付紀錄](evidence/OPS-02/doctor-gpu-capabilities-20261007/delivery.json)。本票沒有阻礙，OPS-05 保持 specified。

[ticket 43](docs/tickets/43-stateful-continuous-ppo.md) 已完成 5／5 驗收與交付。CPU 純量連續核心可從保存電位與延遲歷史接續 PPO，算分、梯度與候選驗證使用同一狀態。22 個頂層專項、372 個相關頂層測試、Mac 完整一般與 race 各 57 個套件，以及 Root／獨立審查通過。2,087 份既有資料與證據、公開簽名、依賴及原範例設定保持。實作 `874ac43` 已推送並核對遠端，見[工程驗證](evidence/LRN-09/stateful-continuous-ppo-20261006/verification.json)及[交付紀錄](evidence/LRN-09/stateful-continuous-ppo-20261006/delivery.json)。本票沒有阻礙。

[ticket 42](docs/tickets/42-distribution-ctc-numerical-correctness.md) 已完成 5／5 驗收與交付。10 個新增數值回歸、相關流程的 124 個頂層測試、Mac 完整一般與 race 各 57 套件、建置、vet 及相依性檢查通過。Root 與獨立審查通過，公開 API、依賴及 2,006 份既有資料與證據保持。實作 `8fb25c2` 已推送並核對遠端，見[數值證據](evidence/TCH-04/distribution-ctc-numerics-20261006/verification.json)及[交付紀錄](evidence/TCH-04/distribution-ctc-numerics-20261006/delivery.json)。本票沒有本機執行阻礙。

[ticket 41](docs/tickets/41-ppo-objective-numerical-correctness.md) 已完成 5／5 驗收與交付：共用位移正規化、非法數值拒絕與有限加權價值計算完成，13 項新增與既有 20 項 RL 頂層測試通過。Root 接手修正第一版的中間溢位與多餘檢查。沒有本機驗證阻礙，Mac 完整一般與 race 各 57 套件、建置、vet、相依性、獨立審查與檔案審查通過，實作 `6d44edf` 已推送並核對遠端，見[交付紀錄](evidence/LRN-09/objective-numerics-20261006/delivery.json)。模型、依賴與非零狀態限制保留。

[訓練框架盤點](evidence/LRN-09/framework-audit-20261006.md) 保存修正前來源 `9202bbf` 的公開入口重現。ticket 41 處理已確認的 PPO 數值缺陷。盤點時非零初始狀態及可塑性／化學機制的 PPO 訓練尚未支援。CPU 純量連續核心的保存狀態現由 ticket 43 補上，其他限制保留。六步期限屬範例的任務定義。數值缺陷尚未被證明是方向學習失敗的原因。

[ticket 40](docs/tickets/40-six-step-deadline-training-comparison.md) 已完成 5／5 工程驗收與交付，實作 `58cb01e` 已推送並核對遠端，見[交付紀錄](evidence/LRN-09/horizon-comparison-20261005/delivery.json)。兩組共 1,200 訓練回合與 3,120 評估回合在兩個新程序重現，原組與舊證據逐筆一致。六步任務期限組的 seed 3 通過完整提示判準，seed 1／2 未通過；原組三個 seed 均未通過。Mac 完整一般與 race 各 57 套件、建置、vet、相依性及獨立核對通過。審查發現的第一回合核對缺口已修正，22 種竄改案例通過，原公開契約及歷史證據保留。[分析](evidence/LRN-09/horizon-comparison-20261005/analysis.md)、[工程驗證](evidence/LRN-09/horizon-comparison-20261005/verification.json)。

[ticket 39](docs/tickets/39-ppo-gradient-diagnostic.md) 已完成 5／5 工程驗收與交付，實作 `0dc4d64` 已推送並核對遠端，見[交付紀錄](evidence/LRN-09/gradient-diagnostic-20261004/delivery.json)。兩個新程序的 600 次完整梯度報告相同，原更新與合成 upstream 的 StepFrom 相同。價值項的共用核心梯度全部較大，倍率中位數為 7.05、4.72、7.45，方向相反為 77、120、123 次。沒有改訓練參數或模型，也沒有確認偏向的唯一根因。Mac 完整一般與 race 各 57 套件、建置、vet、相依性、格式及交接原件通過，獨立重算、17 種竄改檢查及最後審查通過。需求維持 89／91。[診斷](evidence/LRN-09/gradient-diagnostic-20261004/analysis.md)、[工程驗證](evidence/LRN-09/gradient-diagnostic-20261004/verification.json)。

[ticket 38](docs/tickets/38-ppo-training-feedback-diagnostic.md) 已完成 5／5 工程驗收與交付，實作 `e0d1c43` 已推送並核對遠端，見[交付紀錄](evidence/LRN-09/training-feedback-20261004/delivery.json)。固定六步的 600 訓練與 198 探測回合已在兩個新程序重現，與 ticket 37 的完整既有報告一致。左右目標回合接近各半，左右成功卻分別是 3／6、70／3、1／71。提示能改變輸出，最後 greedy 行動仍不隨提示選方向。Mac 一般與 race 各 57 套件、建置、vet、相依性、格式及交接校驗通過。需求維持 89／91。[診斷](evidence/LRN-09/training-feedback-20261004/analysis.md)。

[ticket 37](docs/tickets/37-short-goal-cue-audit.md) 已完成 5／5 工程驗收與交付，實作 `2733ffe` 已推送並核對遠端，見[交付紀錄](evidence/LRN-09/short-goal-cue-audit-20261003/delivery.json)。固定六步、左右共用亂數，39 組／1,560 回合在兩個新程序重現並獨立核對。訓練後 sampled 為 1／40、14／40、13／40，三組提示判準均 false，沒有依提示導航的學習證據。需求維持 89／91。

[ticket 36](docs/tickets/36-synthetic-goal-cue-audit.md) 已完成 5／5 工程驗收並交付：固定模型與 200 更新，39 組共 1,560 回合的報告在新程序重現。訓練後取樣抵達率為 50%、97.5%、67.5%，整體提示工作判準未通過。Mac 完整一般與 race 各 57 套件、建置、vet、相依性與格式通過，實作 `fd069c3` 已推送並核對遠端，見[交付紀錄](evidence/LRN-09/goal-cue-audit-20261003/delivery.json)。需求維持 89／91。

2026-10-03 [ticket 35](docs/tickets/35-frozen-stimulus-history-counterfactual.md) 已完成 5／5 工程驗收並交付：234 配對的兩個新程序報告相同，未干預側與 ticket 34 的行走摘要及完整 trace 指紋相同。每個 seed 的八個有刺激試次在清除歷史後改變輸出及行走，但所有原／清除模型組皆 0／13，預測誤差效應方向不一致。來源與凍結模型不變，實作 `21eb2ae` 與證據修正 `bf5d7fa` 已推送並核對遠端，需求為 89／91。[交付紀錄](evidence/TSK-11/stimulus-counterfactual-20261003/delivery.json)。[結果](evidence/TSK-11/stimulus-counterfactual-20261003/summary.json)。

2026-10-03 [ticket 34](docs/tickets/34-real-trajectory-stimulus-memory-proposal.md) 的完整刺激歷史工程範例已完成 5／5 本機驗收。實作 `55c2ef7` 已推送並核對遠端，交付見 [delivery.json](evidence/TSK-11/stimulus-memory-20261003/delivery.json)。固定 20 train／6 validation／原 13 test，九組模型各 200 更新。測試 MSE 下降，但所有模型控制、種子與訓練前後都命中 0／13，沒有導航學習證據。方向延續均為 0／13，隨機依序為 0／13、0／13、1／13。最終版推論與自主評估各兩次逐位元組相同，完整一般及 race 各 57 個套件通過，相關套件另做全新 race。TSK-11 與 OPS-05 維持 specified，需求為 89／91。[總證據](evidence/TSK-11/stimulus-memory-20261003/verification.json)。

2026-10-03 [ticket 33](docs/tickets/33-real-trajectory-inference-isolation.md) 的推論分割修正已驗證並交付。修正提交 `467d53a` 已推送，遠端 main 核對一致。非法分割不建立輸出，合法真實快照的報告除 runtime 外與修正前相同。Mac 完整一般與 race 測試各 56 個套件、建置、vet、格式及相依性檢查通過，來源與原模型前後指紋一致。[本輪證據](evidence/TSK-11/inference-isolation-20261003/verification.json)。TSK-11 與 OPS-05 維持 specified，需求仍為 89／91。

2026-10-02 [ticket 32](docs/tickets/32-real-trajectory-autonomous-rollout.md) 的自主行走工程評估已驗證並交付。實作提交 `bec3571` 已推送，遠端 main 核對一致。模型與三個控制組在全部 13 個保留試次皆命中 0 次，排除 0、起始命中 0；兩次訓練模型逐位元組相同，自主評估除執行量測外相同。Mac 完整一般與 race 測試各 56 個套件、建置、vet、相依性與格式檢查通過。目標中心只供計分，參數及最佳化器不變；結果不支持學會導航。需求維持 89／91，TSK-11 與 OPS-05 為 specified。[本輪證據](evidence/TSK-11/autonomous-rollout-20261002/verification.json)。

2026-10-02 完成 [ticket 31](docs/tickets/31-direct-upstream-gradient.md)：讀出與編碼器改用 Insyra `BackwardFrom` 傳遞外部梯度。一般與 race 編譯模式各 9 組完整梯度，均與各自的原內積橋接參考逐位相同。修改前訓練 80 步後由修改版接續 40 步，快照與原版連跑 120 步逐位元組相同。Mac 完整建置、一般與 race 測試各 56 個套件、vet、依賴檢查及 Apple M3 Metal 專項驗證通過，Linux／Windows amd64 僅完成交叉編譯。核心精度、公開 API、最佳化器與保存格式未改，需求維持 89／91。[梯度橋接證據](evidence/COR-08/backwardfrom-20261001/verification.json)。

2026-10-01 Insyra 已升級至 v0.3.4，對應提交 `90f3935d02b52ce0fd51a86ed95f340cff9030a8`。新版 WebGPU 型別改用 `gputypes`，核心、梯度橋接與保存格式沿用現有契約。Mac 完整建置、一般與 race 測試各 56 個套件、vet、模組校驗與 Apple M3 Metal 運算／訓練／新程序恢復通過。小型延遲範例由 v0.3.2 訓練 80 步後交給 v0.3.4 接續 40 步，快照與 v0.3.2 連跑 120 步逐位元組相同。Linux／Windows amd64 僅完成交叉編譯。`Tape.Custom`／`BackwardFrom` 與 CPU `EdgeSum` 已有公開 API，本輪尚未採用。GPU 常駐狀態與裝置更新尚待上游契約，需求累計維持 89／91。[升級相容性證據](evidence/insyra-v0.3.4-20261001/verification.json)。

2026-09-25 真實資料範例已涵蓋五類任務的匯入、訓練、獨立推論與評估：Mini LibriSpeech 語音、Gutenberg 文字、KMNIST 單字元 OCR、Blender 開源影音，以及 Titova 等人的果蠅軌跡。軌跡範例是觀測式下一步位移預測，非閉環導航；保留集 MSE 與角度誤差都比方向延續基準差。Mac Metal 的 WebGPU 稀疏前向、反向與小圖訓練器含新程序恢復已通過對照，但神經狀態與 AdamW 尚未留在 GPU，也沒有 Ubuntu 1 執行或全圖加速證據。Mac 上最終原始碼的完整一般測試、建置、vet、依賴校驗與 Linux／Windows amd64 交叉編譯通過；完整 race 全套通過，之後的軌跡輸出路徑修正另以專項 race 通過。因此 TSK-11、OPS-05 都維持 `specified`，需求仍為 89／91。[真實資料總證據](evidence/TSK-11/verification.json)、[GPU 總證據](evidence/OPS-05/verification.json)。

2026-09-24 接手維護：`gridnav.New` 拒絕非有限獎懲設定；`ocr.Stack` 對不合法行尺寸、像素數與頁面大小溢位回錯，不再因這些輸入 panic。README 與資源文件已依 OPS-07／08 的真實全圖實測更新，明確區分一次更新和訓練達標。在 Mac 上，`gofmt`、`go build ./...`、`go test -timeout 30m ./...`、`go test -count=1 -race -timeout 30m ./...`、`go vet ./...`、`go mod verify` 均通過；需求仍為 89／91，TSK-11 與 OPS-05 的受阻條件未變。新發現的 OCR 非有限輸入缺口記於 [AGENTS.md](AGENTS.md#follow-ups)。

2026-09-24 OPS-10 通過，需求追蹤檔現為 89／91 通過、2 項受阻（原始 83／85、NAT 增補 6／6）：乾淨環境腳本以 git archive 匯出已提交版本、連結官方資料，37 步中 35 步通過、沒有失敗，涵蓋建置、完整測試、doctor、所有 fixture 範例、真實子圖匯入與訓練、全圖檢查、短訓練、恢復與效能報告，最後產生完成報告；TSK-11 與 GPU 後端明示受阻。第一次實跑抓到 ablate 輸出目錄與完成報告受阻統計兩個缺陷，修正後重跑，[證據](evidence/OPS-10/verification.json)。除了等使用者的 TSK-11（授權資料）與 OPS-05（GPU 後端決定），規格裡的其他需求都已驗證。

2026-09-24 需求追蹤檔依 ticket 29 與 30 的決策，把 TSK-11 標為 `blocked_data`、OPS-05 標為 `blocked_permission`，不再留在 specified。TSK-11 缺每類任務的授權真實資料：語音與文字已有授權 manifest 讀取器，OCR 與影音的匯入器要等資料格式確定後再做，[紀錄](evidence/TSK-11/verification.json)。OPS-05 缺 GPU 後端技術的決定與可遠端執行的 GPU 機器：能力偵測、執行前拒絕不支援的組合、明示的 CPU 回退與不重跑半次更新的帳本都已完成，[紀錄](evidence/OPS-05/verification.json)。

2026-09-24 OPS-08 通過，需求追蹤檔現為 88／91（原始 82／85、NAT 增補 6／6）：`coimnet benchmark` 的六個階段都分開記初始化、傳輸、暖機與穩態，能耗固定記為未量測；預設 fixture 分開跑兩次，前向活動量逐位元組相同，真實全圖版在 store 模式下跑完（180 秒、峰值足跡 8.06 GiB）。第一次全圖嘗試抓到個體單次呼叫超過 2^20 個值的缺陷，改成分段呼叫後重跑，[證據](evidence/OPS-08/verification.json)。

2026-09-24 OPS-07 通過，需求追蹤檔現為 87／91（原始 81／85、NAT 增補 6／6）：真實全圖 165,122 個神經元與 25,563,197 條邊全部參與一次反向更新，五組可訓練參數都有變動，可塑性與化學調節有作用，三個 bundle 在新程序恢復後續跑摘要相等。第一次執行抓到 encoder收不到梯度的缺陷（輸入只在梯度窗外的第 0 列），修正後重跑。峰值記憶體：預設 GC 足跡 12.22 GiB，設 `GOMEMLIMIT=8GiB` 時 8.01 GiB 且結果相同，預估 7,220 MiB 反映的是存活陣列，[證據](evidence/OPS-07/verification.json)。這只證明整張圖能跑完一次更新並保存恢復，不是訓練結果。

2026-09-24 TSK-06、TSK-07 通過 fixture 驗收，ticket 29 的六個階段都已驗證，需求追蹤檔現為 86／91（原始 80／85、NAT 增補 6／6）。影片：核心逐格產生影格與音訊，原生 PNG、WAV 與時間線完整，ffmpeg 封裝成功與工具缺席都照實記錄；但預設 80 輪時同步偏弱，加長訓練後完整核心與凍結核心的優劣會翻轉，所以不宣稱核心學習的必要性，[證據](evidence/TSK-06/verification.json)。三條生成流程：每張輸出都標明是核心、固定還原器還是外部工具產生，工具像素另列、不算核心能力，替換假工具的測試證明這點，[證據](evidence/TSK-07/verification.json)。剩下的是 OPS-07、OPS-08、OPS-10 的全圖與乾淨環境證據，以及受阻的 TSK-11（授權資料）與 OPS-05（GPU 後端）。

2026-09-24 TSK-02、TSK-03 通過 fixture 驗收，需求追蹤檔現為 84／91（原始 78／85、NAT 增補 6／6）。TSK-02 先前因缺真實語音而維持 specified，但真實資料屬 TSK-11，和 TSK-01 同樣以 fixture 驗收；複核重跑 ASR 全套與兩次範例，報告除耗時外完全相同，同分布保留集 CER 0.78 → 0，跨說話者的範例只降一個字、WER 維持 0.9，[複核紀錄](evidence/TSK-02/review-20260924/verification.json)。TSK-03 修掉兩個問題後重跑：生成有效率改成要求完整合法的 UTF-8，預設輪數改成三個 seed 都學會的 20 輪；三個 seed 的保留語料困惑度從約 260 降到 1.37 到 1.53，任務提示全部補對，教師呼叫 0 次，[證據](evidence/TSK-03/verification.json)。兩項都只證明流程，不宣稱自然語音或中文語言能力。

2026-09-24 TSK-04、TSK-05 通過 fixture 驗收，需求追蹤檔現為 82／91（原始 76／85、NAT 增補 6／6）：文字經編碼器與核心產生表示，固定 clamp 解碼器的輸入型別只有表示、讀不到提示；影像輸出 8×8 PNG、音訊輸出 8 kHz WAV，長度與取樣率精確；`examples run media` 會寫出報告和已訓練核心生成的九個樣本。三個 seed 裡，影像的已見條件全對、音訊對 6 到 7 個，但只訓練編碼器與讀出層的凍結核心對照一樣好，留出條件也全部生成錯，所以只能說流程可用，不能說核心學習是必要的，也沒有組合泛化。[影像證據](evidence/TSK-04/verification.json)、[音訊證據](evidence/TSK-05/verification.json)；真實授權影音屬 TSK-11，仍受阻於資料。

2026-09-24 TSK-09 通過 fixture 驗收，需求追蹤檔現為 80／91（原始 74／85、NAT 增補 6／6）：走廊與延遲關聯兩個任務共用一顆核心，三種排程都只建一個 trainer，每份結果的模型包指紋與核心參數指紋相同，每個任務在自己的個體上評估並記錄血統雜湊鏈，更新份額與宣告比例並列、超過容差會標旗，[證據](evidence/TSK-09/verification.json)。`examples run nav2d`、`multimodal`、`attribution` 三個 CLI 入口已補上；`benchmark` 報告升到 v2（每階段分開初始化、傳輸、暖機、穩態，附活動量、同 seed 重現與「未量測能耗」聲明），OPS-08 的固定與全圖兩份報告尚未產生。OPS-07 的全圖快照改走目錄型格式（manifest 加 gob payload、逐檔 SHA-256、manifest 最後寫入當完成標記），單檔 JSON 格式不變，接上全圖流程後才會實跑。

2026-09-23 晚間補齊五項需求的 fixture 驗收，需求追蹤檔現為 79／91 通過（原始 73／85、NAT 增補 6／6）。COR-06：向量節點、共享參數與容量報告，對展開後的稠密參考，前向與反向誤差都小於 1e-16。LRN-02：`Options.Recompute` 讓訓練步不保存核心歷史、反向逐段重算，結果與完整歷史逐位相同，線上學習與化學層的呼叫次數與快照都不變；`T = 256`、`S = 16` 時最大 RSS 連續核心 1,169 MB → 771 MB、LIF 826 MB → 628 MB，網路層仍保留整段的輸入與輸出，所以不是數量級的節省。TSK-08：二維導航四個任務 × 四種策略 × 3 seed 全部完成，訓練後未見地圖的專家一致率 0.305–0.401、隨機策略 0.243–0.254，成功率仍低，不宣稱策略優劣。TSK-10：缺失與零值分離、ID 不進模型、同步與非同步配對、未見組合分割都有測試；已見配對學會了，留出組合的影像↔文字檢索三個 seed 都是 0，不宣稱形成跨模態概念。TSK-12：七組對照各 3 seed 都能執行並回報，但同時學核心與 encoder 的 normal 基準自己沒學好（未見地圖成功率 0），各組相對它的正差不代表依賴核心或接線較優。五份報告在不同負載下重跑都位元組相同。證據見 [COR-06](evidence/COR-06/verification.json)、[LRN-02](evidence/LRN-02/verification.json)、[TSK-08](evidence/TSK-08/verification.json)、[TSK-10](evidence/TSK-10/verification.json)、[TSK-12](evidence/TSK-12/verification.json)。`examples run nav2d`／`multimodal`／`attribution` 三個 CLI 入口、TSK-09 的共用核心排程與報告、OPS-07 的全圖短訓練（現行 checkpoint 單檔 64 MiB 上限裝不下全圖快照）仍在進行。

2026-09-23 補上 ticket 29 串流檔案保存：`Stream.Save`／`Recognizer.LoadStream` 把神經個體、待處理音訊、CTC 與前處理身分存成單一 64 MiB 上限檔，沿用 checkpoint 的排他發布和嚴格驗證。新程序接續人工音調後，最終狀態摘要與不中斷路徑相同；錯誤 schema、checksum、缺失或 `null` 欄位與超量陣列會拒絕。Mac 完整 Go build/test/race/vet/依賴檢查與 Linux／Windows 交叉編譯通過，[證據](evidence/TSK-02/stream-file-verification.json)只涵蓋人工資料；真實授權語音、跨平台執行及超過 64 MiB 的個體檔仍待驗證。`TSK-02` 暫維持 `specified`。

2026-09-23 完成 ticket 29 第二階段的資料與範例增量：授權 manifest 的 PCM16 WAV 匯入、帶來源指紋與配置前容量檢查的前處理、說話者／場次連通群分割、整檔與串流 CER／WER 及離線耗時報告、`examples run asr`。人工 WAV 端到端測試與完整 Go build/test/race/vet/依賴檢查在 Mac 通過；ASR 專用 race 日誌 72 PASS、0 FAIL。CLI 保留集整檔 CER 13/19 → 12/19，WER 9/10 不變；兩次量測最高 RSS 約 27.8／27.5 MiB。[TSK-02 證據](evidence/TSK-02/verification.json)只支持小型人工音調，`TSK-02` 維持 `specified`；目前總數以需求追蹤檔為準。Ubuntu 1 該次 SSH 認證被拒，真實授權語音尚未驗收；當時未包含檔案保存驗證，後續證據見上段。

2026-09-22 完成 ticket 26 的人工環境學習驗收：取樣式 PPO、正確的時間上限後續價值與 `examples run gridnav --method imitation|ppo` 已整合。三個 seed 各 200 次更新，平均回報 0.318167 → 0.605167，高於隨機基線 0.357333；三組都比訓練前進步，但 seed 1 仍低於自己的隨機基線。模仿的 50／200／500 episodes 原始曲線已保存，500 episodes 時三組一致率皆提高。LRN-09 標為 fixture 範圍 `passed`，原始需求累計 64／85，NAT 增補 6／6。

同一份 439 檔來源在 Mac／Ubuntu 通過完整 build、一般測試、race、vet、依賴與 CLI 續訓比較，兩台各有 57 個相關頂層契約測試通過。每台兩次 PPO CLI 報告逐位相同；跨平台快照摘要不同。審查發現的極端有限獎勵統計溢位已修正。詳見 [LRN-09 證據](evidence/LRN-09/verification.json)、[範例用法](experiment/gridnav/README.md) 與 [資源量測](docs/resources.md)。此輪沒有增加真實全圖、GPU 或自然語音能力證據，TSK-02 仍未完成驗收。

## 既有階段證據

以下保留各階段當時的驗證範圍與計數，現況以上方摘要及 `docs/requirements-status.json` 為準。

LIF 個體、慢速穩定與模型包（ticket 16、COR-04、STA-01）已在 fixture 驗證：`learning.NewIndividual` 對連續與 LIF 核心走同一條路徑，神經狀態改為指名核心的聯集，舊的連續個體檔仍可讀；LIF 可選的慢速穩定用每顆神經元的活動估計調整閾值偏移，手算時序與收斂數字記在 ticket；`coimnet-model-package/v1` 是第三種保存物，只有拓撲指紋、設定、參數、單位與證據清單，六種交叉載入都會被拒絕並指名讀到的是哪一種，LIF 個體跨程序恢復後續跑與不中斷逐項相等。COR-04、STA-01 標 passed，證據見 [COR-04](evidence/COR-04/verification.json)、[STA-01](evidence/STA-01/verification.json)。

受限更新與完整最佳化器流程（ticket 17、COR-10、LRN-03）已在 fixture 驗證：逐邊／逐節點遮罩讓凍結項在含 weight decay 的 50 步後逐位不變、動量與步數為零；固定符號邊改用對數幅度參數化，`learning_rate=1.0` 跑 1,000 步符號不翻轉、幅度 > 0，鏈鎖梯度與中央差分最差相對誤差 4.91e-05；範圍投影計數正確且不動動量；推導參數集的符號依 free／excitatory／inhibitory 政策帶入並回報計數。全零符號與舊快照逐位等於改前。第二階段：損失縮放前後參數逐位相同、溢位整步拒絕；累積三步等於平均梯度的一次 AdamW（手算）、視窗中途快照接續逐位相同；三種排程的學習率表手算、恢復後接續；裁切在平均之後、AdamW 之前。COR-10、LRN-03 標 passed，證據見 [COR-10](evidence/COR-10/verification.json)、[LRN-03](evidence/LRN-03/verification.json)。

局部可塑性（ticket 18 第一階段、LRN-04、LRN-05）與調節來源（ticket 20、SIG-03、MOD-01、MOD-08）已在 fixture 驗證：`plasticity` 套件的活動相關與脈衝時序規則各有手算時序（閘門關只衰退、延遲閘門用殘留參與紀錄、上限與 `w_min` 計數、固定符號不跨零），持續個體的 `AdvanceGated` 逐步套用新的有效權重，關閉時逐位還原，快照帶可選的 `plastic` 區塊；`modulation` 套件的四種來源（外部時序、具名集合活動、內在資源、重播）只產生非負有限的釋放率，獎懲映射把原始值、期望值、轉換值與套用值分開，負回饋變成清除項而不是負濃度；`scripts/check-target-flow.sh` 與 NaN 汙染測試證明目標只進損失。證據見 [LRN-04](evidence/LRN-04/verification.json)、[LRN-05](evidence/LRN-05/verification.json)、[SIG-03](evidence/SIG-03/verification.json)、[MOD-01](evidence/MOD-01/verification.json)、[MOD-08](evidence/MOD-08/verification.json)。

空模型對照與判讀協定（ticket 14、NAT-03／04／05）已在真實全圖驗證：`simulate compare` 用同一份刺激跑原圖與三種空模型（保留度數的重連、打亂正負號、打亂權重）各 3 個 seed，LIF 與連續核心各 10 格（每格 165,122 節點、25,563,197 邊、300 步，兩個矩陣各約 15 分鐘）。指標與門檻事前寫進 protocol；五條 LIF 門檻在原圖與九個空模型格全部通過，所以它們分不出真實與打亂的接線；同一份接線換核心後，原圖在空模型分布中的百分位方向可以相反，歸因結論必須連同核心、參數來源、空模型種類與指標定義一起陳述。報告只陳述指標與位置，不做行為宣稱。證據見 [NAT-03](evidence/NAT-03/verification.json)、[NAT-04](evidence/NAT-04/verification.json)、[NAT-05](evidence/NAT-05/verification.json)。

發布資料到動態參數的 adapter（ticket 13、NAT-02）已在真實全圖驗證：`data derive` 依明示規則檔把逐突觸傳導物質機率經座標配對到全部 25,563,197 條邊（配對比例 1.000、207 秒、RSS 3.87 GB），正負號 +53.3%／−35.9%／unknown 10.8%，unknown 只來自機率門檻與規則本身標 unknown，不補預設值；`simulate run --params` 以推導參數集跑 NAT-01 的同一協定，沉默比例由 2.1% 升到 64.4%、最大全群放電比例由 0.547 降到 0.079、不再觸發旗標。這是兩組參數假設在同一張接線圖上的差異，不是生理結論；歸因仍需 ticket 14 的空模型。證據見 [NAT-02](evidence/NAT-02/verification.json)。

原生模擬 runner（ticket 12、NAT-01）已在真實全圖驗證：`simulate run` 不經 encoder、readout 或訓練器，把 165,122 節點、25,563,197 條邊接上 LIF 核心跑 300 步，93.08 秒、RSS 4.38 GB、無非有限值；參數為明示的工程假設 `engineering_uniform_positive`（全興奮、統一 bias／log_tau／theta_raw），報告與文件皆寫明不是生物參數。刺激結束後活動自我維持並觸發 `max_population_rate_exceeded` 旗標，歸因需 ticket 14 的空模型。證據見 [NAT-01](evidence/NAT-01/verification.json)。

COR-01 狀態分離、STA-02 安全保存、STA-04 個體隔離通過 fixture 驗收。Mac／Ubuntu v25 同源 137 檔的完整 build、unit、race、vet、模組與 CLI 檢查，以及 27 個相關頂層測試／範例全部通過；快照雙向跨機器讀回並接續成功。Windows 僅交叉編譯通過。完整需求為 23／85，另有 6 項 NAT 增補需求（原生模擬），其中 NAT-01 到 NAT-05 已通過，NAT-06 尚未，證據見 [v25](evidence/cpu-reference-20260914/verification-v25.json) 與 [ticket 05](docs/tickets/05-resume.md)。

SIG-04 的輸入／輸出映射保存、重建、圖綁定與獨立替換通過 fixture 驗收。127 檔同源的 Mac／Ubuntu v22 完整檢查、選定輸入節點的跨程序續訓與 Windows 交叉編譯通過，詳見 [映射驗證](evidence/SIG-04/verification.json)。該階段累計為 18／85。SIG-01／02／05／06 維持通過。

真實子圖選取及短訓練範例已完成驗證（ticket 10、DAT-07）。原件與既有完整圖保持不變，證據見 [real-subgraph](evidence/real-subgraph-20260914/verification.json)。

標準化接線圖已在真實 MaleCNS v1.0 三份原件上完成建構、落盤與讀回驗證（ticket 08、09）：`connectome.Build` 與 `data import` 依明示 manifest 產生 `raw_segments`／`annotated_neurons` 視圖與報告，165,122 個選入節點、25,563,197 條邊，所有與獨立稽核共有的計數相等。Mac 與 Ubuntu v7 同一份 95 檔來源的建置、單元、race、vet、模組驗證與跨程序續訓全通過。CPU 參考流程、官方資料下載與 Feather 讀取維持 v4／v5 驗證。完整目標維持原始 W01–W14。

化學濃度、受體與效果（ticket 21 第一階段、MOD-02、MOD-03）與運行中學習與重播（ticket 23 第一階段、LRN-06、LRN-07）已在 fixture 驗證：濃度依 `lambda = exp(-dt/tau)` 衰退並收斂到 `tau*q`（幾何級數手算），區域傳輸不增加總量，負釋放率與非法傳輸都被拒絕；受體佔用率在對數域計算，`c = 1e300, n = 8` 仍為有限值，`unresponsive`／`unknown`／`hypothesized` 三種狀態分開報告且未知係數只在明示允許時才用並標為假設；效果映射在濃度為零時逐位等於未啟用，兩個核心的 `AdvanceModulated` 在 nil 與中性調節下逐位等於 `Advance`。第二階段把化學層接上持續個體：每一步固定走來源釋放 → 濃度 → 佔用率 → 效果陣列 → 調節後的核心一步 → 可塑性，連續與 LIF 各有四步手算表（LIF 的閾值效果抑制了原本會發生的放電），100 步後基礎參數逐位不變，中途快照帶濃度、資源與待處理回饋接續後逐位相同；MOD-04 標 passed，證據見 [MOD-04](evidence/MOD-04/verification.json)。運行中學習把作答、收回饋、更新三步分開，已輸出的答案 hash 在回饋與更新後不變，評估模式拒絕任何更新；重播存放區的先進先出與水塘淘汰、三種抽樣都有固定 seed 的手算序列，中途快照接續的抽樣序列與連續執行相同，測試分割永遠進不了重播。證據見 [MOD-02](evidence/MOD-02/verification.json)、[MOD-03](evidence/MOD-03/verification.json)、[LRN-06](evidence/LRN-06/verification.json)、[LRN-07](evidence/LRN-07/verification.json)。

runner 上的可塑性對照（ticket 19、NAT-06）已在真實全圖驗證：protocol 可宣告 `plasticity` 區塊（規則、邊選擇、閘門通道與比例），runner 在開啟時強制每步一次核心呼叫、每步先由快速狀態算有效權重再更新參與紀錄，`simulate compare` 新增 `original`／`plastic`／`learned_then_frozen` 三格；fixture 三神經元手算逐位相同，NAT-01 的 `protocol_hash` 由測試釘住不變；全腦（165,122 節點、25,563,197 邊、300 步、全部邊啟用、`hebbian_rate`）各跑兩次三格（`decay_p` 0.5 與 0.999），每格約 600 秒、最大 RSS 約 6.6 GB，`original` 格與 NAT-02 的全腦執行逐位相同；`decay_p` 0.999 時 ALIN 集合的 `mean_rate_after` 由 0.246 升到 0.305（凍結後 0.319），下行神經元的 `activity_ratio_after` 由 56.5 降到 54.4（凍結後 6.3），這些是同一刺激下的差值與百分位，報告不做文字判斷；`decay_p` 0.5 時關閉閘門 250 步後快速變化衰退到 8e-75，`learned_then_frozen` 與 `original` 逐位相同，是規則時間常數造成的。證據見 [NAT-06](evidence/NAT-06/verification.json)。

組態與資源預估（ticket 24 第一階段、OPS-03、OPS-06）已在 fixture 驗證：`config` 套件用共用的嚴格 JSON 解碼 `coimnet-config/v1`，從宣告的預設值出發，依檔案、環境變數、`--set` 的固定優先序覆寫並記錄每個葉節點的來源，所有名稱都要對到有版本的實作（未實作就拒絕，不替代），金鑰只以 `{"ref": "env:NAME"}` 保存、展開輸出永不出現值；`resources` 套件逐項估算記憶體並印出代入數字的公式，規格 17.1 的 `16E` 與 15.36 GB 兩個算術示例由測試釘住，超限時 `Check` 拒絕且不改計畫；`coimnet run --config … --dry-run` 展開組態、檢查模型檔與 generator、對 `max_memory_mib` 預估後印出 `coimnet-dry-run/v1` 報告，零副作用、教師呼叫次數 0，退出碼成功 0、拒絕 2、用法錯誤 1。全圖（165,122 節點、25,563,197 邊、f64、AdamW、不留反向歷史）預估 1,026,490,952 bytes，與 NAT-01 實測最大 RSS 4.38 GB 並列但不宣稱吻合。證據見 [OPS-03](evidence/OPS-03/verification.json)、[OPS-06](evidence/OPS-06/verification.json)。

按類型混合（ticket 25 第一階段、COR-05）已在 fixture 驗證：`dynamics.Mixed` 讓每個節點只跟一種規則，連續節點輸出 `phi(v)`、LIF 節點輸出突觸跡，兩種目標都用同一條 `I = external + Σ w·out(t − delay)`，延遲 0 讀本步開始時的輸出，LIF 事件只經突觸跡進下游一次；3 節點手算圖、不雙重計入、4 節點重新編號順序無關都逐位相符；全連續指派逐位等於 `Continuous`、全 LIF 指派逐位等於 `LIF`（`-race` 建置下既有 LIF 核心自己的膜電位差一個 ULP，混合核心把膜電位運算放進不內聯函式固定融合，差異由 build tag 常數釘住並在測試裡檢核）；梯度以平滑測試模式的中央差分與硬事件的手算兩步梯度驗證。`learning` 多了第三個核心、`LIFIndex` 對照與混合個體 profile，`checkpoint` 接受第三種神經狀態；模型包暫時拒絕混合核心（指紋與必填走訪尚未支援）。證據見 [COR-05](evidence/COR-05/verification.json)。

2026-09-17 起派工方式改為：實作票切成 200 行以內的小票派給 opencode 免費模型（`opencode/big-pickle`），主 agent 逐張讀 diff、自己重跑驗證再提交；Opus 只留給 review 備援。教師與蒸餾（ticket 27 第一階段、TCH-01、TCH-02）已在 fixture 驗證：`teacher` 套件的統一契約把回應當資料（含指令字樣的答案原樣保存、不執行），JSONL 紀錄檔拒絕任何屬於保留測試輸入的 hash，離線與重播教師把 `http.DefaultTransport` 換成一律失敗的傳輸仍能回答，HTTP 教師的逾時、5xx 精確重試、4xx 不重試、限速、預算（預設零）、同請求 ID 去重、只送允許欄位、金鑰只放 header 全部對本機 `httptest` 伺服器驗證，真實遠端服務明寫未驗證；封鎖教師對每次呼叫回錯並計數。證據見 [TCH-01](evidence/TCH-01/verification.json)、[TCH-02](evidence/TCH-02/verification.json)。

記憶表現的穩定化寫入（ticket 22 第一階段、MOD-06 後半）已在 fixture 驗證：`plasticity.Model.EffectiveWith` 在上方掛一層可選 `slow`，有效權重為 `(w_base + slow) + plastic`（固定符號邊為 `s * max(|w_base| + slow + plastic, w_min)`），slow 為 nil 時逐位等於既有 `Effective`；`learning.Individual.Consolidate` 依受體平均佔用率開閘，同 episode 去重（`ErrAlreadyConsolidated`）、預算用盡（`ErrBudgetExhausted`）回報而不動狀態，寫入量 `slow += Rate * plastic` 後 `plastic *= Retain` 與三層 `base_l2`／`slow_l2`／`plastic_l2` 都是手算值；把整個 slow 層折進基礎權重的孿生個體輸出、塑膠報告與電位逐位相同，證明 slow 進入整合而不只是裝飾；快照加 `plastic.slow`（`SlowState`）與 `optimizer.episodes`（每次 `TrainEpisode` 成功遞增、`ResetOptimizer` 歸零），checkpoint 往返逐一相等，中途快照在跨 JSON 接續後再推進一 episode 的寫入與去重判斷和連續執行相同。MOD-06 仍待 `evidence/MOD-06/` 證據目錄、MOD-05 閘門與時間窗等第一階段其餘卡片完成後才標 passed。

受體驅動的學習閘門與時間窗、記憶表現與穩定化（ticket 22 第一階段、MOD-05、MOD-06）已在 fixture 驗證：可塑性規則可指名受體，每步由該受體的平均佔用率算閘門（`GateScale × occ`）與參與紀錄的衰退（`clamp(base + span × occ, min, max)`），閘門關時快速變化只衰退不新增，明確閘門序列與受體閘門不得並用；讀出增益只乘在讀出路徑，核心、可塑、化學狀態與參數都逐位不變，清除後輸出與孿生個體逐位相同，文件明寫這是表現受抑制而不是遺忘；穩定化把快速變化按比例寫進較慢的 `slow` 層，只在受體佔用率達門檻、本 episode 未寫入且預算未用完時執行，同 episode 第二次拒絕且狀態逐位不變，報告分開列基礎、慢速、快速三層的 L2，梯度更新不動 `slow`。證據見 [MOD-05](evidence/MOD-05/verification.json)、[MOD-06](evidence/MOD-06/verification.json)。

平台矩陣與治理（ticket 30 第一階段、OPS-04、GOV-01、GOV-04、GOV-05、GOV-06）已驗證：`scripts/platform-matrix.sh` 對 linux/amd64、linux/arm64、darwin/arm64、windows/amd64 交叉編譯並 vet，只有本機 darwin/arm64 真正執行過 unit、race、vet 與 mod verify，JSON 裡 compiled 與 executed 分開標記；`dynamics.Config.Workers` 的並行分區在連續與 LIF 核心都與單執行緒逐位相同（前向與梯度），每步配置量不隨步數增加；治理測試檢查需求 id 集合、狀態值、passed 必有證據、Root 決策有日期；README 能力表四欄與科學界線、授權盤點（182 個模組、68 個未知）與 `blocked_permission` 的發布清單、`docs/INDEX.md` 的 39 個相對連結都經指令核對。證據見 [OPS-04](evidence/OPS-04/verification.json)、[GOV-01](evidence/GOV-01/verification.json)、[GOV-04](evidence/GOV-04/verification.json)、[GOV-05](evidence/GOV-05/verification.json)、[GOV-06](evidence/GOV-06/verification.json)。

適應性評估（ticket 23 第二階段、LRN-10）已在 fixture 驗證：評估協定預先宣告可用回饋、適應與評分分割、題間重設政策；fixed 模式只推論，adaptive 模式在適應分割允許宣告的局部更新但基礎參數凍結；三項污染檢查（參數逐位不變、評分分割拒絕回饋、評分題目送進重播庫全部被拒）與打亂順序逐位相同都寫進報告，CLI `examples run evaluate` 把模式寫進結果檔。全重設政策下兩種模式的成績相同，差異要用不重設可塑狀態的政策才看得到，本次沒有跑。證據見 [LRN-10](evidence/LRN-10/verification.json)。

遷移與敏感資料（ticket 24 第三階段、STA-05、STA-06）已在 fixture 驗證：`checkpoint.Migrate` 讀原件、寫新檔、不覆寫任何檔，報告前後 SHA-256、欄位變更與資訊損失（唯一的真實遷移是聯集出現前的連續個體檔，兩條欄位變更、無損失；同 schema 逐位複製；其他組合明確拒絕），CLI `checkpoint migrate` 把報告印成 JSON；`internal/redact` 遮罩 sk- 金鑰、Bearer、token／api_key 與 JSON 裡的 secret、password，原始文字、音訊、影像只留長度與 SHA-256 前 8 碼，跨 Write 邊界也不漏；`scripts/scan-commit.sh` 擋金鑰樣式、大檔、data/ 原件與教師回應原文，pre-commit 範本要手動 `git config core.hooksPath .githooks`；根套件測試證明沒有遙測字串、只有 download、teacher 與 internal/cli（`data sources` 的 HEAD 檢查，既有偏差）import net/http。證據見 [STA-05](evidence/STA-05/verification.json)、[STA-06](evidence/STA-06/verification.json)。

## 階段目標

ticket133重播恢復的正式修正已通過完整工程與舊存檔接續驗證，本輪暫存已清除且遠端交付完成。接著由ticket134處理極端有限值的評估誤差溢位。LRN-07恢復passed，LRN-10保持specified，工程追蹤90／91。整體零bug與模型品質改善尚未證明。

前一階段 ticket 66 的同一真實單字元化學開關比較已完成五項驗收，固定設定沒有顯示穩定改善。

原始 91／91 工程驗收已完成。TSK-11 依 §13.1–13.6 完成五類有授權真實任務流程與保存恢復，OPS-05 完成裝置稀疏前向／反向、更新／恢復、CPU 對照與實測。§20.2 允許負面研究結果，§17.4 完整常駐是可行時的方向。模型實用性、GPU 全圖加速與生物機制成果分開判斷。

2026-09-15 使用者決定：把整個規格做完。原始 85 項到 2026-09-15 ticket 16 為止通過 26 項、NAT 六項通過五項，其餘依相依順序開票，先做能在 fixture 上驗證的，真實任務資料（TSK-11）與 GPU 後端（OPS-05）需要使用者提供資料或決定時明確標為受阻。路線：
15 強化（已驗證）→ 16 LIF 個體、慢速穩定、模型包（COR-04、STA-01、STA-03 部分）→ 17 遮罩、固定符號、完整最佳化器（COR-10、LRN-03、COR-07 證據）→ 18 局部可塑性（LRN-04、LRN-05、MOD-05 閘門）→ 19 runner 上的可塑性對照（NAT-06）→ 20 訊號來源與回饋三分離（SIG-03、MOD-01、MOD-08）→ 21 化學濃度、受體與調節效果（MOD-02、MOD-03、MOD-04）→ 22 記憶表現、控制器、對照與干預（MOD-06、MOD-07、MOD-10、COR-11）→ 23 運行中學習、重播、適應性評估（LRN-06、LRN-07、LRN-10）→ 24 遷移、敏感資料、SDK／CLI／組態／資源預估（STA-05、STA-06、OPS-01、OPS-02、OPS-03、OPS-06）→ 25 混合類型、向量節點、重算（COR-05、COR-06、LRN-02）→ 26 持續學習矩陣與模仿學習（LRN-08、LRN-09）→ 27 教師與蒸餾（TCH-01..06）→ 28 導航環境、多模態配對、共用核心、歸因（TSK-08、TSK-10、TSK-09、TSK-12）→ 29 真實任務（TSK-01..07、TSK-11，受阻於授權資料）→ 30 平台、裝置、全圖訓練、效能、治理、FlyWire（OPS-04、OPS-05、OPS-07、OPS-08、OPS-10、GOV-01/04/05/06、DAT-06）。

2026-09-15：票 21～30 的契約已全部定案並提交（`docs/tickets/21-*.md` 到 `30-*.md`），每票列 root 決策、分階段契約、驗收與依據；MOD-05 的受體來源與 MOD-09 分別併入 22 與 26。剩餘 31 項需求的規格抽取由 agy Gemini Flash 完成，紀錄在 root 的工作區。受阻項維持：TSK-11 真實任務資料、OPS-05 裝置後端（技術與存取）、DAT-06 的 FlyWire 真實資料、GOV-05 的發布授權，皆等使用者決定或提供。

2026-09-14 使用者定下兩條並列的一級路徑：Connectome 原生模擬（不經訓練直接執行）與可學習模式，見 [研究方向](docs/research-directions/connectome-native.md)、`AGENTS.md` 與 [NAT 增補需求](docs/requirements-addendum.json)。接下來依序做 [12 原生 runner](docs/tickets/12-native-runner.md)、[13 參數 adapter](docs/tickets/13-parameter-adapter.md)、[14 空模型與判讀](docs/tickets/14-null-models-and-behavior.md)，官方發布中能接的檔案（`body-stats`、`tbar-neurotransmitters`、`syn-partners`、`Neuprint_Meta.csv`）在 13 納入，盤點見 [發布盤點](docs/malecns-release-catalog.md)。

[10 真實子圖整合範例](docs/tickets/10-real-subgraph-example.md) 已驗證。98 檔同源的 Mac／Ubuntu v10 建置、單元、race、vet、模組與跨程序恢復驗證全部通過。ALIN 24 節點、110 邊的來源稽核、運算邊與初始化權重核對相等。

SIG-04 已依 §6.4 保存選取、來源、神經元指紋與投影重建資料（ticket 03）。ticket 05 已加入持續個體、選擇性重設、安全快照與隔離驗收，舊 episode 快照缺失／null scalar 靜默變零的 P1 缺口已於 2026-09-14 修正。下一步接續原規格的 LIF 放電核心。人工延遲關聯流程保留為 CPU 數值參考。框架不綁定使用者的單一模型或任務程式。

## 里程碑

| id | 目標 | 負責者 | 狀態 | 驗證訊號 |
| --- | --- | --- | --- | --- |
| 132 | 使用者可以限制適應評估可使用的回饋 | Codex Root／Luna max 唯讀審查 | completed | 4／4。完整工程、專項、舊模型、清理與遠端提交核對完成 |
| 131 | 固定評分保持短期記憶 | Codex Root／Luna max 測試與實作 | completed | 4／4。完整工程、清理及遠端提交核對完成 |
| 133 | 使用者可以拒絕不可能的重播恢復狀態 | Codex Root／Luna max 測試、實作與唯讀審查 | completed | 4／4。完整工程、最後31項治理、48份舊存檔接續、清理與遠端讀回完成 |
| 134 | 使用者可以取得可表示的評估誤差 | Codex Root | ready | 1／4。公開有限值反例與精確參考已確認，正式修正待接續 |
| 130 | 線上作答與回饋更新各自遵守閘門政策 | Codex Root／Luna max 測試與實作 | completed | 4／4。完整工程、清理與遠端提交核對完成，LRN-06 passed |
| 129 | 研究者可以拒絕真正不可表示的記憶寫入報告 | Codex Root／Luna max 測試與契約審查 | completed | 4／4。完整工程、清理與遠端提交核對完成，MOD-06 passed |
| 128 | 研究者可以匯出有限介入差異的報告 | Codex Root／Luna max 實作與唯讀審查 | completed | 4／4。專項、完整工程、清理及遠端提交核對完成，COR-11 passed |
| 127 | 凍結個體可取得正確介入對照且失敗維持原狀 | Codex Root／Luna max 契約、實作與審查 | completed | 4／4。兩項修正、90 項專項、完整工程、清理及遠端提交核對完成 |
| 122 | 研究者可以接續可表示的慢速活動狀態 | Codex Root／Luna max 唯讀審查 | completed | 4／4。專項、正常／舊保存、完整工程、清理及遠端提交核對完成 |
| 116 | 研究者可以在有限尾端設定下使用正確的放電閾值與梯度 | Codex Root／Luna max 測試、實作與唯讀審查 | completed | 5／5。完整工程、正常／舊保存、清理、提交推送及 833 份提交來源核對完成 |
| 115 | 研究者可以保留可表示的極端替代梯度 | Codex Root／Luna max | completed | 5／5。完整工程、相容性、清理、提交與遠端核對完成，見 ticket 115 |
| 91 | 研究者可以隔離並恢復受體驅動的個體宣告 | Codex Root／最低可用派工配置 | completed | 5／5。依 ticket 91 已記錄的宣告隔離、恢復驗證、工程及交付完成狀態同步，本輪沒有重新驗收該票 |
| 90 | 研究者可以寫入慢速記憶後繼續運算 | Codex Root／Luna max 唯讀稽核 | completed | 5／5。完整工程、11 項治理、合法對照、暫存清理、提交推送及 796 份提交來源核對通過 |
| 89 | 研究者可以核對多通道化學與來源歷史符合宣告機制 | Codex Root／Luna max | completed | 4／4。增益範圍必須包含 1 的修正已核准，正式工程、舊保存、清理與遠端核對完成，見 ticket 89 |
| 88 | 研究者可以核對混合神經元的完整化學訓練計算 | Codex Root / Luna max 測試與契約稽核 | completed | 4／4。獨立參考、初始來源漏算修正、完整工程、11 項治理、來源核對、暫存清理與主要推送讀回完成 |
| 87 | 使用者可安全拒絕不可表示的化學配置 | Codex Root／Luna max | completed | 5／5。完整工程、11 項治理、合法相容性及 791 份提交來源核對通過；本輪暫存清除、主要提交推送與遠端讀回完成 |
| 85 | 混合線上更新失敗後只接續未完成階段 | Codex Root／Luna max | completed | 5／5。10 個新增回歸、32 組正常逐位相容、完整一般／race 各 58 套件及 11 項治理通過；暫存清除、推送及遠端核對完成 |
| 86 | 同時操作同一個體時答案版本與回饋加權一致 | Codex Root／Luna max | completed | 5／5。正式 RED／GREEN、20 次 race、完整工程與治理通過；清理、推送及遠端讀回完成 |
| 78–84 | 化學與線上訓練使用一致狀態，共享快照及失敗回饋安全處理 | Codex Root／Luna max | completed | 22個新回歸、完整一般／race各58套件及最後11項治理通過，合法相容性、782份來源、清理與遠端讀回完成 |
| 74–77 | 使用者可取得完整拒絕報告、拒絕非法 OCR 設定及安全接續控制器 | Codex Root／Luna max | completed | RED／GREEN、合法案例逐位比較、完整一般／race 各 58 套件與最後 11 項治理通過，775 份來源指紋相符，暫存清理、推送及遠端讀回完成 |
| 67–73 | 核對設計並修復七類數值、生命週期與輸入缺陷 | Codex Root／Luna max | completed | RED／GREEN、完整一般／race 各 58 套件及最後 11 項治理通過，770 份來源核對、暫存清理與遠端提交讀回完成 |
| 66 | 同一真實單字元任務的化學開關效益 | Codex Root／Luna max | completed | 三組完整配對、六個模型還原、全套檢查通過，主要平均 +0.28 百分點但效果不穩定，暫存清理與主要提交遠端核對完成 |
| 59 | OCR／語音／文字模型保存與新程序讀回 | Codex Root／Luna | completed | 功能與本機／SMB 比對通過，暫存清理、推送核對及原專案同步完成 |
| 60 | 裝置 AdamW 更新與恢復 | Codex Root／Luna | completed | 實際 Metal 與 CPU 對照通過，暫存清理、推送核對及原專案同步完成 |
| 61 | GPU 裝置訓練器及完整 bundle 接續 | Codex Root／Luna | completed | 部分累積窗的新程序精確接續通過，暫存清理、推送核對及原專案同步完成 |
| 62 | 圖片／頁面辨識與編碼器對照 | Codex Root／Luna | completed | PNG／JPEG、頁面、真實 KMNIST 配對驗證通過，暫存清理、推送核對及原專案同步完成 |
| 63 | 真實影音模型新程序生成 | Codex Root／Luna | completed | 120 次更新後 11 份模型／內容相同，暫存清理、推送核對及原專案同步完成 |

### 較早階段紀錄

以下是各階段當時的驗證狀態，保留歷史範圍及限制。

| id | 目標 | 負責者 | 狀態 | 驗證訊號 |
| --- | --- | --- | --- | --- |
| 58 | 使用者可以核對真實全腦分段訓練的效率改善 | Codex Root / Luna max 唯讀稽核 | completed | 4／4。全圖十六次數值相同，七組配對完成；一般與 race 各 57 套件、11 個治理專項通過。主證據 `20a0896` 已推送核對，2.62 GB 本輪副本與暫存已清除 |
| 52 | 研究者可在生物啟發協定執行前拒絕非法數值 | Codex Root / Luna max 測試 | completed | 5／5。17 個相關頂層測試、七份合法報告一致、Mac 完整檢查通過，實作 d3aa982 已推送核對 |
| 53 | 使用者可在專案內保存必要資料與結果，並清除暫存 | Codex Root / Luna max 測試及唯讀盤點 | completed | 5／5。207 份移位指紋相同，479 份必要原件保持，9,151 份暫存清除，實作 d3aa982 已推送核對 |
| 01 | 開發者可核對環境與 Insyra 實際能力 | 主 agent / API 查核 agent | verified_scoped | Mac 與 Ubuntu 實際 doctor、Insyra 數值探測通過 |
| 02 | 使用者可計算稀疏前向與完整梯度 | sparse agent | verified_scoped | 手算、獨立稠密參考、形狀錯誤、有限差分已通過 |
| 03 | 使用者可提供與對齊具名訊號 | 主 agent / Luna 審查 | in_progress | SIG-01／02／04／05／06 fixture 已通過；SIG-03 控制器資料流待完成 |
| 04 | 研究者可訓練連續核心 | 主 agent | verified_scoped | 報告 v2 只置換訓練標籤並拒絕資料流重疊；Mac/Ubuntu v5 來源三個 seed 皆通過 |
| 05 | 使用者可中斷並接續學習 | sparse agent / 主 agent | verified_scoped | COR-01／STA-02／STA-04 fixture 通過；持續電位／延遲史、隔離重設、安全保存與新程序接續已驗證，STA-01／03 尚待完整機制；STA-03（2026-09-19）：含可塑性快速權重、化學狀態、AdamW 最佳化器與資料游標的個體在子程序接續後與連續執行逐位相同，錯游標反例會分歧；順手修掉 checkpoint 拒絕受體閘門規則（`*int` 欄位）的載入 bug；`evidence/STA-03/`（STA-03 passed），STA-01 待完成 |
| 06 | 使用者可保存並續傳官方資料 | sparse agent / 主 agent | verified_scoped | 三份官方原件共 1,109,008,094 bytes，CRC32C 全通過；v4 CLI 另實測自動 CRC32C 驗證 |
| 07 | 使用者可逐批讀取 Feather 原件 | API agent / 主 agent | verified_scoped | annotations 211,577、NT 1,835,518、weights 151,856,684 列完整掃描，前後原件指紋一致 |
| 09 | 使用者可保存並讀回標準化接線圖 | 主 agent | verified_scoped | 容量、溢位、竄改／截斷／順序／索引與路徑替換回歸通過；真實圖 664 MB 在 Mac／Ubuntu 讀回再保存的獨立 SHA-256 均與原件相同，見 graph-store-v2 |
| 08 | 研究者可由官方原件建立可追溯標準化接線圖 | 主 agent（connectome）/ extsort subagent | verified_scoped | fixture 與真實資料皆通過；410 秒、RSS 3.40 GB、暫存 4.1 GB 後清空；raw 零重複 pair；durable GraphStore 切到 09 |
| 11 | 研究者可執行並訓練 LIF 放電核心 | 主 agent 指揮 / Opus 實作 | verified_scoped | 手算時序、tangent 參考與平滑模式有限差分通過，CLI `examples run lif-threshold` 三組 seed 只訓練閾值把保留 MSE 由 0.232 降到 0.052，COR-03／COR-09 已標 passed，COR-04 因慢速穩定未實作維持 specified |
| 12 | 研究者可不經訓練直接執行接線圖（LIF 持續狀態、原生 runner） | 主 agent 指揮 / Opus 實作 | verified_scoped | `LIFState`／`Advance` 與 `Forward` 逐位一致；`simulate` 手算 fixture、兩核心、決定性、分段接續、選擇器、門檻、容量與嚴格 JSON 測試通過；真實全圖 300 步 93 s／4.38 GB，NAT-01 passed；只在 macOS 實測，參數僅工程假設 |
| 13 | 研究者可由發布資料推導動態參數 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | `params` 套件與 `coimnet-parameter-set/v1` 檔案格式、`data derive`／`data validate --params`、`simulate run --params`（`derived_release/v1`，unknown 政策明示）；手算 fixture 由 root 獨立重算一致；真實全圖 25,563,197 條邊推導配對比例 1.000、sign +13.6M／−9.2M／unknown 2.75M，推導參數與均一參數同協定對照；NAT-02 passed；只在 macOS 實測，全圖只跑 exclude 政策 |
| 14 | 研究者可用空模型與判讀協定歸因 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 三種空模型（記憶體衍生物，seed 加 hash 重現）、具名集合、四種指標、事前門檻、`simulate compare`；fixture 手算由 root 重算一致；真實全圖 LIF 與連續核心各 10 格，NAT-03／04／05 passed；每種空模型 3 個 seed、只在 macOS、`population_sync` 未做 |
| 10 | 研究者可選取真實子圖並接上訓練核心 | 主 agent / Luna | verified_scoped | 獨立稽核及全部 ID／運算邊／初始化權重一致；兩平台各重跑一致、固定參數不變；跨平台最後參數指紋不同，僅作人工整合範例 |
| 15 | 強化：續傳、共用嚴格 JSON、邊順序守衛 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | `.part` 截斷續傳、死 pid 鎖回收、`signal` 改用 `strictjson`、推導的邊順序守衛皆有回歸測試 |
| 16 | 研究者可建立 LIF 個體、開啟慢速穩定並發布模型包 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 神經狀態聯集與舊檔升級、慢速穩定手算時序與收斂、模型包六種交叉拒絕、LIF 個體跨程序恢復逐項相等；COR-04、STA-01 passed；只有 fixture、只在 macOS |
| 17 | 使用者可以限制可更新參數、符號與範圍，並使用完整最佳化器流程 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 遮罩、固定符號（1,000 步不翻轉、有限差分 4.91e-05）、範圍投影、推導符號；損失縮放逐位相同、累積手算、三種排程手算與恢復接續、裁切順序；COR-10、LRN-03 passed；只有 fixture、只在 macOS |
| 18 | 研究者可以使用近期參與紀錄、學習閘門與兩種局部更新規則 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 兩種規則手算時序、閘門關／延遲閘門／衰退／上限／`w_min`、關閉逐位還原、快照往返，LRN-04、LRN-05 passed；第二階段（runner 對照）由 ticket 19 完成 |
| 19 | 研究者可以在原生模擬之上開啟可塑性，並報告原有輸出被增強、修改或破壞 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | protocol `plasticity` 區塊、runner 逐步有效權重、`simulate compare` 三格（original／plastic／learned_then_frozen）；fixture 手算逐位相同、NAT-01 `protocol_hash` 不變；全腦兩次各三格（`decay_p` 0.5 與 0.999，每格約 600 s、RSS 6.6 GB），報告只給差值與百分位；NAT-06 passed；只在 macOS、`stdp_pair` 只有 fixture |
| 20 | 研究者可以把觀察、目標與事後回饋分開，並選擇調節訊號的來源 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | `AvailableFeedback`、四種來源手算、獎懲映射四值分開、靜態 import 檢查腳本與 NaN 汙染測試；SIG-03、MOD-01、MOD-08 passed；控制器來源只保留名字（MOD-07） |
| 21 | 研究者可以運行有時間衰退的化學濃度、設定選擇性受體，並調節當下敏感度與有效閾值 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 濃度穩態／清除／中性／非負／傳輸手算、佔用率極端值無 NaN、三種受體狀態分離、效果中性逐位、`AdvanceModulated(nil)` 逐位等於 `Advance`；個體每步順序在連續與 LIF 各有手算表、100 步 `Parameters` 逐位不變、中途快照接續逐位相同、`modulation` 不再依賴 `simulate`；MOD-02、MOD-03、MOD-04 passed；來源對所有區域一致、只有 fixture |
| 22 | 研究者可以讓化學狀態調節局部學習與記憶表現、施加具名干預、訓練小型控制器並和簡單替代模型對照 | 主 agent 指揮 / opencode big-pickle 小票實作 | verified_scoped | 三階段 fixture 驗證完成（2026-09-23）：受體驅動閘門、讀出增益、穩定化（MOD-05、MOD-06）；具名細胞與通道干預（COR-11）；小型可訓練控制器（固定輸入寬度、無目標欄位、只經自身輸出的梯度，有限差分 6e-9）與容量匹配記憶控制器；五組同資料同預算對照（no_modulation、direct_reward、fixed_decay、trainable_controller、capacity_matched）與 `examples run ablate`；`evidence/MOD-07/`、`evidence/MOD-10/`（MOD-07、MOD-10 passed）。控制器視窗狀態不在快照、真實資料未跑 |
| 23 | 使用者可以在運行中依新經驗學習、使用受限容量的重播，並做適應性評估 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 第一階段驗證通過：`Act → Receive → Update` 順序、輸出 hash 不回寫、`Evaluate` 拒絕、重播 fifo／reservoir／三種抽樣手算與快照接續；LRN-06、LRN-07 passed；第二階段（適應性評估）切成小票派給 opencode 進行中；第二階段驗證通過：fixed／adaptive 兩種模式、三種重設政策、污染檢查三項（基礎參數不變、評分分割拒絕回饋、重播拒絕 8／8）、打亂順序逐位相同、CLI 把模式寫進結果；LRN-10 passed（全重設政策下兩種模式成績相同，差異未展示） |
| 24 | 使用者可以用嚴格展開的組態、啟動前的資源預估、完整 CLI 與 SDK，並安全遷移格式與保護敏感資料 | 主 agent 指揮 / Opus 5 實作 | verified_scoped | 三個階段驗證通過（2026-09-19）：組態嚴格展開與資源預估（OPS-03、OPS-06）；SDK 八個可執行範例、87 組非法輸入不 panic、五個取消釋放測試（OPS-01）；CLI `model inspect`／`model validate`、六階段 `benchmark`（匯入、前向、反向、局部學習、調節、快照，附 uptime）、`export`（json／markdown，未支援格式列出清單）、`report`（登錄、成績、限制、來源、完成度的 JSON＋Markdown）與各命令端到端及失敗測試（OPS-02）；遷移不覆寫原件、遮罩與掃描腳本、無遙測（STA-05、STA-06）；`evidence/OPS-01/`、`evidence/OPS-02/`、`evidence/OPS-03/`、`evidence/OPS-06/`、`evidence/STA-05/`、`evidence/STA-06/`；`run`／`evaluate`／`ablate` 的完整組態執行仍由 23／22 的骨架承接（見 OPS-02 limitations） |
| 25 | 研究者可以依神經元類型混合連續與脈衝規則、使用向量節點與共享參數，並以重算降低反向歷史記憶體 | 主 agent 指揮 / Opus 5 實作 | in_progress | 第一階段驗證通過：`dynamics.Mixed` 每節點一種規則、單一輸出契約、不雙重計入、編號順序無關、全 0／全 1 逐位等於既有核心（`-race` 下 LIF 核心自身的 1 ULP 差由 build tag 常數釘住）、平滑模式有限差分與手算兩步梯度；`learning` 第三核心與混合個體快照；COR-05 passed；第二階段（向量節點、共享參數）與第三階段（重算）待派工 |
| 27 | 使用者可以用離線與 HTTP 教師蒸餾學生，並在完全移除教師的情況下評估它 | 主 agent 指揮 / opencode big-pickle 小票實作 | verified_scoped | 三階段 fixture 驗證完成（2026-09-23）：統一教師契約與離線／重播／HTTP／封鎖教師（TCH-01、TCH-02）；學生自編碼、保留集隔離、合法對齊的分布蒸餾與 `StepFrom` 訓練（TCH-03、TCH-04）；`experiment/studenteval` 的 student 模式封鎖教師與網路、teacher_assisted 分開報告並計 fallback、保留集＋獨立來源集與 20% 錯標籤穩健性；工具允許清單與參數驗證、惡意教師回應只當資料（請求數、誘餌 0 次、預算與設定不變、未授權欄位不外傳）（TCH-05、TCH-06 passed）。真實遠端教師與真實任務未跑 |
| 30 | 使用者可以在目標平台使用 CPU 參考路徑、在真實資料上做全圖前向／反向與訓練、核對治理文件與授權，並匯入 FlyWire | 主 agent 指揮 / opencode big-pickle 小票實作、agy Gemini Flash 證據整理 | in_progress | 第一階段驗證通過：平台矩陣把 compiled 與 executed 分開標記（darwin/arm64 實際執行 unit／race／vet／mod verify，linux 與 windows 只交叉編譯與 vet）、`Workers` 並行在連續與 LIF 核心逐位相同且每步配置量不隨步數增加、治理測試四項、README 能力表與科學界線、授權盤點與 `blocked_permission` 發布清單、INDEX 連結檢查；OPS-04、GOV-01、GOV-04、GOV-05、GOV-06 passed；第二～四階段待派工，第五階段裝置後端（OPS-05）與 FlyWire（DAT-06）受阻於使用者；第二階段進行中（2026-09-19）：FlyWire adapter（CSV／gzip → Feather 與 manifest，命名空間 `flywire-<version>`，root id 到 2^63−1 精確往返、溢位拒絕，`docs/flywire-source-audit.md` 標 blocked_data）已提交，拼接禁止（`MappingEvidence`）票已寫好；第五階段 `backend` 能力宣告、明示回退與更新 ledger 已提交（OPS-05 可做部分）；第四階段乾淨環境腳本待 `report` 命令；第二階段 fixture 部分驗證通過（2026-09-19）：FlyWire adapter、`MappingEvidence` 拼接禁止與 named set 命名空間檢查，`evidence/DAT-06/`（DAT-06 passed，profile fixture；真實 FlyWire 檔案 blocked_data）；第四階段 `scripts/clean-env-verify.sh` 已落地（14 步 passed、3 步 blocked_data），效能報告與 OPS-10 證據待派 |
| 26 | 研究者可以量測先學 A 再學 B 的保持與適應、完成專家模仿與行動回饋學習，並執行有來源的生物啟發干預協定 | Codex 主 agent / Luna max / OpenCode big-pickle | verified_scoped | 三階段 fixture 驗證完成：LRN-08、MOD-09 與 LRN-09；第二階段含取樣 collector、3 seed 各 200 次更新、訓練前／隨機對照、同平台逐位重現與 CLI，Mac／Ubuntu 同源通過。保存狀態的 CPU 純量連續核心由 ticket 43 接續驗收；其他核心／重算的非零狀態、前綴梯度切斷與可塑性／化學 PPO 不支援 |
| 28 | 研究者可以在二維環境訓練導航與位置記憶、建立配對／缺失／非同步多模態資料、讓多任務共用同一核心，並比較核心、外圍與接線本身的貢獻 | 主 agent 指揮 / opencode big-pickle 小票實作 | in_progress | 第一、二階段進行中（2026-09-19）：`nav2d` 環境（視野錐、碰撞、終止與時間上限分開、BFS 專家）與四個任務變體、`multimodal` 配對資料契約（值＋presence）已落地並提交；合成產生器、評估與對照待派 |
| 29 | 使用者可以用同一核心做 OCR、語音轉文字、文字生成與文字條件影音生成，並把有授權的真實任務資料帶入框架 | Codex 主 agent / Luna max | in_progress | OCR fixture 已有 TSK-01 證據；ASR 整檔 CTC、因果串流與恢復有前次 Mac／Ubuntu 局部證據，本輪資料匯入、群組分割、離線耗時、CLI 人工範例與 Mac 全套驗證見 TSK-02；嚴格串流檔案保存已驗證人工音調；TSK-11 真實授權資料、跨平台續跑與後續生成階段保留 |
| 33 | 使用者可以拒絕污染保留集的真實軌跡推論快照 | Codex 主 agent / Luna max | done | 真實非法快照拒絕且無輸出，合法結果相同，Mac 完整驗證通過；修正提交 467d53a 已推送且遠端一致 |
| 34 | 使用者可以比較含刺激歷史的軌跡模型與無記憶控制組 | Codex 主 agent / Luna max | completed | 5／5 工程驗收通過，`55c2ef7` 已推送並核對遠端；模型各 0／13，沒有導航改善證據 |
| 35 | 研究者可以確認凍結模型是否使用搬移前刺激 | Codex 主 agent / Luna max review | completed | 5／5 通過，`21eb2ae`／`bf5d7fa` 已推送並核對遠端。234 配對重現、原側與前輪一致，所有模型原／清除組 0／13 |
| 36 | 研究者可以分開判讀回報改善、抵達與使用起始提示 | Codex 主 agent / Luna max review | completed | 5／5 工程驗收與交付，實作 `fd069c3` 已推送。39 組 1,560 回合重現，取樣抵達率 50%、97.5%、67.5%，整體提示判準未通過 |
| 37 | 研究者可以排除兩端搜尋後驗證提示導航 | Codex 主 agent / Luna max 測試與審查 | completed | 5／5 工程驗收與交付，`2733ffe` 已推送且遠端一致；三組提示判準均 false，沒有依提示導航的學習證據 |
| 38 | 研究者可以核對短時限訓練的兩側成功回饋 | Codex 主 agent / Luna max 唯讀審查 | completed | 5／5；完整 Mac 驗證與雙程序重現通過，`e0d1c43` 已推送並核對遠端。成功回饋集中單側，唯一因果機制未確認 |
| 39 | 研究者可以分開核對策略與價值的更新影響 | Codex 主 agent / Luna max 唯讀審查 | completed | 5／5；600 次完整梯度與更新比對、雙程序重現、獨立重算、17 種竄改檢查、Mac 完整一般與 race 驗證及審查通過。實作 0dc4d64 已推送並核對遠端 |
| 40 | 研究者可以比較六步任務期限對提示導航學習的影響 | Codex 主 agent / OpenCode 免費模型 / Opus 審查 | completed | 5／5；實作 58cb01e 已推送並核對；完整一般與 race 各 57 套件、雙程序重現、獨立核對與審查通過；期限組 seed 3 通過，1／2 未通過 |
| 41 | 研究者可以在大分數下正確計算 PPO 機率與損失，並收到非法數值錯誤 | Codex 主 agent / Luna max 測試、初版實作與審查 | completed | 5／5 驗收。原始 12 項有 9 項失敗，Root 補上加權價值極值回歸並修正，13 項新增與既有 20 項 RL 通過，完整驗證通過，實作 6d44edf 已推送並核對遠端 |
| 42 | 研究者可以在大分數下取得正確的蒸餾與文字序列損失 | Codex 主 agent / Luna max 測試、實作與獨立審查 | completed | 5／5；10 個新增數值回歸、124 個相關頂層測試、完整一般與 race 各 57 套件及審查通過，實作 8fb25c2 已推送並核對遠端 |
| 43 | 使用者可以接續連續核心的既有記憶進行 PPO 訓練 | Codex 主 agent / Luna max 測試、實作與獨立審查 | completed | 5／5。22 個專項、372 個相關頂層測試、完整一般與 race 各 57 套件及獨立審查通過。候選失敗原狀保持，實作 874ac43 已推送並核對遠端 |
| 44 | 使用者能區分 GPU 支援條件與本機探測 | Codex 主 agent / Luna max 測試與實作 | completed | 5／5。13 個頂層專項、真實 CLI、完整一般與 race 各 57 套件及 Root 審查通過，實作 d7c8963 已推送並核對遠端 |
| 45 | 維護者能找到每個版本的用途與來源，並在漏登錄時收到失敗 | Codex 主 agent / Luna max 盤點、測試與文件 | completed | 5／5；124 列、173 個本機連結、8 個專項、無 Git 匯出、完整一般／race 各 57 套件及原件查核通過，實作 baf7b91 已推送並核對遠端 |
| 46 | 使用者可以拒絕容量資訊與模型不符的模型包 | Codex Root / OpenCode big-pickle 測試與實作 | completed | 5／5。五個回歸、九個 CLI 流程、完整一般／race 各 57 套件及原件核對通過，實作 d4ffe93 已推送核對 |
| 48 | 研究者可以在導航開始前發現非有限數值設定 | Codex Root / Luna max 報告回歸 | completed | 5／5。四個新增回歸、合法報告、八個 CLI、完整一般與 race 各 57 套件及治理通過，實作 6f2aa38 已推送核對 |
| 49 | 研究者可以保留不受外部設定修改影響的導航報告 | Codex Root / Luna max 測試 | completed | 5／5。回歸、合法報告、CLI、完整一般／race 各 57 套件、治理與原件核對通過，實作 1b088e4 已推送核對 |
| 50 | 研究者可以在比較開始前發現非法數值設定 | Codex Root / Luna max 測試 | completed | 5／5。兩處設定驗證、三個新增回歸、合法 SDK／CLI、完整一般與 race 各 57 套件及治理通過，實作 933ba4c 已推送核對 |
| 51 | 研究者可以保留連續任務執行當時的設定 | Codex Root / Luna max 測試 | completed | 5／5。八組回歸、30 個相關頂層、七份完整報告與雙程序一致、八個 CLI、完整一般／race 各 57 套件及治理通過，實作 3546b88 已推送核對 |
| 54 | 研究者可以保留生物啟發協定執行當時的設定 | Codex Root / Luna max 測試與唯讀審查 | completed | 5／5。2026-10-11 最終來源的完整一般與 race 各 57 套件、11 個治理專項、14 份合法報告雙程序比對通過，驗收副本與快取已清除 |
| 55 | 使用者可以把模型存在網路磁碟並讀回接續 | Codex Root / Luna max 測試與唯讀審查 | completed | 5／5。SMB 資料夾模型保存、讀回、接續與並行保護通過，2026-10-11 最終來源的本機完整一般／race 各 57 套件通過，暫存已清除 |
| 56 | 使用者可以對合法大型模型開啟分段重算訓練 | Codex Root / Luna max 測試、實作與唯讀審查 | completed | 5／5。完整一般與 race 各 57 套件、28 個重算與 11 個治理專項及 CLI 通過，665 份 Go 提交指紋一致；原專案回存、暫存清理、推送及遠端核對完成 |

## 目前阻礙

ticket133重播恢復的必要驗證已正式修正並通過完整工程，本輪暫存已清除且遠端交付完成。ticket134極端有限MSE溢位的已確認缺陷仍待修正，LRN-07恢復passed，LRN-10保持specified，工程追蹤90／91。本階段沒有重新驗收真實品質與生物校準。混合重算是既有明示不支援的組合。

### 既有環境與研究限制

前次直接在 SMB 執行的完整一般與 race 各有 47 個通過套件、10 個失敗套件及 176 次不支援操作的文字出現，沒有觀察到資料競爭警告或測試逾時。那兩輪在最後兩個錯誤處理檔案補強與 INDEX 校正前啟動。當時 Go 依賴校驗反覆回報缺少已有項目，相同檔案下重試通過，media 的測試目錄清理失敗也曾重現，兩者根因未確認。這些失敗紀錄保留，不能用後續本機通過推論 SMB 上的所有儲存操作都可用。網路模型驗證不需要 SSH，本輪沒有遠端執行。

Tickets 54、55 的實作、回歸與本機完整驗收已完成。先前 OpenCode 派工在執行前被自動審核拒絕，原因為外部代理可能讀取私人原始碼且沒有硬性檔案邊界。本次文件派工也在執行前被拒絕，原因為未授權向外部模型傳送私人專案文件。兩次都沒有模型呼叫或 403 回應，依專案規則改用 Luna max，Root 親自核對與驗證。Tickets 52、53 的完整檢查與交付屬前一階段。

[ticket 35](docs/tickets/35-frozen-stimulus-history-counterfactual.md) 的真實軌跡原／清除組均為 0／13。[ticket 38](docs/tickets/38-ppo-training-feedback-diagnostic.md) 確認 seed 2／3 的成功回饋與最後策略集中同一側，但目標兩側都有供給。seed 1 最後 137 更新沒有成功，逾時第一步優勢全部為正。[ticket 39](docs/tickets/39-ppo-gradient-diagnostic.md) 已分離三項原始梯度，觀察到共用核心的局部競爭。梯度與探索對長期學習的因果作用尚未驗證，不能依此選參數。感覺／動作記錄不足，工程成績不能宣稱生物機制。

前一階段的 rl-asr-takeover 跨平台整合已通過（2026-09-22）。該階段 Ubuntu 初次完整測試的 6 個失敗來自測試的浮點捨入假設、固定梯度數字，以及恢復測試把保存值換成 Mac 字面常數。修正限定於五個測試檔，未改核心運算；保留平均、裁切與 AdamW 的嚴格手算對照及快照逐位一致檢查。兩台機器已用同一份最終來源重跑完整驗證通過，原始失敗與診斷日誌一併保存。

2026-09-22 重試 Spark 仍回 `Unknown model gpt-5.3-codex-spark`，collector、runner 與審查使用 Luna `max`，CLI 使用 OpenCode `opencode/big-pickle` 免費模型。Insyra 自訂 tape 梯度接合與最佳化器序列化缺少公開 API，已提 [#375](https://github.com/HazelnutParadise/insyra/issues/375)、[#376](https://github.com/HazelnutParadise/insyra/issues/376)。目前使用有完整數值測試的兩段 tape 接合，以及 CoImNet 自有可保存 AdamW。

嚴格 JSON 的深度與路徑配置缺口已修正，並將 Unicode 重複鍵檢查同步至訊號、快照、下載 metadata 與 manifest。圖檔補上配置前記憶體檢查、計數溢位、未初始化圖、未知 converter 與同次讀取 hash 回條；回歸測試及兩平台 v7 全套驗證通過。

`data download` 的續傳缺口（程序被砍後 meta 仍為 0、失效鎖不回收；2026-09-14 syn-partners 因此重抓）已於 [ticket 15](docs/tickets/15-hardening.md) 修正：每 64 MiB fsync 後寫回 `bytes`，`.part` 長於記錄者截斷後續傳，死 pid 的鎖回收並寫進回條。

Mac 可執行本機測試。Ubuntu 1 過去已實際連線並確認 RTX 4070 12 GB，但 2026-09-23 重試本階段 SSH 時收到 `Permission denied (publickey,password)`，因此本階段沒有 Ubuntu 執行證據。PC 的 App 主機連線存在，但沒有可用的遠端指令工具，SSH 22 逾時，App UI 操作被工具限制拒絕。Windows 實機驗證尚未執行。GPU 硬體存在不等於稀疏訓練後端完成。

## 下一個可驗證成果與 ticket

[ticket133](docs/tickets/133-replay-restore-state-validation.md)已通過完整工程與舊重播存檔接續，必要證據封存、暫存清理與遠端交付均已完成。下一個可驗證成果是[ticket134](docs/tickets/134-evaluation-mse-representability.md)的有限MSE中間溢位修正。工程追蹤90／91，這些測試不作為模型品質或零bug的證明。

[ticket 66](docs/tickets/66-real-ocr-chemical-comparison.md) 已完成 5／5，沒有剩餘任務工作。下一個建議成果是以獨立開發資料改善新圖片的辨識，另留最終測試，避免用本輪測試結果挑參數。尚未建立或執行新票。固定化學設定的結果只適用於本輪人工核心與單字元任務。

先前 [ticket 54](docs/tickets/54-bioinspired-config-ownership.md) 與 [ticket 55](docs/tickets/55-network-model-bundles.md) 均完成 5／5 驗收。OPS-05 必須有裝置稀疏前向／反向、更新／恢復及實際量測。主規格 §17.4 的常駐狀態是條件性效能方向，全圖驗證另由 OPS-07 驗收，不能直接追加為 OPS-05 的無條件門檻。

[ticket 40](docs/tickets/40-six-step-deadline-training-comparison.md) 的期限對照已完成交付。方向學習只有一個 seed 通過，新的初始種子穩定性研究須採固定設定並另存證據。OPS-05 的裝置更新整合已完成，GPU 全圖與 Ubuntu RTX 4070 是未驗證的擴充範圍，不能由 Mac 小圖推論。

與這兩項需求獨立的後續工作記於 [AGENTS.md](AGENTS.md#follow-ups)。ticket 43 補上 CPU 純量連續核心保存狀態的梯度路徑；ticket 26 的其他核心／重算與機制限制保留。跨平台 ASR 串流接續與超過 64 MiB 個體保存尚未驗證，不能以本機人工音調證據代替。

歷史 Mac／Ubuntu v25 是 2026-09-14 的 137 檔來源驗證，不能代表目前 checkout；新階段證據必須帶當次來源指紋。需求累計以 `docs/requirements-status.json` 為準。

## 決策紀錄

2026-10-16：ticket128 的四種介入差異保持正常平方和位元，僅在中間上下溢位改用穩定計算，真正不可表示時保留原 runtime 與來源快取，以同一把操作鎖保護讀取與 cleanup。COR-11 恢復 passed。另確認 MOD-06 三種記憶報告在最高端誤接受，回到 specified，由 ticket129 接續。

2026-10-16：ticket 127 的私有介入對照沿用 runtime 凍結旗標，先完成對照再推進 subject，維持一般 Restore 預設解凍與模型格式。另已公開重現報告範數上下溢位，COR-11 保持 specified，ticket 128 接續修正。

2026-10-16：ticket 122 只在慢速活動率原乘加非有限時，以 FMA 重算同一算式。保留有限路徑、設定合法域、停止梯度、模型格式與依賴。公開紅燈以本機 Rosetta amd64 執行核對，完整框架另用 arm64 驗收。

2026-10-14：ticket 90 僅拒絕會提交不可表示有效權重的記憶寫入，正常公式、兩次掃描與模型格式保持。公開唯讀 Model.Edge 避免複製整份邊選取。受體指標隔離另列後續，不改 ticket 89 的待決策相容性條件。

2026-10-14：ticket 88 以測試私有前向微分核對既有硬事件及替代導數，沒有新增公開 smooth 模式。初始來源漏算先重現，再以核心原有橋接修正。顯式常數、持續歷史與截斷規則保持，補充證據不變更既有 91／91 工程狀態。見[Root 審查](evidence/MOD-02/mixed-chemical-gradient-20261014/root-review.md)。

2026-10-14：ticket 87 只增加私人 float64 向量／網格可表示大小檢查，錯誤早於配置與狀態提交，不增加任意容量上限或改變生理公式。初始神經來源缺乏活動的拒絕符合契約。正常設計疑點及混合核心驗證限制見[Root 審查](evidence/MOD-02/chemical-capacity-20261014/root-review.md)。

2026-10-14：tickets 78–84沿用有限值、化學私有episode、共享參數與回饋間隔契約。新增回歸先在未修實作失敗，Root凍結後才修正，最終來源重新完整驗證。正常公式、公開簽名、保存格式與相依性保持，不追加跨入口整體回滾或生物身分保證。詳見ENG.md及[兩階段計畫](evidence/MOD-02/chemical-boundaries-20261013/online-sharing-plan.md)。

2026-10-13：tickets 74–77 固定正常輸出算式，控制器失敗保留既有狀態，成功參數更新清除舊快取。一般訓練的固定參數與 fresh-forward 順序保持，沒有新增保存欄位或版本。共用契約見 ENG.md，四類修正及限制見[本階段計畫](evidence/MOD-06/refusal-input-audit-20261013/plan.md)。

2026-10-13：tickets 67–73 沿用既有運算與保存格式，錯誤在私有候選提交前拒絕。較大舊鞏固標記保留載入相容，在明確重設最佳化器時對齊時鐘。受體部位紀錄與單一神經元整合是既有假設，沒有另行加入多區室膜模型。共用契約見 ENG.md，驗收與限制見[機制稽核](evidence/MOD-02/mechanism-audit-20261013/README.md)。

2026-10-13：ticket 66 固定三個種子的 ON／OFF 配對，沿用 OCR SDK 與通用 Trainer。控制器及反應係數固定，不增加可訓練參數，不依測試挑參數或早停。小幅且方向不一致的改善保留為研究結果，維持可選機制。見 ENG.md 與本票 protocol。

2026-10-12：依原規格 §13.2 補 PNG／JPEG、同一 OCR 模型的頁面文字、固定／可訓練編碼器對照與空輸出率，依 §13.1／15.3 補影音模型 task bundle 和新程序推論。§20.2 允許負面研究結果，不追加「所有真實任務必須改善」的門檻。OPS-05 以實際裝置稀疏訓練更新／恢復及量測驗收，§17.4 可行時常駐神經狀態、GPU 全圖及其他平台維持未驗證擴充，不能當無條件門檻。共用決策見 ENG.md，範圍由 tickets 61–63 固定。

2026-10-11：ticket 58 事前固定完整 MaleCNS 圖、16 步、段長 8、截斷窗 8，直接量測三個既有 SDK 重算呼叫。只有兩份核心檔以 Go overlay 對照，正式七組交錯配對，所有原始結果保留；人工刺激與目標不計為真實任務或生物成績。詳見本票與完整量測報告。

2026-10-11：ticket 56 新增連續及 LIF 的 ForwardRecompute，以既有段落運算處理有限序列，訓練 observe 呼叫該入口。保留既有反向公式、持續狀態容量保護與公開訓練流程，不新增設定、格式或相依性。

2026-10-10：ticket 55 沿用 v2 資料夾格式及三種 Bundle SDK，資料先同步關閉，完整清單最後以排他建立發布。完成後的取消與同步錯誤保留模型，不確定的發布錯誤保留目錄並說明狀態。單檔契約、格式與依賴保持。

2026-10-10：ticket 54 在 RunBioInspired 的既有 Validate 之後、雜湊及兩種協定執行之前複製 Seeds、PulseSteps、Registry 及 Quantitative。保留 nil／非 nil 空值、完整合法報告及原錯誤，取消後的部分報告也獨立持有設定。Root 凍結 Luna max 的五個頂層測試後實作，沒有新增 API、依賴或數值操作。

2026-10-10：ticket 52 只補 NPF Suppressed 與 Tolerance 的有限值驗證。ticket 53 依使用者要求把必要 data／runs 放回專案已忽略資料夾，指紋相同才搬移，歷史路徑保留為 provenance。realnav 只允許最近 repository 的 runs 新子目錄，其他輸出拒絕保持。已證明可重建或已保存的暫存才清除。

2026-10-09：ticket 51 在既有驗證之後、雜湊與 builder 之前複製完整可變 Protocol。保留可選 nil、空化學列、原錯誤次序與合法報告，不增加不可變 API 或並行修改輸入的保證。Luna max 只寫測試，Root 審查、凍結及實作。

2026-10-09：ticket 50 只把歸因與連續任務的 Comparison.Interval 改成正向範圍判斷，保持原錯誤次序、公開介面與合法訓練。Luna max 補測試，Root 親自審查、凍結及填入兩處判斷。

2026-10-08：ticket 49 於共用報告入口沿用 Seeds 的切片複製補足 Policies。回傳後的雙向元素修改與追加隔離，不新增不可變 API 或同步能力；合法報告與訓練參數保持。OpenCode 人工契約的 --pure 目錄不符，已停止並改由 Luna max 完成測試，Root 自行審查與最小實作。

2026-10-07：ticket 48 沿用共用環境驗證，只拒絕四個環境欄位的非有限值，保留有限值與各入口原有錯誤次序。OpenCode big-pickle 在隔離人工契約中回覆免費服務 403，Luna max 只撰寫指定報告回歸，Root 審查、凍結測試並完成最小實作。

2026-10-07：ticket 47 用共用設定解析修正新報告與雜湊，原預設值、驗證順序、訓練及格式保持。OpenCode 私人原始碼存取被自動審查拒絕，隔離人工契約的派工收到 403，Claude／Antigravity 額度受限，Luna 首次測試派工滿載，後續 max 唯讀審查通過。Root 完成此小範圍修正與驗證。

2026-10-07：ticket 46 核對非 nil 的容量報告，模型包全數參數視為可學。驗證共用既有 Trainer，nil 舊欄位保持；不新增 API、保存版本或資料遷移。

2026-10-07：ticket 45 沿用單一 INDEX，不增加執行時註冊表或指令。正式格式、版本識別、驗證工具與測試反例分開；一致性檢查不依賴 Git 或外部服務，也不掃描模型及歷史 JSON 成果。

2026-10-06：ticket 43 新增 CPU 純量連續核心的保存狀態梯度入口，包含固定延遲歷史，PPO 算分與梯度使用相同初始狀態。共用 Insyra 橋接與既有最佳化器，fresh-zero 路徑、格式、依賴與範例設定保持。其他核心及 Recompute 的非零狀態保留未支援限制。

2026-10-06：ticket 42 修正蒸餾與共用 CTC 的位移正規化，保留原始不可能目標策略。CTC 額外拒絕已重現的條件路徑機率失精，錯誤清空結果。以公開入口手算及獨立路徑枚舉驗收，不調整模型、任務或依賴。

2026-10-06：ticket 41 集中修正通用 PPO 機率、損失及非法數值處理，公開簽名與模型設定保持。蒸餾與文字辨識的同類錯誤另行處理，先於新增帶記憶的 PPO 能力。此數值缺陷尚未證明是方向學習失敗的原因。

2026-10-05：使用者確認 ticket 40 的六步任務期限對照。只在範例內改最後逾時的訓練終止與後續價值，原環境抵達及逾時另存。兩組沿用相同固定模型、種子、預算與 ticket 37 判準，原組須與 ticket 38／37 逐筆一致，所有結果保存。沒有依結果增加容量或搜尋參數。

2026-10-04：ticket 39 重播相同六步模型與原始回合，只增加三項完整梯度與實際更新量的診斷。共用核心的局部競爭不能推論長期因果，介入前需固定時限語意及判準。Python 逐座標重算 Adam，神經梯度以 Go 重播、有限差分及原更新比對驗證。

2026-10-04：ticket 38 固定 ticket 37 全部設定，只增加逐回合訓練診斷與每 20 更新的副本探測。觀察性成功回饋失衡不宣稱唯一根因，沒有調參、修改 PPO 或更新能力狀態。

2026-10-03：ticket 37 在結果產生前固定 6 步與左右共用動作亂數配對，只改測試範例。三個 seed 全部滿足完整抵達、前後／random 改善與清除／反轉提示門檻才記 true，不依結果調參。

2026-10-03：ticket 36 只新增測試範例的提示清除／反轉與逐回合抵達檢查，公開 API、PPO 更新及原模仿模型不變。seed 2 的工作判準通過不等於學得必要提示記憶，20 步的任務可搜尋兩端；後續縮短時限須作另一個事前固定的實驗。

2026-10-03：ticket 35 固定既有模型與 13 test，不訓練或調參。模型輸入先轉換再清除搬移前刺激，保留原始區塊控制後段刺激。工程尺度 1e-6 cm 與全部配對先固定。結果只支持歷史刺激敏感，所有原／清除組 0／13，後續導航方法待定。

2026-10-03：使用者核准位置與已發生刺激的工程代理。保留原模型，完整試次模仿學習及自主控制對照由 ticket 34 驗收，感覺與神經 reward 到達時間保持未知。

2026-09-13：使用者授權開始規劃與實作，沿用既有 Go＋Insyra 及完整需求。先驗證梯度與可學習的小型流程，之後接真實資料，避免在未驗證核心上堆疊任務。

2026-09-13：官方 Feather 需要 nullable int64、dictionary 與 list 型別；Insyra 缺口已提 [#377](https://github.com/HazelnutParadise/insyra/issues/377)，資料 adapter 使用現有相依版本的 Arrow Go v17。查核見 [來源紀錄](docs/malecns-source-audit.md)。

2026-09-13：使用者授權在完成可驗證階段後主動提交與推送本專案，依 AGENTS.md 執行。

2026-09-13：ticket 08 的 root 決策：選取 predicate 用 `annotations.status == "Traced"` 並標為工程選取；端點身份以 manifest 明示並附官方頁面依據；比例保存分子、分母與欄位；節點順序為 namespace 內數值遞增；真實資料驗收限制 4 GiB 記憶體、16 GiB 暫存、256 run；durable GraphStore 切到 09；raw view 以重新讀取原件串流。詳見 [ticket 08](docs/tickets/08-connectome-graph.md)。同輪修正 `feather` 對零長度子陣列 nil buffer 的誤判。

2026-09-13：ticket 09 的 root 決策：圖儲存採自訂固定寬度區段加 JSON footer（非 Arrow IPC），每段與 footer 各有 SHA-256，檔案不含時間戳；發布沿用 checkpoint 的暫存檔＋hardlink＋目錄同步語意；讀回重算三個結果 hash 並檢查結構不變量；store 只保存 weights 原件路徑與指紋。不單獨標記需求通過，DAT-07 待真實子圖實驗。詳見 [ticket 09](docs/tickets/09-graph-store.md)。

2026-09-14：使用者決定框架必須同時支援 Connectome 原生模擬與可學習模式，底層不得綁死其一；五層分離；未訓練不等於沒有假設；歸因必須有空模型對照；官方發布中能接的檔案全部納入範圍。記錄於 `AGENTS.md` 與 [研究方向文件](docs/research-directions/connectome-native.md)，新增 NAT-01～06 增補需求。

2026-09-13：使用者明確界定 repo 僅提供框架，特定模型的訓練程式與模型限定為範例。維持原需求，五類任務與全圖訓練採範例／驗收流程。資源量測寫入 [resources](docs/resources.md)，現有神經網路的關係與可選機制寫入 [model-and-mechanisms](docs/model-and-mechanisms.md)。機制可選沿用原規格，不新增無法查證的全生物機制模式。

2026-09-14：ticket 03 的多通道範例保留各串流的來源序號，以共同神經步號填入矩陣，缺值用 presence 區分。同一步脈衝只在範例內明示加總，核心 API 保持通用。

2026-09-14：訊號持久化數值欄位拒絕明確 null，已宣告可空的品質分數／範圍與省略欄位預設保持原語意。SIG-01 依四種人工數值來源驗收，媒體解碼與生物映射分開追蹤。SIG-04 的明確集合局部測試不取代完整保存／替換流程。

2026-09-14：SIG-04 以獨立 Projection 保存加權映射，既有 Mapping 格式保持相容。InputNodes 指定集合時只配置對應的編碼器欄位，完整梯度經選定索引傳遞。替換保留核心、使用提供的兩側係數並建立新最佳化器。容量與來源條件見 [映射指南](docs/signal-projections.md)。

2026-09-14：本輪依使用者一次三項的要求完成 COR-01／STA-02／STA-04 fixture。新個體 profile 固定 CPU 精度及生命週期，不推論完整訓練工作恢復。跨 CPU 活化值最多四個相鄰 float64 值的載入容許範圍保留歷史原值；雙向真實傳檔驗證見 [跨機器證據](evidence/STA-02/crosscpu-verification.json)。舊 episode 缺失欄位缺口優先列入 ticket 05；checkpoint 分工的測試時序偏差已如實保存。

## 來源與接手

[共用設計](ENG.md)、[近期 tickets](docs/tickets/)、[主規格](docs/handoff/CoImNet_Implementation_Plan.zh-TW.md)、[91 項需求狀態（85 原始＋6 NAT）](docs/requirements-status.json)、[2026-10-12 階段驗收](evidence/goal-91-20261012/verification.json)。交接原件與既有 LICENSE 保持不變。模型品質、生物歸因及未測硬體須依各自證據判斷。
