# Root 範圍與格子

前一輪 ticket129 已完成4／4，最終提交41ea240與遠端一致。LRN-06的線上閘門缺陷已用一般／競態各32個正常數值公開控制重現，本票沿用該根因，不重開猜測。工程追蹤90／91，LRN-06保持specified。

## 私有實作骨架

只准修改 learning/individual.go、learning/individual_online.go、learning/online.go。測試先由另一個worker寫入 learning/online_receptor_policy_test.go，Root審查並執行有效RED後凍結。實作者不能修改測試。

新增 `advanceRowsWithGateSource(ctx context.Context, input [][]float64, gate []float64, useReceptorGate bool) ([][]float64, PlasticReport, error)` 與同簽名的 `advanceRowsWithGateSourceLocked`。原 `advanceRows` 與 `advanceRowsLocked` 各自成為傳入true的薄包裝，原初始化／鎖／完整驗證與候選提交的函式內容搬到對應新helper，不能複製兩套流程。`advanceStepwise` 加入 `useReceptorGate bool`，只在原GateReceptor分支加這個條件。DecayEReceptor、化學來源、effective fast/slow、凍結及錯誤順序都保持。

線上 `advanceVersioned` 在原同一把個體鎖中先計算版本，再呼叫新locked helper，gate=nil且useReceptorGate=false。線上 `apply` 的局部回饋路徑使用新unlocked helper，傳入原mapper建立的完整gate切片且useReceptorGate=false。該切片由Act保存的非空input建立，毋須新增一套nil／尺寸驗證。其餘原生Advance、公開AdvanceGated、Intervene對照與subject仍走true。只有這兩條線上路徑改變閘門來源，不加公開設定、格式、依賴或持久旗標。

所有新helper沿用既有error回傳，不吞錯、不提早提交。線上混合回饋的gradientDone與部分成功契約保持。模型宣告不被改寫，下一次原生推論仍能使用受體；不以暫時改Rule指標或FreezePlasticity的方式繞過問題。

## 測試格子與預期

測試只用公開SDK與獨立控制個體。三種核心都用2節點、單邊0→1、DT=1、輸入與讀出寬度2／1，讀出node1，encoder identity，weight=.9、bias/logtau=0。continuous為tanh；LIF用既有chemLIF範圍及theta_raw0，加入該單邊；mixed用NodeRule{1,0}、相同LIF範圍與tanh，theta_raw只含1項。輸入三列{2,2}，必要的原生正學習控制可再推進三列，不能接受沒有活動的fixture。

化學使用既有兩區、一通道、tau2、DT1與node0假說受體Kd=.5、n1，timeline可選step1脈衝1。塑性用HebbianRate、DecayE/P=.5、PlasticMax8、WMin=.0625，單邊0；可選GateReceptor0／GateScale1。另用DecayEReceptor0、base .5/span -.4/min .1/max .9，確認線上關閉新寫入時化學時間窗仍生效。允許以合法Snapshot/Restore設定初始fast=0或.25，作為closed gate應衰退而非整體凍結的控制。

- Act：三核心×gradient/plastic/mixed/evaluate×有無受體閘門×有無脈衝×初始fast0/.25。與同一份起點、只拿掉GateReceptor與GateScale的closed-gate控制逐位比較輸出、完整Snapshot及ChemistryReport，完整Snapshot僅正規化這兩個宣告欄位。Act不得改Parameters、不得新增fast寫入，eligibility、神經、化學與時間窗正常推進。使用帶active receptor的原生正控制驗證fixture確實會學。
- Update：三核心×有無受體閘門×有無脈衝×score0/1×固定／受體時間窗。先Act，再Receive及Update。plastic-only政策的結果需與去掉GateReceptor的獨立控制先Advance再以mapper的明確gate做AdvanceGated逐位相同，包括完整Snapshot與ChemistryReport，只有宣告欄位正規化。報告Consumed1、PlasticSteps1、GradientSteps0；score1的活動fixture要實際改fast，score0只沿原公式衰退。四種政策其餘行為由Act矩陣與既有線上混合測試一併驗證。
- Native：Act或Update之後，宣告逐位保持，接著一般Advance仍由受體開閘；公開AdvanceGated的非零明確gate仍拒絕且完整狀態不變。用對照證明沒有來源或freeze旗標洩漏。
- Frozen／errors：三核心的FreezePlasticity(true)在Act與plastic Update保持整個fast state，神經／化學仍推進；Evaluate Update仍回ErrEvaluateMode。nil/canceled context的Act、非有限reward gate的Update保持完整Snapshot、ChemistryReport、Log與未消費回饋，改回合法gate後可重試。沿用原錯誤順序，不改正常來源快取語意。

Root單獨執行所有Go：有效RED、一般／競態GREEN、既有online/chemical/receiver/Intervene回歸、原公開控制、至少兩種能編譯但行為錯的隔離對照，再凍結正式來源完成全部格式、build、unit、race、vet、固定依賴與治理。正常無GateReceptor的公開模型結果與舊保存接續另外比較。所有原有8256個終端事件與58通過／4略過套件必須保留。完成才恢復LRN-06，保存證據並清理唯一自有暫存，最後提交推送與遠端讀回。

減法審查：共用原有advance驗證與提交，不另建一套核心、不新增模型設定，不拒絕原本應能組合的生理機制。無關的TrainEpisode獨立episode及pre/post時點定義不更動。暫存只在runs/.tmp/online-receptor-policy-20261016，既有32份其他工作、18模型、六資料、原始依賴與兩個舊暫存根保護。
