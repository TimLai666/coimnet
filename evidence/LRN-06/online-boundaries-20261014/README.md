# 線上學習順序與模型品質查驗

混合線上更新在梯度完成、局部學習失敗後，原本會在重試時再次執行梯度階段。這會重複更新參數、重複累積梯度或改變更新間隔。ticket 85 已修正這個根因，完整一般與 race 各 58 套件通過。本輪約 2.18 GiB 暫存已清除，提交與推送待完成。

## 修正與證據

- 已完成的梯度階段按 action／source 保存，重試只接續局部學習。沒有 target、合法間隔跳過、部分累積及成功更新都只執行一次。整筆成功即移除私人記錄，保留既有部分成功不回滾契約。
- 修改前的正式 RED 有九個缺陷頂層及 22 個失敗情境，正常控制四個子情境通過。修改後的 10 個新增頂層回歸全數通過，相關 18 個頂層、71 個測試事件通過。見 `mixed-retry-formal-red.json`、`lifecycle-red.json`、`mixed-retry-green.json` 與 `frozen-tests.json`。
- 32 組正常公開結果在修改前後逐位元組相同，涵蓋純梯度、純局部學習、混合、Evaluate、化學開關、更新間隔及累積窗。見 `compatibility-comparison.json`。
- CLI 在三個獨立程序分別執行 80 步、接續 40 步及不中斷 120 步，接續快照與不中斷快照逐位元組相同。見 `cli-resume-comparison.json` 與各命令回條。
- 完整一般與 race 各通過 58 套件，一般／race 分別 5,121／5,120 個測試事件，沒有資料競爭警告。差異來自既有大型模型的 race 記憶體跳過，全部跳過理由由 `suite-summary-final.json` 保存。build、vet、相依性與 781 份正式 Go 來源格式通過。785 份來源／模組指紋一致，正式來源只改 `learning/online.go` 並新增三份回歸。最終結果由 `verification.json` 記錄。

測試輸入是有來源指紋的人工 fixture，沒有重新訓練真實 OCR 或量測辨識率。修正保留公開 API、模型保存格式、正常算式及原有模型資料。它不能復原舊模型已經重複套用的更新。

## 另已確認、尚未修正的問題

ticket 86 處理同一個 Individual 被線上學習與外部訓練同時操作時的兩個結果：

1. Act 的輸出使用舊參數，ModelVersion 卻可能標示外部更新後的新參數，違反「產生答案的參數版本」契約。
2. 回饋加權預測與實際訓練之間可以插入外部更新。score=0 因而產生本應為零的額外梯度，診斷梯度範數為 0.010393098556645655。完整快照與舊預測插入控制相同，與完整序列化控制不同，已排除正常 Adam 計數或動量的解釋。

`public-interleave-v3-diagnostic.json` 與 `public-interleave-v3-race.json` 的 PASS 是成功觀察到舊錯誤，不能當作修復通過。`public-interleave-diagnostic.json` 保留 v2 的失敗：那一版錯把正常最佳化器計數當作狀態不應改變，Root 已更正比較方法，沒有因測試錯誤修改框架。

## 機制與品質的適用範圍

本票驗證線上更新符合既定工程規則，沒有新增生物定量證據。受體 `Location.Region` 參與化學區域選取，`Compartment` 保存位置註記，不建立各部位獨立的膜電位。這是 `docs/model-and-mechanisms.md` 明示的設計簡化。原有 91／91 是工程需求驗收，不能推論整個框架沒有 bug 或模型品質已達標。

## 驗證環境與失敗紀錄

基準提交為 `5f26c932981319627e3b76a3e8d47a5ae741c05e`，本機 darwin/arm64、Go 1.26.5、固定 Insyra v0.3.4。全部大型暫存與建置快取在專案內，原生 module cache 只讀。`go mod verify` 以命令中的明示覆寫讀取原生 module cache，沒有使用連結副本的驗證結果。

首次完整一般與 race 在沙箱內遭本機 httptest 監聽權限拒絕，Root 停止本輪程序並保留原始紀錄。正式完整驗收以已核准的本機迴圈介面權限重新執行。見 `sandbox-retry.json` 及 `full-unit.json`、`full-race.json`。

OpenCode 已核對免費模型，但執行探針回報 `Unknown: FileSystem.open (/Users/timlai/.local/share/opencode/log/opencode.log)`，沒有完成模型工作。依專案規則使用原生 gpt-5.6-luna／max，Root 審查實際來源並親自執行驗證。整個專案重新索引遭自動審查拒絕，理由是可能向 codebase-memory MCP 傳送非公開來源，後續以本機檔案查驗。

## 變更

| 檔案 | 變更摘要 |
| --- | --- |
| `learning/online.go` | 保存每筆混合回饋已完成的梯度階段，失敗後只接續局部學習。 |
| `learning/online_mixed_retry_test.go` | 梯度重複、間隔漂移、累積及正常控制回歸。 |
| `learning/online_mixed_retry_extra_test.go` | 合法跳過、非法 gate、多來源與延後回饋回歸。 |
| `learning/online_mixed_retry_lifecycle_test.go` | 無／更換 target、時間倒退、多筆 pending、連續失敗與取消回歸。 |
| `ENG.md` | 共用混合階段與部分成功契約。 |
| `docs/individual-state.md` | 使用者可見的重試行為。 |
| `docs/tickets/85-online-mixed-phase-retry.md` | 混合重試根因、責任與驗收。 |
| `docs/tickets/86-online-individual-boundary.md` | 已重現的同一個體並行問題與下一輪驗收。 |
| `delivery-status.md` | 實際驗收狀態、已確認缺口及下一張票。 |
| `docs/requirements-status.json` | 驗收完成後補入 LRN-06 的本輪工程證據。 |
| 本目錄 | 原始回條、壓縮日誌、凍結測試、相容性、來源指紋、診斷與 Root 審查。 |

## 減法審查

沿用最小的私人階段記錄，成功即清除。沒有複製整個個體或新增回滾層。下一票把同一次答案或回饋更新固定在原有個體鎖內，省去僅為版本標記而建立的完整快照。

## 清理與下一票準備

`cleanup.json` 與 `cleanup-inventory.json.gz` 保存刪除前兩次讀取一致的檔案指紋及刪除後檢查。本輪工作目錄已不存在，清除 13,024 個檔案及 15,046 個唯讀連結，檔案邏輯大小為 2,345,848,209 bytes。兩個既有暫存資料夾、原生 module cache、資料與模型均保留，785 份正式來源指紋保持。

`next-boundary86-preparation.json` 登錄下一票的骨架、測試草稿及 Root 審查。草稿尚未編譯或執行，尚未納入正式測試；失敗時的 goroutine 等候清理與化學、凍結、累積及取消情境需補驗。它不算 ticket 86 的正式 RED 或修復驗收。

派工報告與下一票草稿的原始位元組另以 `*.raw.gz` 保存，可閱讀版本只校正行尾空白。`evidence-normalization.json` 同時記錄原始與校正後指紋；派工報告中的歷史指紋以原始回條核對。
