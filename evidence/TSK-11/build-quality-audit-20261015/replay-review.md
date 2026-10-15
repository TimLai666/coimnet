# TSK-11 replay review

## 最小必要檔案與路徑設定

- 專案根目錄必須仍是 `/Users/timlai/Developer/coimnet`。五個腳本都把它寫死為 `r`／`root`，工作目錄必須是 `runs/.tmp/build-quality-audit-20261015`；`cache-prepare.py.txt:4-7`、`run-command.py.txt:3`、`preflight.py.txt:5-7`、`run-pairs-resume.py.txt:5-7`、`analyze.py.txt:4-6`。
- 清除原暫存後，須先建立工作目錄本身，再建立 `go-cache`、`go-mod-cache`、`go-tmp`。`cache-prepare.py.txt:10-14` 只建立子目錄而不建立父目錄，且固定設定 `GOCACHE`、`GOMODCACHE`、`GOTMPDIR`、`TMPDIR`、`GOPROXY`、`GOSUMDB`、`GOTOOLCHAIN`、`GOMAXPROCS`、`GOFLAGS`。
- driver 必須把 `driver-source.json:3-30` 的五份 archive 內容還原到完全相同的 target：`driver/chemistry.go`、`chemistry_test.go`、`comparison_main.go`、`main.go`、`model.go`，並符合該檔列出的 bytes／SHA-256。`driver-source.json` 只是來源對照，腳本實際驗證的是 `source-before.json` 與 `driver-lock.json` 指向的檔案，見 `run-command.py.txt:5-10`、`preflight.py.txt:16-18`。
- KMNIST 必須存在於 `data/kmnist` 的六個 target。其唯一確認的原始來源、target、大小與 SHA-256 在 `data-copy.json:3-42`，包含 `kmnist_classmap.csv`、`metadata.json`、`t10k-images-idx3-ubyte.gz`、`t10k-labels-idx1-ubyte.gz`、`train-images-idx3-ubyte.gz`、`train-labels-idx1-ubyte.gz`。`preflight.py.txt:19-40` 會同時讀取每筆 `source` 與 `path`，`run-command.py.txt:6-10` 會讀取 target。
- Go module cache 需要原生 `/Users/timlai/go/pkg/mod`、`evidence/TSK-01/glyph-contract-20261014/cache-preparation.json` 的 `selected_source_versions`，以及其對應內容。依據 `cache-prepare.py.txt:7-8`、`17-43`，只允許從該原生 cache 建立自有 cache 的 symlink／metadata。
- 執行入口還需要 `comparison`、`comparison-race`、`dump-state` 三個檔案，以及由訓練產生的 `models/{ordinary,race}/{seed}-{group}`。路徑和參數由 `run-pairs-resume.py.txt:49-52`、`analyze.py.txt:68-83` 固定；這三個 binary 不在 `driver-source.json` 或 `data-copy.json` 提供，且本次禁止編譯，因此目前無法由指定檔案證明它們可在清除後重建。
- `preflight.py.txt:125-128` 另外要求工作目錄內已有 `data-review.md`、`driver-review.md`，再以獨佔建立方式寫入 evidence。這兩份檔案的來源不在本次允許讀取的清單內。

## 確定阻礙與需修正的輸入／輸出契約

1. **暫存清除不等於可重跑。** `run-command.py.txt:12-14`、`preflight.py.txt:9-12`、`127-128`、`run-pairs-resume.py.txt:13-16`、`86`、`analyze.py.txt:10-13` 對 log、receipt、lock、review 與分析輸出使用 `xb`／`x`。原 evidence 目錄若保留既有結果，重跑會因檔案已存在而失敗；若連 evidence 一起清除，腳本又缺少 protocol、manifest、binary-lock 等輸入。契約需改成明確的「新 evidence output root」或可驗證的 run id，並讓所有 producer／consumer 使用同一個 output root，不能把固定 evidence 目錄兼作不可覆寫輸入與輸出。
2. **續跑判定不足以支援清除後重現。** `run-pairs-resume.py.txt:53-63` 只要發現 `<name>-train.log` 就等待 receipt 並跳過訓練，但 `run-pairs-resume.py.txt:49-52` 的 model 位於已被清除的 `runs/.tmp/.../models`。接著 `run-pairs-resume.py.txt:80-86` 仍會對該不存在或未驗證的 model 執行 restore。契約需改為「train receipt、log hash、model 目錄及其 manifest 全部存在且相符才可 resume」，否則必須重新 train；不能只以 log 存在判定。
3. **來源對照與實際驗證 manifest 不一致。** `driver-source.json` 沒有被五個腳本直接讀取；實際輸入是 `source-before.json`、`driver-lock.json`、`protected-models.json`，見 `preflight.py.txt:16-18` 及 `run-command.py.txt:5-10`。因此只還原 `driver-source.json` 的五檔，不能證明可通過正式來源檢查；`analyze.py.txt:14-22` 還硬性要求每個 receipt 的 `formal_source_manifest` 為 `source-before.json` 且數量為 810。契約需明確規定 archive → target → lock manifest 的產生順序、固定 hash 集合及 810 筆來源數，或讓 consumer 直接使用同一份 driver-source manifest。
4. **資料路徑依賴絕對位置。** `data-copy.json:3-42` 的 `source` 固定在 `/Volumes/SSD/Developer/coimnet/...`，`path` 固定在 `/Users/timlai/Developer/coimnet/runs/.tmp/...`；`preflight.py.txt:19-23` 直接以 `Path(p)` 讀取，未依 `root` 重定位。要支援清除後在同一專案重建，契約需統一為 repo-relative path 並由單一 root resolver 展開，或明確保證兩個絕對根目錄永遠存在且不可變。現有指定檔案未定義搬遷到其他 checkout 的行為。
5. **12 組的協定輸入未包含在本次可確認來源中。** `run-pairs-resume.py.txt:8-10`、`33-40` 依賴 `protocol.json`、`binary-lock.json`、`preflight.json`、`execution-order.json`；12 組數量只在 `run-pairs-resume.py.txt:96-97` 以輸出檢查。未讀取 `protocol.json`，無法從本次指定檔案確認 seed、group、build 的精確集合及 execution order；此項標為 NEEDS INVESTIGATION。

## 未驗證事項

- `source-before.json`、`driver-lock.json`、`protected-models.json`、`protocol.json`、`execution-order.json`、`state-probe-lock.json` 的內容、hash 集合及其與五個 driver／六個資料檔案的完整關係未讀取，不能宣稱清除後可完整重播。
- `comparison`、`comparison-race`、`dump-state` 的取得來源、binary hash、可執行權限與相依動態庫未驗證；本次沒有編譯或執行它們。
- `data-review.md`、`driver-review.md` 的內容與來源未驗證。
- `analyze.py.txt:26-28` 將 `before` 與 `after` 都取自同一個 `pick(test_labels, 20, 200)`。這是否為實驗設計或輸入契約錯誤，僅靠本次指定檔案無法判定；不據此作模型品質結論。
- 本報告只確認重播所需的檔案、路徑與腳本契約，未啟動訓練、Go 編譯、網路活動或全模型品質驗證。
