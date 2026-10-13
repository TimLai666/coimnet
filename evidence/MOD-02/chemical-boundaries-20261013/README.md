# 化學與線上訓練一致性驗證

七類已確認缺陷已修復，包含錯誤化學回饋、共享狀態恢復、失敗回饋重試、累積更新計數，以及化學數值／來源／受體更換。觸發、影響與直接失敗見 [findings.md](findings.md)，契約核對見 [root-review.md](root-review.md)。

- 七份新測試、22個頂層回歸通過，負面反例先在原實作失敗，正常控制保持。
- 完整一般與race各58套件通過。一般5,087個pass事件／22個skip，race為5,086／23，差異為既有大型案例。完整限制見[suite-summary-final.json](suite-summary-final.json)。
- 22筆合法公開輸出三輪逐位相同。CLI新程序80+40與連續120步存檔相同，最後建置的合法存檔也保持。
- 格式778份、build、vet、唯讀相依性核對與原始教師目標流檢查通過。最終782份來源指紋固定，沒有改歷史證據或全域快取。
- [完整工程驗收](verification.json)已通過，本輪11,212個暫存檔、5,597個快取連結共1,878,186,656bytes已清除，見[清理回條](cleanup.json)。修正e7fcc71與回歸／證據f0f0d01已推送並核對遠端相符，見[交付回條](delivery.json)，沒有新的辨識率或生物完整性結論。

各命令JSON記錄環境、命令、SHA-256與gzip日誌。source-final.json固定來源，frozen-tests-final.json固定回歸。初始 full-race 是已中止的被取代驗證，最終驗收使用 full-final-race。overlay輸入保存在overlay-inputs，重播須先依映射還原至所記錄的專案runs/.tmp路徑。

大型快取與中間產物只在本輪runs/.tmp，證據保留後只清理本輪暫存。

逐檔變更見[changed-files.md](changed-files.md)，證據指紋見[artifact-manifest.json](artifact-manifest.json)。
