# ASR／OCR 呼叫路徑一致性稽核

本輪沒有確認新缺陷，沒有修改框架 Go、模型格式、學習規則或既有測試。這是明定路徑的唯讀稽核，不是完整零 bug 證明，也沒有新的真實任務品質或生物機制證據。

## Root 核對

| 範圍 | 原始碼與契約 | 判定 |
| --- | --- | --- |
| 中性增益 | `modulation/effects.go:196–209` 先套用零值預設，再要求 `0 < gamma_min <= 1 <= gamma_max`；ticket89 舊檔驗收涵蓋明確拒絕與合法接續 | 使用者已核准的修正存在，沒有再次改動 |
| ASR 分塊 | `tasks/asr/stream.go:111–151` 接續 pending 並以相同 Window／Hop 消費完整窗；`159–203` 明定 Flush 補零與 overlap 行為；`tasks/asr/README.md` 明示整檔／串流尾端差異 | 沒有確認偏離契約 |
| ASR 恢復及失敗 | `tasks/asr/stream.go:75–99,282–398` 保留完整狀態與語意驗證；`stream_file.go:37–115` 使用完整保存物與嚴格載入 | 沒有確認偏離契約 |
| OCR 頁面 | `tasks/ocr/page.go:39–71,134–157` 切分及複製原座標像素；`page_recognizer.go:24–107` 逐行正規化後呼叫同一個 Infer，錯誤回零值 Page | 沒有確認偏離契約 |
| OCR 模型狀態 | `tasks/ocr/recognizer.go:266–293` 走凍結 PredictAll；`learning/trainer.go:273–284` 明定不變更 trainer；`page_recognizer_test.go:14–95` 比較完整快照及非空輸出控制 | 沒有確認狀態污染 |

Root 實際讀取上述原始碼及相關測試。codebase-memory 用於定位，函式尾端與行號另以當前檔案核對，沒有把工具截斷的片段當成完整函式。

## 子代理線索裁決

- ASR 的任意隨機 Feed 分段尚未動態比對，保留為驗證限制。現有固定 1、700 及整段輸入、保存接續、取消重送是已存在的實際測試，不能外推到所有分段。
- OCR 子代理提出的「底層取消後仍回成功」未建立反例。Root 補查 `learning/network.go:669–675`，讀出有限值檢查後已有 context 檢查。`InferPage` 在每行 Infer 後及整頁回傳前也有檢查。最後檢查之後的取消時序或 CER 中途取消沒有重現，且沒有找到成功完成後必須撤回結果的明文契約，因此不判為確認缺陷。
- 頁面切分只承諾單欄水平文字，不把複雜版面、自然辨識錯字或既有 Invert 政策未定事項重新列為本輪缺陷。

## 驗證證據

`validate-evidence.py.txt` 重新核對全部 855 份 Go／module 指紋及 32 份其他工作，再驗證上一輪已完成完整一般／競態日誌的壓縮與解壓 SHA-256，重讀實際 test2json 終端事件。ASR 各 12 個父測試／12 個 pass 事件，OCR 各 7 個父測試／19 個 pass 事件，中性增益各 21 個 pass 事件。沒有本輪新 Go 執行，完整工程證據沿用 byte-identical 來源的 ticket136。完整命令、環境、來源與原始日誌見 `reused-test-evidence.json` 及其引用。

OpenCode 已成功列出免費模型與費用資訊，但兩個稽核命令都被自動核准審查拒絕，沒有啟動模型稽核，也沒有取得 OpenCode 稽核成果。實際分工改用 Codex 內建子代理，模型與設定見 `delegation.json`。

減法審查：沿用共用增益驗證與現有回歸，不增加平行的驗證規則；未使用的派工腳本在保存指紋後清除；既有兩個暫存目錄保持。
