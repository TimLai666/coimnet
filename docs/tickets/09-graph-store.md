# 09 — 使用者可以保存並讀回標準化接線圖

Epic：真實接線資料

User Story：研究者可以把 08 產生的 `annotated_neurons` 視圖與報告以固定格式落盤，
之後不必重跑 151M 列掃描就能驗證並讀回同一張圖。

Blocked by：08 標準化接線圖

Status：done（in-process store／CLI／真實資料讀回；接上訓練核心屬 DAT-07）

## 交付

`connectome.Save` 把 `Graph` 寫成單一檔案並以不覆寫方式發布；`connectome.Load`
逐段校驗後重建同一個 `Graph`（節點、邊、報告、weights 來源與掃描設定），
讀回的圖可在 weights 原件仍可用時再串流 raw view。`LoadWithReceipt` 同時回傳實際通過驗證的整檔 SHA-256，載入回條不宣告寫入耐久性。CLI `data import --out-store FILE` 建構後直接落盤，
`data validate --store FILE` 在新程序讀回並輸出驗證摘要。

## Root 決策（2026-09-13）

1. **格式**：自訂固定寬度區段加 JSON footer，不用 Arrow IPC。理由：區段可逐段
   SHA-256、順序與寬度由 footer 明示、寫入單趟不需回填，讀回驗證與 08 的 hash
   直接對應；Arrow IPC 需要 schema／dictionary 生命週期與另一套 allocator 限制。
   版本升級時沿用主規格 15.6：新檔、保留來源、報告前後指紋。
2. **檔案配置**：`magic "COIMGRF1"` → 區段 `node_ids`（u64 BE）、`node_meta`（位置、
   九個 nullable 字串、傳導物質預測）、`edges`（索引依 footer 的 4／8 位元組寬度、
   絕對列、nullable weight、聚合列數）、`batch_starts`、`report`（GraphReport JSON）
   → footer JSON（schema 版本、converter 版本、manifest／predicate hash、視圖模式、
   計數、各區段 offset／length／SHA-256、node index／edge order／report hash、
   weights 來源檔與掃描設定）→ footer 長度、footer SHA-256、magic。檔案不含時間
   戳，同一張圖兩次 Save 位元組相同。
3. **發布**：同目錄 0600 暫存檔，寫入並 `fsync` 後以 hardlink 發布，目標已存在即
   失敗，發布後同步目錄；目錄同步或取消失敗時回報「已發布但耐久性未確認」，不
   刪除已發布檔案。與 checkpoint 相同語意，實作各自保有。
4. **讀回驗證**：檔案大小、footer 大小與記憶體都有上限；footer SHA-256、每段
   SHA-256、schema 版本、區段連續且不重疊、節點 ID 嚴格遞增、邊索引小於節點數且
   依 canonical 順序、絕對列小於 raw 列數；重新計算 node index／edge order／report
   hash 並與 footer 及報告內的值比對，任一不符以 `ErrStoreCorrupt` 拒絕。
5. **raw view**：不複製 1.05 GB 原件，只保存 weights 的 `SourceFile`（含本機路徑）
   與掃描設定；讀回後 `StreamRawSegments` 仍先比對指紋。
6. **需求對應**：本票是主規格 5.2 衍生物與 15.2／15.6 原則的實作，不單獨標記需求
   通過；DAT-07（真實子圖）待保存明示選取條件並實際用於實驗時再驗收；既有 manifest 已支援單一註記欄位相等選取。

## 驗收

- [x] 同一張圖 Save 兩次得到位元組相同的檔案；Load 後節點、邊、`NeuronIDs`、
  `IndexOf`、報告與 hash 皆與原圖一致，raw view 仍可串流且指紋改變時拒絕。
- [x] rows 與 aggregated_pairs 兩種視圖與空圖都能往返。
- [x] 目標已存在、父目錄不存在或取消時不發布，也不留暫存檔。
- [x] 竄改任一區段或 footer、截斷、錯 magic、錯 schema 版本、內容順序或索引不合法
  （區段 hash 正確仍檢查結構）都以明確錯誤拒絕；檔案、footer 與記憶體超限回 `ErrCapacity`。
- [x] CLI `data import --out-store` 與 `data validate --store` 有端到端與失敗測試。
- [x] 真實資料：08 的圖落盤後在新程序讀回驗證，保存命令、大小、SHA-256、時間與
  hash 比對結果。

## 驗證證據

- SDK：`connectome/store_test.go` 覆蓋往返（位元組相同、節點／邊／報告／hash 相等、讀回後 raw view 串流與指紋改變拒絕）、aggregated 與空圖、既有目標／缺父目錄／取消不發布不留暫存、竄改每個區段與 footer、截斷、錯 magic、錯 schema 版本、三種限制超限，以及區段 hash 正確但順序／索引／節點 ID／報告不合法的內容。Red 狀態：實作前 `go test ./connectome/` 因 `Save`、`Load`、`StoreLimits` 不存在而編譯失敗。
- CLI：`internal/cli/import_test.go` 的 `TestDataImportStoreAndValidateRoundTrip` 覆蓋 `--out-store`、重複落盤拒絕、`data validate` 成功與失敗案例、help 與總覽。
- 真實資料：建構＋落盤 158.97 秒、RSS 2.93 GB，store 664,407,846 bytes；新程序讀回 2.63 秒、RSS 753 MB，`verified=true`，hash 與報告與 import 輸出相等，node index／edge order hash 與 08 的 graph-v1 相同。見 [graph-store-v1](../../evidence/malecns-source-20260913/graph-store-v1/verification.json)。

## 接手修正與跨平台驗證

- 配置前計入 footer／report 輸入、解碼輸入副本與讀取緩衝；先檢查節點配置乘積與平台索引上限，再分項保留額度，避免加總溢位。此額度不包含解碼後 JSON 物件、Go 配置餘量及執行環境。
- 拒絕未初始化圖與未知 converter；已初始化空圖仍可往返。
- 路徑被替換的回歸測試證明 `LoadWithReceipt` 回報原先驗證位元組的 hash，不混用另一份檔案。
- `store_limits_test.go` 與 `store_receipt_test.go` 補上配置限制、溢位、重新計算 hash 的未知 converter、取消與路徑替換案例。原實作實際出現未初始化圖成功保存、10 MiB footer／report 繞過 8 MiB 額度的失敗測試。
- Mac／Ubuntu 同一份 95 檔來源已通過完整 build、單元、race、vet、模組與跨程序恢復驗證，見 [v7 驗證](../../evidence/cpu-reference-20260913/verification-v7.json)。
- 真實圖在兩平台讀回、重新保存的獨立 SHA-256 都與既有圖相同，報告逐欄相等。CLI 單次讀回為 Mac 2.91 秒／752 MB、Ubuntu 34.05 秒／748 MB；保存往返與完整限制見 [graph-store-v2](../../evidence/malecns-source-20260913/graph-store-v2/verification.json)。

## 依據

- [08 標準化接線圖](08-connectome-graph.md)：canonical stream、報告與 hash 定義。
- [主規格 5.2、15.2、15.6](../handoff/CoImNet_Implementation_Plan.zh-TW.md#chapter-05)：衍生物可重建、快照原子性、相容性。
- [共用工程設計](../../ENG.md)：不覆寫、原子寫入、版本變更保存舊原件。
