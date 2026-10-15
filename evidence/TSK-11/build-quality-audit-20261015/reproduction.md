# 在清理暫存後重現固定比較

`prepare-replay.py.txt` 會核對原始來源與資料，將五個 driver 原件還原到新的 `runs/.tmp/102-ocr-replay-<id>`。結果寫在該目錄的 `results/`。既有目錄會被拒絕，原證據保留。

## 所需輸入

- 本儲存庫的 810 筆 Go／module 來源符合 `source-before.json`，五個 driver archive 符合 `driver-source.json`。
- 六個資料檔包含原匯入的 `metadata.json`，大小與指紋符合 `data-copy.json`。本次使用的原件位置列在該檔；準備工具接受另一個持有相同位元組的唯讀資料目錄。
- 本次環境為 Go 1.26.5、darwin/arm64、CGO_ENABLED=1。快取準備沿用 `/Users/timlai/go/pkg/mod` 的 187 個固定版本及其下載中繼資料，見 `cache-prepare.py.txt`。這份重現流程使用該本機快取，沒有驗收其他平台或全新機器。

模型、圖片副本、Go 快取及執行檔都放在新暫存目錄。固定協定、資料切分與排程逐位元組複製，只有工具的專案、工作與結果路徑改成新位置。`preparation.json` 記錄原工具與重定位後的指紋。

## 準備與執行

在 `/Users/timlai/Developer/coimnet` 執行，下列資料路徑是本次已核對的既有原件：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evidence/TSK-11/build-quality-audit-20261015/prepare-replay.py.txt \
  --data-dir /Volumes/SSD/Developer/coimnet/data/TSK-11/kmnist \
  --name 102-ocr-replay-check
```

這一步只準備檔案。`--help` 與不帶參數都會顯示用途、必要輸入及範例。六項準備控制涵蓋正常還原、既有結果、符號連結、缺少資料、錯誤指紋及越出暫存範圍，見 `replay-preparation-final.json`。

新的 `results/replay-commands.json` 提供完整的順序與實際參數。要執行完整 12 組比較，在相同專案根目錄使用：

```sh
CGO_ENABLED=1 PYTHONDONTWRITEBYTECODE=1 python3 - <<'PY'
from pathlib import Path
import json
import subprocess

root = Path('/Users/timlai/Developer/coimnet')
results = root / 'runs/.tmp/102-ocr-replay-check/results'
for item in json.loads((results / 'replay-commands.json').read_text()):
    subprocess.run(item['command'], cwd=root, check=True)
PY
```

命令依序準備專案內快取、編譯兩種建置與狀態讀取器、執行各五個控制測試及公開 OCR 控制、核對原件與切分，再依原排程完成 12 組訓練、新程序恢復及完整分析。三條訓練工作線各固定 GOMAXPROCS=2。任何命令失敗就停止，不以不完整組別取代原定比較。觀察逾時時繼續使用原執行中的程序，不重複啟動訓練。

## 本輪已驗證的範圍

完整工程日誌保存為 `.log.gz`。`engineering-log-archives.json` 對應原始回條中的日誌名稱與 SHA-256，解壓後的位元組與原日誌相同；原失敗也完整保留。

Root 已實際建立全新的檢查目錄，重建普通／競態執行檔與狀態讀取器，各五個測試及各十張真實圖片的 SDK／中性／活動化學控制通過。重新核對的 preflight 與主訓練開跑前逐項相同。小型原始日誌、回條與重定位來源保存在 `replay-smoke/`，Root 核對見 `replay-smoke-verified.json`。

最後分析的固定切分、完整來源清單及完成／恢復標記檢查已補正，16 項控制與既有九項數值控制通過。七個完成組別的 14 個真實宣告也符合檢查，準備工具的六項控制再次通過，見 `analysis-gate-verified.json`。準備工具會還原目前版本的分析器並記錄其指紋。

距離計算沿用穩定的 hypot，避免有限極小／極大差值在平方時下溢或溢位。修正後六項距離控制、九項回條／數值控制、16 項驗收控制與六項準備控制通過，見 `numeric-report-root-verified.json`。原失敗及修改前來源保留，訓練 driver 和協定保持。

這個檢查沒有另外啟動 12 組完整訓練。主比較的驗收與最終品質結論依主證據目錄的完整結果判斷。新暫存內保存的歷史 worker 報告用於核對方法，新工作目錄的實際檢查由 Root 執行。

完成重現後，先將必要日誌、回條、`preflight.json`、`training-complete.json`、`state-comparison.json`、`quality-comparison.json` 及來源指紋保存到新的證據目錄。確認沒有執行中的程序，再檢查並移除該次建立的暫存樹。保留原資料、原模型、其他暫存目錄與原生 module 快取。本輪檢查目錄的清理證據為 `replay-smoke-cleanup.json`。

減法審查：沿用五個原 driver 與固定分析，只補重現所需的路徑準備。清理重建的資料、模型及快取，避免另留重複模型產品。
