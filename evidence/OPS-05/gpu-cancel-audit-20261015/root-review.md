# Root GPU 取消與浮點診斷審查

GPU 取消驗收已有實際硬體證據，沒有新確認的取消缺陷。正式程式、既有測試與模型格式保持。整體零 bug 目標及真實任務品質提升尚未達成。

## 採用的證據

- 公開探針一般／race 各 22 組、1,754 個取消點，包含 Step／StepFrom、滿窗、部分窗、凍結與零排程率。每個錯誤回傳保留完整快照，原物件重試等於不中斷對照。原始 runtime frame 與 occurrence 在兩份 gpu-probe-v2 硬體 log。
- 既有 reduced-limit fixture 在真實 Apple M3 Metal 跑 130 個值、三個區塊，一般／race 各 162 點，涵蓋 Legacy Step、正率及零率 StepChecked、唯讀 Snapshot。這是硬體多區塊交易測試，不是原生裝置容量量測。
- 合計一般／race 各 1,916 個不同案例中的可達取消點。沒有檢查的同步區段、MapAsync 錯誤分支與不固定的 poll 重複不冒充逐點通過。取消使用標準 context.WithCancel，Done／Err 一致。
- Trainer.Snapshot 只複製 host，見 learning/trainer.go:227。Root 的新增測試檔 overlay 唯讀存取 deviceAdam.Snapshot，DeviceAdam 真正讀 currentBank，見 backend/webgpu/device_adam_execute.go:346。一般／race 各 16 組、1,624 個重複點比較完整 Config、Parameters、First、Second、Steps，取消前後及原物件重試均相同。這些點不重複加入 1,916。
- 22 份 before fixture 都有非零 First／Second 與更新歷史，見 nonzero-prefix-proof.json。resident observer 使用同一 fixture 與 prefix，但未保存物理 bank 編號。支援契約要求邏輯狀態與可重試結果保持，不要求 shadow bank 或診斷計量不變。
- 50 次獨立子程序在各自相同建置下，完整 Snapshot／StepResult／Prediction JSON 位元組與對照一致。100 份人工 before／expected 已打包並逐份 roundtrip，見 new-process-resume.json。三區塊 fixture 的降低容量是測試物件設定，恢復可使用原生容量，不是保存的模型設定。

## 分工結果與 Root 裁決

Root 已審查 gpu-cancel-review.md、resident-review.md、float-first-divergence-review.md，核對凍結探針、實際 receipt、完整比較及正式來源。來源審查中的候選、scratch、map/drain 與 host 提交區別符合固定契約，沒有以 worker 摘要替代硬體執行。

resident-review.md 是收件當時的報告，提到 race 執行 receipt 尚未出現。現在 resident-race-hardware.json 已由原執行 wrapper 保存，exit code 0、68.251191 秒、完整 log SHA。這個 metadata 缺口已補，沒有重跑冒充原執行。Snapshot 會改變讀回計量與排程，本票用未增加 resident 讀回的公開 v2 探針作獨立控制，兩者均通過。物理 bank 編號、shadow、錯誤清理分支及所有排程交錯沒有本票完整觀察，不能稱為已確認缺陷或完全驗證。

## 浮點差異

從完全相同的初始快照取得公開 Network.LossGradient，第一次更新 raw Core.Bias[2] 已相差一個 float64 間距。新增公開 Core.Inputs 觀察後，v1 每個既有欄位與 v2 仍一致，未改正式方法。第一次更新只有 row 4、node 2 的 input gradient 不同：普通 3fd3de0b7fb2bee0，race 3fd3de0b7fb2bee1。兩者各自按反向順序取整加總，得到自己的 Bias[2]，見 core-input-gradient-comparison.json。

這把差異推到 Adam 之前，未定位第一個機器運算。編碼器／讀出 float32 邊界、LIF Forward trace 與每個 reverse operand 還未逐一捕捉。實測兩邊的 Adam 值各自符合獨立 FMA 參考，不能據此說 FMA 是根因。跨建置逐位支援契約與辨識品質影響仍未知，保留 AGENTS.md follow-up，不改正常公式。

## 驗收限制

本票是人工圖的交易／恢復驗證，不是完整圖效能、生物定量機制或新一輪 OCR 品質實驗。既有 18 個模型原檔 SHA 相同。全套一般測試有 22 個明示略過的 helper／需另設資料來源或證據路徑案例，見 full-unit-summary.json，沒有把略過當作通過。完整一般／race 均新跑 58 套件，5,778／5,777 pass、22／23 skip、0 fail，build、vet、固定相依性與 806 檔格式通過。略過的實際原因保存在兩份 full-*-summary.json。最後治理、清理與 Git 交付在各自實際 receipt 完成後記入 verification.json。

首次受限環境只有 Software Renderer，Root 沒有 CPU 假通過。原硬體 v1 探針錯把 warmup 當成零率，保留失敗；改正人工 fixture 為明確的 cosine final 0，保留嚴格零率斷言，沒有修改正式程式。OpenCode 免費模型本輪 60 秒逾時，使用已核准的 gpt-5.6-luna／max 唯讀分工。

【減法審查】

- Won't List：本票不新增 GPU 模式，不猜測性修改 Adam 或 LIF，不重跑真實任務當作恢復證據。
- 建議移除：驗收完成後移除本輪測試 binary、快取與 observer overlay，保存來源、命令、完整 log 與人工 fixture 證據。
- 建議簡化：resident 觀察沿用既有 Snapshot，以 test-only 新檔存取，不增加正式 API 或測試鉤子。摘要保留結果，runtime frames 只留原始 log。
- 需要留意：取消交易與微小浮點差異分開驗收，不能把額外觀察點當成新功能或重複擴大通過數。
