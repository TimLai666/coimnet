# CTC 極小誤差與梯度查核

公開的一與兩個時間點案例，在一般／競態均把仍可表示的正誤差及部分梯度回傳為零。四個問題案例與兩個正常控制的完整輸出，在兩種建置內相同，見 [Root 核對](root-review.json)、[單一時間點參考](ctc-single-frame-reference.json)與[兩個時間點參考](ctc-two-frame-reference.json)。這是數值正確性缺口，沒有模型辨識率下降的量測證據。

來源固定為 914 份，與 [148／149](../../../docs/tickets/148-reported-classification-loss.md) 的驗收相同，但沒有更改正式程式。CTC 由 OCR 多字與語音訓練共用，蒸餾沒有直接呼叫這個計算。追蹤與後續驗收由 [ticket150](../../../docs/tickets/150-ctc-tail-representability.md) 管理。

`1e-8` 只檢查各時間點的條件機率總和。既有有限差分測試跳過兩側都小於 `1e-8` 的梯度，不能證明本次近零值的正確性。目前沒有 CTC 專用的輸出誤差門檻，正式回歸與完整修正尚未完成。

回條與日誌由原生產目錄原樣搬移，來源表與環境表原樣複製，指紋見 [artifact-index.json](artifact-index.json)。兩份 `.go.txt` 是公開入口的數值探測程式，不是模型產品。原探測暫存已隨主驗收封存並清除，見[清理核對](../../TSK-01/reported-loss-candidate-20261018/cleanup-root-verification-joint.json)。

OpenCode 免費模型 `opencode/big-pickle --agent plan` 回報 `Error starting FSEvents stream`，60 秒內未完成回覆。唯讀契約查核由 Codex 內建的 `gpt-5.6-luna / max` 執行，Root 核對來源與實際輸出。

## 後續回歸的獨立參考

已用完整小路徑枚舉建立 38 個參考案例，涵蓋一至四個時間點、空目標、重複與不同標籤、類別排列、共同位移及半最小值兩側，最多 81 條路徑。900／1400 位十進位精度的最終 float64、未捨入正負號欄位均相同，且與原一／兩時間點參考相符，見 [Root 核對](oracle-root-verification.json)、[產生器](ctc-small-path-oracle.py.txt)及[參考值](ctc-small-path-reference-v2.json)。

這些是後續回歸的參考輸入，尚未固定正式精度門檻，也沒有執行新增 Go 回歸或修正模型。非法輸入、既有數值拒絕、不可對齊政策與公開訓練入口仍須補齊。兩種精度結果一致不構成任意輸入或所有末位皆正確的證明。

初版產生器的最佳化模式拒絕及重現路徑已補正，原檔與原參考保留。新版已從專案的 docs 子目錄成功執行，python -O 在產生輸出前明確拒絕，見[產生回條](ctc-oracle-v2-generation.json)。OpenCode 本次仍回報 FSEvents 錯誤，55 秒沒有完成回覆，數學唯讀核對使用 gpt-5.6-luna／max。
