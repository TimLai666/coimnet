# 訓練累積溢位與拒絕狀態稽核

原始目標保持為「確保沒有任何 bug」。本票四項里程碑只追蹤這項已確認缺陷。

基準 `cf15bd4cc6d0884866e5393617c5b6ccdd8c55dc`，809 份來源已逐一核對。公開 `Network.LossGradientFrom` 與 `Trainer.StepFrom` 取得每步有限梯度 `9.9894100223432e307`，視窗 3 的第二步仍成功，Sum 出現 Inf。JSON 保存與 RestoreTrainer 隨後拒絕。

根因在共用 stepWithGradient：候選累積器加總後，未滿視窗的分支沒有有限值檢查就提交。已有快照規則明確拒絕非有限 Sum。修正沿用可表示性與候選先驗證的契約，不增加 API、保存版本或新累積算法。

## 四項里程碑

1. 公開重現及共用入口追蹤，確認根因與正常／部分成功契約。
2. Root 先寫並凍結失敗回歸，執行者只填正式修正。
3. 一般／競態／工程、合法模型與新程序接續驗證。
4. 保存證據、清除本輪暫存、提交與遠端核對。

## 路徑與檢查

`Step / StepFrom / StepFromState / Individual.TrainEpisode / 化學 TrainEpisode` → 公開梯度 → mask、loss scale、sum 候選 → 有限性 → partial 提交或完整平均、裁切、AdamW → 候選參數驗證 → 成功提交。

| 情境 | 預期結果與驗證 |
| --- | --- |
| 正負有限梯度相加溢位，視窗未滿 | 回錯、空 StepResult、完整快照保持，仍可 JSON 保存與恢復 |
| 正負有限梯度相加溢位，視窗剛滿 | 回錯且原狀態保持 |
| 一般、外部梯度、stateful、個體與化學個體 | 共用拒絕契約，神經／化學／來源／回饋／報告／episodes 不因失敗改動 |
| 分段重算 | 仍由公開入口產生梯度，拒絕且狀態保持 |
| 被凍結的大梯度 | 不加進 Sum，正常 partial 成功 |
| 拒絕後用相反梯度重試 | 可繼續累積或完成更新，狀態可保存恢復 |
| 正常累積、loss scale、mask、裁切、schedule、取消 | 沿用既有回歸與實際新程序接續，數值和保存語意保持 |
| GPU producer | 共用檢查適用，但未證明其 float32 路徑可到達此溢位，不宣稱硬體重現 |

Root 只新增 `learning/accumulate_overflow_test.go` 並保留凍結指紋。實作僅允許 `learning/trainer.go`，逐項加總候選後立即拒絕非有限 Sum；不得改測試。

【減法審查】

- Won't List：不改平均／裁切順序、正常算式、GPU 契約或 online mixed 的部分成功語意。
- 建議移除：不增加另一套累積器或保存格式。
- 建議簡化：在共用候選加總處加入檢查，讓所有入口沿用。
- 需要留意：工程測試不證明真實任務品質提升或整體零 bug。
