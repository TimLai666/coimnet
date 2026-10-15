# 115 — 替代梯度極端值與公開機制說明稽核

**Epic:** 動態核心數值稽核
**User Story:** 宣告的替代梯度在有限輸入下正確計算，公開說明與既有適應機制一致。
**Blocked by:** ticket 114 交付
**Status:** done

- [x] 公開介面重現分母平方溢位，核對獨立高精度參考。
- [x] 如確認缺陷，凍結失敗回歸與真正下溢控制。
- [x] 最小修正通過跨路徑、正常模型及舊快照對照。
- [x] 完成必要工程驗收與需求證據。
- [x] 保存證據、清除自有暫存、提交並核對遠端。

## Root 決策｜2026-10-16

公開介面已重現正負兩種有限輸入的梯度過早歸零，獨立有理數參考的局部斜率為 1e-310、Bias 約 0.005，見[重現證據](../../evidence/COR-09/surrogate-square-20261016/reproduction-verification.json)。修正保留正常算式，只在平方中間溢位時精確恢復局部斜率，真正下溢直接為零。一般／競態各 51 個凍結事件及正常／舊保存對照通過，見[候選驗收](../../evidence/COR-09/surrogate-square-20261016/candidate-verification.json)。Root 骨架與分工界線見[計畫](../../evidence/COR-09/surrogate-square-20261016/plan.md)。

`dynamics/lif.go` 與 `dynamics/mixed.go` 的事件函式原本使用 `1/(den*den)`。在 `den` 約為 `1e155` 時，平方會溢位，但宣告公式的結果約為 `1e-310`，仍可由 float64 表示。公開一節點重現設定為 DT=log(2)、tau=1、閾值範圍 [0,2]、theta_raw=0、reset=-1、scale=1、initial=0、input=±2e155、upstream=1e308。普通範圍與舊保存數值保持。

原始來源的兩個事件函式及它們的一般、重算與化學耦合呼叫端都要盤點。現有 `lifForwardTangent` 也使用分母平方，不能直接當作此極端值的獨立答案。真正低於可表示範圍的導數、原始非有限值與正常捨入要分開驗證。

本票一併修正 P2 公開文字矛盾：`LIFHomeostasis` 的型別註解、`model-and-mechanisms.md` 與 `individual-state.md` 曾把適應與慢速穩定一起當成反向常數。ticket 11 決策 7、ENG 與既有獨立 tangent 都保留適應狀態跨時間傳遞，三處文字已依實作釐清。慢速穩定偏移視為常數的既有行為保持。

本票不新增公開參數、數值限制或模型格式，也不改生物與學習規則。尚無真實任務品質或生物機制效益的新證據。

## 交付驗收｜2026-10-16

830 份來源的完整一般／競態各 58 套件通過，6,114／6,113 個事件成功、22／23 個略過、零失敗。原有各 6,085 個終端事件保持，只新增 51 個通過。格式、build、vet、模組校驗、固定 Insyra v0.3.4 及最後 31 項治理通過，見[驗收](../../evidence/COR-09/surrogate-square-20261016/verification.json)。

32 份正常核心紀錄、32 次舊神經狀態及 12 次舊化學模型新程序接續相同。必要工作封存為 716,539 bytes，自有 1,661,870,007 bytes 暫存已清除，原模型、資料、模組檔與其他工作保持，見[清理](../../evidence/COR-09/surrogate-square-20261016/cleanup.json)。

修正 `25685dc` 與回歸／證據 `4ff6619` 已正常推送，遠端讀回一致，830 份已提交來源與驗收指紋相同，見[交付](../../evidence/COR-09/surrogate-square-20261016/delivery.json)。COR-04／09 恢復 passed，目前 91／91 工程需求 passed。本票完成 5／5。下一項[閾值尾端斜率假設](../../evidence/COR-09/surrogate-square-20261016/next-audit.md)尚未公開重現，整體零 bug 目標持續進行。
