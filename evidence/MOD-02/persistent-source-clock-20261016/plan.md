# 持續化學來源的絕對時間驗收

Root 決策｜2026-10-16。前輪為有效進展，ticket125 已交付。本票先補已確認的證據缺口，尚未確認正式運算缺陷，不更改模型規則或保存格式。

## 公開流程與固定判準

NewIndividual → 無化學暖機 0／3 步 → EnableChemistry → OfferFeedback → Snapshot 中宣告起始濃度 4、化學 Steps 0／11 → RestoreIndividual → Advance／零 gate 的 AdvanceGated → Snapshot 或 SaveIndividual → 新程序 LoadIndividual／RestoreIndividual → 接續。

來源依神經模型步數讀取時間。化學 Steps 只計算實際動力學更新。FreezeChemistry 暫停來源及濃度更新，不暫停神經步數。凍結旗標不進快照，恢復後須由呼叫端明示套用相同排程，不能宣稱旗標會自動恢復。回饋抵達後持續可用，不是消耗品。

固定 48 案例：Continuous／LIF／Mixed × timeline／feedback controller × 暖機 0／3 × 化學起始 Steps 0／11 × 不凍結／第 2、3 列凍結。LIF／Mixed 使用已宣告的 Homeostasis，所有案例兩節點、零邊、一區一通道，六列化學輸入。只有診斷程式位於本輪 runs/.tmp，正式 Go 來源保持 840 份。

每案兩個公開入口。不凍結時六列的全部 32 種分段，凍結時須在狀態切換的第 1、3 個邊界分段，共 8 種合法分段，合計 1,920 條不同路徑。每個邊界比對獨立濃度與兩個時鐘，參數／optimizer 不變、回饋佇列保留，並與同排程的逐列基準及 Restore twin 比對快照／輸出。不同分段的 ChemistryReport 只按各次呼叫核對，不比較不同長度的累加報告。

## 獨立時間表

化學 DT=ln2、Tau=1，所以每個未凍結步 `c_next = c/2 + q/2`，凍結步 `c_next=c` 且釋放總量為零。起始濃度為 4。

- timeline 的六個相對來源時點釋放為 `[2,5,0,3,0,7]`，Entries 的絕對 Step 是暖機步數加上列索引。
- controller 只讀 feedback，mean／variance 權重為零。兩筆分數 +2、-1 分別在相對列 1、3 抵達，所以列 0 的平均分數為 0，列 1、2 為 2，列 3、4、5 為 .5。獨立釋放為 `log1p(exp(tanh(score)))`。
- 凍結第 1、2 列索引時，上述來源都不被呼叫；濃度與來源視窗不前進。第 3 列恢復，讀當下神經步數，不補播錯過的 timeline，也不把已抵達回饋丟掉。

逐列及逐段濃度／釋放值使用 `1e-12*(1+abs(want))`，雙側拒絕非有限。同環境的逐列／分段／Restore twin 快照與輸出要求逐位一致。此浮點容差不表示跨 CPU 逐位相同。

選暖機 3、化學起始 Steps 11 的十二組 core/source/freeze，在第 1、2、3、5 列後保存，分別由新程序讀取並接續，共 48 次。第 2 個保存點位於凍結途中，子程序明示重套排程。核對實際 Save／Load、末狀態與輸出，不只檢查程序退出碼。

錯誤原子性選十二組不同時鐘案例：預先取消、NaN、gate 長度錯誤、非零 gate 而未開啟 plasticity。每次必須回錯且 snapshot／上一份報告保持，之後合法接續正確。

## 格子與驗證

Root 擁有時間表、計數、誤差與診斷斷言。唯一實作格子 `newIndividual(caseSpec) (*learning.Individual,error)` 只建構指定合法模型、暖機、化學及回饋起點。先以回傳 unfilled 錯誤的格子證明診斷 RED，實作者不得改斷言或預期值。

隔離兩種錯誤必須被拒絕：將來源時鐘改用 chemical Steps、將可用回饋提早一列交給 controller。第二種使用原來源的可用時間防線亦可直接回錯，不能把編譯失敗當有效拒絕。錯誤來源以 Go overlay 隔離，正式檔案保持。

診斷一般／競態、格式、build、vet、固定 Insyra 與治理均須驗證。正式 Go 路徑與每一位元組都等同已交付 ticket125 時，才沿用其完整一般／競態證據，另執行本輪診斷及治理。若重現正式缺陷，先保存最小失敗，再另鎖修正範圍與完整工程驗證。

最後保存必要診斷來源、輸入及日誌，只清除本輪專用暫存，核對原模型、資料、模組及他人工作，再依既有授權正常提交推送與讀回。

## 減法與限制

沿用現有公開 API 及已交付的數值規則，不增加產品選項或第二套核心。這次不重做 ticket125 的獨立膜電位／Homeostasis 算式，不推論真實任務品質、生物校準或全框架零 bug。回饋分數只是宣告的工程輸入。

Root 首次 GREEN 前的覆核補強：在全部 controller 逐列基準，直接讀公開 neural history，獨立建立兩列 oldest-to-newest 視窗與 Filled。凍結列必須保持 SourceStates。這補足只讀 feedback、mean／variance 權重為零時無法由釋放量觀察視窗錯誤的缺口，不改原 48 案例與 1,920 路徑。
