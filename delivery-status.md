# CoImNet 開發進度

## 目前階段

連續值時間點重取樣已完成：即時因果保持、離線線性插值、整數比率對齊與分塊結尾。103 檔同源的 Mac／Ubuntu v14 完整檢查通過，詳見 [取樣驗證](evidence/signal-resampling-20260914/verification.json)。ticket 03 的脈衝／區間與自訂 adapter 整合繼續處理。

真實子圖選取及短訓練範例已完成驗證（ticket 10、DAT-07）。原件與既有完整圖保持不變，證據見 [real-subgraph](evidence/real-subgraph-20260914/verification.json)。

標準化接線圖已在真實 MaleCNS v1.0 三份原件上完成建構、落盤與讀回驗證（ticket 08、09）：`connectome.Build` 與 `data import` 依明示 manifest 產生 `raw_segments`／`annotated_neurons` 視圖與報告，165,122 個選入節點、25,563,197 條邊，所有與獨立稽核共有的計數相等。Mac 與 Ubuntu v7 同一份 95 檔來源的建置、單元、race、vet、模組驗證與跨程序續訓全通過。CPU 參考流程、官方資料下載與 Feather 讀取維持 v4／v5 驗證。完整目標維持原始 W01–W14。

## 階段目標

[10 真實子圖整合範例](docs/tickets/10-real-subgraph-example.md) 已驗證。98 檔同源的 Mac／Ubuntu v10 建置、單元、race、vet、模組與跨程序恢復驗證全部通過。ALIN 24 節點、110 邊的來源稽核、運算邊與初始化權重核對相等。

下一階段補完脈衝／區間訊號對齊與多通道 adapter 的核心整合（ticket 03）。人工延遲關聯流程保留為 CPU 數值參考。框架不綁定使用者的單一模型或任務程式。

## 進行中

| id | 目標 | 負責者 | 狀態 | 驗證訊號 |
| --- | --- | --- | --- | --- |
| 01 | 開發者可核對環境與 Insyra 實際能力 | 主 agent / API 查核 agent | verified_scoped | Mac 與 Ubuntu 實際 doctor、Insyra 數值探測通過 |
| 02 | 使用者可計算稀疏前向與完整梯度 | sparse agent | verified_scoped | 手算、獨立稠密參考、形狀錯誤、有限差分已通過 |
| 03 | 使用者可提供與對齊具名訊號 | 主 agent / Luna 審查 | in_progress | 型別、映射、嚴格 JSON 及連續值重取樣通過；脈衝／區間與多通道 adapter 待做 |
| 04 | 研究者可訓練連續核心 | 主 agent | verified_scoped | 報告 v2 只置換訓練標籤並拒絕資料流重疊；Mac/Ubuntu v5 來源三個 seed 皆通過 |
| 05 | 使用者可中斷並接續學習 | sparse agent / 主 agent | verified_scoped | 新程序完整參數與 Adam 狀態一致，特殊檔案及輸出錯誤回歸測試通過；完整持續個體另做 |
| 06 | 使用者可保存並續傳官方資料 | sparse agent / 主 agent | verified_scoped | 三份官方原件共 1,109,008,094 bytes，CRC32C 全通過；v4 CLI 另實測自動 CRC32C 驗證 |
| 07 | 使用者可逐批讀取 Feather 原件 | API agent / 主 agent | verified_scoped | annotations 211,577、NT 1,835,518、weights 151,856,684 列完整掃描，前後原件指紋一致 |
| 09 | 使用者可保存並讀回標準化接線圖 | 主 agent | verified_scoped | 容量、溢位、竄改／截斷／順序／索引與路徑替換回歸通過；真實圖 664 MB 在 Mac／Ubuntu 讀回再保存的獨立 SHA-256 均與原件相同，見 graph-store-v2 |
| 08 | 研究者可由官方原件建立可追溯標準化接線圖 | 主 agent（connectome）/ extsort subagent | verified_scoped | fixture 與真實資料皆通過；410 秒、RSS 3.40 GB、暫存 4.1 GB 後清空；raw 零重複 pair；durable GraphStore 切到 09 |
| 10 | 研究者可選取真實子圖並接上訓練核心 | 主 agent / Luna | verified_scoped | 獨立稽核及全部 ID／運算邊／初始化權重一致；兩平台各重跑一致、固定參數不變；跨平台最後參數指紋不同，僅作人工整合範例 |

## 目前阻礙

2026-09-13 使用者要求接回 Claude 的進度，分工優先 Spark `xhigh`；本次重新呼叫仍回報用量限制，改用 Luna `max`。Insyra 自訂 tape 梯度接合與最佳化器序列化缺少公開 API，已提 [#375](https://github.com/HazelnutParadise/insyra/issues/375)、[#376](https://github.com/HazelnutParadise/insyra/issues/376)。目前使用有完整數值測試的兩段 tape 接合，以及 CoImNet 自有可保存 AdamW。

嚴格 JSON 的深度與路徑配置缺口已修正，並將 Unicode 重複鍵檢查同步至訊號、快照、下載 metadata 與 manifest。圖檔補上配置前記憶體檢查、計數溢位、未初始化圖、未知 converter 與同次讀取 hash 回條；回歸測試及兩平台 v7 全套驗證通過。

Mac 可執行本機測試。Ubuntu 1 已實際連線並確認 RTX 4070 12 GB，Go 工具放在 `/tmp/coimnet-validation.irVFk8MV`。PC 的 App 主機連線存在，但本輪沒有可用的遠端指令工具，SSH 22 逾時，App UI 操作被工具限制拒絕。Windows 實機驗證尚未執行。GPU 硬體存在不等於稀疏訓練後端完成。

## 下一個可驗證成果與 ticket

[03 訊號對齊](docs/tickets/03-signals.md)：補完脈衝／區間的跨頻率規則，再以多通道 adapter 範例接入核心。連續時間點取樣的因果／離線處理及分塊尾端已驗證。完整個體狀態、LIF、可塑性、調節與全腦/GPU 驗收都還在原始待辦範圍。

圖儲存階段來源與日誌見 [macOS v7](evidence/cpu-reference-20260913/macos-v7/validation.log)、[Ubuntu v7](evidence/cpu-reference-20260913/ubuntu-v7/validation.log) 與 [來源比對](evidence/cpu-reference-20260913/verification-v7.json)。歷史驗證見 [macOS v6](evidence/cpu-reference-20260913/macos-v6/validation.log)（含 connectome、extsort，83 份來源指紋）、[macOS v5](evidence/cpu-reference-20260913/macos-v5/validation.log) 及 [Ubuntu v5](evidence/cpu-reference-20260913/ubuntu-v5/validation.log)。真實圖建構的命令、環境、指紋、報告與交叉核對見 [graph-v1](evidence/malecns-source-20260913/graph-v1/verification.json)。85 項完整需求目前有 14 項附上通過證據（本次新增 SIG-06），其餘保留。最新全套日誌見 [Mac v14](evidence/cpu-reference-20260914/macos-v14/validation.log)、[Ubuntu v14](evidence/cpu-reference-20260914/ubuntu-v14/validation.log) 及 [來源比對](evidence/cpu-reference-20260914/verification-v14.json)。

## 決策紀錄

2026-09-13：使用者授權開始規劃與實作，沿用既有 Go＋Insyra 及完整需求。先驗證梯度與可學習的小型流程，之後接真實資料，避免在未驗證核心上堆疊任務。

2026-09-13：官方 Feather 需要 nullable int64、dictionary 與 list 型別；Insyra 缺口已提 [#377](https://github.com/HazelnutParadise/insyra/issues/377)，資料 adapter 使用現有相依版本的 Arrow Go v17。查核見 [來源紀錄](docs/malecns-source-audit.md)。

2026-09-13：使用者授權在完成可驗證階段後主動提交與推送本專案，依 AGENTS.md 執行。

2026-09-13：ticket 08 的 root 決策：選取 predicate 用 `annotations.status == "Traced"` 並標為工程選取；端點身份以 manifest 明示並附官方頁面依據；比例保存分子、分母與欄位；節點順序為 namespace 內數值遞增；真實資料驗收限制 4 GiB 記憶體、16 GiB 暫存、256 run；durable GraphStore 切到 09；raw view 以重新讀取原件串流。詳見 [ticket 08](docs/tickets/08-connectome-graph.md)。同輪修正 `feather` 對零長度子陣列 nil buffer 的誤判。

2026-09-13：ticket 09 的 root 決策：圖儲存採自訂固定寬度區段加 JSON footer（非 Arrow IPC），每段與 footer 各有 SHA-256，檔案不含時間戳；發布沿用 checkpoint 的暫存檔＋hardlink＋目錄同步語意；讀回重算三個結果 hash 並檢查結構不變量；store 只保存 weights 原件路徑與指紋。不單獨標記需求通過，DAT-07 待真實子圖實驗。詳見 [ticket 09](docs/tickets/09-graph-store.md)。

2026-09-13：使用者明確界定 repo 僅提供框架，特定模型的訓練程式與模型限定為範例。維持原需求，五類任務與全圖訓練採範例／驗收流程。資源量測寫入 [resources](docs/resources.md)，現有神經網路的關係與可選機制寫入 [model-and-mechanisms](docs/model-and-mechanisms.md)。機制可選沿用原規格，不新增無法查證的全生物機制模式。

## 來源與接手

[共用設計](ENG.md)、[近期 tickets](docs/tickets/)、[主規格](docs/handoff/CoImNet_Implementation_Plan.zh-TW.md)、[85 項需求狀態](docs/requirements-status.json)。交接原件與既有 LICENSE 保持不變。所有完成狀態以實際證據為準，近期階段完成不等於整個框架完成。
