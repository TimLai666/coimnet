# LIF 放電閾值訓練範例

這個範例用三顆 leaky integrate-and-fire 神經元跑既有的五步人工延遲脈衝資料，只比較「基礎閾值可不可訓練」造成的差別。資料產生器沿用 `experiment.DelayedEpisode`，沒有改動。

程式示範 `dynamics.NewLIF`、`learning.Config.LIF` 與 `learning.Trainable.Theta` 的串接。這是數值可學習性檢查，不是果蠅行為成績，也不宣稱這組設定是生物上的放電區間。

## 執行

從 repo 根目錄執行：

```sh
go run ./examples/lifthreshold > report.json
go run ./examples/lifthreshold --updates 300   # 預設值，可調 1..100000
go run ./examples/lifthreshold --help
```

程式只把一份 JSON 報告寫到標準輸出，不寫檔案、不保存模型參數。門檻沒過時仍輸出完整報告，並以非零狀態結束。

## 模型與初始化

固定拓撲為神經元 0 →1、1 →1（延遲 1 步的自連線）、1 →2，讀出只看神經元 2 的突觸跡。固定設定：`dt=1`、`tau=1`、`tau_syn=1`、`theta` 範圍 `[0.05, 1]`、`v_reset=-0.5`、不應期 1 步、替代梯度 `fast_sigmoid` scale 2、適應機制關閉。完整理由寫在 `experiment/lif_delayed.go` 的註解。

初始化沿用 `NewDelayedTrainer` 的 SplitMix64 混合器，只有第一條邊的權重依 seed 變動（`1.2 + mix(seed)%100/1000`），其餘為固定值。神經元 2 帶 0.3 的固定偏置，初始 `theta_raw = -2`（`theta_base = 0.163`）低於這個偏置驅動的膜電位，所以未訓練的讀出神經元在每個樣本都會自己放電，包含答案為負、突觸跡本來就無法表示的樣本。閾值要抬到常態驅動（約 0.3）之上、脈衝驅動（約 0.73）之下，神經元 2 才會只在延遲脈衝抵達時放電。

這組初始值是刻意往閾值方向調偏的，目的是讓閾值成為限制因素。編碼器固定注入 `2a` 到神經元 0，任何可達的閾值都擋不住正脈衝，所以放電率不會掉到零。

## 對照與門檻

三組 seed（7、42、123，與延遲關聯基準相同）各跑三個對照，更新預算相同，只有可訓練的參數群組不同：

| 名稱 | 可訓練群組 |
| --- | --- |
| `all` | 全部 |
| `theta_only` | 只有 `theta` |
| `frozen` | 全部凍結 |

訓練樣本取自 seed 1001 的計數器串流，保留樣本取自 seed 1003 共 128 筆。程式會逐一比對兩邊實際產生的計數器，重疊就直接報錯，不只比較 seed。

事前寫死的門檻（`main.go` 常數）：

1. 每組 seed 的 `theta_only` 保留 MSE 必須低於 `frozen`。
2. `theta_only` 至少一顆神經元的 `theta_base` 變化超過 `1e-3`。
3. `theta_only` 訓練後的保留放電率必須嚴格落在 `(0, 1)`。

放電率是保留資料上「有放電的神經元×時間步」佔全部神經元×時間步的比例，用 `learning.Trainer.Spikes` 在凍結的獨立 episode 上重跑取得。`theta_base` 由 `theta_raw` 經宣告的有界轉換 `theta_min + (theta_max-theta_min)*sigmoid(theta_raw)` 算出。

## 結果怎麼讀

報告保存每組 seed、每個對照的訓練前後保留 MSE、逐顆 `theta_base`、最大變化量與訓練前後放電率。未訓練模型的保留 MSE 約 0.232，只訓練閾值後約 0.052，凍結組維持 0.232。

三組 seed 的 `theta_only` 數值幾乎相同：seed 只改變第一條邊的權重，而事件鏈在這個權重範圍內都是飽和的（神經元 0 放電後，神經元 1 必定跟著放電），所以閾值條件的結果對 seed 不敏感。這三組 seed 證明的是協定可重現，不是不同初始化的變異量。

損失下降只說明這個固定人工任務上閾值確實可學，不能推廣成泛化能力、真實接線或生物放電行為的結論。

## 錯誤

未知旗標、多餘參數、更新預算超出 1..100000、計數器重疊、取消、數值失敗、門檻未通過與輸出失敗都會回非零狀態。
