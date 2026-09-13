# 07 — 使用者可以逐批讀取官方 Feather 原件

Epic：真實接線資料

User Story：使用者可以檢查 MaleCNS 三種原始檔的 schema，並以 Go 逐批讀取，保留原始型別與未知值。

Blocked by：01；真實檔案驗收依賴 06

Status：done（Feather SDK 與 CLI，Mac／Ubuntu 驗證）

## 流程與契約

SDK 先檢查一般檔案、容量與 Arrow IPC file magic，再以固定 Arrow Go v17 讀取 schema 與 record batches。呼叫者透過同步 callback 消費當前 batch，生命週期與錯誤必須明示。CLI 提供只讀檢查命令，報告欄位、批次、資料列與容量，不建立整張 DataTable。

此格式 adapter 不決定神經元篩選、不聚合邊，也不把資料列當成已訓練模型。來源仍保存為原始檔，正式圖與視圖另依 DAT-03 以後的需求建立。

## 驗收

- [x] 官方三檔及小型 Go 產生的 Feather V2 fixture 均可讀取，含字典、nullable、list 與大於 2^53 的整數。
- [x] 錯誤 magic、Feather V1、重複欄位、未知／不支援型別、損壞 footer 或 record 回傳錯誤。
- [x] 開啟前與逐批之間可取消，callback 錯誤停止後續處理並釋放 reader。
- [x] 檔案、footer、資料列及 Arrow 配置量有明確上限，超限拒絕，不把一部分資料標成完整匯入。
- [x] 拒絕 symlink、FIFO 及其他特殊檔案，原件不被修改。
- [x] CLI help、容量參數、完整與失敗報告可查；官方來源指紋及實際日誌保存。

容量報告要區分 Arrow 配置器追蹤的位元組與整個程序的實際記憶體，不能互相代替。

## 官方來源與整合驗證

固定 v4 來源完成 [Mac](../../evidence/cpu-reference-20260913/macos-v4/validation.log) 與 [Ubuntu](../../evidence/cpu-reference-20260913/ubuntu-v4/validation.log) 完整驗證。三份官方原件的 CLI 掃描皆為 `complete=true`，掃描前後 SHA-256 一致，命令與指紋見 [驗證紀錄](../../evidence/malecns-source-20260913/cli-v4/verification.json)。

| 原件 | 批次 | 資料列 | Arrow 配置峰值（bytes） |
| --- | ---: | ---: | ---: |
| annotations | 4 | 211,577 | 22,485,440 |
| neurotransmitters | 29 | 1,835,518 | 9,165,504 |
| weights | 2,318 | 151,856,684 | 2,138,240 |

這些是原始資料列數，神經元篩選與標準化圖另行驗收。Windows 僅交叉編譯通過，未執行實機測試。

## SDK 驗證紀錄

- `go test -count=1 ./feather`：通過，涵蓋產生的 Feather V2 fixture、nullable/list/dictionary 實際值、超過 2^53 的 `int64`／`uint64`、ZSTD 解壓、錯誤格式、損壞 record、重複欄位、取消、callback 錯誤、容量限制、symlink、FIFO 與原件不變。
- `go test -race -count=1 ./feather`：通過。
- `go vet ./feather`：通過。
- `evidence/DAT-02/sdk-local.json` 保存環境、fixture 輸入指紋、限制語意及結果；三個命令的輸出保存在同目錄的 log。

`Scan` 在 callback 前檢查固定寬度 buffer、字串與 list offset、巢狀 child 及非 null dictionary index，將 Arrow parser 或這些檢查的 panic 轉成錯誤；callback 自身的 panic 會原樣傳回呼叫端。`MaxArrowBytes` 是 Arrow allocator 的配置追蹤，不代表程序 RSS，也不保證惡意檔案在所有 parser 路徑都不造成其他記憶體壓力。
