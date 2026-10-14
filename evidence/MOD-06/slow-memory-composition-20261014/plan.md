# 快速學習與慢速記憶組合稽核

延續全框架「確保沒有任何 bug」目標。ticket 89 的相容性決策保持 pending，本階段只處理可獨立驗證的記憶組合。

- [x] 核對固定符號、快速／慢速層、化學觸發的原始契約，Root 與獨立稽核區分確認缺陷與疑點。
- [x] Root 建立公開入口重現與有非零控制的獨立參考，凍結回歸後才修正確認根因。
- [x] 適用專項、完整工程與既有合法模型相容性驗證通過，保留來源與測試指紋。
- [x] 同步需求、進度與證據，清除本輪暫存及核對原件保留。
- [x] 已驗證階段依既有授權提交、推送及遠端讀回。

## Root 骨架與責任

Root 保管公用測試、手算期望與正式修改。獨立稽核只能寫 runs/.tmp/slow-memory-composition-20261014/review/report.md，不得改來源、測試或其他文件，不得執行 Go、網路或 Git 寫入。

核對 Consolidate 候選 slow、保留 plastic、對每條啟用邊計算有效權重的共同規則，以及成功／失敗報告和快照。若每個層各自有限但共同有效權重溢位，先用公開 Create／Restore／Advance／Consolidate 路徑判定，不能因猜測就改拒絕條件。正常小值、固定興奮／抑制與 free edge 必須有對照。已有的釋放總量防溢位見 ticket 67，不重做既有修正。

正式修正範圍為 learning/consolidation.go、plasticity/plasticity.go 的無配置 metadata 讀取，以及對應回歸。已重現個體提交前可推進、提交後才因有效權重溢位回錯。不更換慢速記憶、生理方程式、模型格式或觸發順序。

【減法審查】
▸ Won't List：不重新修復已完成的釋放總量 guard，不更改 ticket 89 的待決策合法性。
▸ 建議移除：完成驗證及保存必要證據後刪除本輪快取與診斷暫存。
▸ 建議簡化：沿用現有候選及共同有效權重規則，不增加模型版本或補償層。
▸ 需要留意：新生理規則或重要相容性變更須先決策，不把有限值拒絕誤當成新機制。

## Root 修正契約

Consolidate 保留原本兩次啟用邊掃描及既有錯誤順序。在第一掃描內檢查候選有效權重，固定符號的 raw rho 先轉 exp(rho)，再按 Model.EffectiveWith 的幅度、slow、plastic 與 w_min 規則計算。固定符號下被 w_min 擋住的負向溢位保持可計算，正向不可表示的有效值拒絕。原有候選 slow、三層 norm 與 context 失敗優先序保持。公開 Model.Edge(position) 回傳選取位置的核心邊索引及 w_min，不配置或暴露邊陣列。此為新增唯讀 metadata 方法，不變更現有保存格式或正常機制。

## 後續隔離問題

Root 公開診斷已確認 Rule 的 GateReceptor、DecayEReceptor 在輸入／Config／Snapshot 邊界共用指標。四個個體案例在修改前可 Advance，修改外部副本為 -1 後下一次 Advance 越界 panic。此 P2 留作獨立下一票，ticket 90 的來源與五項分母保持。BaseL2 的單位仍是待釐清，沒有擅自換報告公式。
