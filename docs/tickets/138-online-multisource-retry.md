# 138 — 同一行動的多來源回饋可獨立重試

Epic：運行中學習與模型品質

User Story：同一行動的兩個來源各自完成梯度階段、局部可塑性卻失敗時，時間倒退後仍能分別接續，不共用錯誤的階段標記，也不誤刪延後回饋。

Blocked by：23、85（已完成）

Status：in_progress

對應需求：LRN-06。補充既有行為的回歸保護，沒有已確認的產品缺陷。

## 範圍與驗收

- [x] Root 確認既有覆蓋缺口，定義同 action、不同 source、時間倒退及交錯成功階段的獨立參考。
- [x] 新增公開 API 測試，交叉 UpdateEvery 1／2 與 AccumulateSteps 1／2；完整 IndividualSnapshot、報告、歷史答案及延後回饋保持。兩種刻意錯誤必須使測試失敗。
- [x] 格式、build、完整一般／race、vet、固定模組及治理通過，既有每種建置各 8,651 個終端事件保持。
- [ ] 保存必要證據，清除本輪暫存，提交推送並核對遠端。

## Root 決策（2026-10-17）

保留部分成功契約與來源成對身份。既有多來源案例只有一個未完成標記，既有時間倒退案例使用兩次不同 Act。這次只新增同 action 同時存在兩個未完成來源的測試，不改正式學習流程。

參考由公開 Individual.TrainEpisode 與 AdvanceGated 按實際成功階段交錯。固定 target=0、score=1，使有限 prediction 的加權目標 p+(0-p) 精確為 0，避免重寫私人加權函式。fixture 沒有受體閘門，顯式全 1 閘門可作局部階段參考。不能把不同合法階段順序與一般逐筆完整更新硬比成相等。

相關低風險文件問題：online_mixed_retry_test.go:121-126 的「current implementation advances eligible again」是修正前狀態，建議下次修改該檔時改為描述原缺陷，不列為目前產品行為缺陷。

原 session61304 已實際 terminal 0，九個工程步驟通過。一般 8,633 pass／23 skip、競態 8,632 pass／24 skip，各 58 套件通過，原有各 8,651 個終端事件保持，只新增各 5 pass。Root 已重新解析兩份壓縮日誌及所有回條，見[驗收](../../evidence/LRN-06/multisource-retry-20261017/verification.json)。固定進度 3／4，最後治理、清理與交付另記。
