# 專案內資料保存與輸出路徑審查

Scope: CLEAN。基準提交為 `9f69d2f9a8e7e320171eb12362ad4b469116cd8f`，範圍為 tickets 52、53 的完整 diff 與本機保存操作。

Adversarial review: RUN。Luna max 唯讀審查搬移、輸出路徑與文件，Root 逐項核對程式及實際結果。

## 結論

沒有尚待修正的已確認缺陷。Root 讀過全部程式 diff、兩個完整新測試檔、範例文件及保存紀錄。程式改動限於 NPF 的兩個數值判斷、realnav 的輸出位置，以及對應 help。沒有改訓練、模型、依賴或公開格式。

- realnav 先解析符號連結，再檢查來源資料重疊。最近的 `.git` 定義 repository，只有其 `runs` 新子目錄可以通過 repository 檢查。根目錄、`runs-extra`、既有輸出，以及連到資料或其他 repository 路徑的輸出會回錯。巢狀 repository 按最近 `.git` 判斷，沒有新增「所有祖先 repository」政策。
- 四個新回歸與原 repository／symlink 控制通過。實際移位後推論與舊報告比較，只移除執行量測與快照路徑，其他完整內容相同，見 `realnav-readback.json`。
- 搬移使用同檔案系統的排他 rename，沒有覆寫。逐檔重新讀取並比對 SHA-256，原始資料、報告與模型內容保持。`retained-after.json` 核對最後的完整檔案集合。
- 暫存清理不跟隨符號連結。兩份來源副本逐檔等於指定 Git archive，其他全圖資料與結果保持。舊執行檔只當成可清理的編譯產物，沒有宣稱能重建相同 bytes。

## 文件修正

審查發現 realnav README 的 Windows 測試執行檔仍放 `/tmp`，已改成 `runs/.tmp/` 並附清除命令。Gutenberg 舊結果與一次性 runner 分別保存在 `runs/TSK-11/gutenberg-20260924/`、`runs/.tools/TSK-11/gutenberg-runner/`。舊 runner 若重現時須明示 `--manifest` 指向 `data/TSK-11/gutenberg/manifest.json`，不依賴搬移前的相對位置。

## 驗證限制

本輪證據支持輸入驗證、合法報告不變及本機檔案保存。沒有增加任務學習品質、生物機制、GPU 或遠端平台執行證據。生物啟發報告的設定共用 P2 保留在 AGENTS.md，另票處理。
