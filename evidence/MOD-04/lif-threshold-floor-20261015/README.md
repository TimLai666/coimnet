# 化學門檻下限驗證

有限的負向化學偏移造成中間加法溢位時，耦合訓練原本會在下限投影前回錯。一般模擬則依既定 `max(theta + shift, theta_min)` 取得有限結果。兩個耦合入口已調整為先完成下限投影，再檢查最終門檻。

- [原始重現](reproduction.json)：公開差異與精確有理數參考。
- [失敗回歸](regression-red-verification.json)：一般／競態各 4 個失敗事件、10 個控制通過。
- [候選驗收](candidate-verification.json)：一般／競態各 14 個回歸通過，32 組正常核心與 12 次舊模型新程序接續逐位相同。
- [Root 審查](root-review.md)：契約、完整修改範圍、反向、拒絕與相容性核對。
- [本票與驗收清單](../../../docs/tickets/110-chemical-threshold-floor.md)。

[完整工程驗收](verification.json)已通過：一般／競態各 58 套件、5,929／5,928 個成功事件，22／23 個略過、零失敗。823 份來源的格式、build、vet、固定相依性與 31 項治理通過。[自有暫存](cleanup.json) 1,789,486,258 bytes 已清除，18 個原模型檔、6 個必要資料檔與兩個舊暫存目錄保持。修正 007d4b3 與回歸／證據 76689bf 已正常推送且遠端讀回相同，見[交付](delivery.json)。本票完成 5／5。這些人工控制沒有建立真實辨識率或生物機制的定量結論。

## 保存與重跑

`preserved-work.tar.gz` 保存小型來源、凍結測試、overlay 與 12 個人工舊模型。`work-preservation.json` 列出每個成員的位元組數及 SHA256。正式修正前的完整兩份來源另存為 `lif_coupled.go.before.txt` 與 `mixed_coupled.go.before.txt`。`source-before.json`／`formal-source.json` 分別固定修改前及正式來源。

正式回歸可用 `go test -count=1 -run '^TestThresholdProjection' ./dynamics` 與同指令加 `-race` 重跑。所有 Go 快取、module cache 與 TMPDIR 都應另設於專案內新的 `runs/.tmp/` 子目錄，沿用 `command-environment.json` 的本機離線 module proxy。完成並保存需要的日誌後清除該自有目錄。

重現舊錯誤時，在同一命令加 `-overlay`，把兩個正式 Go 路徑分別對應到兩個 `.before.txt` 原檔。測試保持正式凍結版本。預期三個負向溢位子案例失敗，其餘控制通過。`run-command.py.txt` 與 `engineering.py.txt` 是原始驗收 runner，包含本輪專屬目錄指紋保護，清理後不應原樣重建執行。
