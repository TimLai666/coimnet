# Root 審查與限定驗收

Scope CLEAN。審查基準為 `00bc7016c9380c23ab2154caac10666d798581a5`，四個正式程式檔及一個新回歸檔的範圍與 Root 骨架相符。Root 已讀完整正式差異、新 helper、四個公開載入入口、凍結測試及 worker 報告。

原始公開重現確認兩類缺陷。bundle 解碼先把缺欄位轉成零值，後續正規化又補成合法空值，使化學步數、資源、回饋、訓練選項或證據清單無法辨識遺漏。合法化學 task 的訓練檢查則錯用不支援 map 的通用 walker，使自己的保存物無法載入。兩者都以原始輸入及舊程式實跑確認，見 [confirmed-findings.json](confirmed-findings.json)。

修正沿用既有原始欄位規則，發生在校驗 document 後、解碼與陣列還原前。化學專用檢查保留 resources，只有驗證用副本移除 chemistry，再檢查其餘訓練欄位。沒有更改核心算式、schema、公開 API、依賴或合法舊檔省略規則。

公開入口固定傳入正確型別，模型／個體／訓練的 schema 不符會由公開 loader 拒絕。私有 readBundle 的其他型別只供底層陣列格式測試，不能透過公開入口換入。4097 元素測試實際驗證 manifest 含二進位項目，先接受原文件空陣列，再還原並檢查完整形狀。

Root 在正式修改前凍結回歸，RED-v2 共 22 個失敗與 7 個通過事件，修正後 29 個事件通過。Root 發現原始 RED 的 legacy fixture 錯把已省略的 InputNodes 當成可刪欄位，先換成確實存在欄位的小型舊格式對照，再凍結。拒絕斷言沒有放寬，worker 沒有改測試，見 [test-correction.json](test-correction.json)、[tests-frozen-v2.json](tests-frozen-v2.json)。

48 份舊 Individual 存檔已在修正後的新程序核對完整輸出、化學報告、下一次訓練與狀態位元組。另三種舊保存物的預測及後續更新一致，task 以舊 Trainer 作為參考，因為舊 task loader 正是確認缺陷。八份原始壞存檔在新程式明確拒絕，讀取後原檔指紋保持，見 [old-artifact-compatibility.json](old-artifact-compatibility.json)、[old-scope-compatibility.json](old-scope-compatibility.json)。

對抗性來源審查沒有新確認問題，Root 獨立核對其路由與型別推論。報告原先的 baseline SHA 誤植已退回，以正確完整 SHA 補做有效 diff，修正版見 [adversarial-review.md](adversarial-review.md)。它是來源審查，不取代 Root 的實際測試。

完整一般測試已通過 58 套件，5,731 pass、22 skip、0 fail。第一次完整一般測試只因兩個新測試 schema 尚未登錄而失敗，已補 docs/INDEX.md 的兩個來源列，保留原日誌。正式程式及凍結測試不變。格式、相依性及 build 已通過，race 與剩餘工程仍待完成。

## 補修與目前狀態

上述一般測試是第一版候選結果。Root 用實際既有 OCR 訓練存檔確認第一版誤拒合法 `pending_feedback:null`，因此停止該版 race，沒有提交。原始 writer 與 copyChemicalEpisode 保留合法 nil 集合，單檔 episode 則使用自己的非 null 格式，不能把個體快照規則套到所有訓練 bundle。

Root 新增凍結的公開回歸，六個 nil 集合組合失敗，非空對照、缺欄位及單檔 null 拒絕通過。補修只有 chemical_episode.go 增加明示的 bundle／standalone 旗標，驗證副本僅將已存在的 null resources 與 pending_feedback 視為空集合。缺少鍵仍拒絕，正式解碼保留 nil，Individual 規則沒有改動。修正後原 29 加新 17 共 46 個事件通過，見 [nil-collections-red.json](nil-collections-red.json)、[nil-collections-green.json](nil-collections-green.json)。

六個既有 KMNIST ON／OFF 模型在舊／新程序的保存狀態、SDK 推論及下一次 MSE 更新位元組相同，18 個模型原檔指紋保持，見 [real-example-model-compatibility.json](real-example-model-compatibility.json)。這是 SDK 相容性，沒有重新量測 OCR 辨識率。Root 的第一次實際模型探針錯給每列 target，已依 Trainer.Step 的 final-row 契約修正探針，不更改正式算式，見 [real-model-diagnostic-correction.json](real-model-diagnostic-correction.json)。

補修版本固定為 [source-fixed-v2.json](source-fixed-v2.json) 的 809 份來源，兩個 Root 回歸指紋保持。完整工程已從格式、build、一般與 race 重跑，最後驗收、清理及交付尚待完成。

## 最後來源

最後來源檢查確認 checkEpisodeChemistry wrapper 已經沒有呼叫端，Root 移除四行殘留，沒有修改檢查規則或數值算式，見 [unused-wrapper-removal.json](unused-wrapper-removal.json)。最後來源固定於 [source-final.json](source-final.json)，809 份指紋及兩個凍結回歸保持。

46 個公開回歸、48 份舊個體、三種舊保存物及八份原始壞存檔已再次實跑。六個既有手寫模型的 SDK 相容性也已再次核對，見 [old-artifact-compatibility-final.json](old-artifact-compatibility-final.json)、[old-scope-compatibility-final.json](old-scope-compatibility-final.json)、[real-example-model-compatibility-final.json](real-example-model-compatibility-final.json)。格式、build、相依性及完整一般 58 套件通過，完整 race 與其餘工程尚待完成。先前取消的紀錄保留，不算完整驗收。

另有 14 個公開負面案例實際核對 resources／feedback 的 null 元素、錯誤集合型別，以及回饋時間與化學時鐘的 null。direct training 與 task 入口全部拒絕、回傳空結果且原檔保持，見 [nonempty-collection-results-final.json](nonempty-collection-results-final.json)。最初探針漏建父資料夾，以及紀錄工具把 JSON 陣列誤當測試事件的問題都已修正並保留原始失敗，不算框架缺陷或通過事件。最後命令退出碼為 0，沒有更動正式來源或凍結測試。

【減法審查】

- Won't List：不增加保存版本、遷移流程、生物假設或任務專用核心。
- 建議移除：沒有因本次修正而失去用途的正式程式。
- 建議簡化：四種 bundle 共用既有原始欄位檢查，化學訓練共用專用檢查。
- 需要留意：完整零 bug、真實任務品質提升與生物定量正確性都尚未證明。
