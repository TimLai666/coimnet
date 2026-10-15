# TSK-11 training data review

## Problem

本輪只審查 LIF build-quality 任務會用到的原始 KMNIST 資料與固定切分。觀測範圍限於平衡訓練分割的 6,000 張圖片；測試影像只作原檔指紋與 IDX 完整性核對，不做像素分布探索，也不計算測試成績。

## 來源與授權 metadata

- 原始資料目錄是 `/Volumes/SSD/Developer/coimnet/data/TSK-11/kmnist`。`metadata.json` 宣告 schema `coimnet-realocr-data/v1`、資料集 `Kuzushiji-MNIST`、ROIS-CODH 與官方 GitHub URL，以及 CC BY-SA 4.0 與 CODH/NIJL attribution。依據：`/Volumes/SSD/Developer/coimnet/data/TSK-11/kmnist/metadata.json:2-10`。
- Metadata 宣告四份 IDX gzip 與 classmap 的 URL、壓縮大小與 SHA-256。依據：`/Volumes/SSD/Developer/coimnet/data/TSK-11/kmnist/metadata.json:11-40`。classmap 的十列 mapping 也與 metadata 相符。依據：`/Volumes/SSD/Developer/coimnet/data/TSK-11/kmnist/metadata.json:43-53`、`/Volumes/SSD/Developer/coimnet/data/TSK-11/kmnist/kmnist_classmap.csv:1-11`。
- 目前原始六份檔案的大小與 SHA-256 全部符合既有 `data-copy.json`。原始核對記錄在 `evidence/TSK-11/build-quality-audit-20261015/data-copy.json:1-43`；逐檔實測結果保存在 `training-data-summary.json:30-78`。

## 檢查方式與資料完整性

本輪只用 Python 標準庫 `csv`、`gzip`、`hashlib`、`json`、`math`、`struct`。對六份原檔重算 SHA-256；對四個 IDX gzip 解壓到記憶體並驗證 gzip CRC/trailer、magic、count、尺寸與 payload 長度。這對應原始 loader 先完整讀取 labels、選取，再讀取 images 並檢查完整 payload 的流程。依據：`evidence/TSK-11/chemical-quality-20261013/sources/main.go.txt:306-386`、`:539-611`；實測結構欄位見 `training-data-summary.json:80-109`。

結果沒有缺檔、截斷、錯誤 magic、錯誤尺寸、payload 長度不符或非預期壓縮完整性問題。這是目前本地原檔與固定 metadata/fingerprint 的確認，不代表重新向遠端下載驗證。

## 觀測單位與選取規則

`LoadDataset` 對 labels 先按類別收集原始 IDX 位置，再用 `balancedIndices` 逐 round-robin 取各類相同 rank。依據：`evidence/TSK-11/chemical-quality-20261013/sources/main.go.txt:354-360`、`:660-684`。索引 SHA-256 是把每個位置以 big-endian uint64 串接後計算 SHA-256。依據：`evidence/TSK-11/chemical-quality-20261013/sources/main.go.txt:1053-1061`。

本輪重建結果如下。每類數量與 hash 同時保存在 `training-data-summary.json:111-185`。

- `train`：6,000 張，每類 600，為 train labels 的 round-robin ranks `0..599`。
- `development`：200 張，每類 20，為 balanced test order 的 ranks `0..19`。
- `primary`：2,000 張，每類 200，為 balanced test order 的 ranks `20..219`，符合 protocol 的 per-class offset 20。依據：`evidence/TSK-11/build-quality-audit-20261015/protocol.json:21-24`。
- `confirmation`：2,000 張，每類 200，為 balanced test order 的 ranks `800..999`，hash `9500a51eff8ec4e5ef522459730caae6dd0b3463898ecc86430c4849875937a5` 與 protocol expected 值相同。依據：`evidence/TSK-11/build-quality-audit-20261015/protocol.json:59-63`。
- `development`、`primary`、`confirmation` 三個 test index set 互斥，train 與 test 也來自不同 IDX stream。依據：`training-data-summary.json:184-185`。

## 訓練資料摘要

探索單位是選取後 6,000 張 train images 的 4,704,000 個像素。原始像素是 IDX unsigned byte，依 loader 以 `pixel / 255` 正規化。依據：`evidence/TSK-11/chemical-quality-20261013/sources/main.go.txt:565-611`；實測摘要見 `training-data-summary.json:187-209`。

- 原始 uint8 範圍：`0..255`。
- 正規化範圍：`0..1`。
- 零像素比例：`0.6697604166666666`。
- 平均：`0.191140664599173`。
- population standard deviation：`0.347961470804463`。
- quartiles：`0.0 / 0.0 / 0.17647058823529413`。
- missing：`0`；non-finite：`0`。
- 每類數量：十類各 `600`，見 `training-data-summary.json:112-128`。

## 切分與洩漏限制

目前已確認 index 層級的 disjointness，且沒有用測試影像像素做探索、沒有計算測試分數、沒有用測試結果選參數。依據：`training-data-summary.json:4-10`、`:184-185`、`:220-224`。這只能說明目前固定 index 規則沒有交集，不能把已觀察資料改稱為全新盲測。

Protocol 明確把 held-out primary 描述為先前觀察過的固定 test split，confirmation 也標為固定 task 且不是 global never-observed claim。依據：`evidence/TSK-11/build-quality-audit-20261015/protocol.json:21-24`、`:59-63`。

## CONFIRMED 問題

- **Protocol 可追溯性缺口：** `protocol.json` 有 primary 與 confirmation 的 count/selection，但沒有 development 的 count、selection 或 expected index SHA-256。這不構成目前資料損壞，但使 development fingerprint 不是事前 protocol 欄位；本輪實測 hash 已保存為 `9b4fb30a75bdb1625bf0e636831ec633c86a7a62e74882d6c6fc2e5bc51943d2`。依據：`evidence/TSK-11/build-quality-audit-20261015/protocol.json:21-24`、`:59-63`、`training-data-summary.json:130-146`、`:217-219`。

## 未知與未宣稱事項

- 沒有下載或查詢遠端官方來源，所以官方性只確認到本地 metadata、原檔 fingerprint 與宣告一致。依據：`training-data-summary.json:216-220`。
- 沒有計算 test image pixels 的分布摘要，也沒有計算 primary、confirmation 或任何 test score；test IDX 只作完整性核對。依據：`training-data-summary.json:4-10`。
- 沒有對目前未執行的 LIF 訓練效果下結論。Protocol 明確寫本任務的既有 continuous-core task 不執行 LIF candidate expression。依據：`evidence/TSK-11/build-quality-audit-20261015/protocol.json:69-70`。
- 本輪未執行 Go、build、test、訓練、下載、網路或 Git 操作，沒有產生模型或改動原始資料。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `evidence/TSK-11/lif-build-quality-20261015/training-data-review.md` | 新增本輪來源、指紋、選取規則、train-only EDA、切分限制與未知事項。 |
| `evidence/TSK-11/lif-build-quality-20261015/training-data-summary.json` | 新增可重算的檔案 fingerprint、IDX 結構、split hash/count、訓練像素摘要與查核旗標。 |

## Next

Root 若要把 development 視為正式驗收欄位，應在下一輪 protocol 事前固定其 count、rank 規則與 expected index SHA-256；不要把本輪已觀察的 primary/confirmation 結果稱為全新盲測。
