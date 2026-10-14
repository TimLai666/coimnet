# Root 機制稽核與修正審查

目前確認慢速記憶、受體指標隔離及恢復時的受體綁定三個機制缺口。ticket 90 的慢速記憶候選已修正，受體編號的指標共用另列後續。正式化學增益相容性決策由 ticket 89 管理，沒有在本票更動。

## 已確認缺陷

| 分類 | 觸發與影響 | 判定與範圍 |
| --- | --- | --- |
| P2，已修正待完整驗收 | 原先可推進的個體，其 base／slow／plastic 各自有限。Consolidate 成功後共同有效權重溢位，下次 Advance 回錯 | 修正前公開六案例的三個 Retain=1 組觸發。極端數值為 1e308，沒有證明一般任務辨識率受影響 |
| P2，未修正 | New 輸入、Config 與 Snapshot 流出的 GateReceptor／DecayEReceptor 指標共用。外部變更會改動原模型，負編號造成 Advance 越界 panic | 四個 Model 宣告案例及四個個體案例實測。保存來源／日誌，後續獨立補輸入／輸出隔離，未混入本票 |

第一個缺陷的最早差異是候選驗證只查各層及各層範數，沒有查實際共同有效權重。修正位於 [Consolidate](/Users/timlai/Developer/coimnet/learning/consolidation.go:154)，沿用 [EffectiveWith](/Users/timlai/Developer/coimnet/plasticity/plasticity.go:563) 的相加順序及固定符號下限。

第二個缺陷的來源是 [New](/Users/timlai/Developer/coimnet/plasticity/plasticity.go:143) 與 [Config](/Users/timlai/Developer/coimnet/plasticity/plasticity.go:232) 淺拷貝 Rule。持續個體在 [受體索引](/Users/timlai/Developer/coimnet/learning/individual.go:774) 直接使用共用編號。個體診斷捕捉原本的 panic，測試 exit 0 只表示診斷完成，不能當成修正通過。

恢復還有獨立的 P2 驗證缺口。[RestoreIndividual](/Users/timlai/Developer/coimnet/learning/individual.go:244) 在所有部件恢復後沒有沿用 [checkReceptorRule](/Users/timlai/Developer/coimnet/learning/individual.go:332) 的交叉檢查。公開四個案例確認：兩個正編號越界快照被接受後 Advance panic，另兩個缺少 Chemical 的快照被接受且宣告的受體作用停用。這和所有權隔離一起由下一票處理，原始診斷見 ownership-restore.json。

## 原始設計與實際機制

| 契約 | 審查與驗證 |
| --- | --- |
| slow += Rate*plastic，plastic *= Retain | 原公式及回合／預算更新保持。54 個合法組合的報告、輸出、快照在舊來源、新來源及新程序逐位一致 |
| 固定興奮／抑制保留符號 | 原始 rho 經 exp 轉成幅度後檢查共同有效值。六個負向幅度溢位被 w_min 限制的控制保持可推進 |
| 可塑性選取順序對應實際邊 | 稀疏選取啟用 core edge 2，局部位置是 0。四組回歸在舊來源失敗、新來源通過 |
| 整次成功才提交 | 12 個三種核心／四種符號案例確認拒絕後完整快照、預算及後續推進保持 |
| 不為預驗證複製全圖邊陣列 | 原本兩次掃描保持，新增 Model.Edge 回傳純量索引／下限，不暴露選取陣列 |
| 既有候選、範數與取消優先序 | 候選 slow 及範數錯誤保持先回，取消先於新增有效值錯誤。既有拒絕與時鐘專項通過，精確瞬間的同時取消由來源順序核對 |

## 回歸與相容性證據

- 原始主要 RED：12 個拒絕案例及父項失敗，54 個獨立正常參考與六個下限控制通過。修正後 85 個相關事件通過。
- 稀疏 RED：四個案例及父項失敗。修正後五個事件通過。
- 原始公開探針六個初始狀態皆可推進，三個保留快速層的寫入成功後才變得不可推進。修正後三組在提交前回錯、狀態不變，全部六組可推進。
- 54 個合法組合在三個獨立程序的序列化 SHA256 相同，不以單純容差通過代替逐位相容。
- 根程式、模組及新測試來源共有 796 份，凍結測試 460 份。完整一般／race 各 58 套件通過，一般 5,281 個 pass／22 個 skip 事件，race 5,280 個 pass／23 個 skip 事件。race 多一個既有大型模型包測試因記憶體負擔跳過，一般檢查已實測通過。格式、build、vet、相依性、目標資料流及 CLI 新程序接續相同均通過。最後 11 個頂層治理／31 個事件通過。本輪 9,203 個檔案、24,465 個快取連結與 1,331,266,511 bytes 暫存已清除，兩個原有目錄保留。修正 ccab7e0、回歸／證據 10ca882 已推送且遠端相同，796 份提交來源相符。

## 獨立稽核的 Root 處理

獨立 agent 的第一版 .75*M + .5*M 範例在鞏固前已不可計算，Root 沒有採用它作「鞏固破壞合法狀態」的證據。修正版改為公開可用的 1e308／-1e308／1e308，Root 再實際重現。agent 的 P1 源碼分級未採用，本票數值極端觸發按 P2 處理。三份原始報告保留完整位元組及 SHA256，見 independent-review-receipts.json。修正後唯讀審查沒有確認本票新回歸，未宣稱 agent 跑過 Go。

OpenCode models 本輪實際執行但在開啟自身 log 時失敗，沒有可用模型清單或模型呼叫。依專案規則使用原生 gpt-5.6-luna、max 做唯讀稽核，Root 保管與執行測試、正式修改及交付。

## 疑點與限制

BaseL2 在固定符號下使用 raw rho，slow／plastic 使用有效幅度。原契約沒有明定 BaseL2 的單位，本票保持既有報告，不把未知單位定義判為生理機制 bug。受體位置、濃度、荷爾蒙與神經動態的生物校準，及真實任務準確率改善，沒有由這些人工核心極端值驗證。

初次完整測試六個套件無法建立 httptest localhost listener，原因為沙盒禁止 bind，沒有核心斷言失敗。已以相同凍結來源在允許本機測試 listener 的環境重跑，沒有放寬既有測試。格式檢查第一次包含歷史未格式化的重現 artifact，已按 scripts/verify.sh 的 evidence 排除規則修正；未修改該歷史來源。模組驗證缺少舊的 transitive go.mod metadata，改從既有唯讀 file proxy 補到本輪專案快取，不連網、不改相依性。原始失敗與修正 receipt 一併保留。

## 減法審查

移除本輪快取、虛擬 overlay、執行二進位與診斷工作目錄，保留可重現來源、命令、指紋、日誌及結果。採共同有效權重的提交前拒絕，不另加模型版本、回滾機制或補償層。受體指標隔離及 BaseL2 定義獨立處理。
