# 唯讀狀態所有權審查

原生 gpt-5.6-luna/max 核對目前本機來源後確認，advanceRows 在 learning/individual.go:614-622 只替換 neural、plastic.state、chemical.state/report/sources。核心 state、Kinetics 與 StepWith 均產生獨立候選，chemical.cloneSources 對 controller 的視窗與 release cache 做深拷貝。Root 另讀取實際 advanceStepwise 與提交區塊核對。

因此錯誤還原可保留原欄位引用。slow、resources、pending、expression、參數、optimizer、episodes 與 freeze flags 在這個 advance 只讀。現有 shuffle defer 保留 network pointer，成功標記必須延到報告檢查後；hooks 仍由既有 defer 清除。

Snapshot 不包含最近 ChemistryReport 或 controller release cache，RestoreIndividual 也會預設解凍，不能用重建整個 Individual 取代失敗前 runtime。新的內部控制另確認來源物件與公開 Gradient 快取未變；此快取是否屬可保存 snapshot 契約並未假設。

執行者唯讀，未修改、未執行 Go 或 Git。Root 的實際測試與指紋才是驗收依據。
