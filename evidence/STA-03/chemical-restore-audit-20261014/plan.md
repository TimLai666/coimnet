# 化學存檔恢復與拒絕原子性稽核

延續完整「確保沒有任何 bug」目標，前輪的梯度證據、清理及遠端交付屬實際進展。本輪核對持續個體的神經、化學與控制器狀態在保存後是否精確接續。

Root 骨架：公開 NewIndividual／EnableChemistry／Snapshot／RestoreIndividual 與 checkpoint 三種保存入口。以非零活動與兩個獨立控制器建立填滿及未填滿歷史，涵蓋 continuous／LIF／mixed、先跑後啟用、ResetNeural 保留化學、FreezeChemistry。保存前後在新程序讀回後的下一步輸出及完整狀態相同。破壞第二個來源的狀態時應拒絕，來源 snapshot 與原個體保持。來源時鐘與化學更新步數按 ENG.md 各自的契約核對，不強迫不同時鐘相等。

四項本輪里程碑：共用契約與獨立來源審查；具非零控制的公開／新程序接續與拒絕；確認缺陷先建立失敗測試再最小修正及適用工程驗證；證據、暫存清理及授權交付。這不是完整零 bug 目標的分母。

正式程式及既有測試不變，只有重現結果與原契約矛盾時才判定缺陷。暫存只放本輪 runs/.tmp，既有資料及其他暫存保持。涉及相容性、已確認機制或未指定政策的變更，依 AGENTS.md 取得必要決策。

減法審查：先沿用公開恢復入口與現有時鐘規則，不新增儲存格式、遷移工具或生物假設。只有能區分接續錯誤的案例才執行。

## 已確認根因與修正契約

實跑確認三個個體化學必填欄位、四個訓練設定及模型證據清單缺省後仍被接受。合法含化學的 task-training 保存物則因通用 walker 不支援資源 map 無法載入。共同問題是 bundle 沒有沿用原始欄位契約，或選錯通用 walker。

修正位置固定為 checkpoint/bundle.go、checkpoint/individual.go、checkpoint/chemical_episode.go、checkpoint/task_bundle.go。必要的共用 bundle presence helper 可新增 checkpoint/bundle_presence.go。沒有公開 API、欄位、版本或數值公式變更。

LoadBundle 的流程是校驗 manifest 與原文件，驗證原始必填欄位，解碼並恢復 arrays.bin，最後才正規化與建立模型。個體沿用原 payload walker，訓練沿用化學專用 walker，模型沿用 ModelPackage walker。task-training 的訓練部分共用同一條支援化學資源 map 的驗證。資料或欄位錯誤應回傳錯誤及空結果，讀取不更動檔案。

已抽出到 arrays.bin 的一維陣列在原文件是空 []，omitempty 的選用陣列也可能不出現在原文件。驗證只確認既有必填契約，不在原文件階段要求陣列原長度。訓練的合法 nil slices、舊 SourceStates／InputNodes／Delays／Capacity 省略規則保持。個體及模型的 null 規則與單檔入口一致。

Root 已凍結 checkpoint/bundle_presence_test.go，指紋見 tests-frozen-v2.json。涵蓋缺少／null 必填欄位拒絕且檔案不變、合法化學 task 的讀回及下一次訓練、四種 bundle 的 4097 元素二進位陣列及既有省略規則。實作者不得改測試。Root 先跑 RED，再驗證修正、正常舊保存物、完整工程、清理及交付。

減法審查：共用原有檢查，不另造 schema、遷移工具或驗證框架。正常保存物與接續結果保持。

## 合法舊訓練集合的補充契約

真實 OCR 的 ON 訓練存檔由舊 SaveTrainingBundle 寫出 `pending_feedback: null`，舊 LoadTrainingBundle 可載入。候選共用檢查卻把它當作單檔個體的非法 null，這是本輪新引入的相容性回歸。Root 已停止候選的完整 race，不提交這個版本。

TrainingSnapshot 的化學資源 map 與空回饋集合，在 bundle 原文件必須存在，但合法 nil 可寫成 null。僅此二欄位可在 bundle 訓練的驗證副本中視為空集合。原文件及解碼值保持，缺少欄位仍拒絕，非空 map／回饋的元素檢查保持。單檔 episode 及 Individual 的既有 null 規則保持，不能為了修正 bundle 放寬。

正式補修只准修改 checkpoint/chemical_episode.go，兩份 Root 回歸全部凍結，指紋見 tests-frozen-v2.json 與 tests-frozen-nil.json。新增公開回歸已實際 RED：六個合法 nil 組合失敗，非空對照、缺欄位拒絕及單檔 null 拒絕通過。Root 補修後重跑六個實際既有 OCR 模型、舊保存物對照及完整工程，再清理與交付。
