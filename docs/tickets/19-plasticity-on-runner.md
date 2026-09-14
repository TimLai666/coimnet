# 19 — 研究者可以在原生模擬之上開啟可塑性，並報告原有輸出被增強、修改或破壞

Epic：原生模擬（可學習模式的第一步）

User Story：研究者可以用同一份刺激與判讀協定，比較「無可塑性」「開啟可塑性」「學過之後關閉」
三種執行，說出原有輸出被增強、修改或破壞了哪些；關閉可塑性時逐位還原原生行為。

Blocked by：14 空模型與判讀、18 局部可塑性（第一階段）

Status：draft（契約已於 2026-09-15 定案；待 18 第一階段完成後派工）

對應需求：NAT-06（學習前後在相同刺激與判讀協定下的對照矩陣；關閉學習可還原原生行為）。
主規格 10.3、14.4（對照組）、研究方向第五層。

## Root 決策（2026-09-15）

1. **protocol 的 `plasticity` 區塊**：`Plasticity{Rule plasticity.Rule; Edges *Selector 或 "all";
   GateChannel int; GateScale float64}`，閘門取自刺激矩陣的一個通道（與注入通道分離），每步
   `gate(t) = gate_scale × stimulus[t][gate_channel]`；未宣告 = 關閉。`RunReport.plasticity{rule,
   enabled_edges, gate_channel, clamped_by_w_min, plastic_l2_before, plastic_l2_after}`。
2. **runner 內的順序**：沿用 18 的固定順序（核心前向 → 更新跡 → 套閘門 → 下一步用新的
   `w_eff`）。`w_eff` 每步重算成核心的權重陣列（`dynamics` 的 `Advance` 接受每次呼叫的參數，
   因此分塊推進時在每個分塊前更新一次；為了逐步更新，分塊大小在開啟可塑性時強制為 1，並在報告寫明
   耗時代價）。
3. **三種執行**（`simulate compare` 新增 `learning_variants`）：`original`（無可塑性，逐位等於
   NAT-01／02 的報告）、`plastic`（開啟，報告學後的 plastic 統計）、`learned_then_frozen`（先跑
   `plastic` 得到 `plastic` 陣列，再以該陣列固定為 `w_eff` 且關閉更新，重跑同一刺激）。指標與門檻
   沿用 14；每格的 `deltas_from_original` 直接回答「增強／修改／破壞」：報告不做文字判斷，只給差值
   與百分位。
4. **關閉即還原**：`plastic` 陣列全零且關閉更新時，RunReport 與未宣告 `plasticity` 的報告逐位相同
   （測試）。
5. **真實資料**：推導參數集 + NAT-02 protocol + `hebbian_rate` 規則、閘門為刺激後 20 步的一段常數
   （`gate_channel` 1），啟用全部邊；三格各跑一次，`evidence/NAT-06/`。

## 契約

```go
// simulate
type Plasticity struct { Rule plasticity.Rule; Edges *Selector; All bool; GateChannel int; GateScale float64 }
// Protocol.Plasticity *Plasticity `json:"plasticity,omitempty"`（omitempty，既有 hash 不變）
type PlasticityReport struct { Rule string; EnabledEdges int; GateChannel int; ClampedByWMin uint64; PlasticL2Before, PlasticL2After float64 }
// RunReport.Plasticity *PlasticityReport `json:"plasticity,omitempty"`
// CompareProtocol.LearningVariants []string  // original | plastic | learned_then_frozen
```

## 驗收

- [ ] fixture：三格對照的手算（小圖、hebbian_rate、常數閘門）、關閉即逐位還原、分塊為 1 不改結果、
  `learned_then_frozen` 的 `w_eff` 等於 `plastic` 跑完後的值；`go test`、race、vet。
- [ ] 真實資料：三格各一次，記錄命令、環境、指紋、耗時；報告只給差值與百分位。
- [ ] 文件與 `evidence/NAT-06/`。

## 依據

- 研究方向 NAT-06、主規格 10.3、14.4、7.2（模式表：運行中局部學習）。
