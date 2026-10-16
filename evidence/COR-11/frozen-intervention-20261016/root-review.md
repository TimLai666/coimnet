# Root 差異與驗收審查

Scope CLEAN。基準 efef47526f655a17ad86e51af81ffd4488f15c5c。Root 親自讀取完整正式差異、340 行新測試、原有介入呼叫、Snapshot 與 shuffle 所有權及兩位執行者的回報，沒有只依分工摘要接受變更。

正式實作僅改 learning/intervene.go 的 Intervene 與相鄰註解。私有 twin 從同一份獨立快照重建，額外複製 chemical.frozen 與 plastic.frozen；正常公開 Restore 預設解凍的契約、模型 schema、參數及依賴保持。原先已有的兩條軌跡改變執行順序，沒有新增公開開關或另一套運算路徑。

shufflePlanConfig 在任何推進前驗證視窗並複製配置，不寫回 subject。Snapshot 複製神經、快速與慢速狀態、化學宣告、濃度、資源、待處理回饋與來源記憶。twin 成功前不安裝 subject network 或 hooks，對照回錯或取消因而不會提交 subject。subject 繼續沿用既有 advanceRows 的整次成功才提交，以及 shuffle／hooks 的 defer 復原。單一個體並行 mutation 仍屬明示不支援的操作。

85 個公開案例與五個父測試共有 90 個終端事件。涵蓋 Continuous／LIF／Mixed、兩種來源、全部合法 freeze 組合、fresh neural source、未生效 plan、active block、可控取消與沒有化學層的可塑性凍結。Root 親跑一般與競態各 90 pass，原有與新增各 119 pass。輸出及 Snapshot 採逐位控制，變化量另用獨立 Hypot 算式，取消不使用壁鐘競速。測試計數對應實際日誌，沒有把父測試另算成行為案例。

有效修正前測試為 26 pass／64 fail。首次 fixture 的非法 LIF 閾值邊界依既有 strict contract 修正，保留閾值中點及所有斷言，原稿與失敗原件均保存。遺漏旗標的隔離版本於一般／競態各 45 fail；保留旗標但恢復舊執行順序的版本各 19 fail，證明兩個獨立原因都有回歸敏感性。實作者未編輯測試。

對抗性審查已執行，結果見 adversarial-review.md。新增專項沒有窮舉 expression、來源控制器、pending feedback、shuffle 與 freeze 的全部交叉組合，不能宣稱覆蓋所有可能呼叫。完整工程結果以 engineering-verification.json 及來源指紋為準，841 份來源的完整一般／競態各 58 套件通過，8,005／8,004 pass、23／24 skip，原有各 7,938 個終端事件保持，只新增各 90 pass。格式、build、vet、固定 Insyra v0.3.4、模組、31 項治理及最後 metadata 的 31 項治理通過。

## 已確認的後續缺陷

[P2] [CONFIRMED] learning/intervene.go:616 的 l2Diff 先平方再開根號。公開合法狀態的真實差異 5e307 被回報 +Inf，JSON 匯出失敗；另一公開控制的 2.5e-201 被回報為 0。此 helper 本票未改，四個 delta 使用者及不可表示結果政策由 ticket128 完整修正。證據 next-audit.md、next-norm-observations.json。COR-11 因此維持 specified，不能以本票完成宣稱沒有 bug。

減法審查：保留既有快照與動態路徑，移除「先提交 subject 才做對照」的失敗順序。不增加 snapshot 欄位、freeze 開關或額外相容層。暫存只清本票私有目錄，原模型、必要資料、原始模組及他人工作由完整指紋保護。

本輪自有暫存已清除，必要輸入與隔離回退來源已封存並重新比對。原模型、必要資料、15,046 個原始模組檔案、32 個他人檔案與兩個既有暫存根均經清理前後核對保持，實際移除量及指紋見 cleanup.json。主要提交與遠端讀回以交付回條為準。
