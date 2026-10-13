# 控制器狀態、梯度與輸入契約稽核

本階段修正四類已公開重現的缺陷：記憶控制器失敗污染後續狀態、參數更新後使用舊輸出計算梯度、非法 OCR 背景／雜訊，以及鞏固拒絕報告漏計數。前兩類會使接續輸出或梯度錯誤，後兩類影響輸入有效性與報告判讀。沒有證據顯示既有正式化學對照曾觸發舊快取問題。

修正保持合法輸入的算式與模型保存格式。18 組渲染、9 組小型 OCR 訓練報告，以及視窗 1／2／4 的 60 列控制器正常更新，與基準逐位相同。這些是指定案例的相容性證據，不是實際辨識率改善。

## 檢查結果

16 個新增頂層回歸測試涵蓋四類問題。RED 日誌保留修正前失敗，GREEN 日誌保留修正後通過。完整一般測試通過 58 個套件、5,006 個測試事件，其中 2,066 個頂層測試。build、vet、相依性與模組內 771 份 Go 格式檢查通過。完整 race 也通過 58 個套件、5,005 個測試事件，其中 2,065 個頂層測試。兩輪都沒有失敗或資料競爭警告。最後 11 項治理檢查也通過。已清除本輪暫存，修正與回歸／證據提交已推送並讀回一致。

完整來源指紋包括 773 份 Go 與 go.mod／go.sum，共 775 份。兩份歷史 evidence Go 原件保持，不列入目前模組程式的格式驗收。Insyra 固定 v0.3.4，go.mod／go.sum 與基準相同。

## 證據入口

- [最終工程驗收](verification.json)
- [完整測試與跳過項目](test-summary.json)
- [最後 11 項治理檢查](governance-final.json)
- [四類缺陷與品質影響](findings.md)
- [Root 差異與契約審查](root-review.md)
- [公開回歸結果](targeted-summary.json)
- [OCR 合法案例相容性](compatibility-result.json)
- [控制器正常更新相容性](controller-compatibility-result.json)
- [清理工具修正與保存檢查](helper-review.md)

## Git 交付

修正 `23582a2` 與回歸／證據 `dd185a3` 已推送至 `codex/receptor-chemistry-training`，遠端提交讀回相符。見 [交付回條](delivery.json)。逐檔變更見 [changed-files.md](changed-files.md)，全部證據指紋見 [artifact-manifest.json](artifact-manifest.json)。

## 暫存清理

只刪除本階段建立的 `runs/.tmp/refusal-input-audit-20261013`：9,683 個檔案、1,419,291,751 bytes。清理後目錄不存在，775 份來源指紋相符。兩個既有暫存目錄的身分與第一層項目相符，資料與模型沒有列入刪除範圍。見 [清理回條](cleanup.json)與[逐檔清理清單](temporary-inventory.json.gz)。

## 限制與下一步

一般測試通過的大型模型案例，在 race 模式依既有記憶體規則跳過。其餘未配置的全腦、外部資料證據入口和子程序 helper 跳過清單完整保留，不能把跳過當成新驗收通過。

本階段沒有重訓真實任務，也沒有生物定量正確性或絕對零 bug 的結論。受體區域會選取化學濃度，部位紀錄不建立各部位獨立膜電位，Kd／N 是工程係數。執行途中修改控制器公開宣告的歷史狀態語意仍待釐清，沒有列為已確認缺陷。

首次 race 有磁碟不足的執行錯誤，另有編譯失敗事件而缺少編譯器正文。空間恢復後停止本輪串行試跑，用同一份來源和原完整並行設定重驗。原始失敗、停止與重驗各自保存，沒有放寬測試。build 的 exit 0 包含一則選用模組版本快取寫入遭 sandbox 拒絕的警告，原文保留。

宿主：Codex。OpenCode 模型列舉回 `Unknown: FileSystem.open (/Users/timlai/.local/share/opencode/log/opencode.log)`，沒有模型呼叫。依專案規則由原生 subagents 的 gpt-5.6-luna、max 執行，Root 親自審查與驗證。

💡 減法提醒：直接保留失敗前狀態、清除已失效的快取，不增加第二份參數或新的保存格式。
