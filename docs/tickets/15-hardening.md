# 15 — 維護者可以信任 JSON 深度防護、下載續傳與邊序防護

Epic：狀態保存與資料取得（強化）

User Story：維護者可以在訊號 JSON、官方原件下載與參數推導三處，不再依賴「呼叫端不會給壞輸入」
或「程序不會被砍」的假設。

Blocked by：03 訊號、06 下載、13 參數 adapter

Status：verified_scoped（三項強化皆已驗證；`signal` 的深度上限在本票前已存在，本票只是改用共用的 `internal/strictjson`）

對應需求：STA-06 的一部分（輸入防護）、DAT-01／DAT-02 的續傳保證、NAT-02 的邊序假設。
不新增功能，不改公開格式的位元組。

## Root 決策（2026-09-15）

1. **訊號 JSON**：`signal/json.go` 自帶的 `decodeStrict`／`rejectDuplicateKeys`／`scanJSONValue`
   沒有深度上限，遞迴可被深巢狀輸入打爆。改為呼叫 `internal/strictjson.Decode`（64 層、路徑堆疊、
   大小寫別名重複鍵、未知欄位、尾隨資料），保留 `validateJSONUnicode` 與既有錯誤訊息語意；
   `jsonNumber`／`jsonValues` 的 null 拒絕行為不變。既有 `signal` 測試全綠，另加深巢狀拒絕測試。
2. **下載續傳**：`download.Fetch` 目前只在一次傳輸結束時把 `bytes` 寫回 `.part.meta.json`，程序被砍
   就留下 `bytes: 0` 對上非零 `.part`，重跑被拒。改為：(a) 傳輸中每累積 `CheckpointBytes`
   （預設 64 MiB，`Options` 可調）就 `part.Sync()` 後寫回 meta；(b) 啟動時若 `.part` 長度大於 meta
   的 `bytes`，把 `.part` 截到 meta 記錄的長度（那之前的位元組已 fsync）再續傳，並在回條記
   `resumed_from_bytes` 與 `truncated_bytes`；`.part` 短於 meta 仍拒絕（資料遺失）；(c) `.lock`
   內的 pid 不存在時（`syscall.Kill(pid, 0)` 回 ESRCH）視為失效鎖並回收，回條記 `reclaimed_stale_lock`；
   pid 存在或無法判斷一律不回收。整檔 CRC32C／SHA-256 驗證不變。測試用 `httptest` 伺服器加
   Range 支援，模擬「傳到一半被砍」（手動製造 `.part` 長於 meta）、「`.part` 短於 meta」、
   「失效 pid 鎖」、「活著的 pid 鎖」四種情況。
3. **邊序防護**：`params/derive.go` 的 `alignEdges` 信任 connectome 的 `(source, target)` 遞增順序；
   加一個便宜的執行期檢查，順序倒退即回錯（不是靜默錯配），測試以假的邊序證明會被擋下。
   重複 pair 緩衝的記憶體保留改為只在容量成長時計入差額。

## 契約

```go
// download
type Options struct { ...; CheckpointBytes int64 }   // 0 = 預設 64 MiB
type Receipt struct { ...; ResumedFromBytes int64 `json:"resumed_from_bytes,omitempty"`; TruncatedBytes int64 `json:"truncated_bytes,omitempty"`; ReclaimedStaleLock bool `json:"reclaimed_stale_lock,omitempty"` }
```

`signal` 與 `params` 不改公開 API。

## 驗收

- [x] `signal`：深巢狀 JSON（65 層）被拒且錯誤含路徑；重複鍵（含大小寫別名）、未知欄位、尾隨
  資料、`values:[null]` 行為與改前相同；既有測試全綠。
- [x] `download`：四種續傳情境測試；續傳後整檔 SHA-256 與一次下載完成相同；回條新欄位只在發生時
  出現；既有測試全綠。
- [x] `params`：邊序倒退被擋下；既有 fixture 與真實資料報告不受影響（`data validate --params`
  對 `data/malecns-v1.0/params-derive-v1.coimparams` 仍通過，hash 不變）。
- [x] `go test`、race、vet；ticket 06／13 與 delivery-status 的待辦條目移除或標記已修。（root 於
  收件時更新 delivery-status 與 ticket 13 的待辦；root 另跑三個套件的 gofmt／vet／test／race。）

## 證據（2026-09-15，`evidence/hardening-20260915/`）

環境：darwin/arm64、go1.26.5、`up 9 days`、load average 1.46–2.70。

### 先寫失敗測試

- `red-signal.log`：**沒有紅燈**。`signal/json.go` 在本票之前就已經帶著 `maxJSONDepth = 64`
  與 `TestDecodeSignalRejectsExcessiveJSONNestingWithLongKeys`，所以票面第 1 點「沒有深度上限」
  的前提已經過期；新加的 65／64 層雙向測試在改動前就是綠的。本票第 1 點實際做的是把重複的私有
  嚴格解碼器換成 `internal/strictjson`，不是補上深度上限。
- `red-download.log`：真紅燈（編譯失敗）。`Options.CheckpointBytes`、`Receipt.ResumedFromBytes`、
  `TruncatedBytes`、`ReclaimedStaleLock` 都還不存在。
- `red-params.log`：真紅燈（編譯失敗）。`orderedEdges` 還不存在。

### 下載四種情境

| 測試 | 佈置 | 斷言 |
| --- | --- | --- |
| `TestFetchTruncatesPartLongerThanMetadataAndResumes` | `.part` = 前 1024 B 正確內容 + 700 B 未 fsync 的垃圾，meta 記 1024 | 截到 1024 後以 `Range: bytes=1024-` 續傳；整檔 SHA-256 與同一份來源一次下載完成的回條相同；`resumed_from_bytes` 1024、`truncated_bytes` 700；檔案內容等於來源；`.part`／`.meta`／`.lock` 都清掉 |
| `TestFetchRefusesPartShorterThanMetadata` | `.part` 512 B，meta 記 1024 | 拒絕且錯誤含 “preserved”；`.part` 與 meta 內容都沒被改；目標檔沒有產生 |
| `TestFetchReclaimsLockOfDeadProcess` | `.lock` 寫入已 `Wait()` 回收的子行程 pid | 下載成功；回條 `reclaimed_stale_lock: true`；內容正確；無殘留檔案 |
| `TestFetchRefusesLockOfLiveProcess` | `.lock` 寫入測試行程自己的 pid | 拒絕且錯誤含 “lock”；伺服器完全沒被打到；鎖檔內容原封不動 |

另有 `TestFetchCheckpointsMetadataDuringTransfer`：`CheckpointBytes` 512、body 8192 B，伺服器送出
4096 B 後卡住，測試從磁碟讀到 `.part.meta.json` 的 `bytes` 已經 ≥ 512 且 < 8192，放行後整檔完成。
無擾動情境的回條 JSON 不含三個新欄位，續傳情境含 `resumed_from_bytes` 與 `truncated_bytes`
而不含 `reclaimed_stale_lock`，這一條就是「只在發生時出現」的證據。

### 真實資料

`go run ./cmd/coimnet data validate --params data/malecns-v1.0/params-derive-v1.coimparams
--store data/malecns-v1.0/graph-v1.coimgraph` → `validate-params.json`，6.3 s 牆鐘、離開碼 0、
`verified: true`。整份報告與 `evidence/NAT-02/derive-v1/validate.json` 逐鍵相同，參數集檔
SHA-256 仍是 `c4db0f3f93390b66c417cee2a96d805179e4fb178678615ec537260710c58c77`。

### 偏離與未驗證

1. `signal` 的重複鍵錯誤字樣從 `duplicate JSON field` 變成 `internal/strictjson` 的
   `duplicate JSON key`，路徑格式從 `$.a[0]` 變成 `$.a.0`。拒絕行為不變，但兩個既有測試的字串斷言
   跟著改。不改 `internal/strictjson` 是本輪的檔案責任。
2. 失效鎖的存活判斷用 `os.FindProcess(pid).Signal(syscall.Signal(0))`，不是票面寫的
   `syscall.Kill(pid, 0)`。在 Unix 上就是同一個 `kill(pid, 0)` 探測（ESRCH 視為不存在、EPERM 視為
   存在），但這個寫法可以放在 `download.go` 單一檔案裡而不必開 build tag 檔，Windows 交叉編譯也通過。
   無法判斷（其他 errno）與讀不出 pid 的鎖一律不回收。
3. `resumed_from_bytes` 只要續傳既有 `.part` 就會寫出，不限於發生截斷的情況；`truncated_bytes`
   只在真的丟掉位元組時寫出。
4. 重複 pair 緩衝改成只記 `cap` 實際成長的差額。`pair-buffer-accounting.log`：一個 20,000 條邊的
   pair，舊式記帳向預算要 488,680 B，緩衝實際只佔 172,032 B（2.8 倍高估），新式記帳正好等於
   172,032 B。但**沒有新測試**：`params` 既有 graph fixture 沒有
   重複 pair，`groupRaw` 永遠只有一個元素，要造出重複 pair 需要動 `params/fixture_test.go`，超出本輪
   的檔案責任。既有 fixture 與真實資料的 `peak_accounted_bytes` 不受影響。
5. 邊序防護只掛在推導路徑（`alignEdges`）。`data validate --params` 讀檔不重跑推導，所以真實資料那一
   行證明的是讀檔與報告不受影響，不是防護在全腦上被觸發過。
6. 最後一個驗收項沒有勾，只差後半句。`verify-all.log` 的定版一輪：`gofmt -l .` 無輸出、
   `go vet ./...` 無輸出、`go test -count=1 ./...` 全套件 ok、`go test -race -count=1`
   對 `signal`／`download`／`params` 全綠。（過程中一度在 `dynamics` 編譯失敗，那是另一個 agent 的
   ticket 16 尚未落地的中間狀態，本輪沒有碰那個套件，定版時已恢復。）ticket 06／13 與
   `delivery-status.md` 的待辦條目不在本輪的可改檔案清單內，沒有動，所以這一項留給下一輪收尾。

## 依據

- delivery-status「目前阻礙」的續傳缺口紀錄；ticket 03 的 `signal/json.go` 深度待辦；
  ticket 13 root 審查的邊序防護。
