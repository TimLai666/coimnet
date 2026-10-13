# 化學組合與來源歷史稽核

本輪確認 MOD-04 的零作用增益缺陷，正式程式尚未修復。結論、影響及候選相容性決策見 [Root 審查](root-review.md)，實際命令與輸入指紋見 [驗證](verification.json)。

`neutral-probe.log` 是原程式公開入口的 15 個案例，9 個偏離中性增益。`neutral-candidate-*` 只驗證未採用的 overlay 候選。`history-probe-v3` 及 `initial-bridge-hand-v2` 為最後有效的診斷，早期失敗的分類見 Root 審查。

可用下列命令重現原來源的缺陷，所有輸出與 Go 快取均放入專案內：

```sh
python3 evidence/MOD-02/chemical-composition-20261014/reproduce.py neutral --work runs/.tmp/chemical-composition-replay
```

工作目錄必須不存在。來源須符合 baseline.json。新快取需要取得 go.mod／go.sum 固定的相依套件；離線重現可加 `--module-cache <專案內既有快取目錄>`。命令不覆寫正式程式，neutral 模式成功表示重現了記錄的缺陷。history、initial、candidate-unit、candidate-race 模式分別重現對應診斷；candidate 模式的通過不表示修正已採用。

本輪暫存已清除，見[清理回條](cleanup.json)及[實際檔案清單](changed-files.md)。這是有範圍的工程檢查，完整框架與真實模型品質沒有由本輪重新驗收。
