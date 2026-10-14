# Root 修正審查

## Diff Inspector

Scope: CLEAN。正式來源只修改 plasticity/plasticity.go 與 learning/individual.go，新增兩份公開回歸。既有數值更新、快照欄位及交接原件保留。

Adversarial review: RUN。持續個體快照與恢復會影響後續學習，因此請 gpt-5.6-luna/max 唯讀審查；Root 自行核對完整報告、live source、diff 與實際測試，不把 worker 的來源判讀算成執行證據。

## 原始確認缺陷

1. [P2] CONFIRMED：plasticity.New 保存 Rule 的兩個外部 *int，Config 又傳出同一 pointee。EnablePlasticity 與 Restore 沿用 New，Snapshot 沿用 Config。呼叫完成後改寫宣告或快照會改變原個體選用的受體。12 個 Model 輸入／輸出葉案例與 27 個三核心局部學習葉案例均在舊來源失敗，修改成仍合法的 index 1 就使局部學習量不同，無須用非法值製造崩潰。
2. [P2] CONFIRMED：RestoreIndividual 沒有在部件建好後沿用 checkReceptorRule。27 個三核心缺化學層／空受體／越界葉案例在舊來源被接受，而相同 Enable 宣告被拒絕。缺層會略過受體作用，越界會在下一步索引佔用率時崩潰。失敗快照不得改寫，沒有受體參照的舊缺省仍合法。

修正的位置是 New:130、copyRule:228、Config:245 與 RestoreIndividual:283。New 在驗證前複製指標值，Config 再提供副本。共享輸入指標的兩欄保留值語義，不保留可變身份別名。Restore 在部件驗證完成後，使用已擁有的 Rule 呼叫既有跨部件檢查，錯誤前綴沿用 individual plastic part，回傳 nil。

呼叫期間外部並行修改參數不在既有支援契約內，這次驗證的是成功返回後的副本隔離。Enable／DisableChemistry 仍沿用同一範圍檢查，不新增規則種類或核心特例。修正後來源審查沒有新的 CONFIRMED 缺陷。

## 已確認測試

- 先凍結永久回歸再修正。RED 的 69 個失敗事件包含 66 個葉案例與 3 個父案例，8 個控制通過。GREEN 的 77 個事件全通過，5 個頂層測試。沒有改寫凍結測試。
- 三核心的 gate／window／both，各覆蓋 Enable 輸入、Restore 輸入與 Snapshot 輸出。控制組用 JSON 深複製隔離，且近期參與與快變化均正，不能因共用同一別名或零活動而空泛通過。
- 32 組合法輸出／報告／完整快照，在舊來源、修正來源與新程序讀回舊公開存檔後一致。包含三核心 Hebbian、LIF stdp_pair、nil 化學缺省及合法受體 0／1，所有 8 組 stdp_pair 皆有非零事件跡及實際局部變化。
- 18 個額外葉案例在舊／新來源都通過：兩個受體欄位的負值拒絕及 continuous／mixed 拒絕 stdp_pair。兩個版本各 19 個含父測試事件。拒絕不改快照。
- 完整一般測試：58 套件、5358 個通過事件、22 個依既有條件跳過事件，3 個無測試套件。完整 race：58 套件、5357 個通過與 23 個跳過事件，最後交付仍待完成。相較一般的 22 個 skip，多出既有 checkpoint 大型存檔測試在 instrumentation 下的條件跳過。build、vet、相依性與 CLI 接續通過。

## 診斷限度

初始格式 wrapper 把歷史 evidence 的 Go 重現原件納入 gofmt，因 raster_overflow.go 的保存格式停止。來源指紋相同，未改歷史原件；第二版只檢查框架 Go 來源，format-v2.log 為空。這是 wrapper 範圍錯誤。

初始相容性探針把 Hebbian fixture 改成 stdp_pair 卻沒有建 pre_trace／post_trace，8 個 pair 案例被既有驗證拒絕。第二版先使用公開 NewState 建合法狀態，並將完整結果寫到檔案，避免 test2json 長行分段。永久測試沒有改動，兩版探針與原失敗保留，這不是正式框架缺陷。

worker 的合法 index 1、stdp_pair 與 Decay-E 負值缺口已由 Root 新程序與邊界探針補證。worker 未執行 Go 的限制保留，不將其報告改寫成執行成功。

ticket 89 的正式增益修正尚未核准，本輪未改 modulation/effects.go。BaseL2 的單位與公開可變 Controller 宣告政策仍屬既有待釐清事項，不列成本票確認缺陷。人工小模型證據不證明真實單字辨識率提升、所有生理機制有效或整體零 bug。

【減法審查】
▸ Won't List：不改增益政策、生理公式、模型版本或訓練配方。
▸ 建議移除：驗證終止後清除本輪快取與探針落盤，保留必要重現來源與壓縮日誌。
▸ 建議簡化：使用現有 checkReceptorRule，兩個指標共用一個私有 copyRule，不新增所有權抽象層。
▸ 需要留意：在公開返回界線複製可變值，避免額外複製大型核心參數。

## 全樹 consumer 後續稽核裁決

[worker 後續報告](worker-plasticity-next.md)的 C2 已由 Root 公開 Validate／Build／Run 的 6 個案例重現：連續／LIF 各 gate／window／both，受體 100 沒有綁定仍被接受，局部變化與沒有受體欄位的控制完全相同且非零。這是另一個 P2 缺陷，由 ticket 92 處理；本票正式來源維持兩個修正，MOD-05 已標 failed，最新工程需求 89 passed／2 failed。診斷 exit 0 表示成功觀察既有 bug，不是修正通過。

Root 不接受 worker C1 的「已確認 pre 時點錯誤」分類。主規格 10.3 只有 pre_activity×post_activity，沒有指定 pre(t) 與 post(t+1) 的相對時點；ticket 18 的公式與同票手算表互相矛盾。套件註解、現行 consumer 與已驗證手算表一致。Root 兩列 LIF 實測 eligibility／plastic 為 exp(-1)，若把 pre 定義成該步開始的突觸跡，則會是 1。這確認不同定義會改變數值，尚未決定哪個定義才是要求。分類保留 NEEDS INVESTIGATION，不更改正常學習公式，不把文件矛盾當作已確認模型演算法錯誤。
