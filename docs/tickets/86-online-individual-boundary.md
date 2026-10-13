# 86 — 使用者可同時訓練個體並取得一致的線上答案與回饋更新

Epic：運行中學習與模型品質

User Story：同一個體同時有線上學習與外部操作時，每筆答案對應真正產生答案的模型版本，回饋損失權重不會使用另一版參數的預測。

Blocked by：85（已完成，同一來源交付已核對遠端）

Status：completed（2026-10-14，5／5 驗收與交付完成）

對應需求：LRN-06。正式測試先在舊實作重現 17 個行為 RED，修正後 5 個頂層回歸及 31 個測試事件通過，race 連續 20 次共 620 個事件通過。完整一般／race 各 58 套件、build、vet、格式、相依性、11 個治理檢查及 CLI 新程序接續均通過。788 份來源指紋相符，本輪 1,713,255,539 bytes 暫存已清除，修正與回歸證據已推送且遠端讀回相符。

## 已確認問題

- Act 先完成 Advance 再取得 Snapshot，外部 TrainEpisode 可插入。診斷的輸出與舊模型副本相同，ModelVersion 卻是新模型，違反 ActionRecord 公開註解。
- Update 的 episodePredict 與 TrainEpisode 分別取得個體鎖。外部更新插入後，score=0 使用舊預測作 target，產生梯度範數 0.010393098556645655、更新範數 0.019688654607431307。實際完整快照與「外部更新後使用舊預測」對照相同，與完整序列化對照不同，不能歸因為既有 Adam 動量或正常計數。

診斷、完整控制及來源限制見[查驗證據](../../evidence/LRN-06/online-boundaries-20261014/public-interleave-v3-diagnostic.json)與[診斷來源](../../evidence/LRN-06/online-boundaries-20261014/public-interleave-v3-diagnostic.go.txt)。

## Root 決策（2026-10-14）

兩個問題都應將同一次高階操作需要的個體狀態固定在同一把個體鎖內。維持 Individual → Trainer 的鎖順序，使用私人已持鎖 helper，避免重入相同鎖。保留既有公開簽名、普通訓練算式、模型格式與混合部分成功契約，不把整個 Update 批次變成回滾交易。正式測試必須證明新的正確結果，不能把觀察到舊錯誤的診斷 PASS 當作修復驗收。

## 驗收

- [x] Root 凍結正式 RED 回歸與正常控制，含答案版本及零／中間／完整回饋權重，保持取消與失敗契約。
- [x] Act 的答案與 ModelVersion 取自同一次個體邊界；錯誤不追加答案。
- [x] 回饋加權預測與訓練共用同一個體邊界，一般／化學／凍結／累積路徑保持，外部操作可安全等待及取消。
- [x] 正常相容性、完整 build／一般／race／vet／相依性／治理及 Root 審查通過。
- [x] 證據、暫存清除、提交推送與遠端讀回完成。

## 修正前準備（歷史）

交付基準 `fa5ac1f3129e63ac6fc0ce41b4603bf80c5553b5`。未執行的測試草稿、骨架與 Root 審查已保存於[準備清單](../../evidence/LRN-06/online-boundaries-20261014/next-boundary86-preparation.json)。採用前須先補失敗清理、核對同步穩定性及化學／凍結／累積／取消控制，再取得可編譯的行為 RED；當時尚未完成正式回歸驗收，本票的實際進度以上方狀態及驗收清單為準。

## 已取得的修正證據

[凍結測試](../../evidence/LRN-06/online-individual-boundary-20261014/frozen-tests.json)、[正式 RED](../../evidence/LRN-06/online-individual-boundary-20261014/red-summary.json)、[GREEN](../../evidence/LRN-06/online-individual-boundary-20261014/green-boundary-summary.json)、[20 次 race](../../evidence/LRN-06/online-individual-boundary-20261014/green-race-stability-summary.json)及[移動函式本體逐位核對](../../evidence/LRN-06/online-individual-boundary-20261014/extracted-body-equality.json)。本票不宣稱真實辨識率提升、生物定量正確或整個框架零 bug。

[完整工程驗收](../../evidence/LRN-06/online-individual-boundary-20261014/verification.json)包含 32 組合法逐位比較、跨程序接續與完整一般／race 實際日誌。

[清理](../../evidence/LRN-06/online-individual-boundary-20261014/cleanup.json)及[交付](../../evidence/LRN-06/online-individual-boundary-20261014/delivery.json)完成，原始資料、模型及既有兩個暫存保持。修正 `a008c1e`、回歸／證據 `d73dc5e` 與遠端同一來源。
