# 94 — 研究者可以讀取合法連續輸出的有限平均值

**Epic:** 機制與模型品質稽核
**User Story:** 每個核心輸出有限且平均值可表示時，原生探針不會因中間加總溢位而拒絕整次執行。
**Blocked by:** 無，不依賴 ticket 89 增益決策
**Status:** done

- [x] Root 固定有限平均、真正不可表示總和、正常舊位元與失敗不提交的契約。
- [x] 凍結公開失敗回歸，以獨立手算及同偏差單節點對照確認，不以私有函式假想輸入代替。
- [x] 最小共用修正，連續／LIF、空集合與正常舊模型／新程序接續比較，完整工程驗證。
- [x] 同步 NAT-01 與證據，清除本輪暫存並保留原件。
- [x] 提交、推送與遠端讀回，來源與驗證指紋相符。

Root 已用公開 Build → RestoreState → Run 確認：合法 softplus、DT=1、gain=0、bias=MaxFloat64、log_tau=0、三節點零輸入，各節點輸出為 1.1363587890114286e308，真實平均也是同一有限值，mean_output 卻先加總成非有限值並拒絕。失敗保留神經狀態與輸入。同偏差單節點、較小偏差三節點及 tanh 三節點皆成功。這是已確認 P2，診斷預期 bug，不是修正通過，見[公開證據](../../evidence/NAT-01/native-mean-audit-20261014.json)。初始診斷時 NAT-01 標 failed，沒有真實任務品質影響結論。

Root 骨架：保留可靠正常範圍的既有索引順序；只修正實際中間溢位且平均值有限的情況。真正不可表示的 sum_output 仍拒絕，空集合沿用 allow_empty，數值錯誤不提交。不得縮小合法輸出範圍、改核心或模型格式、移除有限值檢查或排除報告欄位來消除失敗。具名時間視窗的 MetricMeanOutput 另含原始 numerator 報告，若追蹤確認同類缺陷，先核對其獨立輸出契約，不能直接更動 numerator 語意。

同類公開診斷確認：一節點探針成功時，具名集合的原始 OutputSum 仍可能溢位，Run 卻成功提交並使 Measurements 無法 JSON 編碼。修正保留 raw numerator 的語意，在提交神經與快速狀態前拒絕。永久回歸在原來源有 8 個失敗事件，修正後 13 個事件通過。16 組正常配置的全部 JSON 欄位相同，舊狀態在新程序序列化恢復後接續也相同；完整工程驗證已通過。見[回歸](../../evidence/NAT-01/native-mean-20261014/green.json)、[正常相容](../../evidence/NAT-01/native-mean-20261014/compatibility-comparison.json)及[新程序接續](../../evidence/NAT-01/native-mean-20261014/serialized-resume.json)。

完整一般／race 各 58 套件、format、build、vet、相依性及 CLI 80+40 新程序接續與 120 步連續結果通過。另以合法 LIF 大突觸跡恢復狀態補驗：原來源 3 個失敗事件，修正後一般與 race 各 3 個通過，真正量測總和溢位回錯且不提交。NAT-01 恢復 passed，最後 11 項治理與本輪 1,211,375,287 bytes 暫存清理已完成，原有目錄保留。修正 37cca36 與回歸／證據 ea5d0ec 已推送並核對遠端，802 份提交來源相符，見[交付](../../evidence/NAT-01/native-mean-20261014/delivery.json)。見[工程驗收](../../evidence/NAT-01/native-mean-20261014/verification.json)。
