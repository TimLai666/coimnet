# PPO 數值修正的獨立審查

2026-10-06。唯讀審查者為 gpt-5.6-luna，effort max。審查目標為 objective.go、兩份數值測試、RL README 與 ENG.md，未涵蓋其餘追蹤文件。來源 objective.go SHA-256：30df00d2fdf9e9b0f5dbc4a9d4f28f7e5e763ceeb14d1668f6a3dee8ba44cf98。

## Diff Inspector

Scope: CLEAN。
Adversarial review: RUN，檢查公開 PPO 數值、裁切、遮罩與相容性。

### Findings

審查者沒有回報已確認的 P0／P1／P2。Root 親自讀完整 diff 並跑測試後，範圍內沒有剩餘的確認問題。

### Needs investigation 與 Root 判定

審查者提出兩種極端數值情況。Root 以公開 Loss 入口重現，見 review-probe.go.txt／review-probe.log，再用 Python decimal 500 位精度獨立計算，見 review-reference.json。

| 情況 | Root 實際觀察 | 本 ticket 的判定 |
| --- | --- | --- |
| value=MaxFloat64，target=-MaxFloat64，ValueCoef=1e-320 | value-target 本身溢位，Loss 回錯誤與空結果。理想加權損失約 1.29e297、梯度約 7.19e-12 可表示 | 此修正保留 float64 中間差值的可表示範圍，不新增跨溢位差值計算。README 明示此限制 |
| logits=[0,-750]，EntropyCoef=MaxFloat64 | Exp 的尾端機率下溢成零，熵分項與梯度為零。高精度加權熵約 -2.57e-15 | ticket 契約保留零機率跳過熵乘積的既有行為，不新增放大下溢尾端的對數域加權運算。README 明示此限制 |

兩個案例都沒有修改模型、係數範圍或公開 API，也沒有把理想高精度運算宣稱為已支援。若要擴大 float64 中間運算的動態範圍，應另定義範圍與公開入口回歸，不能用此次機率相減修正代替。

### 驗證責任

審查者回報 go test -count=1 ./learning/rl、gofmt -d 與 git diff --check 均退出 0，來源 SHA 符合。Root 自己執行 RL、collector、完整建置、一般測試、race、vet、相依性與保存指紋檢查，結果以 verification.json 為準。README 的限制說明是 Root 在審查後補上的文件修正，Go 來源保持原審查指紋。
