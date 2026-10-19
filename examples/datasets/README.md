# 訓練用資料集清單與下載腳本

這是範例，不是框架的一部分。這裡列出之後用 CoImNet 訓練各種用途模型時會用到的公開資料集，並提供一支可重跑的腳本，把每個資料集下載下來、逐檔核對 sha256，再各自打包成一個壓縮檔。資料本身不進 git：預設存到專案內已忽略的 `data/datasets/`。

- `catalog.json`：每個資料集的來源、上游大小、授權、授權狀態、是否需要先同意條款、用途（訓練或只當評測）。
- `fetch.py`：只用 Python 標準函式庫加 `zstd` 指令。沒有 `zstd` 時改產生 `.tar.gz`。

## 用法

```sh
# 先看會抓哪些、多大
python3 examples/datasets/fetch.py --list
python3 examples/datasets/fetch.py --list --tier full

# 抓 core 全部（約 34 GB 下載量，處理過程需要約兩倍空間）
python3 examples/datasets/fetch.py

# 只抓某些、或只抓某一類
python3 examples/datasets/fetch.py --only tinystories toolace
python3 examples/datasets/fetch.py --category pii audio

# 發佈模型前只保留授權明確可用的資料
python3 examples/datasets/fetch.py --clear-only
```

完成後每個資料集會有兩個檔案：

- `data/datasets/archives/<id>.tar.zst`：壓縮檔，檔案順序固定、時間戳與擁有者歸零，同樣的上游內容會得到同樣的 tar。
- `data/datasets/archives/<id>.json`：回條，記錄上游版本（Hugging Face 的 commit、Zenodo 的版本）、每個檔案的大小與 sha256、授權、壓縮檔的 sha256。

已經有壓縮檔和回條的資料集會直接跳過；下載中斷時重跑會從斷點續傳。下載完的原始檔在打包後刪除，加 `--keep-raw` 才保留。解開：`zstd -dc data/datasets/archives/tinystories.tar.zst | tar -x -C 目標資料夾`。

標成 gated 的資料集（xLAM、The Stack smol、FineVideo）要先登入 Hugging Face、在資料集頁面按同意條款，再把 token 放進 `HF_TOKEN` 環境變數。沒做的話腳本會跳過它們並在最後的摘要列出，其他資料集照常完成。

## 分級

- `core`：一台 Mac 加一張 RTX 4070 就訓練得動的規模，下載量約 34 GB。
- `full`：再加上 FineWeb-Edu 全部 10B token、FineVideo 60 個分片、FSD50K、UCF101，以及授權受限、只能內部比較的資料，下載量約 116 GB。

## 壓縮方式

- 只含文字欄位的 parquet 會先轉成 JSON Lines（`原檔名.parquet.jsonl`，每行一筆、所有欄位都保留），再用 zstd -19 壓。parquet 內部已經用 snappy 壓過，直接再壓幾乎不會變小；轉成純文字後 TinyStories 從 1.0 GB 壓到 368 MB。回條同時記錄原檔 sha256（`sha256`）與轉檔後的 sha256（`stored_sha256`）。要保留原 parquet 加 `--keep-parquet`。
- 全部是 JSON、文字檔的資料集也用 zstd -19；含圖片、聲音、影片的資料集上游已是壓縮格式（JPEG、FLAC、MP4），無損再壓幾乎不會變小，只用 -3 打包。
- 需要 `pyarrow` 才會轉換 parquet。沒裝時腳本直接停下並提示安裝；確定要照原檔打包就加 `--keep-parquet`。

## 其他選項

- `--work 路徑`：下載與打包用的暫存位置，`--dest` 在網路磁碟時設到本機磁碟。
- `--match 回條資料夾`：照之前的回條重抓同一版本的檔案，sha256 不同就報錯。換一台機器時用它確保拿到一模一樣的資料。
- `--raw-only`：只下載並核對，不打包，原始檔留在 `--work`，適合直接拿來訓練。

## 授權狀態

- `clear`：授權允許拿來訓練並發佈模型。部分要求署名（CC-BY）或資料再散布時沿用同授權（CDLA-Sharing、CC-BY-SA），模型卡要列出來源。
- `unclear`：上游沒寫授權（CIFAR、Moving MNIST）。可以拿來和既有架構比準確率，發佈的模型不要拿它訓練。
- `restricted`：只限研究或非商業（Tiny ImageNet、ESC-50、HMDB51）。只放在 `full`，只當內部評測。

## 資料集

| 類別 | id | 用途 | 上游大小 | 授權 |
|---|---|---|---:|---|
| 寫文章 | tinystories | 小模型寫故事，第一個語言基準 | 1.0 GB | CDLA-Sharing-1.0 |
| 寫文章 | babylm-2026-strict-small | 1000 萬字的少量資料賽道 | 54 MB | MIT |
| 寫文章 | fineweb-edu-10bt | 一般英文網頁文字（core 取 3/14 個分片） | 6.5 GB／28.5 GB | ODC-BY-1.0 |
| 寫文章 | wikipedia-zh | 中文寫作（繁簡混雜，訓練前用 OpenCC 轉繁體） | 1.7 GB | CC-BY-SA-3.0 |
| 寫文章 | smol-smoltalk | 指令遵循與對話 | 971 MB | Apache-2.0 |
| 工具呼叫 | xlam-function-calling-60k | 經實際執行驗證的函式呼叫（gated） | 96 MB | CC-BY-4.0 |
| 工具呼叫 | hermes-function-calling-v1 | 多輪函式呼叫、JSON 模式、代理任務 | 67 MB | Apache-2.0 |
| 工具呼叫 | glaive-function-calling-v2 | 大量函式呼叫對話 | 271 MB | Apache-2.0 |
| 工具呼叫 | toolace | 2.6 萬個 API 的複雜工具對話，含中文 | 37 MB | Apache-2.0 |
| 工具呼叫 | bfcl | Berkeley 函式呼叫排行榜，只當評測 | 12 MB | Apache-2.0 |
| 寫程式 | the-stack-smol | 30 種語言各 1 萬個程式檔（gated） | 2.9 GB | 各檔自身的寬鬆授權 |
| 寫程式 | magicoder-oss-instruct-75k | 程式指令問答 | 203 MB | MIT |
| 寫程式 | mbpp | 入門 Python 題目附測試，只當評測 | 0.6 MB | CC-BY-4.0 |
| 寫程式 | humaneval | 164 題 Python，只當評測 | 0.1 MB | MIT |
| 圖片 | mnist | 冒煙測試 | 18 MB | MIT |
| 圖片 | fashion-mnist | 比 MNIST 難的同格式資料 | 36 MB | MIT |
| 圖片 | imagenette-320 | 10 類 ImageNet 真實照片 | 342 MB | Apache-2.0 |
| 圖片 | cifar10／cifar100 | 最多現成對照結果的彩色小圖 | 各 0.14 GB | 未標示 |
| 圖片 | tiny-imagenet（full） | 200 類 64×64 | 161 MB | 僅限研究 |
| 聲音 | speech-commands-v2 | 35 個英文指令詞 | 2.4 GB | CC-BY-4.0 |
| 聲音 | librispeech-clean | 英文語音辨識 100 小時加驗證與測試集 | 7.1 GB | CC-BY-4.0 |
| 聲音 | fsd50k（full） | 200 類一般聲音事件 | 24.7 GB | 每段各自的 CC 授權，含部分非商業 |
| 聲音 | esc50（full） | 環境聲音分類，只當評測 | 773 MB | CC-BY-NC-3.0 |
| 影片 | moving-mnist | 1 萬段 20 格數字移動影片 | 819 MB | 未標示 |
| 影片 | hmdb51 | 51 類人體動作，只當評測 | 870 MB | 僅限研究 |
| 影片 | ucf101（full） | 101 類動作，只當評測 | 7.0 GB | 未標示 |
| 影片 | finevideo | CC-BY 授權的 YouTube 影片加場景、問答與描述（gated，core 4 個分片約 130 支） | 2.2 GB／約 30 GB | CC-BY |
| 個資去識別化 | openpii-1.5m | 30 種語言、19 類個資標註，含簡體中文 | 5.3 GB | CC-BY-4.0 |
| 個資去識別化 | nemotron-pii | 英文 55 類以上個資與醫療資訊 | 307 MB | CC-BY-4.0 |
| 個資去識別化 | gretel-pii-masking-en-v1 | 各領域英文文件個資標註 | 25 MB | Apache-2.0 |

大小是 2026-10-18 從上游查到的下載量；實際值以回條為準。

## 已知缺口

- 個資資料的中文部分是中國大陸格式，沒有台灣身分證字號、09 開頭手機號碼、台灣地址格式。要做台灣用的去識別化模型，需要另外合成這些格式的資料。
- 影片類沒有同時「授權明確」又「有標準準確率排行」的資料集：UCF101、HMDB51 是最常用的動作辨識基準，但授權不明或限研究；FineVideo 授權明確，適合訓練，但不是分類基準。
- 繁體中文的一般文字語料目前只有維基百科。
