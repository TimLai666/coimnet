# MOD-02 化學配置容量對抗式審查

基準：`57cc92d2864bd19f2424a4daa57edad60ad2763d`。

候選來源與凍結測試指紋均與目前工作樹相符：`source-final.json` 有 791 筆，檔案 SHA-256 為 `601b622639eb179f7f8d259791099050c658863dd8570009628878212331817f`；`frozen-tests.json` 檔案 SHA-256 為 `7b2c8458e44441ac2e15adba737404a083f59966300e865f55c1afdd87c83d14`。逐筆比對沒有 MISSING 或 MISMATCH。

## Confirmed findings

未找到可由目前程式、完整差異與 bounded 動態驗證成立的 P0、P1 或 P2 缺陷。

## Needs investigation

- Replay 沒有獨立的純量容量檢查，寬度來自既有矩形 trace，並在 `modulation/sources.go:224-233` 複製既有列。這符合本票禁止捏造巨大 slice 的範圍，因此目前不是已確認缺陷。缺少的事實是支援的外部呼叫者是否能提供接近執行環境容量邊界、且已經存在的合法矩形列。若要驗證，應在隔離程序以實際來源的既有 trace 做 bounded 檢查，不新增巨大配置。

## Changed

| 檔案 | 變更摘要 |
|---|---|
| `/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/review/report.md` | 新增本次唯讀審查報告。 |

## Verified

- `/Users/timlai/Developer/coimnet/modulation/capacity.go:13-35` 使用 portable `MaxInt`，先以除法檢查再做乘法，網格檢查包含外層 slice 元素 header 與每列 float64 位元組數。
- `/Users/timlai/Developer/coimnet/modulation/chemistry.go:69-84` 在 `lambda`、`alpha`、`rate` 與後續狀態配置前拒絕不可表示的濃度或 transport 網格。
- `/Users/timlai/Developer/coimnet/modulation/controller.go:96-100,129-142,191-199` 與 `/Users/timlai/Developer/coimnet/modulation/memory_controller.go:72-74,98-114,146-153` 在 ring 複製／observe 前驗證容量，成功計算前不提交 window、cache 或 offset。
- `/Users/timlai/Developer/coimnet/modulation/sources.go:37-43,106-115,174-205`、`/Users/timlai/Developer/coimnet/modulation/config.go:41-49` 涵蓋 standalone source、`SourceSpec.Build`、`Release` 與 controller source；`Channel+1` 不可表示時回傳 error。
- `/Users/timlai/Developer/coimnet/modulation/effects.go:92-106` 在 occupancy 掃描、effect group 與三個 node 陣列配置前檢查 nodes；`BackwardEffects` 先經由同一入口。
- `/Users/timlai/Developer/coimnet/learning/chemistry.go:117-141,291-315` 先完成 chemistry runtime 驗證與建構，最後才替換 individual runtime，拒絕時不提交持續狀態。
- 依指定隔離環境執行：`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMODCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-mod-cache GOCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-cache GOTMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-tmp TMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-tmp go test -count=1 -json -timeout=3m -run '^(TestChemicalCapacity|TestIndividualChemicalCapacity)' ./modulation ./learning`，exit code 0。
- `git diff --check` exit code 0；三個新增檔案的 `git diff --no-index --check` 均無 whitespace 診斷。

## 未驗證範圍

- 本票明確排除可表示但可能造成 OOM 的一般大型配置與巨大 allocations。
- 本次沒有重跑完整 `go build ./...`、全套 unit、race、vet 或相依性檢查，這些由 Root agent 執行中。
- 沒有用跨架構執行環境動態執行 32-bit 測試；portable 算術由 `strconv.IntSize` 與 source 內容靜態核對。

## Skills

Skills：diff-inspector
