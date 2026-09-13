# CoImNet 開發進度

## 目前階段

CPU 參考流程已在固定 v5 來源完成驗證，包含未初始化核心與等待訓練鎖時取消的修正。Mac 與 Ubuntu 1 的建置、單元、race、vet 與跨程序續訓全通過。官方資料下載與 Feather 逐批讀取另有 v4 實測，三份原件均可完整讀取。完整目標維持原始 W01–W14。

## 階段目標

把已驗證的資料讀取接到不可變解剖圖，明示選取規則與原始接點統計，再以範例驗證真實圖訓練能力。人工延遲關聯流程保留為 CPU 數值參考。框架不綁定使用者的單一模型或任務程式。

## 進行中

| id | 目標 | 負責者 | 狀態 | 驗證訊號 |
| --- | --- | --- | --- | --- |
| 01 | 開發者可核對環境與 Insyra 實際能力 | 主 agent / API 查核 agent | verified_scoped | Mac 與 Ubuntu 實際 doctor、Insyra 數值探測通過 |
| 02 | 使用者可計算稀疏前向與完整梯度 | sparse agent | verified_scoped | 手算、獨立稠密參考、形狀錯誤、有限差分已通過 |
| 03 | 使用者可提供與對齊具名訊號 | signal agent | in_progress | 型別、時鐘、映射及嚴格 JSON 已通過；重取樣與跨頻率對齊待做 |
| 04 | 研究者可訓練連續核心 | 主 agent | verified_scoped | 報告 v2 只置換訓練標籤並拒絕資料流重疊；Mac/Ubuntu v5 來源三個 seed 皆通過 |
| 05 | 使用者可中斷並接續學習 | sparse agent / 主 agent | verified_scoped | 新程序完整參數與 Adam 狀態一致，特殊檔案及輸出錯誤回歸測試通過；完整持續個體另做 |
| 06 | 使用者可保存並續傳官方資料 | sparse agent / 主 agent | verified_scoped | 三份官方原件共 1,109,008,094 bytes，CRC32C 全通過；v4 CLI 另實測自動 CRC32C 驗證 |
| 07 | 使用者可逐批讀取 Feather 原件 | API agent / 主 agent | verified_scoped | annotations 211,577、NT 1,835,518、weights 151,856,684 列完整掃描，前後原件指紋一致 |

## 目前阻礙

Spark 工具回報用量限制，已依使用者順序改用 Luna `max`。Insyra 自訂 tape 梯度接合與最佳化器序列化缺少公開 API，已提 [#375](https://github.com/HazelnutParadise/insyra/issues/375)、[#376](https://github.com/HazelnutParadise/insyra/issues/376)。目前使用有完整數值測試的兩段 tape 接合，以及 CoImNet 自有可保存 AdamW。

Mac 可執行本機測試。Ubuntu 1 已實際連線並確認 RTX 4070 12 GB，Go 工具放在 `/tmp/coimnet-validation.irVFk8MV`。PC 的 App 主機連線存在，但本輪沒有可用的遠端指令工具，SSH 22 逾時，App UI 操作被工具限制拒絕。Windows 實機驗證尚未執行。GPU 硬體存在不等於稀疏訓練後端完成。

## 下一個可驗證成果與 ticket

08：依主規格第 5 章建立標準化接線圖。先完成端點、原始接點與選取規則的只讀統計，確認全圖所需容量與重複紀錄處理，再實作具名視圖。這部分尚未驗收。完整個體狀態、LIF、可塑性、調節與全腦/GPU 驗收都還在原始待辦範圍。

已驗證來源快照與日誌見 [macOS v5](evidence/cpu-reference-20260913/macos-v5/validation.log) 及 [Ubuntu v5](evidence/cpu-reference-20260913/ubuntu-v5/validation.log)，兩平台 69 份來源指紋相同。[官方 CLI 驗證](evidence/malecns-source-20260913/cli-v4/verification.json) 保存命令、原件指紋與逐批讀取結果。資源效能表保留明示版本的 v4 量測，不宣稱為 v5 量測。85 項完整需求目前有 8 項附上通過證據，其餘保留。

## 決策紀錄

2026-09-13：使用者授權開始規劃與實作，沿用既有 Go＋Insyra 及完整需求。先驗證梯度與可學習的小型流程，之後接真實資料，避免在未驗證核心上堆疊任務。

2026-09-13：官方 Feather 需要 nullable int64、dictionary 與 list 型別；Insyra 缺口已提 [#377](https://github.com/HazelnutParadise/insyra/issues/377)，資料 adapter 使用現有相依版本的 Arrow Go v17。查核見 [來源紀錄](docs/malecns-source-audit.md)。

2026-09-13：使用者授權在完成可驗證階段後主動提交與推送本專案，依 AGENTS.md 執行。

2026-09-13：使用者明確界定 repo 僅提供框架，特定模型的訓練程式與模型限定為範例。維持原需求，五類任務與全圖訓練採範例／驗收流程。資源量測寫入 [resources](docs/resources.md)，現有神經網路的關係與可選機制寫入 [model-and-mechanisms](docs/model-and-mechanisms.md)。機制可選沿用原規格，不新增無法查證的全生物機制模式。

## 來源與接手

[共用設計](ENG.md)、[近期 tickets](docs/tickets/)、[主規格](docs/handoff/CoImNet_Implementation_Plan.zh-TW.md)、[85 項需求狀態](docs/requirements-status.json)。交接原件與既有 LICENSE 保持不變。所有完成狀態以實際證據為準，近期階段完成不等於整個框架完成。
