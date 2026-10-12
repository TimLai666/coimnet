# 真實影音模型保存與新程序推論

原規格 §13.1／15.3 的任務模型保存及独立推論不能只以同程序 NewNetwork 代替。沿用 realmedia 既有 newModel、訓練、predictSamples、scorePredictions 和 PNG/WAV/timeline/optional ffmpeg 預覽，新增示範專用 task bundle config，不變更 tasks/media 或通用 checkpoint，也不要求真實保留集品質勝過基準。

## Root骨架與契約

新 examples/realmedia/model.go：mediaModelConfig 綁定 ManifestSHA256、OutputSpec、encoder/decoder/scale profile、TextFeatures、TimeFeatures、Seed、ImageScale、AudioScale。newMediaModelConfig 產生這個例子唯一現行設定。Config的非空來源SHA須合法64hex，clock必須既有validateExampleOutput相符，所有欄位必須與同helper解析結果相等。

saveMediaModel(ctx,directory,config,snapshot)error：沿用 checkpoint.SaveTaskTrainingBundle，任務schema coimnet-realmedia-model/v1。驗證固定學習Config與Options及完整TrainSnapshot，不允許換拓樸、encoder/readout大小、優化器profile；參數及動量可為合法已訓練值。不可重新初始化missing內容，錯誤不留partial完成模型、不覆寫既有目錄。

loadMediaModel(ctx,directory,expected)(snapshot,verifiedManifestSHA,error)：一次LoadTaskTrainingBundle+DecodeTaskConfig，exact比對來源與現行profile，再返回完整已驗證snapshot及同次Load的完成清單SHA。不重新讀另一份manifest取得來源。nil/cancel、corrupt/unknown/null/missing configs、形狀/optimizer mismatch、錯schema、來源/decoder/clock mismatch都拒絕。

fitFromSnapshot(ctx,samples,snapshot)(fitResult,error)：ValidateSamples與snapshot後依snapshot.Config/Parameters的同predictSamples/scorePredictions計算，Before==Snapshot保持全部optimizer/updates；BeforeScores==AfterScores。獨立推論，不呼叫Step/StepFrom，不重新訓練或生成新模型。

runWithModel(ctx,dataRoot,outDir,modelInput,modelOutput)error：兩model flags互斥；既有run呼叫空flags保持60epochs訓練預覽與trained-model.json產出。新增CLI --model-output NEW_BUNDLE、--model-input BUNDLE與usage/help。loaded模式Epochs=0且完整trained-model.json同原模型，report Model.mode/restored、verifiedSHA/updates明示讀回，新增this_run_updates，累積Updates不要誤標成重訓次數。新程序生成同數值PNG、WAV、timeline及score；mp4 container/time metadata不作逐位保證。

## 容量、操作與失敗

原 bounded manifest、authorized data fingerprints、Import解碼預算及outDir排他建立/失敗刪own成果保持。Model-input/Output不重疊raw data或outDir，不接受既有output model dir。讀回mode的原core_weights必須變動檢查只適用訓練，禁止為了滿足它而更新。完整bundle保存後不能因後續report寫入失敗刪掉已提交模型，錯誤須保留成功model路徑的partial-success說明。模型與資料來源在讀回前核對，錯誤不產出partial report。

## 三條完整流程與檢查歸屬

1. 同licensed小sample訓練→保存完整task bundle→獨立Load與fitFromSnapshot→完整model/optimizer/updates及predict/score相同；CPU下一次StepFrom與不中斷路徑逐位一致。
2. 子程序Load真實bundle而非直接JSONsnapshot→相同predict並writePreview→PNG/WAV/timeline逐位一致；子程序不訓練，report epochs/this_run_updates=0。
3. 真實licensed短片原dataset/seed/epochs同budget訓練→新程序modelinput→同report score/model digest及PNG/WAV/timelineSHA；核心及decoded quality負面結果照實保留，模型與素材不重新下载；相關full native checks及證據保存後清owned暫存。

測試worker僅model_test.go／model_run_test.go，Root審查並實際RED後，另一worker只填model.go／main.go，不改tests。trainer.go先保持，若必要小共用抽取由Root定契約再授權。不得改source manifest、task schema通用loader或多餘抽象。
