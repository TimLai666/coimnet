# PPO 更新 API 使用指南

`learning/rl` 提供從零初始狀態開始的 PPO 更新。呼叫端負責與環境互動並收集每一步的觀察、行動、機率及回饋。

## Update 的呼叫與回傳

```go
updated, report, err := rl.Update(ctx, ind, rollouts, actions, cfg)
```

`Update` 從快照建立新個體，保留原本的 `ind`。呼叫端成功後再換成回傳的指標：

```go
next, report, err := rl.Update(ctx, ind, rollouts, nActions, cfg)
if err != nil {
    return err  // ind 保持原狀，先處理錯誤再決定是否重試
}
ind = next
```

`ctx` 必須非 nil，`ind` 必須是已初始化的個體，`actions` 至少 1，`rollouts` 至少一個。

呼叫端須依序收集、更新並採用回傳個體，更新期間不要修改 rollout 或同時替換同一個體。`Update` 使用呼叫開始時的快照，不會合併另一條執行緒後來做的更新。

## Update 的前置檢查

開始算梯度之前，`Update` 先驗證下面各項，任一不符就回錯誤並保留原個體：

- 讀出寬度要等於 `actions + 1`：前 `actions` 個是動作 logits，最後一個是價值。
- 模型必須宣告 `Config.ReadoutEveryStep`，否則只有最後一步有梯度。
- `cfg.MiniBatch` 只能為 1，一次 `StepFrom` 處理一個完整 rollout。
- 個體不能帶 plasticity 或 chemistry，帶了就回 `ErrUnsupportedMechanism`。
- 每個 rollout 的 `InitialNeural` 必須與同設定、零電位新個體的初始狀態相同，任意遞迴狀態目前不接受。

## Rollout 的欄位

```go
type Rollout struct {
    PolicyVersion   string
    InitialNeural   learning.NeuralState
    InitialPlastic  *learning.PlasticPart
    InitialChemical *learning.ChemicalPart
    Steps           []Transition
}
```

`PolicyVersion` 由 `rl.PolicyVersion(ind)` 產生：SHA-256 摘要，內容是網路的 canonical JSON 組態，以及 weights、bias、log_tau、theta_raw、encoder、readout 六組基本參數，任何一位元不同摘要就不同。fast 權重與化學狀態刻意不含在摘要，collector 另外記在 `InitialPlastic`／`InitialChemical`。更新時 rollout 的 `PolicyVersion` 或初始 fast／化學部分與個體不符合，整個 rollout 回 `ErrStalePolicy` 被拒絕，不會用到一半。

現階段個體不能帶 plasticity 或 chemistry，所以 `InitialPlastic`／`InitialChemical` 實際都只能是 nil，比對時要與個體的快照部分 deep-equal。`Steps` 至少要有一個 transition。

Rollout 必須由可信的 collector 提供，`LogProb` 與 `Value` 須保存收集當時的實值。策略版本只識別模型，不是 rollout 內容的簽章，也不驗證獎勵或觀察的真實性。從外部載入時，呼叫端負責來源與完整性驗證。

## Transition 的欄位

```go
type Transition struct {
    Obs             []float64
    Action          int
    LogProb         float64
    Value           float64
    Reward          float64
    Done            bool
    Timeout         bool
    BootstrapValue  float64
}
```

- `Obs` 寬度要等於 `Config.InputSize`。
- `Action` 要在 `[0, actions)` 內。
- `LogProb` 是該動作當下的對數機率，必須有限且不超過 0。
- `Done` 表示環境真的終止，`Timeout` 表示時間到被切掉，兩者分開。
- `Timeout` 的最後一步要填 `BootstrapValue`，也就是 collector 當下記下的 `V(s_{t+1})`。`Done` 那步的後續價值當 0。
- `Done` 與 `Timeout` 不能同時成立，只有最後一步可以帶其中一個，rollout 也必須以其中一個結束。

優勢與價值目標由 `Advantages(steps, GAEConfig{Gamma, Lambda})` 計算，GAE 反向遞迴：`delta = reward + gamma·V_next·(1 − done) − V`，`A = delta + gamma·lambda·(1 − done)·A_next`，`Done`／`Timeout` 結束遞迴。

## BurnIn、TimeLimit、設定

- `cfg.BurnIn` 指定的前綴不計直接損失，也不進報告平均，但後續步驟的梯度仍可經核心回傳到前綴。這個入口沒有切斷前綴梯度。`BurnIn` 必須小於 rollout 長度，確保至少有一個計分步。
- `cfg.TimeLimit` 是每個 rollout 的上限，步數超過就拒絕。最後一步是 `Timeout` 時，步數要恰好等於 `TimeLimit`。
- `PPOConfig` 其餘欄位：`Gamma`／`Lambda` 在 (0, 1]、`ClipEpsilon` 在 (0, 1)、係數有限且非負、`Epochs` ≥ 1。每個 epoch 依 rollout 順序各做一次 `StepFrom`。

## 後端

`learning.Trainer.StepFrom` 是模仿與 PPO 共用的梯度入口。`Update` 從個體快照還原訓練器，保留最佳化器動量、更新次數與尚未完成的梯度累積。每個 rollout 的輸出梯度按步相加，`PPOReport` 則回報最後一個 epoch 計分步驟的損失平均、機率比平均與裁切比例。

## 目前狀態

`coimnet examples run gridnav --method ppo` 提供 3 個 seed 各 200 次更新的完整人工範例，包含取樣收集、下一筆觀察的 timeout bootstrap、訓練前與隨機基線對照，以及同平台重現檢查。用法見 [走廊範例](../../experiment/gridnav/README.md)，實際驗證見 [LRN-09](../../evidence/LRN-09/verification.json)。

任意非零初始狀態、切斷暖機前綴梯度，以及可塑性／化學機制的 PPO 梯度仍未支援，依 [ticket 26](../../docs/tickets/26-continual-matrix-imitation-ppo-and-bio-inspired-protocols.md) 保留後續工作。人工走廊的成功不代表完整果蠅圖已完成回饋學習。
