# Root 下一階段骨架與驗收：線上回饋及共享狀態

目前公開來源正在凍結驗收，測試草稿只能放在本暫存目錄，以 .go.txt 保存。不得更動正式 .go、docs 或既有 evidence。

## 81：化學線上分數使用實際訓練 episode

根因：online.episodePredict 呼叫未帶 Individual 化學／expression 邊界的 trainer.Predict，TrainEpisode 卻使用私有 chemical episode。預測必須使用相同的零神經初始狀態、目前化學／來源／feedback／資源、expression、source clock、initial activity 與 frozen 設定，且不能推進持續狀態。沿用 trainChemicalEpisode 建立私有 Network 的宣告邏輯，避免兩份邊界建構。

格子：episodePredict(ctx,input) -> ([]float64,error) 使用同一化學 boundary，普通個體保持原樣。來源 stateful controller 與 timeline 不得偷前進。錯誤／取消不提交。不可修改公開格式或生理公式。

驗收：公開 Act -> SetTargets -> Receive -> Update；score 0、.5、1、-1、2 對照手算 effective = actual_episode_prediction + clamp(score)*(target-actual_episode_prediction) 的 twin TrainEpisode；fresh optimizer、WeightDecay=0 時 score 0 參數逐位不變。無化學對照、非零化學 boundary 與 expression 必須真的改變 prediction；回饋後持續神經／化學／source／expression 不變。優先 CPU continuous 與 LIF。

## 82：共享 optimizer 與 accumulator restore

根因：RestoreTrainer 只驗 shape/finite，不檢查共享群組內 First、Second、Steps 與 partial Sum 一致。共享參數既有 invariant 要在 restore 時成立，不自動重寫使用者 snapshot。

格子：既有 RestoreTrainer 在完成一般 state 驗證後，按 Config.Sharing 建立的既有 flat indices 拒絕同群組不一致的 First／Second／Steps／Accumulator.Sum；沒有共享與正常共享快照原行為保持。錯誤回傳 nil trainer，不改呼叫端輸入。公開 RestoreIndividual 繼承拒絕。

驗收：共享兩條邊、bias/log_tau 群組與單一／未共享 entry 控制；先真的訓練產生合法 snapshot，再逐項只改一個群組成員 First、Second、Steps 或 partial Sum，應拒絕且輸入不變。合法 partial window restore 下一步與原 twin 逐位相同，之後多步仍共享；個體存檔也覆蓋。

## 83：Online GradientSteps 真正套用計數

根因：丟棄 StepResult 後每次 TrainEpisode 成功都計為更新，忽略 AccumulateSteps partial boundary。

格子：Online apply 只在 StepResult.Applied 為 true 時增 GradientSteps；Consumed 與 accumulator 依原規則。無新報告欄位。

驗收：AccumulateSteps 1、2、3；每一筆 Act -> feedback -> Update，前 k-1 次 report.GradientSteps=0、optimizer.Updates 不變、accumulator.Count 正確，第 k 次為1、Updates 增加、accumulator 清空。測試普通及化學個體；partial 後 restore 繼續，report 不虛報。

所有草稿由 Root 審查後才納入，先保存舊版 RED 再實作，測試與實作人分離。完整驗證依 AGENTS，合法相容性與新程序模型接續均必跑。疑點不自動擴張成已確認 bug。

## 84：失敗後重試不得遞進更新間隔

Root 核對 online.apply 發現 eligible 在預測及訓練成功前遞增。UpdateEvery=2 時，第2筆訓練失敗後重試變成第3筆並跳過訓練，卻把失敗 feedback 消耗。格子：使用私有候選 eligible，成功或合法間隔跳過後才提交；回錯不能遞進 interval。若 gradient 已成功而後續 plastic 失敗，既有部分成功語意需另記，不以本修正偷偷改成全部回滾。

驗收：公開 snapshot.Optimizer.Episodes=MaxUint64 形成合法可載入但不能再TrainEpisode的個體；UpdateEvery=2，第一筆合法間隔消耗，第二筆在完整公開流程回錯，兩次重試都必須持續回錯、Consumed=0、GradientSteps=0且完整snapshot不變。UpdateEvery=1正常成功及2下正常skip/套用控制。保存所有原始錯誤，不以panic/錯誤 fixture當成功。
