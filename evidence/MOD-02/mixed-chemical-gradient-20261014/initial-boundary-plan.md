# 首步來源的初始狀態梯度

契約稽核指出同一路徑的遺漏：`ChemicalEpisode.InitialActivity == nil` 時，首步來源讀核心初始原始輸出；反向的 `addActivity` 與輸出結果卻排除 `Step == -1`。連續與混合核心原本能將 `Step == -1` 傳至 `Core.Initial`。明示初始活動則保持常數邊界。先執行重現，再判定缺陷。

此修正不改來源釋放值、輸出、現有可訓練參數或保存格式。影響是 `Gradient.Core.Initial` 的完整性，不能推論成目前辨識率下降的原因。

## 凍結契約與檔案責任

- 測試作者只新增 `learning/chemical_initial_gradient_reference_test.go`，可使用已凍結參考 helper，不改原 456 份測試或正式程式。
- Root 親自執行重現、核對手算值與停止邊界。
- 重現後才修正 `learning/chemical_sequence.go`：保留初始活動是否為常數的私有旗標，只在未明示且當前反向區塊從首步開始時接收及回傳 `Step == -1`。其他負索引、視窗外活動與明示常數仍排除。
- 反向結果順序依步號遞增，保留既有拷貝、有限值與錯誤原子性。連續／混合的既有核心負一步橋接足夠，LIF 初始突觸輸出為常數零，沿用核心排除規則。

## 重現與停止條件

1. 混合 controller、`InitialActivity=nil` 的全路徑梯度符合前向微分，明示初始活動控制組符合舊值。
2. 單節點 softplus 連續核心，以 controller 與 neural source 分別手算初始梯度。化學為一區、一訊號、零初始濃度，`DT=Tau=1`、`Kd=N=1`、增益 `1+occupancy`、輸入與映射及讀出係數 1、bias=0、log_tau=0。
3. `a0=softplus(0)`，`q=q(a0)`，`c=(1-exp(-1))*q`，`o=c/(1+c)`，`v1=(1-exp(-1))*(1+o)`。未明示活動時 `dL/dv0=sigmoid(v1)*(exp(-1)+(1-exp(-1))^2/(1+c)^2*dq/da0*0.5)`；明示活動只保留 `sigmoid(v1)*exp(-1)`。
4. controller 的 mean/variance 以一列一節點計算，variance=0；參數 `[1,0,0,1,0]` 對應 `q=softplus(tanh(a0))`。neural source 為 `q=a0`。完整歷史與一列視窗均須成立，化學關閉／明示邊界的其他輸出與梯度保持。

新的回歸必須在修正前實際失敗，在修正後通過。前一輪 race 因需先重現而主動中止，其紀錄保留，不當作最終驗收；以最終指紋重跑完整工程驗證。
