# Ticket 75：OCR 範例事先拒絕不合法渲染設定

**Epic:** 模型輸入契約稽核
**User Story:** 使用者提供不合法背景或雜訊設定時立即收到錯誤，不得到全數失敗的成功報告
**Blocked by:** 無
**Status:** done

## Root 決策與驗收（2026-10-13）

`FixtureConfig.Validate()` 漏檢查 `Background` 與 `Noise`，但下游 glyphs 的公開範圍為有限 `[0, 0.5]`。`Render` 的比較式也會放行 NaN，`Generate` 在空批次略過選項檢查。依同一範圍在上游設定與下游渲染拒絕，Render／Generate 共用私有檢查，不夾限或更改合法渲染算式。

- [x] 獨立測試重現兩欄的負值、超界值、NaN 與正負無限大未被正確拒絕。
- [x] Validate、RunOCRFixture、Render 與 Generate 拒絕非法設定並回零結果；合法 0 與 0.5 保留。
- [x] 合法輸入與修正前的指定渲染／範例報告一致，完整工程驗證、證據、暫存清理與交付完成。

Root 修改 `tasks/ocr/fixture.go` 與 `tasks/ocr/glyphs/render.go`；測試執行者只新增 `tasks/ocr/fixture_validation_test.go` 與 `tasks/ocr/glyphs/options_validation_test.go`。反相字距的既有語意另行查證，不在本票改動。

減法審查：直接補既有驗證，保留既有函式與模型格式。
