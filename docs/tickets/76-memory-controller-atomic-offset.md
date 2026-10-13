# Ticket 76：記憶控制器失敗不污染後續輸出

**Epic:** 化學對照機制的狀態安全
**User Story:** 使用者可在 Offset 失敗後以合法輸入接續，得到與未發生失敗的個體相同結果
**Blocked by:** 無
**Status:** in progress

## Root 決策與驗收（2026-10-13）

`MemoryController.Offset(step, context)` 在摘要或輸出失敗前已改寫活動視窗，與化學 Controller 的私有候選提交不同。先沿用 Controller.Clone 複製摘要視窗，完整摘要與 MLP 成功後才提交視窗及梯度快取。隱藏層的乘積與加總須有限，不能以 tanh 將無限大掩蓋成合法輸出。正常容量、參數布局、輸出算式與訓練次序保持。

- [x] 公開 Offset／Gradient 的 RED 重現摘要溢位、非法資源及輸出溢位會消耗活動歷史。
- [x] 修正後失敗回零與錯誤，保留上一筆成功梯度；合法接續的輸出、損失與梯度和未呼叫失敗輸入的對照相同。
- [x] 隱藏層有限乘積或加總溢位明確回錯，不以飽和後有限值回成功。
- [ ] 合法多步 Offset、Gradient 與既有手算／有限差分通過，完整工程驗證、證據、清理及交付完成。

Root 修改 `modulation/memory_controller.go`；獨立測試只新增 `modulation/memory_controller_atomic_test.go`。公開參數更新後的快取有效性另行查證，不在本票順手更改。

減法審查：沿用現有摘要複製能力，不新增公開狀態型別或保存格式。
