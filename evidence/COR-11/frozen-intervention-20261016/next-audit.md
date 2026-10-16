# 已確認的介入報告範數溢位

嚴重度 P2。合法有限的狀態差異在平方中間值上溢，讓本應有限的 L2 變成 +Inf，返回的 InterventionLog 無法 JSON 編碼。

目前 `learning/intervene.go:616-622` 的 l2Diff 對每個差先平方再開根號。公開 LIF／STDP witness 的參數 APlus=1e308、DecayPre=.5、初始 PreTrace=1，介入強制一次事件，產生有限 Eligibility=5e307。L2 應為 Hypot(5e307,sqrt(2))=5e307，實際 PlasticDelta=+Inf，JSON 錯誤為 `json: unsupported value: +Inf`。

決定性證據為 next-norm-probe-v2.json／.log，輸入保存在 next-norm-probe.go.txt 及最終 work archive。首次 probe 使用不合法的 decay_pre=1，被正式建構正確拒絕，日誌與初稿保留，不列模型缺陷。

這個 helper 未被 ticket127 修改。該票只交付 freeze 及錯誤提交順序修正。下一項依 ticket128 先檢查四個 delta 使用者、上下溢位及不可表示真實範數，再補完整失敗原子性；未在完整 race 運行中改動正式來源。COR-11 不因 ticket127 專項通過而恢復 passed。

同一 helper 亦已由獨立公開 Continuous／Hebbian probe 重現下溢：原 bias=1e-100，未介入的 eligibility=2.5e-201，clamp 後為零，真實 L2=2.5e-201，實際 PlasticDelta=0。輸入／輸出及命令見 next-small-norm-probe.go.txt、next-small-norm-probe.json／.log。四個 delta 的平方上溢與下溢都由 ticket128 一起核對。

Root 已核對共用失敗契約：`docs/model-and-mechanisms.md:73` 要求中途取消或數值錯誤不能留下半次更新。ticket93 已明定原生幅度的正常位元保持、可表示值穩定計算與真正不可表示結果拒絕。這可作 ticket128 的既有原則依據，但四個介入 delta 的錯誤政策及狀態復原仍需在該票獨立重現與驗收，不能由原生 runner 的通過結果推論。
