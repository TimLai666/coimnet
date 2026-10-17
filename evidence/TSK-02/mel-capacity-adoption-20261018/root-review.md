# Ticket145 正式採用核對

正式 frontend.go 與已驗證候選 SHA 完全相同，兩份新回歸也與事前凍結的公開／私有回歸逐位元組相同。其他 863 份既有 Go／模組來源保持，866 份來源已固定於 formal-source.json。

MelFilterbank 在基本 Validate 之後、任何濾波器配置之前，檢查邊緣陣列、頻率／每列及外層切片標頭的原生位元組尺寸。LogMel 使用既有 error 分支回傳 nil 與零報告。基本 Validate、STFT、FrontEndVersion、正常算式與模型格式保持。私有尺寸 helper 不配置音訊陣列，各列獨立檢查，不限制未實際配置的總乘積。

Root 重新解析正式一般／競態日誌，各100個 pass 與隔離候選逐項相同；原有81個結果保持，恰好新增19個事前固定 pass，見 focused-root-review.json。公開巨大尺寸測試先以獨立大整數證明 runtime 配置前拒絕的條件，私有 oracle 安全檢查只可表示而不實際配置的界線。三項正常控制涵蓋手算濾波器、零音訊完整 LogMel 報告與 STFT 的未使用 Mels。

首次全套程序84436實際exit1，一般測試8708pass／23skip及一個TestFormatIndex失敗。新增unsafe匯入使版本常數由第11行移至12行，但docs/INDEX.md:179沿用舊行號。Root已更新該列label與anchor，未修改或放寬測試；31項治理重查全數pass。失敗日誌及實際terminal保留，見format-index-repair.json。

新全套程序97205使用engineering-final標籤，與首輪結果分開保存，866份Go／模組來源保持相同，修正後的INDEX指紋另存validation-inputs.json。完整配方要求保留前一版全部8713個測試結果並只新增19個pass，另驗證格式、build、vet、固定模組、31項治理及最後治理用二進位檔。Root 已重新解析修正後完整一般日誌，8709pass／23skip、58pass／4skip套件，8713舊結果全部保持，僅新增19pass；正常音訊、九份報告與21中性增益控制保持，見unit-root-review.json。原程序97205已實際terminal exit0（3179bd）。Root逐筆重新解析完整一般／競態與全部九項receipt，8,709／8,708 pass、23／24 skip，各58套件通過，原8,713結果保持，只新增19pass；31項治理通過，正式866來源保持，見full-engineering-root-review.json及verification.json。本票4／5，最後31項治理已通過，1,648,123,020bytes自有暫存與快取已清除，原始模組、模型、資料與其他工作保持。提交推送與遠端核對待完成。

算術可表示不保證 runtime 或可用記憶體容量。舊模型相容性對照只支持兩個已封存的人工小型存檔，不外推全部模型、任務品質、生物機制或其他平台。整體零 bug 目標持續。
