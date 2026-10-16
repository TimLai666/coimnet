# 化學與活動穩定並用的持續狀態獨立驗收

## Root 骨架與判準｜2026-10-16

前輪為已交付的 progress。本票核對既定公開契約，尚無已確認的正式缺陷。保留目前生物假設、合法設定、模型格式與學習目標。

入口：NewIndividual（Config.Chemistry=nil）→ EnableChemistry／SetExpressionGain → Snapshot 中放入已宣告的非零初始 concentration → RestoreIndividual → Advance 或全零 gate 的 AdvanceGated → Snapshot。持續個體沒有 episode 的顯式 InitialActivity／SourceStep，本票從零神經狀態與新鮮 controller window 開始，化學 clock 與 source clock 都從 0 起算。controller 的 nil 初始 activity 依 observe 契約當作零列，filled 仍前進。

唯一實作格子：既有 referenceForward 簽章、算式與回傳語義保持，只補四個 Root 已宣告的私有欄位：voltage/adaptation 為 T+1×nodes 的 dual 既有局部陣列，refractory 為同形 int；concentration 為 T+1×regions×channels，保存起始 state 與每次 chemistry step 後的既有 dual state。無 chemistry 時 concentration=nil。不得新增 production helper 呼叫、改舊運算或更動 Root 檢查。

## 固定驗收

1. 手算純 LIF 兩節點四步，DT=ln2、TauSyn=tau=1、base=1、reset=-1、無 adaptation/refractory、q=.5、target=.25、eta*dt=1、hmax=.4；node1 輸入 4、13/4、0、0。一區一通道，初始 concentration=1、DT=ln2、tau=1，外部 timeline 沒有釋放。受體 Kd=.5、N=1，BetaScale=.5、expression Scale=1。濃度為 .5、.25、.125、.0625，offset 為 1/4、1/6、1/10、1/18。開啟時 raw trace 為 1、.5、.25、.125，voltage 為 -1、29/24、157/240、511/1440，r 為 .5、.25、.125、.0625，h 為 .25、.25、.125、0。關閉時 raw trace 為 1、1.5、.75、.375，voltage 為 -1、-1、-9/20、-71/360，r/h 缺省。逐步核對公開 Snapshot 與 float32 expression/readout，確保 raw history 不受 expression 影響。
2. 固定 40 案例：mixed/LIF × controller/neural × Sum/Max × open-positive/open-negative/gain-upper/threshold-upper/floor。沿用前輪雙數參考與 Homeostasis 參數，InitialActivity=nil、SourceStep=0。核對每個分段邊界的 voltage、adaptation、refractory、r/h、必要 delayed history、concentration、chemical steps、controller window/filled 及讀出。獨立 float64 狀態容差 1e-12*(1+abs(want))，雙側拒絕非有限值；float32 readout 使用既有 1e-5+1e-4*abs(want)。沒有要求不同 CPU 算式逐位相同。
3. 每案兩個公開入口，各跑四步的全部 8 種分段，共 640 條路徑。每條都有 live 與 restore twin，每段後核對獨立值、Snapshot/輸出相同，再重新 Restore twin。每次都檢查參數與 optimizer 不變。八組 open-positive 另關閉 expression，神經、化學與 controller state 必須不變、讀出差異可觀察。
4. 八組 core/source/mix 的 open-positive，各在 split=1/3 存檔並由新程序 Load/Restore/AdvanceGated 接續，共 16 次新程序，核對獨立末狀態與輸出。worker helper 在父程序明示 skip，子程序有真實斷言與退出碼，不把 entrypoint skip 算成驗收路徑。八組錯誤/取消案例檢查空輸入、錯誤 gate 長度、非零 gate、NaN 與預先取消均不提交 Snapshot/Report，之後仍可正確接續。不聲稱本票驗證了 call 內部任意時點取消。
5. 新 shape 檢查先因缺少私有 state 回傳而 RED。舊七個 legacy record 在一般/race 各自建置前後保持。隔離兩種正式錯誤：第四步 Snapshot 的 h 寫錯、同一 call 內誤把 expression 後活動餵給 source，必須被新驗收拒絕。
6. 格式、build、完整一般/race、vet、固定 Insyra v0.3.4、模組與 31 項治理均通過。保存來源與原模型/資料/模組/他人工作指紋，必要證據保存後只清除本輪 runs/.tmp，再正常提交推送與遠端讀回。

## 失敗與範圍

| 情況 | 本票行為 |
| --- | --- |
| 建立、推進、Restore、Save/Load 或子程序出錯 | 保留原錯誤，驗收失敗 |
| 狀態形狀、任一座標、clock、輸出或參數/optimizer 不符 | 驗收失敗，不放寬判準 |
| 非有限或非法輸入/取消 | 回錯，Snapshot/Report 保持，後續可接續 |
| 多 source/channel、非零神經初值、任意外部 controller 初史、可塑性學習 | 本票沒有新增結論 |
| 極端浮點域、真實任務品質或生物校準 | 本票沒有新增結論 |

【減法審查】沿用同一個獨立前向參考，只回傳已計算的狀態；不再造一套核心，不增加產品 API 或設定。多程序只選八組有代表性的機制配對與兩個保存點。
