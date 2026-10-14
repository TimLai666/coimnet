# 累積溢位修正的對抗性來源審查

Scope CLEAN。基準 `cf15bd4cc6d0884866e5393617c5b6ccdd8c55dc`。Reviewer 為本輪實際接受的 native `gpt-5.6-luna`、effort `max`，任務 `individual_failure_review_98`。Reviewer 唯讀，沒有執行 Go、測試或 Git 寫入。以下是來源審查，不取代 Root 的實測。

沒有確認 P1／P2，沒有由目前差異支持的新待查疑慮。

正式差異在 `learning/trainer.go:460-466`，有效 mask 的候選加總後加入 finite guard。候選 Sum 先由 copyAccumulator 深拷貝，拒絕時 Count 尚未增加、tr.accumulator 尚未寫回，參數／Adam／updates 也尚未提交。有限加總值沒有改寫，後續平均、裁切、CPU 或 device 路徑維持。

共享梯度先經 reduction 與共享群組的凍結規則再進累積。guard 不讀凍結項，device 更新也在 guard 後方，沒有新旁路。核對來源為 `learning/trainer.go`、`learning/accumulate.go:34-39`、`learning/sharing.go:118-162` 及 `learning/gpu_device_trainer.go:123-216`。

Reviewer 核對 Root 新測試指紋 `e1057f58c02821bb4ff2600f3a4cdf98c4a63f6d2b4a68f786c57d9c1c6ae134`，來源涵蓋 Step、StepFrom、StepFromState、個體、化學個體、重算、partial／full window、JSON／Restore 與相反梯度重試。未直接執行 GPU 或共享的溢位重現，不宣稱那些 producer 可以產生此尺度的梯度。

## 來源審查檢查點重建

最初 reviewer 回報 Skills：無，不能作為技能入口已完成的證據。Root 已重開來源審查檢查點，要求同一 reviewer 實際讀取 software-engineering-guidelines、diff-inspector、human-writing 與完整寫作參考、project-memory，完成 load 且不寫記憶。Reviewer 在同一三行差異與兩份固定指紋上重新完成審查，Scope CLEAN，沒有確認缺陷或新增待查。重新審查沒有修改 Go、測試、Git 或記憶，也沒有重啟仍在執行的 Root 驗證。這是補完來源審查，不把第一次未證明的入口追認為完成。

Root 對照正式來源 SHA `404e2d932e369c48ee15c69c0f249b860ec7732135d26616ce34a376922ea686` 與凍結測試 SHA `e1057f58c02821bb4ff2600f3a4cdf98c4a63f6d2b4a68f786c57d9c1c6ae134`，兩者相符。
