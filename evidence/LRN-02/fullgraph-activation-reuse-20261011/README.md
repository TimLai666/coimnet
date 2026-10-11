# 完整 MaleCNS 圖的分段訓練效能

2026-10-11，在 Apple M3／16 GiB RAM／macOS 27.0.1／Go 1.26.5 上，以完整 165,122 個神經元、25,563,197 條邊比較 ticket 57 修改前後的三個 SDK 呼叫。八組獨立程序交錯執行，第一組只作暖機，正式結果來自七組配對。

## 結果

| 呼叫 | 修改前中位數 | 修改後中位數 | 配對耗時減少 |
| --- | ---: | ---: | ---: |
| 分段前向 | 2.774 s | 0.948 s | 66.24% |
| 分段反向 | 11.935 s | 4.335 s | 63.06% |
| 完整 Trainer.Step 更新 | 18.300 s | 8.460 s | 52.56% |

配對耗時減少為 `100 × (1 − median(每組修改後時間 / 每組修改前時間))`，與兩個中位數直接相除不同。直接相除的減幅依序為 65.82%、63.68%、53.77%。全部七組的時間、最小值、最大值及配置量保存在 [performance-summary.json](performance-summary.json)，沒有刪除較慢的組。

| 記憶體觀察 | 修改前 | 修改後 | 差值 |
| --- | ---: | ---: | ---: |
| 前向每次累計配置 | 69,010,264 B | 71,664,920 B | +2,654,656 B |
| 反向每次累計配置 | 390,309,760 B | 438,086,400 B | +47,776,640 B |
| 完整更新每次累計配置 | 3,350,565,456 B | 3,400,996,752 B | +50,431,296 B |
| 整個程序尖峰 RSS 中位數 | 3.720 GiB | 3.760 GiB | +40.39 MiB |
| 整個程序尖峰記憶體足跡中位數 | 5.535 GiB | 5.567 GiB | +32.75 MiB |

累計配置量不是同時存活的記憶體。RSS 與足跡是 macOS `/usr/bin/time -l` 的整個程序觀察，包含載入、建構、核心呼叫、Trainer 建構、快照與摘要。它們不是某一階段的峰值。RSS 在修改前為 2.807–4.088 GiB，修改後為 3.359–4.084 GiB；共用桌面的其他負載與記憶體壓力未隔離，不能用這個分布保證固定峰值。

十六次程序的所有前向輸出、五項核心梯度、五組更新參數及 Adam 動量／二階矩／步數的 Float64bits 或 uint64 摘要完全相同，StepResult 也相同。五項核心梯度皆非零，完整更新有實際套用。檢查點 2 列、單段最多 9 列、完整歷史比較值 17 列保持。原始結果經 [verify_measurement.py](verify_measurement.py) 再獨立核對，見 [independent-verification.json](independent-verification.json)。

## 量測範圍

這是公開 MaleCNS 資料建立的完整接線圖。推導參數、統一神經純量、零延遲、tanh、輸入脈衝與目標都是明示工程設定。模型不裁切、不重新排列邊、不調參。16 步、段長 8、截斷窗 8、單一 CPU 執行緒、啟動環境設定 GOGC=100、GOMEMLIMIT=8GiB。程序讀回執行緒數與記憶體上限，GOGC 未另做 runtime readback。有效權重由既有 `learning.EffectiveWeights` 轉換，原始 store 與參數集的 SHA-256 由 `fullgraph.LoadModel` 核對。

計時涵蓋 `ForwardRecompute`、`BackwardRecomputeStats` 與啟用 Recompute 的 `Trainer.Step` 公開呼叫，包含呼叫內的必要驗證。載入、建構、明示 GC、快照與摘要不算進階段時間。Trainer.Step 還包含編碼、反向、裁剪、AdamW 與更新後預測，所以不能把反向加速當成整個訓練器的加速。

修改前只透過 Go overlay 換回 `90c6b93bb902fb16ce62ac937ae0e379becbb8ff` 的兩份核心檔。其餘來源與修改版同為 `38adbe4b6b1668c42dece0be7388eae5e75f36d4`，669 份 Go／模組指紋與 ticket 57 已完整驗收的版本相同。[build-source.json](build-source.json) 記錄兩檔指紋，[builds.json](builds.json) 記錄建置命令與執行檔指紋。

這次證明完整圖的分段重算加速及數值一致。它不證明長時間訓練達標、其他段長／動態／CPU／GPU 加速、能源下降、接線優於一般網路或生物機制正確。

## 框架完成度與模型能力

現行需求共 91 項，89 項 passed，TSK-11（真實任務達標）與 OPS-05（完整裝置稀疏訓練驗收）仍為 specified。框架已有可驗證的 CPU 訓練、推論、模擬與保存能力，可以用來做研究實驗；尚無證據支持完整全腦模型已能執行實用任務。

| 既有真實任務範例 | 保留集結果 | 能支持的結論 |
| --- | --- | --- |
| 英文文字 | 下一個位元組猜對 1 → 306／1,024，困惑度 260.18 → 14.18 | 小型文字預測學習有改善；生成內容仍破碎，不代表流暢對話 |
| 單字元 OCR | 200 個樣本正確率 0 → 10% | 流程能訓練；尚不足以實用，非整頁辨識 |
| 語音 | 2 段保留語音的字元錯誤率 0.943 → 0.981，詞錯誤率保持 1 | 沒有品質改善 |
| 影音 | 獨立來源綜合 MSE 0.066055 → 0.066538，生成畫面近黑 | 沒有品質達標 |
| 軌跡／自主行走 | 單步誤差比方向延續基準差，自主模型命中 0／13 | 沒有導航能力證據 |

這些範例使用小型工程核心，並非完整 MaleCNS 圖。這次讀取既有原始報告核對結果，沒有重訓任務或重新驗證歷史大型產物。資料來源、報告 SHA-256、完整數字與限制見 [capability-review.json](capability-review.json)。下一個能力門檻應先選定一項真實任務，事前固定保留集、簡單基準與實用門檻，再確認學習是否有效。

## 重現

只核對這次已保存的原始結果，不需要大型資料、Go 或訓練：

```sh
python3 evidence/LRN-02/fullgraph-activation-reuse-20261011/verify_measurement.py
```

重新量測須使用原生磁碟的同版本專案，並準備 [data-copy.json](data-copy.json) 指定指紋的兩份資料，置於專案 `data/malecns-v1.0/`。先保存既有證據目錄，再在新的專案副本重建同名輸出目錄，避免覆寫本次紀錄。原始程式保存在 `measure.go.txt`，原始執行流程保存在 `run_measurement.py.txt`；複製至專案 `runs/.tmp/fullgraph-activation-20261011/` 時移除 `.txt` 尾碼。

建立該暫存目錄內的 `baseline/`、`bin/`、`cache/`、`tmp/`。用 `git show 90c6b93bb902fb16ce62ac937ae0e379becbb8ff:dynamics/recompute.go` 與同一提交的 `dynamics/forward_recompute.go` 取得兩份基準，建立 Go overlay 的 Replace 對照。以 [builds.json](builds.json) 的相同命令建置兩版，再以 `python3 runs/.tmp/fullgraph-activation-20261011/run_measurement.py` 執行。路徑須改為新副本的絕對路徑；保持同樣設定與順序。事前 protocol.json 的 SHA-256 存入暫存目錄的 `protocol.sha256`。

`/usr/bin/time -l` 需要可讀取 macOS 核心時間資訊；本次首次沙盒呼叫在該權限被擋後失敗，原始資料保存在 `sandbox-time-failure/`，不計入正式十六次程序。一般測試首次也因沙盒禁止建立本機測試伺服器失敗，日誌保留於 `validation/`；後續原生驗證另存 `validation-native/`，兩者分開判斷。較大的測試日誌以 gzip 原樣壓縮，`log-archives.json` 記錄壓縮檔及解壓後指紋；`commands.json` 的原始日誌指紋保持。

## 軟體驗證

原生磁碟的格式、模組校驗、build、量測程式 vet、一般測試、race、全專案 vet、相依性及目標隔離九項檢查全部通過。一般測試為 57 個有測試的套件、1,774 個頂層通過、19 筆略過事件；race 為 57 個有測試的套件、1,773 個頂層通過、20 筆略過事件。CLI 入口套件沒有測試，兩次都另列一個套件略過。略過包含未啟用的歷史大型證據測試、子程序輔助測試與 race 下的大型檔案案例，不算通過。新證據登錄後的 11 個治理專項也通過，詳見 [validation-summary.json](validation-summary.json)、[final-governance.json](final-governance.json) 及 [verification.json](verification.json)。

## 保存與清理

保留小型程式、設定、指紋、原始數值摘要與驗證日誌。大型資料副本、執行檔與 Go 快取是本輪暫存，完成正式保存後清除，以 cleanup.json 的實際回條為準。減法審查：效能證據已足夠，不增加訓練功能或另存全腦模型。
