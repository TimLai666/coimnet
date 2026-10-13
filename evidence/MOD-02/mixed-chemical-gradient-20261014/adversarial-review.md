# 最終初始活動修正的唯讀審查

審查來源 `learning/chemical_sequence.go` SHA-256 為 c73b9c655e451ac5dd882547a260c69e48ea5379b4b08c53c4dd2cf3b5ac9305。獨立審查者是原生 subagent gpt-5.6-luna／max，沒有修改檔案或執行 Go 測試。Root 親自核對來源、完整 diff 與數值驗證結果，最後測試結果以 verification.json 為準。

本次靜態審查範圍沒有新的可確認缺陷。這不是零 bug 結論。

- `learning/chemical_sequence.go:500` 至 529：-1 僅在首個反向區塊且沒有明示 InitialActivity 時保留。其他負步、視窗外步及非首段負步排除，非負步的原截斷條件保持。
- `learning/chemical_sequence.go:594` 至 607：初始活動先回傳，其餘依遞增步號回傳，values 保持複製，形狀、有限值及錯誤處理保持。
- `learning/chemical_sequence.go:342`：旗標是私有欄位，沒有更動前向、來源釋放、可訓練參數群組或保存格式。
- `dynamics/coupled_reverse.go:134`：fresh continuous 可接收 -1，持續歷史停止。
- `dynamics/mixed_coupled.go:474`：只對連續節點傳遞 -1，LIF 初始突觸輸出保持常數零。

Mixed 測試指紋為 09da8868555df9f45804aa18c4865ce376d0c06d7c66b7e2286ec6d0145b7242；Initial 測試為 36f1889f9cc38933ebfdf009bc00a4f31d1aedc7b51b27a8cd3304c2de4d044f。指紋核對不取代 Root 的實際執行。

未由本審查者執行測試、race 或真實品質量測。多訊號及其他歷史組合的獨立驗證範圍依 root-review.md 與 verification.json 管理，不把缺少新參考當成已確認的品質缺陷。
