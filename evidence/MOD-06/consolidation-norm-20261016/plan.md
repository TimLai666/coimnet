# 記憶寫入報告的最高端表示能力

Root 決策｜2026-10-16。前一輪 ticket128 已完成實作、完整驗收、清理及遠端讀回。本輪接續已公開重現的三個記憶報告位置，MOD-06 保持 specified。上一輪為 progress，並非等待或無進展。

Consolidate 的 BaseL2 helper 與 SlowL2／PlasticL2 逐項 Hypot 在最高端會把真正不可表示的 norm 回報 MaxFloat64，然後寫入慢速／快速狀態並消耗預算。三種公開重現、有限最高值與明顯溢位控制均已存在，先對目前來源重播，再凍結獨立回歸。

正式實作預定只改 learning/consolidation.go 的三種報告範數及必要私有 helper。沿用鎖、gate／already／budget／取消及非有限有效權重的既有錯誤順序、兩種記憶公式、保存格式與參數合法域。一般大小的逐項 Hypot 位元保持。最高端若精確 norm 可表示，應接受並給有限報告；真正不可表示則回同一 report-norm error，完整 Snapshot、化學報告與 Used／LastEpisode 保持。保持正常兩次掃描，不為所有邊新增候選陣列或高精度逐項運算；最高端如需額外精確計算，僅在那條少見路徑執行。

Root 擁有預期與所有驗收。測試格子只可寫 learning/consolidation_norm_test.go，使用公開 SDK 建構／Restore／Snapshot／Consolidate，獨立 8192-bit 平方和與平方根作最高端 oracle。主要向量為 [MaxFloat64, MaxFloat64*2^-27]、[MaxFloat64, MaxFloat64*2^-26]、[1.3e308,1.3e308]，三種層、正負／排列控制。一般位元控制包含零、3/4、負號、最小次正常值與有限大數，與原索引順序 Hypot 比較。拒絕控制核對完整狀態、報告與預算，並包含 gate／budget／already／nil／取消的前置錯誤優先序。實作者不可修改測試。

驗收順序：有效 RED、凍結測試與期望、最小實作、一般／競態專項與既有鞏固回歸、公開三層 probe、至少兩種可成功編譯但錯誤的隔離對照、最終來源凍結、完整格式／build／unit／race／vet／依賴／治理。完整結果須直接對照 ticket128 的 8,127 個終端事件，不改舊結果。通過工程才更新 MOD-06；保存證據、清除唯一自有暫存、提交推送與遠端讀回後才勾 4／4。

減法審查：不重建模型、不增加公開設定／保存欄位，不改未定義的 raw-rho／有效幅度單位。只修可表示性判斷，不把數值修正當成真實辨識品質或生物校準成果。暫存限 runs/.tmp/consolidation-norm-20261016，既有 AGENTS.md、兩個其他 evidence 目錄、18 份模型、六份資料與兩個舊暫存根均保護。
