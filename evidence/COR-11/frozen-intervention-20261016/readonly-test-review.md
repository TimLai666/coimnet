# 控制與契約審查

兩位 native gpt-5.6-luna/max 執行者分別查閱 runtime 與 snapshot 所有權、既有介入／凍結覆蓋，並獨立核對最小 witness、85 個案例及五個父測試的計數。執行者沒有跑 Go，Root 已實跑並核對全部結果。

Root 以公開來源與實跑進一步定案：無作用介入應與保持相同 freeze 的明確控制相同。原未凍結 twin 會衰減濃度或快速狀態，導致假差異。另以可控取消覆蓋兩種 freeze 都關閉的普通個體，確認回錯後已提交是獨立的順序缺陷。

LIF fixture 首次把 v_reset 與 theta_min 都宣告為零，被正式驗證拒絕。Root 在首次 GREEN 前改用 .1／1.9 範圍、保留閾值中點 1，未變更任何斷言。初稿與失敗日誌保留，修正依據見 fixture-contract-correction.json。之後有效 RED 為 26 pass／64 fail，所有 90 個 terminal 皆實際執行。

active block 與 DisableChemistry 控制使用沒有受體回授的 Hebbian 規則及零 timeline，期望輸出相同、濃度差為零成立。其他來源的任意作用不由這項控制推論。Context 計數器只在本測試同一執行緒中使用，藉由實際完整 Advance 的 Err 呼叫數決定取消點，不使用壁鐘競速。最後以一般／競態均通過及兩種隔離回退均失敗驗證敏感性。
