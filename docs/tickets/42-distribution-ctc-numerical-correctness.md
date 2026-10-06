# 42 — 研究者可以在大分數下取得正確的蒸餾與文字序列損失

Epic：教師蒸餾與序列訓練的數值正確性。

User Story：研究者使用既有公開入口，在有限、可表示的計算範圍取得正確損失與梯度，非法數值回錯誤。

Blocked by：無。ticket 27／29 的公開入口已存在，ticket 41 已重現同類缺陷。

Status：in_progress，4／5。

對應需求：TCH-04、TSK-01、TSK-02。只補充軟體正確性證據，不改模型、任務、依賴或既有需求狀態。

## Root 修正骨架與共用契約

根因是先把最大分數加回 log-sum-exp 再相減，浮點捨入會丟失正規化量。蒸餾的 KL 與 CE、CTC 各影格都須在位移後計算 log 機率。CTC 的動態規劃 log-sum-exp 是路徑合併，不可一併刪除。

公開簽名保持：`DistributionDistiller.Loss(TeacherDistribution, []float64, int) (float64, []float64, DistributionReport, error)`、`ctc.Loss([][]float64, []int) (float64, [][]float64, error)`、`ctc.LossWith([][]float64, []int, Options) (float64, [][]float64, Report, error)`。

- 相等分數 0、±1e16、±1e308：均勻老師的 KL 為 0；混合 CE 為 (1-Mix)×ln(2)，梯度按既有溫度、縮放計算。單影格標籤 1 的 CTC 為 ln(2)，梯度 [0.5,-0.5]。
- 可表示的非均勻分數共同位移後，損失與梯度不變；溫度先作用於位移後差值，避免相等的大分數除溫度時溢位。
- 分布蒸餾保留 declared_map、partial top-k 不重新正規化、零機率、Mix、Scale 及報告。關閉的 KL 不產生 0×Inf。非有限學生分數拒絕。
- CTC 保留 blank=0、連續重複標籤、空目標、ErrNoValidPath 與顯式 ZeroOnImpossible。以獨立枚舉路徑及條件佔用機率驗證多影格的損失與梯度。
- 每個 CTC 影格的條件路徑機率總和須在 1e-8 內接近 1，否則視為數值精度失敗並回錯誤。`[[0,-1e16],[0,-1e16]]`、target=[1] 原本回 [[0,-1],[0,-1]]；這不是無合法路徑，ZeroOnImpossible 不得吞掉數值錯誤。Root 先新增 `posterior_numerical_test.go` 驗證拒絕。
- 不可表示的正規化差值、損失或梯度回錯誤，錯誤不回部分 loss／grad，輸入逐位保留。有限差值與係數的既有 float64 運算上限保持，不保證所有有限輸入組合都有可表示結果。

## 流程、失敗路徑與責任

分布／文字標籤 → 既有公開 Loss → 位移正規化 → 梯度回傳 → 既有蒸餾、OCR、ASR 訓練流程。純計算不寫模型或快照，不需要遷移。

| 操作與例外 | 回傳結果 | 驗證責任 |
| --- | --- | --- |
| 相等分數、共同位移、溫度 | 手算或獨立參考一致 | 本票新增公開入口測試 |
| nil／空輸入、非法對齊、非法標籤 | 沿用公開錯誤 | 既有及新增測試 |
| NaN／Inf、不可表示結果 | 回錯誤且 loss／grad 清空 | 本票新增極值測試 |
| 空／重複目標、無合法路徑 | 保留 blank 與顯式策略 | 新增枚舉及既有測試 |
| 重複呼叫、輸入保留、並行使用 | 無共享可變狀態 | 輸入逐位檢查與 race |

Root 固定契約、審查測試、跑 red／green、完成全套驗證及交付。測試 agent 只可新增 `distill/distribution_numerical_test.go` 與 `tasks/ocr/ctc/numerical_test.go`。不同實作 agent 分別只可改 `distill/distribution.go` 或 `tasks/ocr/ctc/ctc.go`，不得修改測試檔，認為測試錯就停下回報。Root 擁有追蹤文件、證據與獨立審查修正。

## 驗收

- [x] 公開入口新增失敗測試在原始版本重現，保存 red 日誌與來源指紋。
- [x] 手算、共同位移、溫度與混合、partial、CTC 枚舉及非法值回歸全部通過。
- [x] 既有相關流程與完整 gofmt、build、test、race、vet、相依性檢查通過；race 使用 60m timeout。
- [x] Root 與獨立審查完成，公開 API、依賴、歷史證據及交接原件保留。
- [ ] 保存命令、環境、來源指紋、結果與日誌，更新追蹤紀錄，提交、推送並核對遠端。

減法審查：移除蒸餾內重複的機率正規化，直接修正既有計算；不引入跨套件公開抽象或訓練選項。

## 驗證證據

[數值證據](../../evidence/TCH-04/distribution-ctc-numerics-20261006/verification.json) 記錄環境、命令、635 個 Go 檔案的來源指紋與日誌。原始十項頂層測試有九項失敗、一項策略對照已通過，修正後十項通過。相關流程 124 項頂層測試、完整一般與 race 各 57 套件、建置、vet、模組及原件完整性檢查通過。Root 與獨立審查通過，唯一數值策略疑點確認為原始行為。提交與推送核對尚待完成。
