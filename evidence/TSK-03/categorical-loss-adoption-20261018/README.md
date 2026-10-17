# 分類誤差的數值穩定性

媒體請求頭與文字生成的分類誤差原本會因共同大偏移、極小尾項及中間加總捨入而失真。受控合法模型的公開 Step／NLL 已重現非零誤差被算成 0。原版、被反例推翻的候選、失敗日誌與獨立 Decimal／Fraction 參考均保留。

正式採用的是 `media-stable-candidate.go.txt` 與 `textgen-aggregate-candidate.go.txt`，版本紀錄見 [adoption-v2.json](adoption-v2.json)。兩處使用補償尾項加總及 Log1p，文字另對跨位置的一般及極小值分別補償加總，極小值在跨組平均或跨位置加總後才轉回原尺度。媒體另在極端上限區間精確重算平均。公開簽名、模型格式、梯度分母與更新算式保持，回報誤差的最後位元可改變。

877 份正式 Go／模組來源記錄於 [formal-source-v2.json](formal-source-v2.json)。十一份凍結回歸與 68 個預期新終端事件記錄於 [tests-frozen-v2.json](tests-frozen-v2.json)。正式一般／競態專項均通過，見 [formal-v2-focused-root-verification.json](formal-v2-focused-root-verification.json)。包含父測試的事件數不能解讀為 68 個獨立使用情境。原 876 份來源及 65 項回歸的 manifests 保持，舊收據不改指向。

14 組梯度與三個受控公開觀察的更新及狀態保持，見 [aggregate-focused-root-verification.json](aggregate-focused-root-verification.json)。四份先前原版人工存檔已用新程序讀取並接續，完整狀態與下一次更新保持，媒體十二次正常更新也保持，見 [aggregate-controls-root-verification.json](aggregate-controls-root-verification.json)。原始四份模型沿用 [既有封存](../../TSK-04/request-head-audit-20261018/original-textgen-checkpoints.tar.gz)。此對照不推論所有模型或其他平台。

九項完整工程驗證已全部通過，一般 8,777 pass／23 skip、競態 8,776 pass／24 skip，各 58 套件通過，原 8,732 個結果保持，只新增 68 pass，見[Root 核對](full-engineering-root-review.json)。最後 31 項治理通過，2,041,297,098 bytes 自有暫存已清除，見[清理](cleanup.json)。修正 `fc026d4` 與主要證據 `4f92071` 已推送且遠端讀回相同，本票 5／5，見[交付](delivery.json)。`packages-candidate-race` 是已被反例推翻的舊候選，已主動停止，其部分結果不列驗收。所有必要證據、原模型、資料、原始模組及其他工作保留。

本次沒有模型任務品質、生物機制有效性或全域零 bug 的完成宣稱。工作範圍與固定五項完成條件見 [ticket146](../../../docs/tickets/146-categorical-loss-stability.md)。

既有分類梯度的另一個缺陷已在 [ticket147](../../../docs/tickets/147-categorical-gradient-stability.md) 以公開更新對照重現，尚未修正。本票工程通過不能作為框架沒有 bug 的證據。
