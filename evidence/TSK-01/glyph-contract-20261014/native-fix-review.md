# Ticket 95 容量契約審查

## Problem

`glyphs.Generate` 原本會讓不可由 `int` 表示的樣本容器、像素緩衝或含括端點的亂數長度範圍一路進入 runtime，造成 `IntN` 或 `makeslice` panic；`RunOCRFixture` 也需要在建立 seed、核心與訓練資料前拒絕同一類宣告。

## Done

本次以唯讀方式審查指定 diff、兩個新增容量回歸、既有 glyph／fixture 型別與呼叫鏈，以及 `red.json`、`probe-observed.json`、`green.json`。沒有確認的 P1/P2 缺陷。

### Confirmed Findings

無確認缺陷。

### 契約判定

- `/Users/timlai/Developer/coimnet/tasks/ocr/glyphs/render.go:109-112` 以 `n > floor(MaxInt/sizeof(Sample))` 拒絕外層 `[]Sample` 容量溢位。這正是 `make([]Sample, 0, n)` 的 `int` 可表示性檢查，不是可用 RAM 或 Go runtime 可配置量保證。
- `/Users/timlai/Developer/coimnet/tasks/ocr/glyphs/render.go:113-119` 的上限等價於 `8*(9L-1)*sizeof(float64) <= MaxInt`。令 `W=floor(MaxInt/64)`，合法長度為 `L <= floor((W+1)/9)`，所以 `width=9L-1`、`width*8` 與像素位元組乘積均不溢位。
- 同一個最大字長界線也支配目前的中介配置：每個 rune 最多 4 bytes 的 UTF-8 表示、`[]rune` 每元素 4 bytes，以及 `Box` 陣列每元素的固定欄位大小，都小於每字元由 `64*(9L-1)` bytes 提供的像素界線。`Render` 先做 `[]rune` 轉換，再於 `/Users/timlai/Developer/coimnet/tasks/ocr/glyphs/render.go:19-24` 呼叫共用檢查；對可由目前程序實際交付的正常輸入，寬度與索引計算保持原契約。
- 對 `n>0`，該字長界線遠小於 `MaxInt`，因此 `/Users/timlai/Developer/coimnet/tasks/ocr/glyphs/render.go:80` 的 inclusive `IntN(maxLen-minLen+1)` 不會再形成非正數或範圍溢位。`n==0` 在 `/Users/timlai/Developer/coimnet/tasks/ocr/glyphs/render.go:106-107` 先返回，保留未使用的寬字長範圍與既有空批次結果，也不會呼叫 `IntN`。
- `/Users/timlai/Developer/coimnet/tasks/ocr/fixture.go:169-177` 顯示 `RunOCRFixture` 先執行 `FixtureConfig.Validate`，再複製 seed、算 config hash；seed 迴圈在 `:189-193`，資料生成與核心建立在 `:217-235`。因此 `/Users/timlai/Developer/coimnet/tasks/ocr/fixture.go:95-100` 的訓練／測試尺寸檢查會在 seed、core、training 前拒絕。
- `glyphs.Sample`、`Image`、`Box`、`FixtureConfig`、`OCRReport` 欄位與 JSON 標籤沒有變更；正常輸入仍沿用同一個亂數建構、渲染與模型格式。`Invert` 字距欄位的文件描述與實際值仍是既有待釐清事項，未列為本票缺陷，也未改變其行為。

### Needs investigation

- `[NEEDS INVESTIGATION] /Users/timlai/Developer/coimnet/tasks/ocr/glyphs/render.go:78-81`：共用檢查只保證 `n*sizeof(Sample)` 能由 `int` 表示，沒有保證 Go runtime `maxAlloc` 或實體 RAM。當 `n` 等於 `floor(MaxInt/sizeof(Sample))` 時，檢查會接受，但在多數 64 位 Go runtime 上，`make([]Sample, 0, n)` 仍可能因 `maxAlloc` 在真正配置前回 `makeslice: cap out of range`。缺少的事實是本專案是否要把 runtime 可配置上限納入公開契約，以及各支援 GOARCH 的 `maxAlloc`／資源錯誤語意。安全查證方法：先從目標 Go runtime 取得該 GOARCH 的 `maxAlloc`，選一個 `n` 使 `n*sizeof(Sample) <= MaxInt` 且 `n*sizeof(Sample) > maxAlloc`，以 `defer recover` 呼叫 `Generate(..., n, 1, 1, ...)`；runtime 應在配置前拒絕，不應真的配置大陣列。此 recipe 未執行，不能把它升級成已驗證缺陷，也不能把目前檢查宣稱成所有大型宣告安全。
- `[NEEDS INVESTIGATION] /Users/timlai/Developer/coimnet/tasks/ocr/glyphs/render.go:19-21`：`Render` 在共用檢查前先建立 `[]rune(text)`。若外部已能提供超過 runtime 配置上限的字串，該中介可能在 `ValidateGenerateDimensions` 之前 panic；需先定義是否要保證這類 runtime 級輸入，以及在不配置大字串的前提下設計可控的入口測試。這不是本輪已確認的 int 溢位缺陷。

## Reviewed files

- `/Users/timlai/Developer/coimnet/tasks/ocr/glyphs/render.go`
- `/Users/timlai/Developer/coimnet/tasks/ocr/fixture.go`
- `/Users/timlai/Developer/coimnet/tasks/ocr/glyphs/generate_capacity_test.go`
- `/Users/timlai/Developer/coimnet/tasks/ocr/fixture_capacity_test.go`
- `/Users/timlai/Developer/coimnet/tasks/ocr/glyphs/glyphs.go`
- `/Users/timlai/Developer/coimnet/tasks/ocr/glyphs/glyphs_test.go`
- `/Users/timlai/Developer/coimnet/tasks/ocr/glyphs/options_validation_test.go`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/red.json`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/probe-observed.json`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/green.json`
- `/Users/timlai/Developer/coimnet/AGENTS.md`
- `/Users/timlai/Developer/coimnet/README.md`
- `/Users/timlai/Developer/coimnet/docs/initialization.md`
- `/Users/timlai/Developer/coimnet/docs/handoff/AGENT_START_HERE.zh-TW.md`
- `/Users/timlai/Developer/coimnet/docs/handoff/CoImNet_Implementation_Plan.zh-TW.md`
- `/Users/timlai/Developer/coimnet/ENG.md`
- `/Users/timlai/Developer/coimnet/docs/resources.md`
- `/Users/timlai/Developer/coimnet/docs/tickets/95-ocr-generate-representability.md`

## Evidence and unverified items

- `probe-observed.json:2-17` records the three pre-fix public Generate panics: inclusive `IntN`, sample slice capacity and fixed-length rune allocation. `red.json:11-22` records the expected failing regression run. `green.json:11-22` records the final targeted run with exit code 0, 20 test pass events and 0 failures.
- This review did not run Go, compile, race, vet, full tests, or any large allocation. The green evidence is read from the supplied artifact and was not independently rerun here.
- Physical RAM, allocator headroom, runtime `maxAlloc`, direct oversized `Render` text, and complete project validation remain unverified.

## Next

無。若要擴大公開契約至 runtime／資源安全，應另票先定義跨平台上限與錯誤語意，再補不配置大記憶體的回歸。
