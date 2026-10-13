# 使用者可安全拒絕不可表示的化學配置

## 目標與已確認根因

公開宣告只確認 Channel+1 能以 int 表示，未確認 float64 向量、活動視窗與濃度網格的位元組大小。Root 以基準 57cc92d 的同一診斷二進位檔，獨立重現 21 個小型子程序中的 13 個 makeslice panic。所有輸入只含小型現有 slices，巨大宣告皆為數學上不可表示的配置。

本票是 MOD-01、MOD-02、OPS-01 的拒絕契約修正，不改正常動態／梯度、公開簽名或保存格式。不設定任意固定容量上限，不捕捉 panic 來掩蓋驗證缺口，不嘗試可表示但巨大的記憶體配置。實體記憶體不足、一般大型合法設定與生物定量正確性不是本票完成條件。

## 骨架與責任

| 格子 | 入口與輸出 | 檔案責任 | 想要的結果與驗證 |
|---|---|---|---|
| 測試 | 公開 Validate／Build／Release／Offset／NewChemistry／ApplyEffects／EnableChemistry，非法值回 error | 測試 worker 僅新增 modulation/capacity_validation_test.go、learning/individual_chemistry_capacity_test.go | 位元組不可表示的通道、節點、區域及網格總大小都回錯，輸出空值，控制器及持續個體失敗保留狀態；凍結前在舊程式看到行為 RED |
| 算術驗證 | 私人向量／float64 網格大小檢查，成功回 nil、不可表示回 error | 獨立實作 worker 只可新增 modulation/capacity.go，修改 sources.go、config.go、controller.go、memory_controller.go、chemistry.go、effects.go | 正值、int 位元組界線及網格乘法在 make 前驗證；入口一致，錯誤早於可變狀態提交，禁止修改測試 |
| 整合與交付 | 正常結果逐位對照、原始重現、完整工程驗證 | Root 管理證據、文件與提交 | 原本的合法模型和來源記憶保持；本輪暫存全數清除，推送後讀回提交 |

向量檢查依 portable MaxInt / 元素位元組大小。濃度與視窗網格另檢查外層 slice header 及 rows*columns 的位元組大小，使用除法先確認乘法不溢位。所有 controller ring 檢查在複製／observe 前完成。來源必須同時覆蓋 SourceSpec.Build 與 standalone 入口，Replay 寬度來自已存在的矩形 slice，不捏造巨大 slice。

## 失敗與邊界矩陣（本票唯一所有者）

| 情況 | 呼叫者可見結果 | 驗證 |
|---|---|---|
| Channel=MaxInt-1／無法表示 float64 byte width | Build／Validate／Release error，無 panic | 每種 scalar 寬度來源，包含已有 MaxInt 回歸 |
| Controller／MemoryController Nodes 的單列或視窗總位元組不可表示 | Validate 與運算 error，原視窗／梯度快取保持 | 先小型成功，再改成非法宣告並還原，狀態及 gradient 相同 |
| Regions 外層或 Regions*Channels 總配置不可表示 | NewChemistry／ChemistryConfig.Validate error | MaxInt Regions、4 channels 的乘法邊界，小型正常濃度控制 |
| ApplyEffects nodes byte width 不可表示 | nil gain／offset／threshold、error | 空 effects 及有效 effect 的公開呼叫 |
| 已啟用化學的個體換入非法 Regions | EnableChemistry error，完整快照及最後報告保持 | 小型公開個體前後指紋比較 |
| 零／負值及缺省／nil | 沿用既有 error／neutral 契約 | 原套件測試與現有 sources 邊界測試 |
| 合法關閉、來源／受體／控制器、保存接續 | 數值／格式與修正前一致 | 前後逐位對照、完整 unit／race、CLI 新程序接續 |

## 三條完整驗收流程

1. SourceSpec 建立來源 → 小型 Release 成功；同一型別不可表示通道 → Build 或 Release error，無 panic。
2. 小型控制器成功 → 記錄視窗及 gradient → 改入不可表示宣告 → error → 還原合法宣告後視窗與 gradient 相同。
3. 公開個體啟用合法化學並 Advance → 記錄完整快照／報告 → EnableChemistry 非法區域 → error，前後相同；合法保存以新程序接續保持。

## 5 項固定完成條件

- [x] 公開重現、完整位置盤點、正式 RED 及測試凍結。
- [x] 全部同根因的化學宣告在配置前拒絕，正常算式保持。
- [x] 正常逐位對照、完整 gofmt／build／unit／race／vet／相依性與 Root 審查。
- [x] 證據、需求追蹤與治理同步，限制與略過明示。
- [ ] 本輪暫存清除、獨立修正／回歸提交、推送及遠端讀回。

## 減法審查

合併重複的配置算術檢查，不增加設定、保存欄位、容量預設值、recover 或大型配置測試。初始未觀察活動的拒絕已有明示契約，另存排除證據，不任意把它改成零活動。
