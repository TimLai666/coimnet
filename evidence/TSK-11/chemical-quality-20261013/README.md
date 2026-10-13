# 固定化學設定沒有顯示穩定的 KMNIST 辨識改善

同一個真實單字元任務的三組配對中，開啟化學的主要辨識率平均增加 0.28 個百分點，另一批確認圖片增加 0.22 個百分點。兩組都有一個種子退步。主要測試的分類損失三組都變差，目前沒有足夠證據建議預設開啟這組化學設定。

| 種子 | 關閉辨識率 | 開啟辨識率 | 主要差值 | 確認差值 |
| --- | ---: | ---: | ---: | ---: |
| 20260924 | 73.25% | 73.15% | -0.10 pp | -0.70 pp |
| 20260925 | 71.45% | 72.25% | +0.80 pp | +0.50 pp |
| 20260926 | 71.50% | 71.65% | +0.15 pp | +0.85 pp |
| 三組平均 | 72.07% | 72.35% | +0.28 pp | +0.22 pp |

pp 是百分點，差值為開啟減關閉。主要與確認分割各 2,000 張，每類各 200 張。六組最終辨識率都低於 ticket 64 的 80% 品質門檻，主要分割也低於原四個最近鄰分類器的 81.85%。完整分母、分類損失、各類混淆矩陣、同一張圖片的配對錯誤及逐輪曲線見 [comparison-results.json](comparison-results.json)。

訓練辨識率為 98.40–99.40%，主要測試為 71.45–73.25%。模型對訓練圖片的表現遠高於新圖片，化學開啟沒有消除這個差距。這次沒有獨立隔離樣本數、圖片表示、模型容量或正規化的影響，因此不能把其中某一項當成已確認根因。建議下一輪使用獨立開發資料比較改善新圖片辨識的設定，事前另留最終測試，避免用本輪測試挑參數。

## 比較固定了哪些條件

[protocol.json](protocol.json) 固定 ticket 64 的 KMNIST 官方訓練資料前 6,000 張平衡樣本、20 輪及每組 120,000 次更新。兩組的初始參數、圖片順序、學習率 0.002、損失、23,516 個可訓練參數及最後一輪計分方式相同。保留全部三個種子，沒有依結果挑種子、早停或選最好的一輪。

主要測試沿用 ticket 64 已觀察過的每類第 20–219 張。確認分割每類第 800–999 張的指紋已出現在本輪品質分數產生前的 preflight。兩者互不重疊，開發監看用的每類前 20 張也排除於主要分割。這是固定的後續比較，沒有全新盲測宣稱。Python 從原始 IDX labels 獨立重建索引並核對指紋，見 [data-copy.json](data-copy.json)、[training-data-audit.json](training-data-audit.json) 與 [preflight.json](preflight.json)。

模型沿用人工 CPU 連續核心：52 個節點、1,248 條邊，24 個隱藏節點，整張圖片輸入後再傳遞一步。資料是真實手寫單字元，模型沒有使用真實全腦接線，也沒有驗證多字單字、整頁或自然語言辨識。

## 這次化學機制的範圍

開啟組使用神經活動摘要產生化學釋放，經濃度衰減與受體反應改變隱藏節點的暫時敏感度，完整反向回傳活動來源的梯度。控制器不讀答案、事後回饋或資源。反應係數與五個 controller 向量值固定，沒有新增可訓練參數。每張獨立圖片重設化學狀態，只有兩步訊號歷史。

這些濃度、受體與敏感度係數是明示的工程設定。這次沒有量測荷爾蒙、生物受體密度、細胞內獨立膜電位，或長時間個體狀態的效益。三個種子的結果只適用於本設定與本任務，不推論其他化學設定或所有生理機制都無效。

六次訓練序列執行並交錯 ON／OFF 順序。記錄的逐輪訓練時間 ON／OFF 比為 1.099、1.073、0.828，系統負載不同且結果方向不一致。這些時間是觀察值，不能全部歸因於化學成本。各程序的負載、環境與實際命令保留於 logs 回條。

## 運算與來源驗證

OFF 與公開 OCR SDK 在十張真實圖片上的輸出、損失、最佳化器及連續更新逐位相同。中性化學的輸出、完整梯度與更新同樣一致，active 設定則確實產生有限且不同的輸出、梯度與更新。五個專項與 race 包含取消不提交、設定與快照隔離，以及未知組別拒絕。

六個模型各在新程序載入 TrainingBundle，設定、評分、完整參數、最佳化器與下一次更新指紋都一致。評估前後完整模型指紋不變。三個 OFF 的完整快照、辨識率與混淆矩陣精確重現舊模型。

初版分析把舊評分算式 `log(sum)+max-logit` 與 SDK 算式 `-(logit-max-log(sum))` 要求為相同浮點值，因 0–4 個浮點最小間距的差異失敗。凍結模型的獨立 probe 在同一批 logits 上精確重現兩種算式，分析修正為分別比對各自原值，沒有放寬容許誤差或更動訓練。原分析、來源鎖與失敗紀錄保留，見 [historical-score-rounding.json](historical-score-rounding.json)。

另補上最終來源核對，確認 760 份框架來源、五份 driver、wrapper／分析、編譯程式及六份資料副本。故意修改或移除來源都被拒絕。補強發生於訓練啟動後，早期 runner 只有 protocol／driver 檢查，不能聲稱當時已逐次核對所有來源。沒有確認需要修改化學數值核心的新缺陷。審查與工具限制見 [review.md](review.md)，完整命令與驗收結論由 [verification.json](verification.json) 記錄。

## 保存與重現

完整可重現來源以 `.go.txt`／`.py.txt` 保存在 [sources](sources/)，避免證據程式自動加入框架套件。訓練、評分、失敗與還原的原始 JSONL／stderr 以 gzip 保存在 logs，解壓後 SHA-256 與命令回條相符。模型三檔的指紋在 comparison-results 的 model_files。

本輪六個模型放在已忽略的 `models/TSK-11/chemical-quality-20261013/<seed>-<off|on>`。三個舊模型及官方資料原件保留。本輪資料副本、Go 快取、驗收程式與執行暫存都位於專案 `runs/.tmp/chemical-ocr-20261013`，清理結果見 [cleanup.json](cleanup.json)。逐檔清理盤點保存在專案已忽略的 `runs/audit/chemical-ocr-cleanup-20261013.json.gz`，只將摘要與指紋納入 Git。

重跑時在新的專案內暫存目錄還原五份 driver 的檔名，再用相同來源建置 comparison。將官方六份 KMNIST 檔案放在專案內資料目錄，先核對 data-copy 的 SHA-256。Go 1.26.5、GOMAXPROCS=2，快取、模組及 Go 暫存都指定在新暫存目錄。每個種子依 protocol 的相同設定執行以下入口，模型輸出必須使用新路徑，避免覆寫成果：

```text
comparison --data-dir <verified-KMNIST-dir> --group off --seed 20260924 --model <new-off-dir> --mode train
comparison --data-dir <verified-KMNIST-dir> --group on --seed 20260924 --model <new-on-dir> --mode train
comparison --data-dir <verified-KMNIST-dir> --group off --seed 20260924 --model <new-off-dir> --mode restore
comparison --data-dir <verified-KMNIST-dir> --group on --seed 20260924 --model <new-on-dir> --mode restore
```

其餘兩個種子及交錯執行順序見 run_pairs 原件。正式 driver 的資料、更新數與設定沒有因分析修正改變。原始來源基準為 `44943f5aae4d157cd3a22e2858607066784ee382`。

## 減法審查

移除本輪可再產生的快取與資料副本，保存原件、六個模型與核對必要的證據。沿用現有 OCR SDK 與通用 Trainer，不為一次比較新增公開 API。化學效益尚未穩定，維持可選機制，避免增加預設訓練成本。
