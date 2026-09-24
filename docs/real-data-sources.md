# 真實任務範例的資料來源

CoImNet 的儲存庫交付框架；以下資料只用於 `examples/` 的小型驗收。原始資料、轉檔結果與訓練模型不納入 Git。每個範例的本機 manifest／metadata、原檔 SHA-256、訓練與保留分割、命令、結果及限制，見對應的證據檔。授權說明只按來源頁的明示範圍判讀。

| 範例 | 官方來源與權利範圍 | 本機原檔與驗證 |
| --- | --- | --- |
| 語音轉文字 | [OpenSLR SLR31 Mini LibriSpeech](https://www.openslr.org/31/)，CC BY 4.0 | [六筆語音證據](../evidence/TSK-11/real-asr-20260924/verification.json)：官方封存檔 MD5／SHA-256、轉為 WAV 後的 manifest 指紋及說話者／場次分割 |
| 英文文字生成 | [Project Gutenberg #11](https://www.gutenberg.org/ebooks/11)、[#1342](https://www.gutenberg.org/ebooks/1342)、[#35](https://www.gutenberg.org/ebooks/35)；目錄標示「Public domain in the USA」，仍須遵守 [Project Gutenberg 使用條款](https://www.gutenberg.org/policy/license.html)。不宣稱其他地區的權利狀態 | [三本書證據](../evidence/TSK-11/real-text-20260924/verification.json)：原始純文字保留官方首尾與條款，訓練只選本文固定視窗 |
| 單字元 OCR | [ROIS-CODH Kuzushiji-MNIST](https://github.com/rois-codh/kmnist) 與[資料頁](https://codh.rois.ac.jp/kmnist/)，CC BY-SA 4.0；署名保留在外部 metadata 與報告 | [KMNIST 證據](../evidence/TSK-11/real-ocr-20260924/verification.json)：四個 IDX gzip、官方字元映射、各檔 SHA-256 與完整格式檢查 |
| 果蠅二維移動軌跡 | [Titova 等人的 Dryad 資料](https://datadryad.org/dataset/doi:10.5061/dryad.vdncjsz0b)；Dryad 的[重用說明](https://datadryad.org/help/guides/reuse)標示資料為 CC0。使用作者連結的[補充資料 GitHub 儲存庫](https://github.com/strawlab/titova_et_al_displacement_supplemental/tree/0eb07940a9ffdedd97ada81f75e9d5f7bface579)中固定提交的同名 gzip 檔 | [軌跡證據](../evidence/TSK-11/real-nav-20260925/verification.json)：原檔留在 Git 外；SHA-256 為 `83e13b7057957cf41a45dc91996a4519be7066519242b6912c65b2418cb91670`，Git blob 與固定提交的樹相符。Dryad 直下載受阻，**未驗證**此鏡像檔與 Dryad 封存檔逐位元組相同；實驗只支持來源固定的單步軌跡預測流程 |
| 文字條件影像、音訊與影片 | [Blender Foundation 的 Elephants Dream](https://orange.blender.org/press/) 影片標示 CC BY 2.5；[Big Buck Bunny](https://peach.blender.org/about/) 影片標示 CC BY 3.0。只使用官方影片內嵌音軌，不使用另列的配樂檔 | [影音證據](../evidence/TSK-11/real-media-20260925/verification.json)：兩部 MP4 的原檔指紋、來源隔離分割、影格／音訊時間線、輸出檔指紋與保留集觀察；生成內容品質未通過 |

這些執行都屬各任務的局部證據，不能直接推成自然語音辨識、整頁 OCR、閉環導航、通用文字生成或果蠅全圖模型的品質。整體 TSK-11 進度見[開發狀態](../delivery-status.md)與[需求證據](../evidence/TSK-11/verification.json)。
