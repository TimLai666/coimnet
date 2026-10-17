# 語音串流子代理唯讀報告

工具：Codex 內建子代理 `asr_stream_consistency_137`，`gpt-5.6-luna`，`max`。
以下保留子代理結論，Root 裁決見 `root-review.md`。沒有執行 Go 或修改檔案。

- `tasks/asr/stream.go:37–64`：NewStream 複製建立當時參數；`75–151`：Feed 保存完整 checkpoint，失敗恢復神經、pending、CTC、文字及 frame 計數。
- `tasks/asr/stream.go:153–203`：Flush 補零未分析尾端，已有完整 frame 且只剩 overlap 時不重複分析。
- `tasks/asr/stream.go:263–398`：Snapshot／RestoreStream 保存並核對音訊、神經、CTC、文字、前處理、字母表及 frame 可達性。
- `tasks/asr/stream_file.go:34–115`：檔案 schema、checksum 與欄位檢查後再恢復串流。

沒有確認的普通有效輸入缺陷。固定 Window／Hop 消費 pending，分塊不改變完整視窗順序。既有 `stream_test.go:138–182` 涵蓋 700、1 與單一大分塊；`stream_validation_test.go:155–200` 涵蓋中途取消、完整快照回復及重送。

整檔捨棄短尾與串流補零是 README 明示差異，同串流並行呼叫也明示不支援。

限制：沒有動態驗證任意隨機分段，特別是 Window−1、Window、Window+Hop−1 的直接 Feed 分段。這是證據缺口，沒有失敗重現。下一個最小檢查可比較這些分段的最終 Text 與完整 Snapshot。
