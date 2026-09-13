# 06 — 使用者可以保存並續傳官方資料

Epic：真實接線資料

User Story：使用者可以下載、驗證並續傳官方 MaleCNS 原始檔案。

Blocked by：01

Status：done（官方原件取得、SDK 與 CLI）

## 交付與流程

SDK 接受來源、輸出位置與明確容量上限，先檢查來源及磁碟，再寫入暫存。中斷時保留有版本及 ETag 的進度，下一次下載先驗證伺服器版本及 Range 回應。完整檔案校驗後以不覆寫方式發布，回傳來源、大小、取得時間、標頭及雜湊性質。CLI 可下載並保存回條。

## SDK API

```go
type Options struct {
	MaxBytes       int64
	Retries        int
	ExpectedSHA256 string
	ExpectedCRC32C string
}

type Receipt struct {
	SchemaVersion  string `json:"schema_version"`
	URL            string `json:"url"`
	ETag           string `json:"etag"`
	StatusCode     int    `json:"status_code"`
	ContentLength  int64  `json:"content_length"`
	ContentType    string `json:"content_type,omitempty"`
	LastModified   string `json:"last_modified,omitempty"`
	AcceptRanges   string `json:"accept_ranges,omitempty"`
	ProviderHashes map[string]string `json:"provider_hashes,omitempty"`
	AcquiredAt     string `json:"acquired_at"`
	SHA256         string `json:"sha256"`
	HashStatus     string `json:"hash_status"`
	Bytes          int64  `json:"bytes"`
	// UpstreamCRC32C remains for compatibility and mirrors provider_hashes.crc32c.
	UpstreamCRC32C string `json:"upstream_crc32c,omitempty"`
}

func Fetch(ctx context.Context, client *http.Client, url, path string, options Options) (Receipt, error)
```

`MaxBytes` 必須為正數，`Retries` 限定為 0 至 3；兩個 Expected 值是呼叫者提供的官方校驗值，SHA-256 使用十六進位字串，CRC32C 使用 GCS `X-Goog-Hash` 的 base64。來源若回傳 `X-Goog-Hash: crc32c=...`，SDK 會自動計算並比對完整檔案，即使呼叫者沒有另傳 ExpectedCRC32C。

Receipt 的 `SchemaVersion` 固定為 `coimnet-download-receipt/v1`。

回條中的 `SHA256` 永遠是本機計算值，`UpstreamCRC32C` 單獨保留來源標頭。只有 Expected 值或來源 CRC32C 實際比對成功時才回傳 `HashStatus: upstream_verified`，否則為 `locally_recorded`。

Receipt 另外保存 HEAD 成功回應中可重現且非敏感的公開 allowlist：來源 URL、status、Content-Length、Content-Type、Last-Modified、Accept-Ranges、ETag，以及 `X-Goog-Hash` 中支援的 `crc32c`／`md5`。不會把任意回應標頭（例如授權資料）寫入回條；GET 仍會驗證 ETag、長度及 HEAD／GET 之間可用 provider hash 的一致性，CRC32C 另會比對完整下載內容。

下載進度放在 `<path>.part` 與 `<path>.part.meta.json`，metadata 只接受版本、URL、ETag、長度及目前位元組數，並拒絕重複（含大小寫別名）、未知欄位、尾端 JSON、非 regular 檔案及超過 64 KiB 的 metadata。`<path>.lock` 以 `O_EXCL` 建立，既有 lock 會明確視為可能的 stale lock。完整 `.part` 以同目錄 hardlink 發布到目標，既有完整目標與未知暫存檔不會被覆寫或刪除；自己的有效 partial 會隨續傳進度更新，成功發布後清理。

Unix（Darwin、Linux）以 `x/sys/unix.Statfs` 檢查可用空間，Windows 使用 `GetDiskFreeSpaceEx`；其他平台會明確回報不支援。hardlink 與目錄同步要求檔案系統支援相應能力，發布後的目錄同步或取消錯誤會明確指出檔案已發布但耐久性尚未確認。

## 驗收

- [x] 真實官方檔與本機 HTTP 測試伺服器皆有可重現紀錄。
- [x] 未知／過大來源、磁碟不足、錯誤長度、破損內容及空路徑拒絕。
- [x] 取消保留可續傳進度，重試有次數上限，傳輸或校驗失敗不發布完整檔名；發布後的耐久性或輸出錯誤明確回報已發布。
- [x] Range 被忽略、ETag 改變或 Content-Range 不符時拒絕拼接。
- [x] SHA-256 自行計算標為 `locally_recorded`，只有實際比對官方校驗值才標 `upstream_verified`。
- [x] 同一路徑並行只有一個下載者，既有完整檔不可覆寫，未知暫存檔不被刪除或重設。
- [x] SDK 測試涵蓋空回應、非 2xx、網路中斷及錯誤回條。
- [x] CLI help 及整合後錯誤行為可查。

## SDK 驗證紀錄

- 先執行 `go test -count=1 ./download`，新增回應 `Read` count、`Body.Close` 與 Receipt 欄位測試後，因測試所需 helper 與 Receipt 欄位尚未實作而編譯失敗（RED）。
- `gofmt -d download/*.go`：通過。
- `go test ./download`：通過，含 fresh、取消續傳、有限重試、Range／ETag／長度、官方 CRC32C、自訂 checksum、磁碟空間、strict metadata、非 regular artifact、lock／overwrite／並行、錯誤 response body close、Read count 邊界及公開回應標頭 allowlist 測試。
- `go test -race ./download`：通過。
- `go vet ./download`：通過。
- `GOOS=linux GOARCH=amd64 go test -c ./download` 與 `GOOS=windows GOARCH=amd64 go test -c ./download`：通過，平台磁碟空間實作可編譯。

三份官方原件的完整下載回條見 [來源紀錄](../malecns-source-audit.md)。固定 v4 來源另以 annotations 真實檔驗證自動讀取並比對伺服器 CRC32C，沒有提供 checksum 旗標，見 [CLI 來源驗證](../../evidence/malecns-source-20260913/cli-v4/verification.json)。完整 SDK、CLI、race、vet 與跨程序續訓見 [Mac](../../evidence/cpu-reference-20260913/macos-v4/validation.log) 及 [Ubuntu](../../evidence/cpu-reference-20260913/ubuntu-v4/validation.log)。

此工作只取得原始檔，Feather 匯入、圖篩選與神經訓練另依 DAT-02 以後的需求驗收。
