# 15 — 維護者可以信任 JSON 深度防護、下載續傳與邊序防護

Epic：狀態保存與資料取得（強化）

User Story：維護者可以在訊號 JSON、官方原件下載與參數推導三處，不再依賴「呼叫端不會給壞輸入」
或「程序不會被砍」的假設。

Blocked by：03 訊號、06 下載、13 參數 adapter

Status：ready（契約已於 2026-09-15 定案；驗收項目驗證後才勾選）

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

- [ ] `signal`：深巢狀 JSON（65 層）被拒且錯誤含路徑；重複鍵（含大小寫別名）、未知欄位、尾隨
  資料、`values:[null]` 行為與改前相同；既有測試全綠。
- [ ] `download`：四種續傳情境測試；續傳後整檔 SHA-256 與一次下載完成相同；回條新欄位只在發生時
  出現；既有測試全綠。
- [ ] `params`：邊序倒退被擋下；既有 fixture 與真實資料報告不受影響（`data validate --params`
  對 `data/malecns-v1.0/params-derive-v1.coimparams` 仍通過，hash 不變）。
- [ ] `go test`、race、vet；ticket 06／13 與 delivery-status 的待辦條目移除或標記已修。

## 依據

- delivery-status「目前阻礙」的續傳缺口紀錄；ticket 03 的 `signal/json.go` 深度待辦；
  ticket 13 root 審查的邊序防護。
