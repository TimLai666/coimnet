# 唯讀數值契約審查

原生 gpt-5.6-luna/max 以已存在的 codebase-memory 索引定位，並核對目前本機來源。learning/intervene.go:616-622 的差異平方先上／下溢，四個使用點為 :181、:189-191。simulate/plasticity.go:261-324 提供正常位元保持、tinySquares 影響判斷、scaled compensated fallback 與最高端精確捨入界線的既有原則，Root 已完整讀取該實作及 boundary tests。

真正的最近偶數溢位界線為 2^1024 - 2^970。math.Hypot 單獨不足以判定最高端，平方和直接改成精確或補償算法則可能改變正常舊位元。Root 的正常控制包含 [1]+1024*[2^-27]，極端判準由獨立 8192-bit reference 計算。

執行者建議 helper 改成 error 簽名與增加 shape 防禦。Root 採用更小的內部變更：保留既有 float64 簽名和已驗證等形有限資料前提，四個使用者統一驗證最終值；沒有公開 shape 反例，不新增防禦式拒絕。真正不可表示的報告仍必須回錯並保持原狀。

執行者未改檔、未跑 Go。標量參考及 Root 的有效 RED 不等於最終正式修正通過。
