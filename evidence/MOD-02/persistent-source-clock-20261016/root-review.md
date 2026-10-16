# Root 審查

Scope: CLEAN。只新增可重現診斷證據、更新共用時間契約及需求追蹤。840 份正式來源與 ticket125 每個路徑及位元組均相同。

Root 完整讀取 fixture.go，逐一核對公開 Config 型別、暖機先後、兩種來源的絕對時間、受體宣告及 snapshot 起點。執行者沒有修改 main.go，Root 用先前 skeleton 的完整位元組確認責任分界。Root 在首次 GREEN 前補了 controller window／Filled 的獨立觀測，原案例、容差及分段判準沒有降低。

兩個公開入口沿用同一排程，freeze 切換時強制分段，沒有把重複切法灌入 1,920 路徑。六列全部 32 種分段及 frozen 合法 8 種分段分別核對。報告依每次呼叫的列數及 release 累加檢查，沒有拿不同長度報告作不成立的全量相等比較。

Root 實跑一般／競態、96 次實際新程序、48×2 次原子性拒絕，以及四個成功編譯後被拒絕的隔離錯誤。Python 另從宣告重新計算全部末濃度和兩個時鐘，檢查案例鍵與新程序組合完整性，並核對 118 份命令回條及日誌 SHA。上一輪完整一般／競態的原始日誌重新雜湊、解壓並重算終端事件，本輪全框架 build／vet、依賴及治理重新執行。

未確認新正式運算缺陷。沒有 API、模型版本、保存格式或預設值變動。附加 OpenCode 呼叫未執行，因當輪 models 已實際失敗；分工使用專案指定的 native gpt-5.6-luna/max，主 agent 自行完成驗證。

## Needs investigation

`learning/intervene.go:85,135-153` 以未含 freeze flag 的 snapshot 重建對照，若和 FreezeChemistry 並用，可能造成對照解凍。這個組合尚未重現，沒有列為 confirmed bug。詳見 readonly-findings.md，後續先執行可區分兩邊凍結政策的最小控制。

## 減法審查

建議移除本輪可重建 binary 與三種 Go cache，必要來源及 checkpoint 存入一份可核對的壓縮檔。直接沿用公開 API，不增加產品開關、來源時鐘欄位或另一套正式求解器。已驗證同一來源時不重跑整套長時間 race，保留明示的舊完整證據及本輪增量驗證。

最終治理首次執行確認本輪 verification.json 漏列必填 test_log。已補上真實日誌連結，未修改治理測試或正式來源。失敗原始回條及日誌保留為 final-governance，修正後另以 final-governance-v2 驗證，不覆寫失敗證據。
