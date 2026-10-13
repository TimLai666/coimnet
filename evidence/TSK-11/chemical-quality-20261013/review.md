# Ticket 66 配對比較審查

審查範圍為範例 driver、固定 protocol、資料／更新公平性、一般化學正向與反向、新程序恢復，以及證據來源。框架來源基準為 `44943f5aae4d157cd3a22e2858607066784ee382`。

## 已確認問題及處理

P2：原 runner 只核對五個 driver 檔案及執行時讀取的 protocol，最終驗收未自動核對框架、wrapper、分析腳本與編譯程式。補上 [source-lock.json](source-lock.json) 與 [verify_freeze.py](sources/verify_freeze.py.txt)。760 份框架來源、五份 driver、三份 wrapper／分析來源、編譯程式及六份資料副本通過，故意修改或移除來源的負對照會被拒絕。

補強發生於正式訓練啟動後，屬最終來源稽核。它不代表早期每次啟動已有逐次自動上鎖。正式 protocol 與初版只差確認分割的 metadata，確認分割的指紋已在正式訓練前的 preflight 中出現。訓練設定、driver、編譯程式與正式 runner 都未因此修改。

## 數值與任務核對

- ON 在 `learning/network.go:595` 進入化學正向，`learning/chemical_episode.go:212` 呼叫耦合核心。`dynamics/coupled_modulation.go:171` 在神經更新前改變輸入驅動。反向由 `learning/network.go:429`、`dynamics/coupled_reverse.go:29` 與 `learning/chemical_sequence.go:427` 回傳來源活動的梯度。
- OFF 的損失函式與 `tasks/ocr/recognizer.go:332` 相同，輸入由原 SDK 產生。Root 已實際核對十張真實圖片的輸出、損失、最佳化器與連續更新逐位一致。
- 中性設定的輸出、完整梯度及更新與 OFF 一致。active 設定的輸出、梯度及更新不同且有限。取消不提交更新，快照雙向隔離通過。
- 原件保留，訓練只使用 6,000 張官方訓練圖片。主要及確認分割分別取官方測試資料每類位置 20–219 與 800–999。評估前後完整快照指紋相同。
- 兩組的可訓練參數為 23,516。ON 額外五個固定值是 controller 向量，其他固定反應係數另列於 protocol，沒有把所有工程常數合稱為五個參數。
- 每組固定 120,000 次更新，不依測試選參數、挑種子、早停或選最佳 epoch。主要測試分割先前已觀察，沒有盲測宣稱。

## Root 收件及工具

Root 閱讀完整 slot 實作與測試，核對允許檔案及凍結測試指紋，親自重跑五項測試及真實圖片 SDK 比對。RED 有四個正向案例因未實作 slot 失敗，未知 group 的拒絕案例通過。實作 agent 沒有修改測試。

宿主：Codex。依專案規則，測試、slot 實作與唯讀稽核使用 `collaboration / gpt-5.6-luna / reasoning_effort=max / fork_turns=none`。本輪先嘗試 OpenCode，其模型清單呼叫失敗，沒有執行 OpenCode 模型：

```text
Error: Unexpected error
Unknown: FileSystem.open (/Users/timlai/.local/share/opencode/log/opencode.log)
```

獨立稽核由 `chemistry_ocr_protocol_audit` 及 `chemical_ocr_experiment_review` 完成，數字與結論仍由 Root 對照原件裁定。最終來源核對的 P2 已補足，未確認化學數值核心的新缺陷。品質結果及完整驗證由其他證據記錄，不以審查意見代替。

## 歷史評分分析修正

初版 analyzer 以相同浮點值比對舊評分與 SDK 損失，實際運算順序不同而失敗。Root 的 historical-score-probe 在相同凍結 logits 上精確重現兩種算式，六筆結果各自與舊報告及本輪原始報告完全相同。analysis 改為分別要求各自 exact，不使用容許誤差，原版分析、來源鎖與失敗日誌保留。訓練設定、driver、編譯程式及六組模型沒有變更。見 [historical-score-rounding.json](historical-score-rounding.json)。

最終獨立核對另從 raw JSONL 重算三組主要／確認差值、平均及配對錯誤，與 comparison-results 完全一致。12 份訓練／還原回條及日誌指紋相符。歷史 CE 的六筆 probe 僅指三個 OFF 種子的主要／訓練分割，沒有延伸到 ON 或確認分割。

## 清理邊界稽核

獨立唯讀稽核沒有發現 cleanup／archive helper 的確認缺陷。Root 核對固定目標、逐檔盤點與指紋條件，連結不追蹤、不同掛載拒絕，並以 33 個唯一檔案保護三個舊模型、六個新模型及六份資料原件。封存不排除失敗回條，解壓日誌必須符合原始指紋。實際清理及最終封存只依 cleanup.json 與 logs/index.json 判定，不以程式審查取代執行結果。
