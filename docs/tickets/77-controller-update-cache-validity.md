# Ticket 77：控制器更新後不混用舊輸出與新參數

**Epic:** 化學控制器梯度契約
**User Story:** 參數更新後，公開反向入口明確要求重新計算輸出，避免產生跨兩組參數的梯度
**Blocked by:** 無
**Status:** in progress

## Root 決策與驗收（2026-10-13）

Controller 與 MemoryController 的 Update 成功後保留舊輸出快取，但 Gradient／BackwardActivity 會讀取新參數。正常 caller 都會先重新 Release／Offset，不受修正影響。沿用 RestoreState 的快取失效規則，只有成功更新清除快取，活動視窗保留，失敗更新則保留參數與快取。Gradient 的說明明示固定參數及重新 forward 條件，不新增參數指紋或保存格式。

- [x] 公開小值重現兩種控制器 Update 後仍接受舊快取的錯誤梯度。
- [x] 成功 Update 後 Gradient／BackwardActivity 回零或 nil 與錯誤，直到新 Release／Offset 成功。
- [x] 失敗 Update 不改參數、活動視窗、上一筆合法輸出或其梯度。
- [x] 重新 forward 的輸出、損失與梯度與相同參數及歷史的獨立對照相同。
- [ ] Controller 的 nil Gradient 回錯而非 panic；完整工程驗證、證據、清理與交付完成。

Root 修改 `modulation/controller_objective.go` 與 `modulation/memory_controller.go`，測試 slot 只新增 `modulation/controller_update_cache_test.go`。直接修改公開 Parameters slice 的有效性由 caller 負責，這一票不新增攔截機制。

減法審查：成功更新直接清除既有快取，不另存第二份參數或引入新抽象。
