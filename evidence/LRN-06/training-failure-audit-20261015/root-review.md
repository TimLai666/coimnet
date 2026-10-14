# Root 審查與實際驗證

Scope CLEAN。Root 已讀完整三行正式差異，修改只有 `learning/trainer.go`；新回歸由 Root 寫入且凍結，worker 沒有修改測試。正式來源固定於 source-final.json，810 份來源指紋保持，舊測試、go.mod 與 go.sum 不變。

公開 tanh 重現使用三個神經元。神經元 0 是獨立輸入，1→2 的自由權重是有限的 1e308；輸出與電位都是零，故前向合法。最末列的上游梯度為 2.5，真正公開反向得到 bias[1] 梯度 9.9894100223432e307。手算為 `2.5 * (1-exp(-1))^2 * 1e308`，沒有越過 Insyra 的 float32 輸入／讀出界線。第二次 partial 加總成 Inf，舊程式仍回成功，JSON 與 RestoreTrainer 拒絕，見 accumulator-public-probe-v2.log。

Root 原始探針 upstream 列數不符，以及回歸初始電位的兩次 fixture 設定錯誤，都是診斷程式的錯誤，沒有列為框架缺陷事件。原始來源與失敗日誌保留，修正只提供合法 API 形狀與三個明示零電位，拒絕斷言不變，見 diagnostic-correction.json 與 test-correction.json。worker 開始實作前，真正 red-v3 為 15 pass／15 fail，14 個失敗葉案例都在 partial Sum 被接受，正常 full-window 與 frozen 控制通過。

修正沿用可表示性及候選先驗證契約：每項有效 mask 加總後立即檢查 finite，失敗丟棄候選，沒有更改加總順序、平均、裁切、Adam、API 或保存格式。Root green 與 scoped-race 各 30 pass、0 fail；所有七種入口、正負方向、partial／full window 的完整快照保持且可以 JSON／Restore，並可用相反梯度重試。原始公開探針在修正後也回錯、完整快照保持、沒有 Inf，見 public-probe-fixed.log。

worker 報告把凍結 manifest 的檔案雜湊拿來與 Go 測試檔的雜湊比較，並列出另一票的 manifest。Root 已直接解析本票 tests-frozen-v3.json，核對它登錄的測試檔 SHA 與實際檔案相符。manifest 本身與 Go 檔案本來就是不同位元組，這不是測試被改動。原報告 trainer-fix.md 保留。

六個既有 KMNIST ON／OFF 模型的保存狀態、SDK 推論及下一次 MSE 更新與基準逐位相同，18 個原檔指紋保持。另 32 個人工 SDK 設定涵蓋連續／LIF、視窗 1／3、loss scale 0／1024、recompute 與 sharing 開關，192 個獨立程序的完整／中途／恢復 JSON 與舊程式逐位相同，64 次子程序恢復與連續執行的快照位元組一致。人工輸出以正常累積恢復證據保存，不發布真實模型參數，也沒有重新量測任務辨識率。

第一輪完整一般測試因沙箱拒絕 httptest 的 loopback 監聽而中止，六個套件在斷言前 panic，原日誌保存。相同 810 份來源在 require_escalated 重新執行後，完整一般 58 套件、5,778 pass、22 skip、0 fail。同一來源的完整 race 也通過 58 套件、5,777 pass、23 skip、0 fail。格式、module verify、build、vet、module list、目標流程、CLI 延遲／LIF 及新程序恢復全部通過，接續與連續快照逐位相同。沒有為測試修改框架、測試、監聽權限或永久安全設定。

【減法審查】

- 建議移除：不增加另一套累積器、保存版本、遷移或專用訓練流程。
- 建議簡化：所有入口共用三行候選檢查。
- 需要留意：此範圍不證明整個框架零 bug，也沒有生物或真實任務品質提升結論。

最後 11 項治理、31 個通過事件已由 Root 執行。清理前兩次內容指紋與無開啟檔案檢查通過，本輪 1,396,734,821 bytes、9,929 個一般檔及 41,797 條暫存連結已清除。保護的原有目錄與 18 個模型原檔保持，驗證清單保存於 cleanup-inventory.json.gz。修正 `5e26db8` 與回歸／證據 `0393ad8` 已正常推送，Root 讀回遠端相同。810 份已提交來源與最後工程來源逐位相同。本票四項驗收完成，原始零 bug 目標保持 active。
