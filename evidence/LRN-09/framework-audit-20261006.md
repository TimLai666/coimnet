# PPO 框架盤點：先修機率計算，再補帶記憶的分段訓練

2026-10-06。檢查來源為 `9202bbf348accada3a0bfe89638e176993257140`，檢查前工作目錄乾淨。環境為 Darwin arm64、Go 1.26.5。本輪是通用框架盤點，沒有修改 Go 程式、模型、訓練參數、依賴或需求通過狀態。

## 已重現的數值錯誤，P2，尚未修正

[`LogProb`](../../learning/rl/objective.go#L134) 與 [`Loss`](../../learning/rl/objective.go#L76) 共用的運算先算 `max + log(sum)`，再從原分數減掉結果。當最大值很大，浮點數會丟失 `log(sum)`。兩個相等的有限分數本應各有 0.5 機率，`[1e16,1e16]` 卻各得到 1。相同策略的 PPO 比值因此從 1 變成 2，正優勢案例的策略梯度被錯誤裁切成零。

同一個公開入口另有輸入檢查缺口：`LogProb([0,NaN],0)` 與 `Loss` 都回傳 NaN 且錯誤為 nil。此結論是直接呼叫公開函式的結果，不能據此宣稱整個 `Update` 已寫入非有限參數。更新路徑另有檢查與候選個體隔離。

來源檔 `learning/rl/objective.go` SHA-256：`bdb8ec420400fa12bb5e4f34e78faefa81a76974a4b5e2998fb9b9f378c73281`。

本機命令：`go run /private/tmp/coimnet-framework-probe-20261006.go`，退出碼 0。重現程式原文如下，須放在儲存庫外執行：

```go
package main
import (
 "fmt"
 "math"
 "github.com/TimLai666/coimnet/learning/rl"
)
func main() {
 c:=rl.PPOConfig{Gamma:1,Lambda:1,ClipEpsilon:.2,ValueCoef:.5,EntropyCoef:.01,TimeLimit:1,Epochs:1,MiniBatch:1}
 for _,shift:=range []float64{0,1e16,1e308} {
  lp,e:=rl.LogProb([]float64{shift,shift},0)
  loss,g,v,le:=rl.Loss([]float64{shift,shift},0,math.Log(.5),1,0,0,c)
  fmt.Printf("equal_logits_shift=%g log_prob=%.17g expected=%.17g log_error=%v ratio=%g loss=%g gradient=%v value_gradient=%g loss_error=%v\n",shift,lp,math.Log(.5),e,loss.Ratio,loss.Loss,g,v,le)
 }
 lp,err:=rl.LogProb([]float64{0,math.NaN()},0)
 loss,g,v,le:=rl.Loss([]float64{0,math.NaN()},0,math.Log(.5),1,0,0,c)
 fmt.Printf("nan_input log_prob=%g log_error=%v loss=%g gradient=%v value_gradient=%g loss_error=%v\n",lp,err,loss.Loss,g,v,le)
 for _,done:=range []bool{false,true} {
  a,t,e:=rl.Advantages([]rl.Transition{{Value:1,Reward:1,Done:done,Timeout:!done,BootstrapValue:2}},rl.GAEConfig{Gamma:1,Lambda:1})
  fmt.Printf("termination_done=%v advantage=%v target=%v error=%v\n",done,a,t,e)
 }
}
```

實際輸出：

```text
equal_logits_shift=0 log_prob=-0.69314718055994529 expected=-0.69314718055994529 log_error=<nil> ratio=1 loss=-1.0069314718055995 gradient=[-0.5 0.5] value_gradient=0 loss_error=<nil>
equal_logits_shift=1e+16 log_prob=0 expected=-0.69314718055994529 log_error=<nil> ratio=2 loss=-1.2 gradient=[0 0] value_gradient=0 loss_error=<nil>
equal_logits_shift=1e+308 log_prob=0 expected=-0.69314718055994529 log_error=<nil> ratio=2 loss=-1.2 gradient=[0 0] value_gradient=0 loss_error=<nil>
nan_input log_prob=NaN log_error=<nil> loss=NaN gradient=[0 0] value_gradient=0 loss_error=<nil>
termination_done=false advantage=[2] target=[3] error=<nil>
termination_done=true advantage=[0] target=[1] error=<nil>
```

推薦修正集中在現有機率與損失計算：共用位移後的 log-softmax，驗證相等分數與所有分數加同一常數後結果不變，檢查非法輸入及計算溢位。動作遮罩是否允許 `-Inf` 必須依既有呼叫端與文件確認，不能在未盤點前擅自改變契約。

## 已明示的通用能力限制

[`Update`](../../learning/rl/update.go#L123) 只接受全新零狀態的 rollout，並拒絕可塑性與化學狀態。原因是計分可讀初始狀態，但 `Trainer.StepFrom` 的前向與反向從零開始，任意移除保護會讓更新梯度對不上收集時的輸出。限制已記在 [RL README](../../learning/rl/README.md#目前狀態) 與 [ticket 26](../../docs/tickets/26-continual-matrix-imitation-ppo-and-bio-inspired-protocols.md)，不是本輪新發現的演算法錯誤。

實際影響是 PPO 不能直接訓練帶既有神經記憶的片段，也不能和這兩種生物機制組合使用。這不是所有訓練路徑的限制。下一步應先定義從指定狀態開始的一致前向與反向，再決定片段前的歷史是否停止傳遞梯度，分階段補驗證與能力。

## 六步範例與框架缺陷分開判讀

[`Advantages`](../../learning/rl/advantage.go#L33) 已分別支援真正終止 `Done` 與暫時截斷 `Timeout`。上述手算案例確認 Done 的後續價值為零，Timeout 使用記錄的後續價值。六步期限對照改的是範例的任務定義，不能歸因為底層公式不支援終止。

[ticket 40 的分析](horizon-comparison-20261005/analysis.md) 只有一個初始種子通過完整提示判準。[ticket 39 的分析](gradient-diagnostic-20261004/analysis.md) 顯示共用核心的兩類梯度有競爭，尚未確認唯一失敗原因。本輪數值重現也沒有證據能解釋先前的方向學習失敗。

## 工程核對與獨立審查

`go test -count=1 -v ./learning/rl` 退出碼 0，20 個頂層測試通過。包含手算終止、逾時的後續價值、有限差分、非零初始狀態拒絕及未支援機制拒絕。既有測試通過不能取代新增數值案例。上面的公開 API 重現已證明存在漏測。本輪沒有新增或修改儲存庫內的 Go 程式，因此沒有重跑全套建置、race 或 vet，也沒有宣稱新功能完成。

宿主為 Codex。獨立審查是唯讀來源審查，主 agent 自行跑上述重現與既有測試後採用兩個數值發現。審查者未執行 Go。其他未重現的溢位情境保留為修正時的驗證範圍，不當作已觀察到的完整訓練失敗。

| 工具 | 實際模型與旗標 | 結果 |
| --- | --- | --- |
| agy | `claude-opus-4-6-thinking`，`--mode plan --print-timeout 5m --output-format json` | 本輪實際額度耗盡，退出碼 3 |
| Claude CLI | `--model opus --permission-mode plan --allowedTools Read,Grep,Glob --disallowedTools Write,Edit,Bash,Agent,Task --output-format json`。模型回條為 `claude-opus-5-5` | 成功完成唯讀審查，退出碼 0 |

agy 錯誤原文：`Individual quota reached. Please upgrade your subscription to increase your limits. Resets in 96h45m5s.`

首次外部審查遭自動核准審查拒絕，理由是尚未確認原始碼可外傳。透過 GitHub API 確認 `TimLai666/coimnet` 為公開儲存庫，main 與本輪來源提交一致，審查目標逐檔與公開提交相同後，同一審查獲准重試。沒有傳送本機訓練資料或模型。

減法審查：人工方向任務保留作為驗證範例，新的種子穩定性測試排在框架正確性之後。修正共用機率計算可處理根因，不需為單一人工模型增添新的訓練選項。
