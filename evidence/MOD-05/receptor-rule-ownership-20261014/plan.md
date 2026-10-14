# 受體宣告所有權與恢復驗證

原始目標「確保沒有任何 bug」保持 active，前輪有已驗證修正、清理與交付，是 progress。ticket 91 的固定五項驗收不代替整體目標。ticket 89 的相容性決策保持 pending。

- [x] Root 凍結 New／Config／Snapshot／Restore 的受體所有權與既有交叉驗證契約。
- [x] 公開失敗回歸先重現，Root 審查凍結後才修正兩個確認根因。
- [x] 三種 CPU 核心、正常／缺省／拒絕、完整工程、合法快照逐位相容及新程序接續通過。
- [x] 同步需求與證據，清除本輪暫存，原有目錄保留。
- [x] 提交、推送及遠端讀回，提交來源與驗證指紋相符。

## Root 骨架

immutable plasticity.Model 擁有 Rule 的兩個 *int。New 先取得指標值的獨立副本再驗證／保存，Config 再給出新副本。GateReceptor 與 DecayEReceptor 的 nil／0／正值及同一輸入指標情形都保留值語義。呼叫期間外部並行改參數仍是既有禁止的資料競爭，不擴張該保證。

RestoreIndividual 保持既有部件錯誤順序，在所有部件成功建立後、回傳個體前沿用 checkReceptorRule。缺少化學層、零受體或索引越界要回錯並回傳 nil；沒有受體參照的舊快照仍可缺省 Chemical。不改合法索引、正常計算、模型格式或增益範圍。

正式修改僅 plasticity/plasticity.go、learning/individual.go 及對應永久回歸。Root 擁有測試、期望、正式修改與驗證。唯讀 worker 只可新增 runs/.tmp/receptor-rule-ownership-20261014/review/report.md，不執行 Go、網路或 Git 寫入，不可改來源或測試。

公開驗收：Model.New／Config 的兩個指標隔離；三種核心在 Enable／Restore／Snapshot 副本改動後，實際局部學習及完整快照與獨立控制相同；Restore 與合法 Enable 的拒絕一致；合法關閉、閘門、時間窗及兩者組合保持逐位相同。

【減法審查】
▸ Won't List：不改未核准的增益合法性，不替 BaseL2 自定新單位，不改生理方程式。
▸ 建議移除：驗證後刪除本輪快取與診斷暫存，保留重現來源與必要日誌。
▸ 建議簡化：使用現有 checkReceptorRule 共用已宣告的邊界，不新增模型版本或替代恢復路徑。
▸ 需要留意：只複製可變的兩個指標值，不複製全部核心參數或另加所有權框架。
