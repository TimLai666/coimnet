# 已核准的中性增益修正

延續 ticket 89 的四項驗收。使用者已核准：零值沿用原預設後，敏感度範圍必須有限且滿足 0 < GammaMin <= 1 <= GammaMax，拒絕不符合規則的舊設定／快照。範圍 [1,1] 合法。正常前向、反向、預設與保存格式保持。

Root 骨架：只修改 modulation/effects.go 的共用 checkedEffect 條件及公開說明。ApplyEffects、BackwardEffects、ChemistryConfig.Validate、Network／Trainer 建構與恢復、Individual 啟用／恢復及 checkpoint 解碼沿用同一驗證，不分別補償神經訊號。永久回歸先在未修正來源執行，確認公開拒絕缺口及失敗不提交；再採用最小候選。

驗收：1. 凍結的公開 RED／GREEN 與相鄰 float64 邊界。2. 實際舊來源合法模型的新程序載入、前向及下一次訓練相同；不合法舊快照明確拒絕，原始檔不變。3. gofmt、build、完整一般／race、vet、相依性、CLI 接續及最後治理。4. 必要證據保存後只清本輪 runs/.tmp/neutral-gain-adoption-20261014，拆分修正／回歸提交、正常推送與遠端／來源讀回。

減法審查：使用現有共用入口拒絕宣告，不新增模式、資料遷移、自動改寫設定或任務個別繞路。不把工程驗收當作辨識率提升、生物定量驗證或絕對零 bug。
