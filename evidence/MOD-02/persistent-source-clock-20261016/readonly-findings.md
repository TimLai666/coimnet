# 時間契約唯讀盤點

兩名既有 native gpt-5.6-luna/max 執行者分別盤點既有測試與時鐘所有權。Root 重新讀取以下當前程式確認主要結論，執行者本身沒有執行 Go 或寫入正式檔案。

- `learning/individual.go:677-704`：持續來源使用神經模型步數，非 chemical State.Steps。
- `learning/chemistry.go:168-200`：凍結列跳過來源與 kinetics，未凍結列先按神經時間篩選可用回饋。
- `learning/chemistry.go:346-376`：凍結旗標不進快照，回饋抵達後持續可用。
- ticket125 來源只讀活動，未獨立驗證 persistent timeline／feedback 的絕對生效時間。既有 episode 時點測試不代替本票持續入口。

## 後續線索，不列已確認缺陷

`learning/intervene.go:85,135-153` 以 snapshot 重建對照個體，snapshot 不含 FreezeChemistry 的 runtime 旗標。若在已凍結個體執行 Intervene，對照可能解凍，使 ActivityDelta 混入自然化學演化。尚未執行最小重現，也未確認 Intervene 支援 frozen runtime 的契約。下一次先用非零濃度和兩邊同為 frozen 的控制確認，再判斷是否要修。

`learning/chemistry.go:374-376` 的舊註解仍稱來源不讀 score，但 Controller 已讀取。屬文件過時，正常運算不因此改變。本票只保存線索，沒有為修註解改動已驗證的正式來源。
