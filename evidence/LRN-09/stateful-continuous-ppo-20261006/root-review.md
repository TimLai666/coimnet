## Diff Inspector

Scope: CLEAN
Reviewed: base `16718541d6adce7cbc15b74308ccfe37c25c461f` 至最終 Go 原始碼指紋 `c433afec16e04bd78290127850a5336a03b5e247e3b6f41b8de9fdbcc0ce7949`，包含所有既有檔案 diff、新入口及測試完整內容、README、ENG、ticket 與交付追蹤。

### Findings

No confirmed open findings.

原審查的 P1 候選驗證缺口已修正：`learning/trainer.go:537` 依 optional initial 選擇同一保存狀態前向，Step／StepFrom 的 nil 路徑保持原 Predict，提交點保持。`candidate-red.log` 先證實錯誤提交；`feature-final.log` 的立即更新與未完成累積控制現皆在提交前拒絕，參數、AdamW、更新次數與原累積視窗逐位保持。Root 自行重跑原 probe 也不再提交候選，見 candidate-probe-final.log。

### 契約與關聯追查

- `dynamics/continuous.go:158,233,424`：零狀態與保存狀態共用前向迴圈及 delayed source reader。保存歷史只供讀取與權重梯度使用，不反向寫入先前段。原 Forward 的計算順序及初始 activation 梯度保持。
- `dynamics/state_forward.go:15`：使用既有 ValidateState，輸入與未來歷史容量在配置前驗證，uint64 步數先防溢位。最新歷史沿用保存的原值，trace 擁有副本。
- `learning/network.go:501`、`learning/stateful.go:16`：具體 CPU scalar continuous 型別及狀態聯集驗證；完整 Insyra encoder／readout tape、固定符號與共享參數反向沿用既有流程，沒有新建第二套梯度橋接。
- `learning/stateful.go:47`：沿用 context lock、更新溢位檢查與 stepWithGradient。拒絕 Recompute，不以未支援模式默默回退。失敗不發布 trainer／AdamW／累積器變更。
- `learning/rl/update.go:209,278,314`：同一 InitialNeural 供每個 epoch 計分與梯度；所有 rollout 的版本、機制、初始狀態、步驟及優勢資料先檢查。執行期數值或取消失敗也只丟棄私有候選，傳入個體不變。
- `experiment/ppo_collect.go:58,69`：原走廊範例仍明確重設每個 episode 的零狀態。既有 collector 不受新增入口影響。已搜尋全樹 InitialNeural 與三個新方法的產生、傳遞與呼叫位置。
- `learning/trainer.go:80`：TrainingSnapshot 保存參數與最佳化器，神經歷史由呼叫端提供；完整保存沿用 IndividualSnapshot，沒有格式或資料遷移。

### 直接驗證

Root 自行執行公開入口失敗重現及成功驗收，並非以 agent 摘要取代。最終來源的 22 個頂層專項與 372 個相關套件頂層測試通過。修正前 57 個完整一般套件曾通過，最終來源完整一般 57 個套件通過；最終完整 race 的 57 個套件也通過，見 full-race-final.log 與 verification.json。前向比 Advance，參數／輸入梯度比獨立有限差分，第一輪 AdamW 比獨立公式，多 epoch 比手動公開 Loss 更新，新程序保存恢復比完整 checkpoint bytes。原零狀態與重算的控制通過。

測試 helper 的複製缺陷先由獨立控制重現，Root 僅修正一行來源，未改原斷言。原八檔、修正後九檔及最終十檔凍結均保留；source-integrity 檢查恰好只差該行。詳見 root-test-review.md。

完整 gofmt、build、vet、mod verify、tidy-diff、原件指紋與 SIG-03 依賴方向通過。CLI doctor/data sources、原 delayed/LIF 範例及 80+40／120 恢復比較通過，暫存產物已清理。API 的 go doc 與證據腳本 --help 已實際檢查。最終完整驗收記於 verification.json；交付待提交及推送後另外記錄。

### 適用範圍及減法審查

純本機 library 入口，沒有新增 SQL、身分認證、對外請求、權限或資料遷移。驗證證據限 Mac arm64 人工數值與整合，不涵蓋全圖效能、其他平台、導航學習或生物機制。

移除實作第一版重複的逐值驗證及整段參數掃描，驗證留在既有入口。前向與反向共用 delayed source reader，減少兩份延遲索引規則。保留單一 encoder／readout 與最佳化器流程，沒有額外 PPO 設定或模型範例。

容量疑慮已依 `dynamics/state.go:15-17` 與 `docs/resources.md:171` 查證：MaxStateValues 是 State voltage、retained history、當次 Advance output 的分別元素限制，不是程序記憶體上限，Trace 私有起點列未納入該契約。ForwardFromState 的 input／返回 output 與未來 State history 上限保持；沒有因此擴大容量修改。

獨立收尾審查確認 P1 修正有效，無未解確認 finding，詳見 adversarial-review.md。Root 已複核依據及直接前後重現。
