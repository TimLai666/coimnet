# Test Report — mixed chemical gradient — 2026-10-14

## Summary

完整一般與 race 各 58 套件通過。一般測試 2,111 個頂層通過、race 2,110 個頂層通過，略過案例逐項列在 engineering-summary.json。專項一般／race 各 14 個測試事件通過，格式、build、vet、相依性及最後 11 項治理檢查通過。全部 793 份來源與 457 份凍結測試相符。

## Changes tested

混合 continuous／LIF 的來源活動、兩區域傳輸、顯式受體位置、敏感度、閾值、表現與輸入／參數梯度，以測試私有前向微分核對。隱式初始活動則以獨立混合參考及四組 softplus 手算控制核對，保留顯式常數與截斷規則。

## Bugs found and fixed

[P2] learning/chemical_sequence.go:500、:597 排除了首步來源 Step=-1，導致公開 Core.Initial 漏算。回歸 TestChemicalInitialGradientReference 的五個案例在舊來源失敗、修正後通過，其餘標量輸出及參數／輸入梯度與等值顯式控制逐位相同。沒有為修正改動既有測試。

## Regression tests added

learning/chemical_mixed_reference_test.go 與 learning/chemical_initial_gradient_reference_test.go。四個 overlay 分別切斷活動梯度、忽略受體位置、反轉閾值符號、移除表現種子，均被數值斷言抓到。

## Issues and recommendation

本輪沒有其他已確認且未修復的缺陷。SHIP：本次有界工程修正通過。多訊號、其他來源、MixMax、清除、持續歷史與混合重算的獨立完整對照尚未新增，不能宣稱整體零 bug。沒有重測真實 OCR 或生物機制。
