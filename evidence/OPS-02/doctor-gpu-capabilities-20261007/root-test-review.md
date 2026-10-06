# Doctor 能力回歸審查

Root 依 ticket 44 讀過完整 `internal/cli/doctor_test.go` 差異。公開 Doctor 與
JSON 檢查 GPU 六項能力、16 個限制／執行欄位、零延遲值、原 CPU 欄位、
CPU 不輸出 details、Insyra 分離與 v1 格式。

缺少可選工具的案例會以空 PATH 實際呼叫 Doctor。第一版手動組裝硬體狀態的
測試沒有執行產生報告的入口，已退回並替換。隔離測試確認每份 details 各自擁有
指標；CLI 與 context 案例涵蓋 nil、取消、JSON、help、非法參數與寫入失敗。
Root 還原原 parser 測試名稱，改擴大篩選器，避免無關重新命名。

Root 在可編譯骨架實跑凍結測試：四項新行為失敗，九項既有行為控制通過，見
`focused-final-red.log`。測試 SHA-256 在 `frozen-tests.json`，實作前 Go
來源在 `red-source.json`。實作 agent 只能改 `doctor.go` 與 `run.go`，
不得修改測試。
