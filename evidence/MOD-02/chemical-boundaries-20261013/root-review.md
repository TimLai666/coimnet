# Root 審查

Scope: CLEAN。Root 親自審查八份 production diff、七份新回歸與其正常控制組，核對資料產生端、使用端及既有契約。測試與實作由不同責任者完成，Root 執行所有 Go 驗證。

## 契約與資料流

- Kinetics.Step 拒絕無限合成率，Backward 先經 Step，狀態與輸入不變。有限指數飽和、零 boost 與區域傳輸保持，獨立高值參考與非零梯度控制驗證。
- SourceSpec.Build、NeuralActivity.Validate／Release、InternalResource.Release 的 MaxInt 加一溢位一致拒絕。原測試草稿曾要求 raw Channels() 改變無錯誤 accessor 契約，Root 退回，正式測試只驗證已承諾的拒絕入口。
- EnableChemistry 在個體鎖內先驗證 plastic／expression 受體引用，才提交濃度歸零與資源／回饋接續。continuous／LIF 的拒絕後快照、報告、Advance、TrainEpisode 與還原均對照 untouched twin。
- chemicalEpisodeNetwork 共供化學 TrainEpisode 與線上 episodePredict，私有複製濃度、來源視窗、resource、pending feedback、expression、SourceStep、InitialActivity，保留 frozen。兩個 caller 均以 Individual → Trainer 取得鎖，helper 不鎖定、不交換共用 Network、不推進持續狀態。ordinary 路徑仍用 Trainer.Predict。
- 線上有效目標由同一明示 episode 的獨立預測計算，score 0、.5、1、-1、2、continuous／LIF 及 frozen 控制通過。沒有把零 loss 等同已累積 Adam 動量一定不改參數，零分不誤更新的反例使用新 optimizer／零 decay。
- RestoreTrainer 先檢查形狀、有限值、accumulator，再沿用 sharePlan 的 weights／bias／log_tau flat 索引驗證 First、Second、Steps、Sum。多成員群組相同，singleton／-1 獨立，拒絕不修改呼叫者快照。RestoreIndividual 經同一路徑，完整與 partial 接續逐位比較。
- GradientSteps 只計 Applied。UpdateEvery 的候選位置只在合法 skip 或成功訓練後提交，失敗保留 feedback 及到期間隔。既有混合梯度／塑性部分成功沒有新增整體回滾保證。

Adversarial review: RUN，觸發為多套件、持續狀態、共享恢復及線上訓練。Luna max 對八檔及 chemical_source_state、chemical_sequence、kinetics_gradient 的唯讀審查沒有確認新的缺陷。Root 將其 fresh activity 與跨入口並行疑點保留於 findings.md，沒有把靜態審查當成測試證據。

## 工具與環境

opencode models 在開啟既有 opencode.log 時回 Unknown: FileSystem.open，沒有成功呼叫免費模型。依專案明確 fallback，三個原生 subagents 使用 gpt-5.6-luna／max。命令、環境、來源指紋與完整原始日誌均保存。

格式檢查沿用 scripts/verify.sh 排除 historical evidence，最終778份 module Go 檔通過。先前找到的歷史 raster_overflow.go 與基準逐位相同，保持原件。task-local symlink module cache 不適用 go mod verify，保留該失敗後唯讀原有 module cache，實際回傳 all modules verified。所有新增編譯／測試快取在本輪 runs/.tmp。

第一輪 race 因新增四個已確認缺陷而停止，保留停止及失敗回條。最終782份 Go／模組來源固定後重跑完整一般與 race，各58套件通過，僅最終 race 作最後驗收，不能把先前已通過的部分套件當成完整成功。

## 驗收限制

沒有新的真實 OCR 品質比較、生物定量、其他平台、GPU 或 NAS 故障耐久性驗證。合法公開輸出及存檔相容，不代表所有模型都未受歷史錯誤影響，更新框架不會撤銷已寫入模型的舊更新。原91／91是原定工程需求驗收，無法證明零 bug。

最終驗收、清理與推送以 verification.json、cleanup.json 及 delivery.json 的實際回條為準。

Git提交檢查對文字patch的合法context縮排回報space before tab。兩份patch改以gzip保存，解壓後逐位相同，沿用既有*.gz binary，不新增忽略規則或修改正式程式。見patch-encoding.json。
