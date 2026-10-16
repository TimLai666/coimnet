# 133 — 使用者可以在恢復重播時拒絕無效狀態

**Epic:** 重播與恢復一致性
**User Story:** 使用者可以恢復合法的重播狀態，無效的指紋與不可能的抽樣計數會明確回錯。
**Blocked by:** 無
**Status:** in_progress

## Root 決策｜2026-10-16

Root 以公開 Restore 重現兩個 P2 驗證缺口。只保存指紋時，64 個 z 的字串被當成 SHA-256 接受。固定保留、reservoir 容量 2、已見 3 筆且仍保留 2 筆的狀態，即使 Draws=0 仍被接受；正常 Add 在第三筆已需要抽樣。來源及結果見[公開探測](../../evidence/LRN-10/policy-composition-20261016/next-audit-ordinary.json)。

這些案例沒有證明正常 Add 寫出的快照會壞掉。先核對合法狀態、拒絕邏輯與相容性，再修最小必要驗證；不可把 task-balanced 的跨呼叫公平性或重複內容身分等未定義事項混入。

## 驗收

- [x] 以公開 SDK 固定無效指紋及不可能計數的失敗回歸。
- [x] 合法 Add／Sample／Restore 接續與兩種保存政策保持對照通過。
- [x] 對拒絕原狀、大小與有限執行時間做相稱檢查。
- [ ] 完整工程、相容性、暫存清理與交付完成。

Root在保持848份正式來源不變的overlay中編譯候選回歸，一般／競態各9 fail、28 pass。七個非法狀態葉節點均因Restore誤接受而失敗，24組合法政策接續及drop／uppercase控制通過。正式測試與修正尚未採用，見[失敗驗證](../../evidence/LRN-10/feedback-permission-20261016/future-replay-red-verification.json)。

後續Root以四列公開案例確認：seen為0／1、FIFO容量1、FIFO加time_balanced容量2都不可能消耗亂數，Restore卻接受Draws=1。一般／競態及初版候選皆誤接受。初版候選的96項重播測試通過只涵蓋原有案例，尚不完整，不可正式採用。第一項驗收重開，需補齊失敗回歸並從Add／Sample何時可能取亂數推導通用限制。見[追加計數驗證](../../evidence/LRN-10/feedback-permission-20261016/future-replay-counter-verification.json)。

Root已補齊三份獨立回歸，原始正式程式的一般／競態各34 fail、40 pass，全部74個事件都成功編譯。新增覆蓋checkpoint的Validate／Save／Load、無法取亂數的歷史、合法已抽樣鄰近狀態與5秒子程序拒絕上限。另確認所有seen項仍保有時的drop／unique-task計數缺口。48份修改前寫出的重播checkpoint已在另一程序接續，兩種建置各24組與原始控制完全相同。正式修正尚未採用，本票1／4，LRN-07保持specified。見[本輪紅測試](../../evidence/LRN-07/restore-validation-20261016/red-verification.json)與[追加公開探測](../../evidence/LRN-07/restore-validation-20261016/additional-counter-verification.json)。

Root已在851份正式來源上驗證：一般689 pass，競態688 pass、1 skip。原有615個相關事件保持，新增74個事件全數通過。八列公開非法狀態已拒絕，八列既有回饋權限控制保持；48份修改前存檔在新程序接續逐位元組相同且原檔不變。本票3／4，完整工程、清理與交付仍待完成，LRN-07保持specified。見[正式驗證](../../evidence/LRN-07/restore-validation-20261016/diagnostic-verification.json)。

851份來源的完整一般／競態各58套件通過，8,517／8,516 pass、23／24 skip，原有各8,466個終端事件保持，只新增各74 pass。格式、build、vet、固定Insyra v0.3.4、模組及31項治理通過。LRN-07恢復passed，工程追蹤90／91，LRN-10保持specified。本票3／4，待最後檢查、封存、清理及交付。見[完整工程](../../evidence/LRN-07/restore-validation-20261016/engineering-verification.json)。

最後治理檢查在驗收紀錄補齊requirement欄位、將歷史診斷另列來源後，31個事件全數通過，沒有修改測試規則。60份必要輸入已封存逐檔驗證，1,739,953,161 bytes本輪暫存已清除，851份來源、18個模型、6份資料、15,046份原始模組與其他工作保持。本票3／4，待已授權提交／推送與遠端讀回。見[最後檢查](../../evidence/LRN-07/restore-validation-20261016/final-governance.json)及[清理](../../evidence/LRN-07/restore-validation-20261016/cleanup.json)。
