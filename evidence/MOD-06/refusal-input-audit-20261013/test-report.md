# Test Report：控制器與輸入契約，2026-10-13

## Summary

最終一般與 race 共 10,011 個通過測試事件，失敗 0。這是兩輪的事件數，重複執行的案例不計為新的獨立測試。四類缺陷由 16 個新增頂層回歸測試覆蓋，修正前的 RED 失敗另行保留。

## Changes tested

- 鞏固的三種拒絕保留目前計數及完整快照，成功回傳提交後計數。
- OCR 設定、渲染及空批次拒絕負值、超界、NaN 和無限大，合法端點保留。
- 記憶控制器的失敗保留活動視窗及梯度快取，合法接續與未發生失敗的對照相同。隱藏層溢位明確回錯。
- 控制器成功更新後拒絕舊快取，重新計算輸出後可取得梯度。失敗更新保留參數、視窗和快取。
- 18 組渲染、9 組 OCR 小型報告及 60 列控制器正常多步更新，與基準逐位相同。
- 格式、build、完整一般／race 各 58 套件、vet、相依性及最後 11 項治理通過。

## Bugs found & fixed

四類根因、影響與程式位置見 [findings.md](findings.md)。舊快取問題沒有既有正式訓練曾觸發的證據，鞏固報告問題沒有已確認的品質影響。

## Issues found, not yet fixed

執行途中改動控制器公開宣告時，歷史狀態應保留、重設或拒絕尚未指定。這是待釐清的契約，不列為已確認缺陷或本階段阻礙，見 [root-review.md](root-review.md)。

## Regression tests added

- `learning/consolidation_report_test.go`：拒絕計數與完整快照。
- `modulation/memory_controller_atomic_test.go`：失敗污染、nil 與隱藏層溢位。
- `modulation/controller_update_cache_test.go`：成功／失敗更新、重新計算輸出與 nil 梯度。
- `tasks/ocr/fixture_validation_test.go`：範例設定拒絕與端點。
- `tasks/ocr/glyphs/options_validation_test.go`：渲染、一般批次及空批次的選項。

## Recommendation

SHIP。本階段修復與工程驗證通過。真實任務辨識率沒有重新量測，生物定量正確性和整個框架零 bug 也沒有由這些測試證明。

實際命令、環境、來源指紋與日誌見 [verification.json](verification.json)，全部跳過項目與新增測試名稱見 [test-summary.json](test-summary.json)。
