# `realocr`：官方 KMNIST 真實資料範例

這是明確標示為範例的真實資料入口。它從 ROIS-CODH 官方 KMNIST IDX gzip 檔案讀取有上限的小樣本，使用既有 `tasks/ocr.Recognizer` 完成訓練、獨立推論與評估，輸出固定 seed 的 JSON 報告，且不保存模型或 checkpoint。

KMNIST 是 28×28 灰階的單字元くずし字資料集。每筆資料只代表一個日本平假名類別，這個範例把 28 個影像欄位當成 CTC 時序，不能推論成自然頁面 OCR 能力。它沒有頁面版面、切行、閱讀順序、多字元文字或古籍上下文。

## 來源與授權

官方來源如下，程式會要求 `metadata.json` 完整記錄這些資料，並核對 `kmnist_classmap.csv` 的內容、URL、SHA-256 與授權欄位：

- Repository: <https://github.com/rois-codh/kmnist>
- Dataset page: <https://codh.rois.ac.jp/kmnist/>
- Train images: <https://codh.rois.ac.jp/kmnist/dataset/kmnist/train-images-idx3-ubyte.gz>
- Train labels: <https://codh.rois.ac.jp/kmnist/dataset/kmnist/train-labels-idx1-ubyte.gz>
- Test images: <https://codh.rois.ac.jp/kmnist/dataset/kmnist/t10k-images-idx3-ubyte.gz>
- Test labels: <https://codh.rois.ac.jp/kmnist/dataset/kmnist/t10k-labels-idx1-ubyte.gz>
- Label mapping: <https://codh.rois.ac.jp/kmnist/dataset/kmnist/kmnist_classmap.csv>

官方授權是 CC BY-SA 4.0，建議署名為：`KMNIST Dataset (created by CODH), adapted from Kuzushiji Dataset (created by NIJL and others), doi:10.20676/00000341`。下載頁與 README 都說明資料是十個平假名行的代表字元，classmap 的實際映射由程式重新解析，不在程式內猜測。

## 下載與 metadata

資料必須放在 Git 外，例如：

```sh
data_dir=/Users/timlai/Developer/coimnet-data/TSK-11/kmnist
mkdir -p "$data_dir"
curl -L --fail --retry 3 -o "$data_dir/train-images-idx3-ubyte.gz" \
  https://codh.rois.ac.jp/kmnist/dataset/kmnist/train-images-idx3-ubyte.gz
curl -L --fail --retry 3 -o "$data_dir/train-labels-idx1-ubyte.gz" \
  https://codh.rois.ac.jp/kmnist/dataset/kmnist/train-labels-idx1-ubyte.gz
curl -L --fail --retry 3 -o "$data_dir/t10k-images-idx3-ubyte.gz" \
  https://codh.rois.ac.jp/kmnist/dataset/kmnist/t10k-images-idx3-ubyte.gz
curl -L --fail --retry 3 -o "$data_dir/t10k-labels-idx1-ubyte.gz" \
  https://codh.rois.ac.jp/kmnist/dataset/kmnist/t10k-labels-idx1-ubyte.gz
curl -L --fail --retry 3 -o "$data_dir/kmnist_classmap.csv" \
  https://codh.rois.ac.jp/kmnist/dataset/kmnist/kmnist_classmap.csv
shasum -a 256 "$data_dir"/*
```

在同一資料夾建立 `metadata.json`。`schema_version` 必須是 `coimnet-realocr-data/v1`；`files` 必須列出上述五個檔案、官方 URL、下載後 SHA-256 與大小；`license.name` 必須是 `CC BY-SA 4.0`，並保留官方署名；`mapping` 必須逐列保存官方 CSV 的十個 `index`、`codepoint` 與 `char`。程式也會逐檔檢查 IDX magic、樣本數、28×28 shape、完整 payload、gzip checksum、標籤範圍及檔案上限。

## 執行

以下是小樣本實跑設定。`--train-count` 和 `--test-count` 是每個 split 的總數，程式依 label 以固定 round-robin 選取，避免只取到檔案開頭的單一類別。訓練與測試資料是兩個不同 IDX split，測試資料不會進入更新迴圈。

```sh
go run ./examples/realocr \
  --data-dir /Users/timlai/Developer/coimnet-data/TSK-11/kmnist \
  --train-count 1000 \
  --test-count 200 \
  --epochs 1 \
  --seed 20260924 \
  --report /tmp/coimnet-realocr.json
```

stdout 和 `--report` 是不含執行時間的 deterministic JSON，包含：

- `before`／`after` 的 CER、整行 Exact 與樣本數。
- 官方來源 URL、授權、classmap、metadata、IDX magic／shape／完整 split 數量與每個來源檔的實際 SHA-256。
- 固定 seed、訓練設定、選取索引指紋、設定 hash、程式原始碼／binary 指紋。
- 資料範圍與限制，並明示 `model_persisted: false`。

執行時間只寫到 stderr 的 `elapsed_ms`，不放進報告，所以相同 build、資料與設定連續跑兩次時，報告可逐位元組比較。這些數字只驗證本範例在 bounded KMNIST 單字元資料上的流程，不能當成頁面 OCR、一般日文 OCR 或官方全量 benchmark 結論。
