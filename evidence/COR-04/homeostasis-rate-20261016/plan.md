# Ticket 122 事前範圍與驗收

研究者可以在既有合法設定及有限活動率下接續放電模擬，不因中間乘積溢位而提早中止。

## 已確認根因

同一份來源的公開 Advance 在 arm64 成功，GOARCH=amd64 建置並在本機 Rosetta 實際執行時失敗。Rate=1e308、dt/tau_rate=2、spike=0 的實數結果 -1e308 有限；未融合的中間乘積先成 -Inf。見 public-probe-amd64.log。這不是 Intel 實體硬體驗證，也沒有證明真實任務品質受影響。

## 骨架與唯一正式實作格子

`dynamics/lif.go` 的 `(*LIF).stabilise(rate, offset, spike) (float64,float64)` 保留原算式。僅 next 非有限時，以 math.FMA 重算同一乘加，再走既有 h 更新及呼叫端有限檢查。正常有限算式、合法域、時點、停止梯度、API、格式及依賴保持。

共用呼叫位置：lif.go、lif_state.go、lif_segment.go、lif_coupled.go、mixed.go、mixed_state.go、mixed_coupled.go。單一 helper 修正覆蓋全部，不複製新算法。Root 自行填入這個短格子；Luna max 只讀契約與數學審查。

## 驗收（本票唯一權責）

1. Root 先新增正式失敗測試。兩種核心的公開持續入口核對正負 Rate、spike 0/1、Eta 0/非零與上限；Rate=M, q=2 接受，q=3 真正不可表示時保持拒絕，輸入與失敗回傳保持。
2. 從零 Rate 開始，q=3、只第一步放電，1023 步的精確末值 3*2^1022 有限。一般、非空耦合、分段與耦合重算，以及混合前向須成功。第1024步真溢位須拒絕。公開反向及重播須完成。此案例是極端工程回歸，非生理建議設定。
3. arm64 與本機 Rosetta amd64 的新回歸、既有 homeostasis 測試通過；正常核心與舊保存對照在各建置內保持。完整 gofmt/build/unit/race/vet/module verify、固定 Insyra 及治理通過，所有舊測試事件不退步。
4. Root 完整 diff 審查、來源指紋核對。證據保存後只清本票暫存，原模型、資料、模組及其他工作保持。提交推送後遠端讀回及提交內容一致才交付。

## 錯誤與例外

空／零／取消／非法狀態沿用原錯誤與完整套件回歸；不新增非有限輸入的接納。真正不可表示 Rate 仍由既有呼叫端拒絕，h 既有有界夾限保持。每個狀態與參數由呼叫者持有，成功與失敗皆不得改寫。

## 減法審查

只修共用 helper，不新增模式、參數或相依性。不收窄 tau_rate 合法域。獨立 homeostasis 梯度參考是後續候選，不混入本票。
