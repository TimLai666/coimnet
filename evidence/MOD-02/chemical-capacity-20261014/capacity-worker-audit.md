# Chemical entry and declared capacity audit

日期：2026-10-14（Asia/Taipei）

## Confirmed findings

### F-01：來源通道向量允許位元組大小不可表示的寬度，`Release` 會 panic

觸發條件：

- `NeuralActivity{Channel: MaxInt-1}`。`Channel+1 == MaxInt`，`Validate()` 回傳 nil，`Release()` 在 `sources.go:149` 配置 `make([]float64, n.Channels())`。
- `InternalResource{Channel: MaxInt-1}`。`Release()` 的檢查只拒絕負數與 `MaxInt`，在 `sources.go:194` 配置 `make([]float64, r.Channels())`。
- `Controller{Channel: MaxInt-1, Nodes: 1, Hidden: 1, SummaryWindow: 1}`。`Validate()` 回傳 nil，`Release()` 在 `controller.go:178` 配置通道向量。
- `ExternalTimeline{ChannelCount: MaxInt}` 或 `MaxInt-1`。`Validate()` 只檢查 `ChannelCount <= 0`（`sources.go:37-40`），`Release()` 在 `sources.go:72` 配置通道向量。

實際錯誤：隔離子程序以 `recover` 觀察到 `runtime error: makeslice: len out of range`。所有上述長度的元素大小乘積都已超出 `int` 可表示的位元組大小，沒有嘗試配置可成功分配的大型記憶體。

公開組態入口也會放行同一條路徑：`SourceSpec.Build()` 接受 Neural、InternalResource、Controller 的 `Channel: MaxInt-1`，並接受 `SourceSpec.Channel: MaxInt-1` 搭配 `ExternalTimeline.ChannelCount: MaxInt`；Build 本身回傳 nil error，第一次 `Release()` 才 panic（`config.go:40-145`）。

影響：普通公開 API 輸入可由錯誤回傳契約外溢成 panic，違反主規格 16.1 的「錯誤回傳、不以 panic 回應普通輸入錯誤」，也沒有涵蓋 18.4 的超大形狀防護。

建議：在每個來源的公開驗證或 `Release()` 入口檢查向量長度及其元素大小的可表示性，使用數學上的 `int`／位元組邊界，不新增任意固定容量上限。

### F-02：Controller 與 MemoryController 的 `Nodes` 沒有容量驗證，首次活動視窗配置會 panic

觸發條件：

- `Controller{Nodes: MaxInt, Channel: 0, Hidden: 1, SummaryWindow: 1, Parameters: 5 個}`。`controller.go:84-95` 的 `Validate()` 回傳 nil，`Release()` 進入 `observe()`，在 `controller.go:210-214` 執行 `make([]float64, c.Nodes)`。
- `MemoryController{Nodes: MaxInt, Hidden: 1, SummaryWindow: 1, Parameters: 5 個}`。`memory_controller.go:64-90` 的 `Validate()` 回傳 nil，`Offset()` 交由同一個 `Controller.observe()` 配置節點列。

實際錯誤：兩條隔離子程序都回收 `runtime error: makeslice: len out of range`。輸入只配置了 1 個視窗列與 5 個參數，沒有預先配置大陣列。

影響：Controller 與容量匹配的 MemoryController 均可接受位元組大小不可表示的節點數，第一次公開運算會 panic，和 F-01 相同地違反 16.1／18.4。

建議：在兩個公開宣告的節點數驗證中加入可表示性檢查，沿用元素大小與 `int` 邊界，不以任意 1M 等數值取代算術檢查。

### F-03：Chemistry 接受 `Regions=MaxInt`，`NewState()` 會 panic

觸發條件：`Chemistry{Regions: MaxInt, Channels: 1, DT: 1, Tau: []float64{1}}`。`NewChemistry()` 在 `chemistry.go:56-105` 回傳非 nil kinetics、nil error，因為只檢查 `Regions >= 1`，而初始化陣列只依 `Channels` 配置。

實際錯誤：第一次呼叫 `Kinetics.NewState()` 在 `chemistry.go:169` 的 `make([][]float64, k.config.Regions)` 回收 `runtime error: makeslice: len out of range`。同一個宣告放進一個節點的 `ChemistryConfig`，`config.Validate(1)` 也回傳 nil（`config.go:201-255`）。

影響：化學宣告驗證與實際狀態建立之間沒有共同的區域容量門檻，使用者會先得到成功，再在建立初始狀態時 panic，違反 16.1／18.4。

建議：在 `NewChemistry` 或其唯一共用的形狀檢查中驗證區域矩陣／狀態外層配置的可表示性；不設定任意固定區域上限。

### F-04：相鄰的化學效果輸出也接受不可表示的節點數

觸發條件：公開呼叫 `ApplyEffects(nil, nil, MixSum, MaxInt)`。`effects.go:84-102` 只拒絕小於 1 的節點數，之後在 `effects.go:145` 配置三個 `[]float64` 輸出陣列。

實際錯誤：隔離子程序回收 `runtime error: makeslice: len out of range`。這是同一個公開化學層的容量問題，未修改正式檔案。

建議：若本輪修正納入化學層共用容量檢查，效果輸出應使用同一套算術可表示性檢查；不要為效果 API 另造固定上限。

## 全部來源清單與防護差異

| 公開型別 | 寬度／形狀來源 | 現有防護 | 安全診斷結果 |
|---|---|---|---|
| `NeuralActivity` | `Channel+1` | 拒絕負數與 `Channel == MaxInt`（`sources.go:103-109`），沒有 `MaxInt-1` 的 byte-size 檢查 | **Confirmed F-01**：`MaxInt-1` 通道在 `Release` panic |
| `InternalResource` | `Channel+1` | `Release` 拒絕負數與 `Channel == MaxInt`（`sources.go:168-196`），沒有 `MaxInt-1` 的 byte-size 檢查 | **Confirmed F-01**：`MaxInt-1` 通道在 `Release` panic |
| `ExternalTimeline` | `ChannelCount` 直接指定 | 只拒絕 `<= 0`（`sources.go:37-40`） | **Confirmed F-01**：`MaxInt`、`MaxInt-1` 都在 `Release` panic |
| `Controller` | `Channel+1`，另有 `SummaryWindow`／`Hidden` 上限 | 拒絕負數與 `Channel == MaxInt`（`controller.go:80-115`）；`Nodes` 只要求至少 1 | **Confirmed F-01/F-02**：通道 `MaxInt-1` 與節點 `MaxInt` 都在 `Release` panic |
| `MemoryController` | 沒有通道向量，活動視窗列寬為 `Nodes` | `Hidden`／`SummaryWindow` 有上限，`Nodes` 只要求至少 1（`memory_controller.go:60-90`） | **Confirmed F-02**：`Nodes=MaxInt` 在 `Offset` panic |
| `Replay` | 第一列 `len(Trace[0])`，每列須等寬 | 檢查非空、非零寬、矩形、有限且非負（`sources.go:206-248`）；沒有獨立 scalar width 上限 | 小型控制通過；沒有建立不可表示 row 的安全公開 fixture，未確認 panic |
| `Chemistry`／`Kinetics` | `Regions`、`Channels` 與 `Tau` 長度 | `Regions`／`Channels` 只要求至少 1，`Tau` 長度必須等於 Channels（`chemistry.go:56-105`） | **Confirmed F-03**：`Regions=MaxInt` 由 `NewChemistry` 接受，`NewState` panic；`Channels=MaxInt,Tau=[1]` 先回 error |
| `ChemistryConfig` | `Chemistry`、每通道一個 source、每節點一個 region | 呼叫 `NewChemistry`，但沒有額外區域 byte-size 檢查（`config.go:201-255`） | **Confirmed F-03**：一節點、`Regions=MaxInt` 的宣告 `Validate` 回 nil |
| `ApplyEffects` | `nodes` 個 gain／offset／threshold 陣列 | 只拒絕 `nodes < 1`（`effects.go:84-102`） | **Confirmed F-04**：`nodes=MaxInt` 在輸出陣列配置 panic |

## 正常小配置控制

同一個診斷二進位檔的小型控制全部成功：

- NeuralActivity channel 0：`rates=[0]`，nil error。
- InternalResource channel 0：`rates=[0]`，nil error。
- ExternalTimeline channel count 1：`rates=[0]`，nil error。
- Controller nodes 1／hidden 1／channel 0：`rates=[0.6931471805599453]`，nil error。
- MemoryController nodes 1／hidden 1：`offset=0`，nil error。
- Replay trace `[[0]]`：`rates=[0]`，nil error。
- 既有 `modulation` MaxInt 防護與普通 channel 測試：`go test` exit 0，`0.448s`。

## Needs investigation

- `Replay` 的寬度與步數來自呼叫端已建立的 slice。要證明 row 複製或 `SourceSpec.Build` 的 deep copy 在已存在的超大外部 slice 上是否還會 panic，需要一個不在本輪允許範圍內的外部大資料來源或受控序列化測試；本輪沒有用 `unsafe` 或大型配置建立這類輸入。
- `ExternalTimeline.Entries`、`Controller.Inputs.Resources`、`Transport.Fraction` 與 receptor cell lists 也依呼叫端 slice 長度配置。它們沒有新的 scalar byte-size 門檻，但本輪未嘗試任何可分配的大型長度，不能據此宣稱所有大型輸入都安全。
- 本報告只確認公開 API 的 capacity／panic 行為，不提出或驗證修正，也沒有判斷任意固定容量是否符合未來需求。

## 文件、命令、退出碼、環境與輸入指紋

診斷程式與實際輸出：

- 程式：[main.go](main.go)，SHA-256 `56162ca50954e6ea1c5089db08fc80740f91ecf7368d1906c72d14e7a1ffeba1`。
- 已編譯診斷二進位檔：[diag](diag)，SHA-256 `97d3b0dda0836877043c40ca22486c42762cd2faa3ad09fbb16d051f1f4e06d3`。
- 實際輸出：[run-20261014.log](run-20261014.log)。父程序 exit 0；每個 child 有 10 秒 timeout，所有 child 均在 timeout 前結束。回收的 panic child exit 42，回傳錯誤均為 `runtime error: makeslice: len out of range`。
- 既有目標測試輸出：[targeted-tests.log](targeted-tests.log)，exit 0。

所有 Go 命令均使用：`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`、任務專用 `GOMODCACHE`／`GOCACHE`／`GOTMPDIR`／`TMPDIR`，Go `1.26.5 darwin/arm64`、`CGO_ENABLED=1`。診斷 fixture 只建立小型 slices、參數與單列狀態，沒有嘗試位元組大小可表示但可能成功的巨大配置，也沒有網路、遠端、App、Git 寫入或正式檔案修改。

對照輸入來源指紋：`baseline.json` 對應 commit `57cc92d2864bd19f2424a4daa57edad60ad2763d`，目前受影響正式檔案 SHA-256 與基準相同：

```text
modulation/sources.go       caac2fa9a0854eece0c17b97dc17f9e69eb3e7dfc85910575f924098e04af426
modulation/config.go        07afd7684f18afe02dd751ffd4069d0c44ec2a2eea0a7f6004f5a66427bb2ea0
modulation/chemistry.go     f68476dc3bb291e1c13a22d3b198675c16b534beb3ee77ee1baeac39f9f9d826
modulation/controller.go    b46d2f8675071dd4ccee261c6a6be2c412b55a81acd4877834747e9f73e4da01
modulation/memory_controller.go 8528d9282accb89032b2e083a750cde00e9e477998d7fdf7e9f45c6537d5dc7e
modulation/effects.go       42db496fccb4e2d6afde985510bf2334805f6ae78ce8d657579fe0185bb93222
```

## 未驗證項目

本輪沒有執行完整測試、race、vet、全腦或大型真實配置，也沒有修改正式 Go／測試／文件檔案。Replay 的超大既有 row、可分配但很大的設定與任何修正後行為均未驗證。
