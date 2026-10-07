# 格式索引來源核對

主 agent 依本機原始碼及測試斷言核對分類。這份紀錄驗證文件準確性，不作為新增的執行相容性或任務學習證據。

| 版本 | 來源與實際行為 |
| --- | --- |
| model-package v2 | `checkpoint/package_test.go:254-256` 建立未來版本／被改過的模型包，`304-312` 要求拒絕。`checkpoint/package.go:26` 宣告 v1，`209-210` 拒絕其他版本。 |
| lif-state v0 | `dynamics/lif_state_test.go:358` 改動版本，`388-400` 要求驗證及接續失敗。`dynamics/lif_state.go:82-83` 只接受 v1。 |
| replay v0 | `replay/replay_test.go:485-509` 要求恢復失敗。`replay/store.go:53-55` 只接受 StateVersion v1。 |
| simulate-parameters v1 | `simulate/parameters.go:18,261` 用於區分 SHA-256 雜湊的用途，沒有宣告檔案格式。 |
| logmel v1 | `tasks/asr/audio/frontend.go:11,176` 識別感覺輸入的特徵定義。 |
| textgen-fixture v1 | `tasks/textgen/corpus.go:298` 識別人工語料的來源。 |

獨立唯讀盤點涵蓋框架報告、治理 JSON、shell／Python 驗證報告及 13 個測試資料／反例版本。其中 12 個是拒絕案例，另一個是隱私掃描的人造資料。六份在 Go 測試中實際產生的報告屬於驗證工具格式。

主 agent 親自讀過完整測試、INDEX diff 及 124 個來源位置。最終文件改用四份 Go 報告的產生位置，修正 LIF v0、隱私測試資料與 ASR 套件的說明。兩份跨程序重現報告的產生程式會組合版本字串，因此索引引用核對程式中的直接版本字串。

最終索引有 124 個不重複版本，分為正式 87、規則識別 10、驗證工具 14、測試資料／反例 13。173 個本機文件連結有效。主 agent 的格式／治理專項與無 Git 匯出測試通過，凍結測試指紋保持。
