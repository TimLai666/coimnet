# 化學回授與慢速穩定的獨立訓練驗收

## Root 骨架與判準｜2026-10-16

本票延續 ticket123，核對既定數學規則，不改生物假設、合法設定或模型格式。目前確認的是私有 reference 拒絕 enabled Homeostasis，尚無新正式運算缺陷。

資料流：固定輸入 → float32 encoder → float64 神經核心、活動來源與化學回授 → float32 readout → 公開 LossGradientFrom／Trainer.StepFrom。沿用既有雙數前向參考，不呼叫正式 dynamics/modulation 的 forward/backward；r/h 只保留 nominal float64，h 在化學偏移與 floor 之前加入閾值，停止 dr/dh，adaptation 與活動化學的導數保留。

唯一實作格子：`referenceForward` 維持簽章，移除不支援 homeostasis 的 panic，初始化零值 rate/homeostasis 的 T+1 行，兩條 LIF 事件分支均在事件後更新狀態。refractory 分支以 spike=0 更新後才 continue。continuous 分支保持，停用時兩個新回傳欄位為 nil。Root 先建立這兩個私有欄位與全部驗收，worker 只能填函式體，不能修改檢查。

## 固定驗收

1. 兩節點四步手算，node0 continuous 中性、node1 LIF。DT=ln2，lambda=alpha=kappa=1/2，base=1，零權重／bias／log_tau，identity encoder/readout，無 chemistry/adaptation，homeostasis q=1/2、eta*dt=1、target=1/4、hmax=.4。node1 輸入 4、15/4、0、0，只有第二步 upstream=1。refractory 0／1 兩組均對首步輸入得到 1/16；錯把 dr/dh 傳回在 refractory0 會得到 17/1296。逐行 r/h、輸出與公開完整梯度都核對。
2. 固定 80 組：mixed／LIF × controller／neural × MixSum／MixMax × open-positive／open-negative／gain-upper／threshold-upper／floor × 明示／省略 InitialActivity。homeostasis TauRate=2*DT、TargetRate=.3、Eta=1/DT、HMax=.25。四步、兩節點、四邊、單一化學 source/channel。比較完整輸出與所有 core、theta、encoder、readout、initial/input 梯度。保留原 1e-5+1e-4*abs(reference) 的 float32 邊界容差，雙側拒絕非有限值。
3. 每組核對累積第一步的 17（mixed）／18（LIF）個可訓練座標，再第二次相同 StepFrom 後核對真實參數、Adam 一二階矩與每個步數。全歷史每組一條，LIF 另做 segment 1、3，共 160 條訓練路徑。固定 learning rate=.005、epsilon=.01、無 clip／weight decay／sharing／mask／projection，accumulate=2。首次 Adam 的獨立結果為 p-lr*g/(abs(g)+epsilon)、m=(1-beta1)*g、v=(1-beta2)*g²；由既有梯度誤差傳播推導容差，不從結果挑門檻。
4. 16 組 open-positive 配對核對 h 開關與 chemistry 開關的作用。h 開關只要求完整梯度差異可見，不強迫硬事件輸出必然不同。混合不支援 recompute，不宣稱該入口。停用宣告與原本未宣告的參考／公開結果逐位保持，既有七個 legacy record 在一般／race 各自建置內保持。隔離錯誤 h 導數與漏更新 refractory 的對照必須被新檢查拒絕。
5. 完整格式、build、一般／race、vet、固定依賴與治理通過。保存來源與原模型／資料／模組指紋、清除本輪工作目錄、審查並正常提交推送及讀回，才完成四項里程碑。

## 失敗、範圍與減法

| 情況 | 本票行為 |
| --- | --- |
| 建構、梯度或訓練回錯 | 保留錯誤，驗收失敗 |
| 形狀、非有限值、任一座標或更新狀態不符 | 驗收失敗，不放寬原判準 |
| 空值、取消、非法設定 | 沿用既有公開邊界驗收，本票不改合法域 |
| 真實任務品質、生物校準、持續狀態、正截斷視窗、多 source/channel | 沒有本票新增結論 |
| MCP 重新索引被自動審查拒絕 | 只讀本機來源，不改用其他方式外傳 |

【減法審查】沿用一個既有 dual reference 與公開訓練入口，不建第二套核心、不加產品設定、不改訓練目標。
