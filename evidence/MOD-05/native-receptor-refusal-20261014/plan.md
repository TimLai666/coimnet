# 原生受體宣告拒絕驗證

原始目標「確保沒有任何 bug」保持 active。前輪完成修正、完整驗證、清理與交付，是 progress。ticket 92 的五項驗收不代替整體目標。ticket 89 的相容性決策與局部學習的時間點歧義保持 pending，不修改正常公式。

- [x] Root 固定原生入口既有刺激閘門與受體引用的拒絕契約。
- [x] 公開失敗回歸先凍結，再由 Root 補最小共用檢查。
- [x] 連續／LIF、兩種適用規則、開啟／凍結、零／正受體、合法缺省、正常舊模型與完整工程通過。
- [x] 同步 MOD-05 與證據，清除本輪暫存，原有目錄保留。
- [ ] 提交、推送及遠端讀回，提交來源與驗證一致。

## Root 骨架與期望

simulate.Plasticity 的 gate(t)=GateScale*stimulus[t][GateChannel] 沒有化學佔用率綁定，完整 Rule 不得默默接受 GateReceptor／DecayEReceptor。validatePlasticity 在既有結構、通道、注入、尺度、規則種類與 Rule 數值驗證之後拒絕非 nil 的任一受體引用。原本非法宣告的錯誤優先序保留。Frozen 也不得攜帶未服務的引用，因它仍是尚未實作的宣告。正受體 0／100 都拒絕，不把零當成缺省。真正缺省是 nil。

正式來源僅 simulate/plasticity.go 的共用檢查與契約註解，入口為 Protocol.Validate、DecodeProtocol、Build、BuildVariant；Caller 不必等第一次 Run 才發現不支援。錯誤須明示 receptor、chemistry 與 native simulation，不新增模型版本、化學資料或其他 API。

Root 擁有永久回歸、期望、正式修正、驗證與交付。唯讀 worker 只可新增 runs/.tmp/native-receptor-refusal-20261014/review/report.md，不執行 Go、網路或 Git 寫入。

公開拒絕矩陣：continuous/hebbian、LIF/hebbian、LIF/stdp 三個合法核心／規則組合，gate／window／both、index 0／100、Frozen false／true 與四個公開入口。每次用獨立設定與 JSON 指紋確認拒絕不改輸入。合法控制：沒有 block、nil 引用的活動相關／脈衝時序規則、凍結／可學習、零／非零刺激閘門的舊／新來源輸出、狀態與完整 RunReport 完全相同（該格式沒有實際 wall_seconds，不排除欄位），Protocol hash 不變。

【減法審查】
▸ Won't List：不新增原生化學層、不修改增益範圍或學習時點。
▸ 建議移除：不替沒有資料的受體製造假佔用率，驗證後清掉本輪快取。
▸ 建議簡化：讓四個入口共用現有 validatePlasticity，避免在 Run 重複設防。
▸ 需要留意：只拒絕未生效的受體引用，正常刺激閘門及固定衰退保持。
