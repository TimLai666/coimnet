# Root 審查

## Diff Inspector

Scope: CLEAN。正式實作僅 learning/consolidation.go，多一個私有精確補正函式，三個範數共用。測試來源由獨立 worker 寫入，Root 全文審查與凍結後未修改。Root 親自執行所有 Go 命令，不以 worker 宣稱代替證據。

逐項 Hypot 的已捨入前綴可能在最高端造成誤接受、誤拒絕及合法值少一個 ULP。公開 API 一般／競態重播均確認雙向問題，不能把單次 Hypot 的數值保證延伸為整個 reduction 的保證。先前唯讀 worker 的相反推論已依實測撤回。

安全粗界線：n>=2，累加結果 c 至少是最大絕對元件 A。若 c<=M/n，真範數<=sqrt(n)A<=M/sqrt(n)，離最近偶數溢位中點有足夠餘裕。n 轉 float64 的捨入遠小於 sqrt(n) 餘裕。n<2 沿用原值，其餘才重算。正常資料不新增高精度逐項成本與候選陣列。

精確路徑涵蓋平方指數 -2148 到 2048，再留 64 bits 給向量長度，4260 bits 足夠保存可配置 float64 切片的精確平方和。三種來源重建原本已捨入的元件，與實際慢速／保留寫入算式一致。平方和先與 (2^1024-2^970)^2 比較，避免 double rounding 在溢位中點誤分；低於界線再求根轉 float64。

Base 只回傳候選 norm，不提早回錯。Slow／Plastic 在既有候選有限性與延後 effective weight 檢查之後補正。最終 norm 錯誤仍在第二個取消檢查與延後 effective error 前，所有記憶與計數器寫入維持在全部檢查之後，沿用個體鎖。不新增公開參數、依賴或保存格式，不重定義 raw rho 單位。

## 驗證

RED 129 事件，74 pass／55 fail。修正後一般／競態各129 pass；含既有鞏固測試各224 pass。公開原始九個與擴充十五個案例在兩種建置輸出逐位相同，有限值成功、真正不可表示錯誤且狀態／額度保持。回退舊 Hypot 與拒絕時偷增 Used 兩個隔離對照都成功編譯，各被55個事件拒絕，未改正式來源。見 diagnostic-verification.json。

Findings: 本次已審查 diff 未確認新增缺陷。完整工程仍以 engineering-verification.json 的實際完成結果為準。

減法審查：採單一私有 helper，不重建整個模型、不配置候選向量、不增加使用者設定；原模型及資料保持。這是極端數值與失敗原狀修正，不是辨識率改善、生物定量校準或全框架零缺陷證明。

## 接續稽核

唯讀 worker 找到線上 reward gate 與受體宣告衝突。Root 追到更早的作答路徑，並用公開 API 一般／競態各16個正常數值控制重現：四種政策的 Act 都能提前建立快速記憶，包含 evaluate；無受體與無脈衝控制保持零。LRN-06回到specified，由ticket130接續，來源核對證明不是ticket129引入。本票MOD-06驗收不被當成該新缺陷的修正，工程追蹤不能宣稱91／91。

Root未採用僅拒絕plastic建構的提議，因為不能解決gradient／evaluate作答且會縮減可組合機制。下一票依原OnlinePolicy契約明確指定單次操作的閘門來源，原生受體學習與公開雙重閘門拒絕保持。TrainEpisode的獨立episode契約已明示，沒有把未使用持續fast／slow列為確認缺陷。
