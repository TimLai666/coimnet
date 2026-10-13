# 機制實作與模型品質缺陷稽核

本輪確認並修正七類缺陷。重設最佳化器後，新的訓練回合可以再次鞏固記憶。同一回合不能靠重設或計數回繞重複寫入。無效圖片、音訊與不可表示的數值會在提交前回錯。這些結果證明指定契約的修復，沒有量測辨識率改善，也不能證明整個框架沒有其他 bug。

基準提交：`3cf1f623b8031cc5f3e2041f9cbef9a8f7f60ce1`。日期：2026-10-13。原始設計以 `docs/handoff/`、tickets 22／25／65、`ENG.md` 與公開 API 的契約核對。回歸使用人工小圖與可手算的大值，沒有下載或訓練新的全腦模型。

## 已重現並修正

| 工作 | 根因與影響 | 修正與公開驗證 |
| --- | --- | --- |
| 67 化學釋放報告 | 每列釋放與濃度有限，總量仍可溢位。舊 Advance 成功並提交狀態，報告含 Inf 而無法編碼 JSON，快照本身仍可編碼。 | 在私有候選檢查 ReleaseTotal，整次失敗保留神經、化學、來源視窗、可塑性及舊報告。連續／LIF、timeline／controller、gated／非 gated 八種組合，有限邊界、呼叫間歸零與凍結均驗證。 |
| 68 重設後的新記憶 | Episodes 歸零卻保留舊 LastEpisode，新的回合被錯判為已鞏固。 | 保留記憶值及 lifetime 預算，將去重標記對齊新時鐘。已／未寫入、取消／非法重設、快照恢復與新回合一次寫入均驗證。 |
| 69 機制宣告 | NewNetwork 接受不支援的向量固定符號；SourceSpec 接受宣告通道以外的非零 timeline／replay，與實際執行限制不同。 | 共用建構入口先拒絕。向量全零／缺省、純量固定符號及符合通道的來源保持合法。Standalone 多通道來源不改契約。 |
| 70 OCR 像素 | 手動建立的 Image 可帶入 -1 或 2，違反 [0,1] 影像契約，TrainLine 會更新模型。 | 公開回歸涵蓋 Inputs 三種模式、Columns、SegmentLines、TrainLine，回錯不改訓練快照，0／1 邊界合法。真實 OCR 範例加入相同的範圍檢查。 |
| 71 ASR 特徵 | STFT 接受 NaN／Inf 或有限但運算溢位的樣本，LogMel 可成功回傳壞特徵，Floor 也接受正 Inf。 | 樣本、頻譜與特徵有限性及 Floor 正有限值在相應入口拒絕。失敗不回部分特徵。合法特徵的計算算式不變。整檔／串流另由既有回歸檢查，相容性逐位比較只涵蓋下述 STFT／LogMel 樣本。 |
| 72 鞏固數值 | slow 累加與報告範數未驗證就提交。平方中間值也會讓可表示的 1e200 範數變成 Inf。 | 同一把鎖內先驗證候選及報告，再提交，Hypot 計算範數。真正不可表示的數值回錯且不改記憶或預算。 |
| 73 回合計數 | uint64 遞增回繞成零，TrainEpisode 改變模型，Consolidate 可對同一回合重複消耗預算。 | 最大值先拒絕而保持完整快照，最大值前一回合合法。舊較大標記快照仍接受，明確 ResetOptimizer 重新對齊後，新訓練回合可鞏固。 |

回歸先在缺陷版本失敗，再在修正版本通過。原始 RED／GREEN 及三次未通過的完整驗證保存在 [日誌索引](regression-log-manifest.json)。前次成功 replay fixture 寫到錯誤通道，與單通道契約及 runtime 拒絕規則矛盾，因此把成功樣本修正到宣告通道；另新增負例繼續拒絕原來的錯誤情境，沒有放寬斷言。

## 實作符合的設計與仍有的假設

- 逐步執行使用上一列神經活動，計算釋放、濃度、受體佔用率及效果，再推進神經核心與可塑性。`TrainEpisode` 的化學反向已存在，來源活動回授與讀出表現調節會參與梯度；控制器參數及受體反應係數在這條路徑固定。原文件「訓練看不到化學」已校正。
- 受體的 Region 選擇對應區域的濃度。Compartment 保存資料來源描述，沒有另外建立細胞內多區室膜模型。這是目前設計假設，還不能以受體位置完整重現生理反應。
- 未知受體沒有反應係數時維持 skipped。允許假設且提供係數時才標 assumed。中性數值不等於實測不反應，來源未知統計保留。
- ResetNeural 重設神經與來源時鐘，保留化學濃度、化學更新步數與來源視窗。現有 initial-state 契約與測試明確要求這個行為。
- 快速可塑性、近期參與紀錄及局部學習已有 CPU 小圖實作與證據；機制表已修正。完整圖長期學習品質與生物定量正確性仍沒有本輪證據。
- 單檔 SaveIndividual 的 hardlink／目錄同步需求保留。網路磁碟使用 SaveIndividualBundle／LoadIndividualBundle，文件已分清兩種公開入口。本輪沒有操作 SMB 或 SSH。

## 相容性

正常圖片的三種模式之 Inputs、一次 TrainLine 回報與快照，以及 768 個有限正弦樣本的 STFT／LogMel 特徵，基準與最終修正的 JSON 完全相同：29,662 bytes，SHA-256 `bcfe9b6fa8785cbec299fca872974c1e95633cfbc458164d618c17e7975e48fa`。見 [基準輸出](compatibility-before.json)、[最終輸出](compatibility-final.json) 與 [比較範圍](compatibility-result.json)。

沒有增加保存欄位或 schema 版本。非法設定與輸入會較早回錯。ResetOptimizer／計數上限屬刻意修復的生命週期行為；鞏固範數採 Hypot，報告捨入可能改變。正常資料的對照不涵蓋所有模型或生物條件，也沒有證明歷史 OCR 低辨識率由本輪缺陷造成。

## Verified

build、完整一般與 race、vet 及相依性五項檢查均通過，一般與 race 各 58 個套件。最終結果由 [verification.json](verification.json) 與 [full-validation.json](full-validation.json) 記錄。套件、測試事件、跳過項目與跨程序恢復結果保存在 [test-summary.json](test-summary.json)。770 份來源指紋核對相符，見 [source-manifest.json](source-manifest.json)。需求證據更新後，另執行 [11 項治理檢查](governance-final.json)，全部通過。

重跑使用 Go 1.26.5、darwin/arm64 與固定 Insyra v0.3.4。`go mod verify` 檢查已快取依賴，沒有更新套件。首次完整驗證遭沙箱禁止 localhost 測試連線；後續使用允許本機 HTTP 測試的環境。另兩次完整驗證因新 ticket 的決策標頭缺日期而失敗，已修正文書，沒有放寬治理測試。前幾次 runtime t.TempDir 使用系統暫存並由測試自行清除；最終驗證另將 TMPDIR 指向專案內。

預設的整套測試會跳過未配置的全腦與部分外部資料證據入口，及獨立執行時無作用的 child helper。跳過清單不可當成驗收通過。本輪沒有重跑全腦、跨 CPU 或受體生物干預的量測。

## Diff Inspector

Scope: CLEAN。Root 讀取完整 production／test／文件差異，執行公開回歸。獨立對抗審查重新核對最終 tracked Go diff，沒有新的確認 P0／P1／P2。靜態審查不能證明沒有其他 bug，完整編譯與執行證據由 Root 驗證。分工詳見 [review.json](review.json)。

## Changed

逐檔變更表見 [changed-files.md](changed-files.md)，所有新增證據、日誌與指紋索引見 [artifact-manifest.json](artifact-manifest.json)。

## Actions

本輪只清除自行建立的 `runs/.tmp/mechanism-audit-20261013`，原本 `runs/.tmp/62`、`runs/.tmp/real-task-quality-20261012` 保留。實際結果見 [cleanup.json](cleanup.json)。Git 提交、推送與遠端讀回見 [delivery.json](delivery.json)。

## Notes

另有既有的 P2 診斷報告不一致：ConsolidationReport 註解說拒絕寫入仍回報預算計數，但 Used／LastEpisode 只在成功分支填入。已記在 [AGENTS.md](../../../AGENTS.md) 的 Follow-ups。實際記憶與預算狀態沒有因回報欄位缺漏而改寫，尚無模型品質受到影響的證據。此項不列為本輪已修復的七類缺陷。

## Next

在同一真實手寫字辨識任務、固定資料分割與種子下，量測修正對效果的影響。機制稽核與工程測試仍須和訓練品質、生物正確性分別驗收。

💡 減法提醒：將固定符號檢查收回共用建構入口，移除 Trainer 的重複判斷，沿用快照格式。
