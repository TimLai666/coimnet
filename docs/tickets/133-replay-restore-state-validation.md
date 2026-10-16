# 133 — 使用者可以在恢復重播時拒絕無效狀態

**Epic:** 重播與恢復一致性
**User Story:** 使用者可以恢復合法的重播狀態，無效的指紋與不可能的抽樣計數會明確回錯。
**Blocked by:** 無
**Status:** ready

## Root 決策｜2026-10-16

Root 以公開 Restore 重現兩個 P2 驗證缺口。只保存指紋時，64 個 z 的字串被當成 SHA-256 接受。固定保留、reservoir 容量 2、已見 3 筆且仍保留 2 筆的狀態，即使 Draws=0 仍被接受；正常 Add 在第三筆已需要抽樣。來源及結果見[公開探測](../../evidence/LRN-10/policy-composition-20261016/next-audit-ordinary.json)。

這些案例沒有證明正常 Add 寫出的快照會壞掉。先核對合法狀態、拒絕邏輯與相容性，再修最小必要驗證；不可把 task-balanced 的跨呼叫公平性或重複內容身分等未定義事項混入。

## 驗收

- [ ] 以公開 SDK 固定無效指紋及不可能計數的失敗回歸。
- [ ] 合法 Add／Sample／Restore 接續與兩種保存政策保持對照通過。
- [ ] 對拒絕原狀、大小與有限執行時間做相稱檢查。
- [ ] 完整工程、相容性、暫存清理與交付完成。

Root在保持848份正式來源不變的overlay中編譯候選回歸，一般／競態各9 fail、28 pass。七個非法狀態葉節點均因Restore誤接受而失敗，24組合法政策接續及drop／uppercase控制通過。正式測試與修正尚未採用，見[失敗驗證](../../evidence/LRN-10/feedback-permission-20261016/future-replay-red-verification.json)。

後續Root以四列公開案例確認：seen為0／1、FIFO容量1、FIFO加time_balanced容量2都不可能消耗亂數，Restore卻接受Draws=1。一般／競態及初版候選皆誤接受。初版候選的96項重播測試通過只涵蓋原有案例，尚不完整，不可正式採用。第一項驗收重開，需補齊失敗回歸並從Add／Sample何時可能取亂數推導通用限制。見[追加計數驗證](../../evidence/LRN-10/feedback-permission-20261016/future-replay-counter-verification.json)。
