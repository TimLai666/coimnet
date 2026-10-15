# Ticket 103 最終程式審查

Scope: CLEAN。Root 已讀取六個正式核心檔的完整差異、兩個新增回歸檔的完整內容，以及 `ENG.md`、`AGENTS.md` 與本票驗收條件。最終來源由 `source-final.json` 的 812 個 SHA-256 固定。委派審查的依據見 `adversarial-review-final.md`，執行結果以 Root 的命令回條為準。

## Findings

No confirmed findings.

五處可設定閾值變換都只在有限端點的跨度成為正無限大時改用端點縮放。普通有限跨度的算式與運算順序保持。混合核心的持續狀態沿用同一個變換，沒有漏掉第六份公式。人工範例的固定 `0.05..1` 範圍不會觸發跨度溢位。

普通前向、重算區段及 LIF／混合持續狀態的最終閾值檢查，均在發布結果之前回錯。批次及單步失敗不提交呼叫端狀態，永久測試分別涵蓋適應與穩態造成的溢位。化學耦合原本已有最終閾值檢查，本次只補上其跨度計算。

測試在實作之前固定，Root 修正了獨立參考值的一個浮點間距，以及測試複製 nil／空切片不相等的問題，再重跑失敗。凍結回條為 `tests-frozen-final.json` 與 `effective-tests-frozen.json`。實作之後沒有放寬斷言。

Root 已獨立讀回最終相容性封存的 206 個成功回條與日誌，核對完整序列化內容與跨程序接續。普通與競態建置各對照自己的舊來源，沒有把跨建置取整差異誤判為本次回歸。結果見 `final-validation/compatibility-archive.json` 與 `final-validation/normal-compatibility.json`。大範圍保存物僅為小型人工測試樣本，見 `final-validation/wide-recovery.json`。

本次沒有新增參數、公開 API、依賴或模型格式。沒有新增共用抽象，避免改變正常運算的取整順序。正式程式未包含診斷輸出或測試專用分支。

另見 `evidence-doc-review.md` 的文件查證。Root 已重新計數目標競態的 40 個成功事件，與完整競態分開標記，並補上完整一般測試計數的直接連結。兩個實際原生派工的模型、effort 與工具接受回覆已從本對話工具紀錄擷取到 `delegation-dispatch.json`。64 次訓練接續在報告中明確拆成舊來源與最終來源各 32 次。

## Needs investigation

`dynamics/modulation.go:95` 與 `dynamics/lif_coupled.go:249` 對負向位移在取下界前溢位的處理不同。Root 已在原來源與最終來源重現相同結果，見 `modulation-floor-follow-up.json`。現有契約未指定應先拒絕或允許取下界，已記錄到 `AGENTS.md`，不列為這次修正造成的缺陷。

## 減法審查

- Won't List：本票不增加模型設定或更換正常算式，真實任務的長期品質量測另行驗收。
- 建議移除：驗證完成後移除本輪可重建快取與測試執行檔，必要日誌壓縮保留。
- 建議簡化：重複的相容性回條與日誌合併封存，保留逐檔指紋與獨立讀回結果。
- 需要留意：避免為尚未確認的取下界契約差異增加新的可選行為。
