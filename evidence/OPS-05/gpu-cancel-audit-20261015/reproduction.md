# 重現本輪驗證

每個 *.json command receipt 記錄實際 argv、時間、exit code 與 *.log SHA。環境見 environment.json，正式來源指紋見 source-before.json，實際輸入與預期快照見 gpu-resume-fixtures.tar.gz。

本輪暫存完成驗收後會清除。重現時建立專案內新的 runs/.tmp 子目錄，在該目錄還原以下來源與本機 Go 快取。使用新的證據目錄與新標籤保存輸出，原本驗收 receipt 不覆寫。

| 保存的來源 | 原暫存檔名 | 建置／執行 receipt |
|---|---|---|
| gpu-cancel-probe-v2.go.txt | gpu_cancel_probe.go | gpu-probe-v2-build.json、gpu-probe-v2-race-build.json、兩份 gpu-probe-v2-*-hardware.json |
| multichunk-probe.go.txt | audit_gpu_cancel_test.go | multichunk-test-build.json、multichunk-race-build.json、兩份 multichunk-*-hardware.json |
| resident-probe.go.txt | audit_gpu_resident_test.go | resident-test-build.json、resident-race-build.json、兩份 resident-*-hardware.json |
| float-probe-v2.go.txt | float_probe.go | float-probe-v2-build.json、float-probe-v2-race-build.json、兩份 float-probe-v2-*.json |

Go overlay 只新增虛擬測試檔。multichunk-overlay.json 與 resident-overlay.json 記錄正式虛擬位置至原暫存位置的對應，重現時把暫存目的地替換成自己的專案內目錄。未新增正式方法。

run-command.py.txt 與 cache-prepare.py.txt 保存當時工具原件。它們含原絕對路徑，重現時需在副本中指向新的專案內暫存／證據目錄。不要在已交付目錄重建 cache-preparation.json 或覆寫歷史輸出。native 模組來源只作唯讀連結，Insyra 保持 v0.3.4。硬體執行需要可用 Apple M3 Metal，Software Renderer 不可替代。多區塊測試沿用既有降低容量 fixture，不量測硬體上限。

新程序恢復的命令與每一個完整比較在 new-process-resume.json，執行工具在 new-process-resume.py.txt。完整工程命令在 full-unit.json、full-race.json、build.json、vet.json、mod-verify.json、modules.json、format.json 與最後 governance-final-validated.json。沒有設定資料／證據環境變數的全套略過項目另列於 full-*-summary.json。
