# Root 審查：導航報告設定隔離

Scope：CLEAN。基準為 974afa3421a8d2d415026d02d3284fbb0aca38c4。唯一既有 Go 修改是共同的 RunNav2D 報告 producer，一行複製及一行公開註解；一個新測試檔由 Luna 完成，Root 完整讀取、獨立 RED 及凍結。

## 結論

沒有本票新增的確認缺陷。原 Policies 共用已由同一個入口修正，suite 的四次 RunNav2D 呼叫自然取得各自的副本。Seeds 已有複製，沒有重複 helper。字串元素不含可變的深層物件，切片複製足以隔離可觀察的修改。

## 契約與相容性

- producer 與所有消費者：experiment/nav2d_suite.go 的 RunNav2D、RunNav2DSuite；internal/cli/nav2d.go 的 runNav2D。完整圖索引盤點及呼叫追蹤見 contract-discovery.json。
- Nav2DConfig 只有 Seeds、Policies 是切片，其餘為值；複製在 Validate、環境解析與輸入寬度成功之後，在 config_hash 與訓練迴圈之前。
- 公開簽名、JSON 欄位與格式、nil／取消／非法設定次序、單筆 Failed／Error 與 suite 部分成功分支沒有變更。
- 合法策略／種子順序、輸入與訓練結果的八份完整報告、24 筆結果在兩個新程序中與基準逐位元組相同，見 valid-comparison.json。八條真實 CLI 流程見 cli-result.json。
- 兩份切片互不影響是回傳後的契約，沒有允許執行期間並行修改輸入。修改 report.Config 本身不會自動重新計算雜湊。

## 測試審查

完整新測試的 SHA-256 為 120aab9459ce98d5597ab3a30952c63c6f3b5add027e74a4cb0ce28a45f09635，修正之後未改動。三個頂層測試原本全 RED，修正後全 PASS，七個 Seeds 正向控制在 RED 就通過。每份初始報告都有四筆無 Failed 的 Runs。雙向修改、每個 suite 任務與其他三份報告，以及額外容量下的追加都以完整 JSON／可觀察的內容驗證，不比指標。雜湊以 encoding/json 加 SHA-256 獨立重算。

20 個相關頂層測試通過，包含既有有效設定、有限值、取消及 CLI；完整一般與 race 各 57 套件以 -count=1 通過，沒有快取結果。建置、vet、格式、相依性及八個治理／索引測試通過。654 份凍結 Go 來源與 3,321 份保護原件指紋相符，見 preservation.json。

## 適用範圍與減法

這是局部設定隔離，沒有 SQL、權限、資料遷移、服務操作或畫面。增加的複製受既有四種策略清單上限約束，沒有核心數值或全腦資源改變。依 diff-inspector，小型低風險修正由 Root 審查，不另派專家審查。

Comparison.Interval=NaN 是上票已確認的獨立 P2，保留在 AGENTS.md；不把它混進此修正。訓練調參及模型學習結果不在本票驗收內，需求狀態維持。建議簡化已採用：suite 共用 producer，不另寫清單隔離邏輯。
