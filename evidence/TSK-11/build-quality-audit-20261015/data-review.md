# TSK-11 build-quality-audit-20261015：資料切分與比較方法審查

審查範圍是 `plan.md`、`protocol.json`、`data-copy.json`、前一輪 `chemical-quality-20261013` 的協定、訓練資料稽核與來源程式。只用 Python 標準函式庫在記憶體讀取本輪 `runs/.tmp/build-quality-audit-20261015/data/kmnist` 的 gzip／IDX。沒有讀測試圖片做 EDA、計算測試品質或調參。

## Problem

本輪要確認 6,000 張訓練、200 張開發、2,000 張主要評估與 2,000 張確認評估是否由官方 KMNIST IDX 以固定且可重建的索引產生，並確認普通／競態建置的同圖片配對方法與三個 seed 的推論界線。

## Confirmed

### 1. 資料來源、檔案指紋與 IDX 結構

- `data-copy.json` 記錄的六份副本大小與 SHA-256 都與本輪檔案相同：classmap `151` bytes、metadata `2,331`、train images `18,165,135`、train labels `29,497`、test images `3,041,136`、test labels `5,120`；六份雜湊依序為 `88ba4be4bd52d986ae100b425d5c28437c81829c87800c5832a2253edc758374`、`e5d19fcbf83240be5a73e6a4ef2b63e385a7e32ca802188aa0911fc05867b9c3`、`51467d22d8cc72929e2a028a0428f2086b092bb31cfb79c69cc0a90ce135fde4`、`e38f9ebcd0f3ebcdec7fc8eabdcdaef93bb0df8ea12bee65224341c8183d8e17`、`edd7a857845ad6bb1d0ba43fe7e794d164fe2dce499a1694695a792adfac43c5`、`20bb9a0ef54c7db3efc55a92eef5582c109615df22683c380526788f98e42a1c`。依據：`evidence/TSK-11/build-quality-audit-20261015/data-copy.json:1-43`。
- `metadata.json` 宣告資料集為 `Kuzushiji-MNIST`，來源 URL 為 ROIS-CODH／官方 GitHub，並列出相同五份資料檔的 URL、SHA-256 與壓縮大小。依據：`runs/.tmp/build-quality-audit-20261015/data/kmnist/metadata.json:2-40`。這確認的是本地副本與既有官方來源記錄一致，沒有在本輪重新下載或即時查詢遠端。
- 記憶體 Python 解壓並驗證 gzip CRC／trailer 後，IDX 結構為：train images magic `2051`、`60000×28×28`、payload `47,040,016` bytes；train labels magic `2049`、`60000`、payload `60,008` bytes；test images magic `2051`、`10000×28×28`、payload `7,840,016` bytes；test labels magic `2049`、`10000`、payload `10,008` bytes。train labels 每類 `6,000`，test labels 每類 `1,000`。
- 原始 loader 會先讀完整 labels／images、檢查 magic、數量、尺寸與 payload 長度，再呼叫平衡選取。依據：`evidence/TSK-11/chemical-quality-20261013/sources/main.go.txt:306-386`、`:539-611`。

### 2. 四個分割的重建結果

`balancedIndices` 先按 label 建立十個原始 IDX 位置清單，再以 `round-robin` 取每類同一 rank，保留各類在官方 IDX 中的原始順序。依據：`evidence/TSK-11/chemical-quality-20261013/sources/main.go.txt:660-684`。comparison 以 `TrainCount=6000`、`TestCount=10000` 載入後切片：`Test[:200]`、`Test[200:2200]`、`Test[8000:10000]`。依據：`runs/.tmp/build-quality-audit-20261015/driver/comparison_main.go:44-63`。

索引雜湊是對每個 IDX 位置依程式 `digestIntSlice` 的 big-endian uint64 串接後計算 SHA-256。依據：`evidence/TSK-11/chemical-quality-20261013/sources/main.go.txt:1053-1061`。

| 分割 | IDX 來源與程式切片 | 筆數／每類 | 重建 index SHA-256 | 結論 |
| --- | --- | ---: | --- | --- |
| train | train labels 的 round-robin 前 `6000` 個，即每類 rank `0..599` | `6000`／`600` | `bb87f08092d04ad4f4fb16715f44b6e46f44e4d558db6bb5f63be57a3d11434f` | 與既有分析預期值相同，依據 `evidence/TSK-11/chemical-quality-20261013/sources/analyze.py.txt:63-66` |
| development | test labels 的 round-robin `Test[:200]`，即每類 rank `0..19` | `200`／`20` | `9b4fb30a75bdb1625bf0e636831ec633c86a7a62e74882d6c6fc2e5bc51943d2` | 由本輪獨立重建；原協定沒有保存這個 expected hash |
| primary | test labels 的 `Test[200:2200]`，即每類 rank `20..219` | `2000`／`200` | `a25ee4d0b8c80fa5dc347e8fa821d90f67eec5e8365dc06fcd20ff83eb734f43` | 與既有分析預期值相同，依據 `evidence/TSK-11/chemical-quality-20261013/sources/analyze.py.txt:21-36` |
| confirmation | test labels 的 `Test[8000:10000]`，即每類 rank `800..999` | `2000`／`200` | `9500a51eff8ec4e5ef522459730caae6dd0b3463898ecc86430c4849875937a5` | 與協定 expected hash 相同，依據 `evidence/TSK-11/build-quality-audit-20261015/protocol.json:59-63` |

訓練 IDX 與測試 IDX 是不同來源串流；development、primary、confirmation 三個 test 子集合互斥。test round-robin 中未使用的是每類 rank `220..799`，共 `5,800` 張，沒有被誤算成任何評估分割。

### 3. 訓練影像 EDA

只對 train images 的上述 6,000 張、共 `4,704,000` pixels 做 EDA。結果為 range `[0,1]`、mean `0.191140664599173`、population std `0.347961470804463`、quartiles `0.0 / 0.0 / 0.17647058823529413`、missing `0`、nonfinite `0`、zero fraction `0.6697604166666666`，每類 600 張。與前一輪 `training-data-audit.json:1-30` 的所有數值完全相同；該稽核也明載像素由 IDX uint8 除以 255，且不使用測試圖片統計或分數做選擇。

### 4. 來源程式與比較方法

- 本輪 driver manifest 的五份檔案目前各自與 manifest 及 archived `.go.txt` byte-identical：`chemistry.go` `cccd6c83f5cb8e0288dd06fc9dc1784f88846f3e46ef57f7874ae6db543417cd`、`chemistry_test.go` `92a5d025794525cbe3a0e716e95a4ea983f384679bb6c5462ff1e1aea94be41d`、`comparison_main.go` `a275baa2e7a8ba8b368cdebd535886ea54943433ac700e2f728538d0f991f618`、`main.go` `f44ee3d0bbcdb06d04ca6eaf7bdecdc43d55aabec571fcfc716e401bbbcbccf3`、`model.go` `1d8349ff9fc4dc68a49ba60ec13af089c4c48eb8ca429ab07b4e568766e506a3`。依據：`evidence/TSK-11/build-quality-audit-20261015/driver-source.json:1-31`，並由本輪記憶體 Python 重算。
- 普通／競態比較的固定契約是兩個 build、OFF／ON 兩組、三個 seed、每組 6,000 張 train、20 epochs、每組 120,000 updates；protocol 也明確禁止調參、挑 seed、早停及挑最佳 epoch。依據：`evidence/TSK-11/build-quality-audit-20261015/plan.md:3-13`、`evidence/TSK-11/build-quality-audit-20261015/protocol.json:2-24`、`:50-71`。
- 每次相同 `(seed, group)` 執行會以同一個 `rand.NewPCG(seed,37)` 建立 permutation，逐 epoch shuffle 同一個 train index order；每張輸入由相同 `ds.Train[i]` 進入同一個 `trainSample`。依據：`runs/.tmp/build-quality-audit-20261015/driver/comparison_main.go:85-100`。因此「應如何配對」已明確是同 seed、同 group、同資料位置的逐圖片配對。
- 前一輪獨立分析對兩個模型的同圖片預測用 `zip(predictions, truth)` 計數，並核對總數與雙方 correct 數。依據：`evidence/TSK-11/chemical-quality-20261013/sources/analyze.py.txt:91-99`。這證明逐位置配對的計算方法；它原本比較的是 chemical OFF／ON，不是本輪 ordinary／race 實際結果。

### 5. 三 seed 的推論限制

固定 seed 只有 `20260924`、`20260925`、`20260926`。依據：`evidence/TSK-11/build-quality-audit-20261015/protocol.json:14-19`。計畫與協定都限制只能報告三個 seed 的觀察範圍、不得做母體推論；前一輪分析亦將「three seeds are insufficient for population confidence」列為明確限制。依據：`evidence/TSK-11/build-quality-audit-20261015/plan.md:11-12`、`evidence/TSK-11/build-quality-audit-20261015/protocol.json:50-51`、`evidence/TSK-11/chemical-quality-20261013/sources/analyze.py.txt:117`。

## Needs investigation

1. **開發分割沒有被寫入本輪 protocol 的 expected fingerprint。** 程式確實使用 `Test[:200]`，本輪重建得到每類 20 張及 hash `9b4fb30a75bdb1625bf0e636831ec633c86a7a62e74882d6c6fc2e5bc51943d2`；但 `protocol.json` 只有 primary／confirmation 的欄位，沒有 development count、selection 或 hash（`evidence/TSK-11/build-quality-audit-20261015/protocol.json:21-24`、`:59-63`）。這是協定可追溯性缺口，不是目前已證實的資料錯誤。下一輪應把 development 的索引規則與 expected hash 一起事前固定。
2. **本輪 build evidence 尚沒有 ordinary／race 的實際執行回條或 selection rows。** `plan.md` 與 `protocol.json` 定義了兩個 build 與 12 次完整訓練（`evidence/TSK-11/build-quality-audit-20261015/plan.md:9-12`、`evidence/TSK-11/build-quality-audit-20261015/protocol.json:65-71`），但目前檢閱的 build evidence 只有計畫、協定、資料副本、來源 manifest 與準備資料，沒有每個 `(build, group, seed)` 的輸入 hash、初始參數 hash、逐圖片 predictions 或 ordinary／race pair receipt。因此只能確認配對方法與 deterministic source code，不能把 ordinary／race 實際配對執行或結果宣稱已驗證。正式結果需保留兩邊相同的 train／development／primary／confirmation hashes、initial snapshot hash 與同圖片 prediction vectors。
3. **官方性目前是本地 metadata／指紋層級的確認。** `metadata.json` 的 URL、授權與檔案雜湊格式正確，且副本與宣告相符；本輪依限制沒有下載或向遠端重新核對，所以不能把它表述成當日遠端內容已重新驗證。
4. **品質結論不能外推到 LIF。** 協定明確寫本任務不執行 LIF candidate expression（`evidence/TSK-11/build-quality-audit-20261015/protocol.json:69-70`）；即使 ordinary／race 連續核心結果完成，也不能替代 LIF 品質證據。

## Verified / not executed

- 已執行：唯讀檔案讀取、五份 driver source manifest／archive byte 比對、記憶體 Python stdlib 的 gzip／IDX／SHA-256／index reconstruction，以及 train-only EDA。
- 未執行：Go build、Go test、race、訓練、推論品質計算、測試圖片 EDA、參數搜尋／調參、下載、Git、任何其他暫存或記憶寫入。
- 沒有修改測試檔、Go source、原始資料、模型、Git 狀態或既有 evidence。

## New file

本輪唯一新增檔案：`runs/.tmp/build-quality-audit-20261015/data-review.md`。

## Next

Root 完成 ordinary／race 執行後，先核對每個 pair 的 selection row 與四個 split hash，再進行同圖片 prediction／discordance 分析；在此之前，資料切分可判定為 Confirmed，ordinary／race 實際配對只能維持 Needs investigation。
