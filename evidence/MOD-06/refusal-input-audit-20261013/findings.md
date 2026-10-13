# 已確認的機制與輸入缺陷

基準：`2f7a79c723a8c33dd63675084b942581da91c547`。本階段是 tickets 74–77 的四類修正，前一階段七類保留於 `evidence/MOD-02/mechanism-audit-20261013/`。

| 等級與狀態 | 觸發、根因及影響 | 修正與證據 |
| --- | --- | --- |
| P1 CONFIRMED，已修正 | 記憶控制器先成功，再收到摘要溢位、NaN 資源或讀出溢位。`Offset` 在確認整次成功前已推進活動視窗，後續合法輸出因此不同。有限數乘積／加總溢位也可能被 tanh 隱藏成有限值。會影響繼續執行的對照狀態及訓練。 | `modulation/memory_controller.go:95` 改為私有候選、逐步有限值檢查及成功提交。`red-memory-atomic-distinct-row.json`、`red-memory-nil.json` 與 `green-memory-atomic.json`。 |
| P1 CONFIRMED，已修正 | Controller 與 MemoryController 成功 Update 後，舊輸出仍可交給 Gradient，反向卻使用新參數，產生混合兩組參數的錯誤梯度。既有正常 caller 每次都重新 forward，沒有證據顯示此前的正式對照曾觸發。 | `modulation/controller_objective.go:147`、`modulation/memory_controller.go:212` 成功更新清除輸出快取，失敗保留。Gradient／BackwardActivity 要求新的 Release／Offset。nil Gradient 一併回錯。`red-controller-cache.json` 與 `green-controller-cache.json`。 |
| P2 CONFIRMED，已修正 | OCR 範例未驗證背景與雜訊，非法設定可得到所有 seed 失敗但函式回成功的報告。glyph Render 的負向比較放行 NaN，空 Generate 略過選項檢查。污染輸入或誤讀成功報告的風險，沒有重測真實辨識率。 | `tasks/ocr/fixture.go:51`、`tasks/ocr/glyphs/render.go:90` 共用有限範圍契約，入口拒絕。`red-ocr-public-entrypoints.json`、`red-empty-batch.json`、`green-ocr.json`。 |
| P2 CONFIRMED，已修正 | 曾成功鞏固後，閘門關閉、同回合重複或預算用完的報告把 Used／LastEpisode 回成零，與公開報告說明不符。權重及完整快照本來就未改，沒有已確認的模型品質影響。 | `learning/consolidation.go:131` 在拒絕判斷前填入既有計數，成功回傳更新後計數。`red-consolidation.json` 與 `green-consolidation.json`。 |

合法路徑的指定相容性對照：18 組渲染與 9 組 OCR 小型範例報告逐位相同，見 `compatibility-result.json`。兩種控制器正常 Release／Offset → Gradient → Update 共 60 列逐位相同，見 `controller-compatibility-result.json`。這些結果不包含真實任務品質改善。

生物機制的既有假設保留：受體區域會選取化學濃度，Compartment 保存研究部位而不建立每部位獨立膜電位，工程 Kd／N 不是量測值。見 `docs/model-and-mechanisms.md` 的受體列。四類修正沒有變更此設計或保存格式。
