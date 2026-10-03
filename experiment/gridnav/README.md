# 走廊環境學習範例

這個人工範例示範如何把循環神經網路接上專家模仿或行動回饋學習。它沒有載入果蠅接線，也不保存可供使用的預訓練模型。

```sh
go build -o bin/coimnet ./cmd/coimnet
./bin/coimnet examples run gridnav --method ppo > ppo.json
./bin/coimnet examples run gridnav --method imitation --updates 500 > imitation.json
```

`--method` 預設 `ppo`，`--updates` 預設 200，接受 1 到 100000。兩種方法都跑種子 1、2、3。PPO 的預設是一個完整 episode 接一次最佳化器更新；模仿模式的預算則是專家示範的 episode 數。

## 環境與模型

走廊長 7 格，起點在中央。第一次觀察提示目標在左或右，之後提示消失。觀察只有提示、是否在左右邊界與經過時間，行動是左、右或停留。每步扣 0.01，到達目標獲得 1，最多 20 步。策略不取得目標座標或專家行動。

PPO 範例使用 12 個連續動態神經元、96 條連線，4 個觀察節點連到 8 個循環節點。三個行動分數與一個價值估計可讀取全部 12 個節點。固定輸入映射與時間常數，只更新連線權重、偏差與讀出，共 156 個參數。模仿範例沿用既有的 16 個循環節點與兩跳讀出結構。兩者結構不同，成績不能用來宣稱演算法優劣。

## PPO 的收集與更新

每個 episode 從副本的零神經狀態開始，依 softmax 機率取樣，保存實際行動機率、價值與策略版本。到達目標時後續價值為零；時間到期時，讓同一副本處理最後的下一筆觀察，取得後續價值。原個體不因收集或評估改變。信用分配使用 GAE 優勢估計與 PPO 損失，經既有完整梯度路徑更新核心與讀出。

初始化使用 `PCG(seed, 0)` 與 `PCG(seed, 2)`。隨機基線、訓練取樣、評估取樣分別使用 `0x1001`、`0x1002`、`0x1003` 作為第二個 PCG 種子。評估前後重設相同的評估亂數來源，每個 seed 評估 40 個 episode。評估環境種子為 `1000 + episode`；訓練使用 `imitationTrainSeed` 的混合規則。環境只有左右兩種目標，分開的種子不代表新的任務種類或複雜導航泛化。

SDK 入口為 `experiment.DefaultPPOExperimentConfig()` 與 `experiment.RunPPO(ctx, config)`。較低層的 rollout 與更新契約見 [PPO API](../../learning/rl/README.md)。目前限完整、零初始狀態的 episode，`MiniBatch=1`，不支援可塑性或化學狀態；`BurnIn` 預設 0，非零時只遮掉前綴直接損失，不切斷後續梯度。

## 報告與退出碼

PPO JSON 保留每個 seed 的訓練曲線、更新前後回報、隨機基線、終止／到期次數、失敗原因與最終完整個體快照摘要。摘要包含最佳化器狀態，但報告本身不能恢復訓練。相同平台與組態重跑須得到相同報告；跨 CPU 的浮點結果不保證逐位相同。

`passed` 需要所有 seed 執行成功，且訓練後平均回報同時高於訓練前與隨機基線。標準差是三個 seed 平均回報的母體標準差。失敗 seed 保留在結果中，不參與平均，並使整體不通過。CLI 在門檻未過時仍輸出完整 JSON，再傳回非零退出碼。

模仿模式輸出專家一致率與環境回報，沒有 `passed` 學習門檻；退出成功只代表所有 seed 執行成功。既有曲線使用 50、200、500 個 episode，200 個 episode 時有一個 seed 的一致率下降，500 個 episode 才是既有一致率改善測試的預算。完整量測與命令見 [LRN-09 證據](../../evidence/LRN-09/verification.json)，資源數字見 [記憶體與時間](../../docs/resources.md)。

## 起始提示與抵達檢查

此測試範例保持既有模型與三組 200 次 PPO 更新，補訓練前後 sampled／greedy 的原提示、清除提示、反轉提示，以及不讀模型的 random。每組使用環境種子 1000～1039，左右各 20 回合；共同 PCG 串流為 0x1004，每回合重新建立，與舊 CLI 連續評估串流分開。目標只供計分，參數、神經狀態及最佳化器不因評估改變。

```sh
audit_dir=$(mktemp -d)
COIMNET_GOAL_CUE_EVIDENCE="$audit_dir" go test -count=1 -v -run '^TestPPOGoalCueEvidence$' ./experiment
```

輸出 report.json，不覆寫既有檔案。39 組條件、1,560 回合的完整行走紀錄保存在 [report.json.gz](../../evidence/LRN-09/goal-cue-audit-20261003/report.json.gz)，較小的[摘要](../../evidence/LRN-09/goal-cue-audit-20261003/summary.json)可直接閱讀。用 `python3 evidence/LRN-09/goal-cue-audit-20261003/verify_report.py` 獨立重算位置、獎勵、終止旗標、摘要與指紋。

| seed | sampled 訓練前抵達 | 訓練後抵達 | 清除提示 | 反轉提示 | greedy 訓練後原提示／清除提示 |
| --- | --- | --- | --- | --- | --- |
| 1 | 20／40 | 20／40 | 20／40 | 20／40 | 20／40、20／40 |
| 2 | 23／40 | 39／40 | 32／40 | 24／40 | 40／40、40／40 |
| 3 | 16／40 | 27／40 | 21／40 | 16／40 | 40／40、40／40 |

事前的描述性工程判準只有 seed 2 通過，整體 goal_cue_gate=false。seed 1 的左目標是 0／20，seed 3 為 8／20，未達每側 0.8。seed 2 未訓練時 greedy 已是 40／40，訓練後反轉提示也可 40／40。seed 3 的 greedy 則從 0／40 變成 40／40，但清除提示仍是 40／40。

20 步足以先走到錯端再折返，走遍兩端最少只需 9 步。因此本輪支持有限的策略與抵達改善，沒有三組一致依提示導航的證據。環境只有兩種目標情境，不能把 40 回合當作 40 種獨立導航問題，這也不是果蠅接線或生物記憶的驗收。固定判準及限制見 [ticket 36](../../docs/tickets/36-synthetic-goal-cue-audit.md)。

## 六步左右配對提示檢查

[ticket 37](../../docs/tickets/37-short-goal-cue-audit.md) 同時把環境與 PPO 的時限設為 6 步，其餘預設值、模型與 200 更新不變。從中央搜尋兩端至少要 9 步，六步必須選對方向。環境種子 1000～1039 依左、右目標各自排序，再逐一配成 20 對。每對的左右回合使用相同 PCG(1000 + pair_index, 0x1005)，訓練前後與各種提示干預也重設相同來源。

```sh
short_audit_dir=$(mktemp -d)
COIMNET_SHORT_GOAL_CUE_EVIDENCE="$short_audit_dir" go test -count=1 -v -run '^TestPPOShortGoalCueEvidence$' ./experiment
python3 evidence/LRN-09/short-goal-cue-audit-20261003/verify_report.py
```

第一個命令在新資料夾建立完整 report.json，既有檔案會在訓練前拒絕。第二個命令依[兩個程序的紀錄](../../evidence/LRN-09/short-goal-cue-audit-20261003/reproduction.json)，核對兩份完整報告、實際測試日誌與[摘要](../../evidence/LRN-09/short-goal-cue-audit-20261003/summary.json)。兩個新程序的 39 組／1,560 回合報告完全相同。清除提示的左右觀察、輸出及動作在任一側終止前逐步相同，每對最多成功一側，抵達率上限 50%。全部 729 種六步動作序列也沒有能搜尋兩端的序列。

| seed | sampled 訓練前抵達 | 訓練後抵達 | 清除提示 | 反轉提示 | greedy 訓練後原提示／清除／反轉 |
| --- | --- | --- | --- | --- | --- |
| 1 | 7／40 | 1／40 | 1／40 | 0／40 | 0／40、0／40、0／40 |
| 2 | 9／40 | 14／40 | 14／40 | 13／40 | 20／40、20／40、20／40 |
| 3 | 7／40 | 13／40 | 17／40 | 18／40 | 20／40、20／40、20／40 |

random 各為 7／40。三個 seed 的事前判準都未通過，short_goal_cue_gate=false。seed 2 的 greedy 只到左目標，seed 3 只到右目標，清除或反轉提示仍是同樣成績。這次排除了搜尋兩端的捷徑，但沒有學會依提示導航的證據。沒有依結果調整參數，舊二十步報告保留。這仍只有兩種人工目標情境，不能宣稱一般導航、真實接線或生物記憶。
