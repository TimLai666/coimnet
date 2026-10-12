# `realocr`：官方 KMNIST 真實資料範例

這是明確標示為範例的真實資料入口。它從 ROIS-CODH 官方 KMNIST IDX gzip 檔案讀取有上限的小樣本，使用既有 `tasks/ocr.Recognizer` 完成訓練、獨立推論與評估，輸出固定 seed 的 JSON 報告，並可選擇保存完整模型 bundle 或載入既有 bundle 只做評估。

KMNIST 是 28×28 灰階的單字元くずし字資料集。每筆資料只代表一個日本平假名類別，這個範例把 28 個影像欄位當成 CTC 時序，不能推論成自然頁面 OCR 能力。它沒有頁面版面、切行、閱讀順序、多字元文字或古籍上下文。

## Raster 與頁面入口

`tasks/ocr.LoadImage` 是有上限的 PNG／JPEG 入口，會在完整解碼前檢查 encoded bytes 與 pixels（零值上限是 16 MiB 與 1,048,576 pixels），回傳同一次讀取的灰階 raster、來源 SHA-256、原始／輸出尺寸、比例、來源座標區域與格式 metadata。預設以明示的灰階與 ink polarity 保存像素，指定正值輸出高度時才以 `nearest-v1` 縮放；方向固定為 `stored-pixel-order`、文字方向固定為 `horizontal-left-to-right`，目前不做 EXIF 自動旋轉或 ICC 轉換。

`Recognizer.InferPage` 使用同一個 Recognizer 將單欄水平頁面分行、依來源座標裁切區塊並保留閱讀順序，再以 nearest 方式縮放，再把每個區塊的文字寫入 `Page.Blocks[].Text`。`PageOptions` 的 `Threshold` 與 `MaxLinePixels` 控制分行與每行容量；空頁回傳非 nil 的空 blocks，`EvalReport.EmptyOutputs`／`EmptyOutputRatio` 只把解碼後完全空字串計入。

realocr 的 `--encoder` 只接受 `fixed`（預設）或 `trainable`。報告會保留 `config.encoder`、`encoder_profile.mode`、encoder／core／readout capacity 與實際 updates，方便把新增的 encoder 容量和核心更新分開閱讀；這個範例仍只涵蓋 bounded KMNIST 單字元流程，不宣稱自然 OCR 品質。

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

資料放在專案已忽略的 `data/`，不納入 Git。從專案根目錄下載：

```sh
data_dir=data/TSK-11/kmnist
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
mkdir -p runs/TSK-11
go run ./examples/realocr \
  --data-dir data/TSK-11/kmnist \
  --encoder fixed \
  --train-count 1000 \
  --test-count 200 \
  --epochs 1 \
  --seed 20260924 \
  --report runs/TSK-11/realocr-report.json
```

### 模型保存與載入

`--model-output NEW_DIR` 會在訓練後以新的目錄保存完整 task-model bundle，包含 `manifest.json`、`document.json` 與 `arrays.bin`，其中保存 OCR 任務設定、完整最佳化器狀態與更新次數。`--model-input DIR` 會載入 bundle，依目前資料目錄與固定的 balanced round-robin 選取評估測試 split，並跳過訓練；載入的模型若與目前範例固定任務設定不一致會拒絕。兩個旗標互斥，`--model-output` 的目錄必須不存在，既有路徑不會覆寫。

以下使用目前 README 的資料路徑與相同固定設定，兩次命令分別保存與載入模型。`realocr-bundle-20261012` 是新的 bundle 目錄；報告仍放在 `runs/TSK-11`：

```sh
mkdir -p models/TSK-11 runs/TSK-11
go run ./examples/realocr \
  --data-dir data/TSK-11/kmnist \
  --encoder fixed \
  --train-count 1000 \
  --test-count 200 \
  --epochs 1 \
  --seed 20260924 \
  --model-output models/TSK-11/realocr-bundle-20261012 \
  --report runs/TSK-11/realocr-save-report.json

go run ./examples/realocr \
  --data-dir data/TSK-11/kmnist \
  --encoder fixed \
  --train-count 1000 \
  --test-count 200 \
  --epochs 1 \
  --seed 20260924 \
  --model-input models/TSK-11/realocr-bundle-20261012 \
  --report runs/TSK-11/realocr-load-report.json
```

有模型旗標時，報告最上層的 `model.sha256` 是 canonical `manifest.json` 的 hash，綁定 `document.json`、`arrays.bin` 與拓撲；`model.updates` 是 bundle 保存的歷史更新數。報告最上層的 `model_persisted` 在 `model.mode: "saved"` 時為 `true`，在 `model.mode: "restored"` 時為 `false`；訓練統計位於 `train.epochs` 與 `train.updates`，載入模式兩者都是 `0`。

stdout 和 `--report` 是不含執行時間的 deterministic JSON，包含：

- `before`／`after` 的 CER、整行 Exact 與樣本數。
- 官方來源 URL、授權、classmap、metadata、IDX magic／shape／完整 split 數量與每個來源檔的實際 SHA-256。
- 固定 seed、訓練設定、選取索引指紋、設定 hash、程式原始碼／binary 指紋。
- 資料範圍與限制，以及最上層的 `model_persisted` 與（使用模型旗標時）`model` 保存資訊。

執行時間只寫到 stderr 的 `elapsed_ms`，不放進報告，所以相同 build、資料與設定連續跑兩次時，報告可逐位元組比較。這些數字只驗證本範例在 bounded KMNIST 單字元資料上的流程，不能當成頁面 OCR、一般日文 OCR 或官方全量 benchmark 結論。
