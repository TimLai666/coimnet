# 106 — 使用者可把操作與教學成果留在專案內

**Epic:** 本機資料與暫存保存
**User Story:** 使用者執行操作腳本與 README 教學時，預設工作目錄、快照與原始資料位於專案內，既有結果與其他工作保留。
**Blocked by:** 53、105（已完成）；工程驗收與自有暫存清理已完成，等待交付
**Status:** in progress

- [x] 核對兩個呼叫端、原始保存契約、來源指紋及隔離驗收範圍。
- [x] 凍結回歸並重現舊路徑，再完成最小腳本與說明修正。
- [x] 完成新路徑、失敗保留及適用工程驗證，保護原始輸入與其他工作。
- [ ] 保存證據、清除本票暫存、提交並核對遠端。

## Root 決策｜2026-10-15

沿用 ticket 53 的專案內暫存原則，調整兩個操作腳本與三處教學的預設位置。明示工作目錄、原本輸出／快照保護、非空匯入暫存留供檢查的契約保持。另修正專案內匯出目錄會繼承外層工作區版本戳記的問題，乾淨匯出流程追加 `-buildvcs=false`，其他 Go 旗標保留。

目前 3／4。兩份凍結回歸已逐位元組納入正式測試，一般／競態各 14 個事件通過。驗收讀取實際 Go 程式的建置資訊，確認預設工作目錄清理、明示保存、外層版本資訊隔離及同級成果保護，見[正式專項驗收](../../evidence/OPS-03/operational-temp-paths-20261015/formal-targeted-verification.json)。815 份 Go／module 的完整一般與競態各 58 套件通過，5,840／5,839 個測試事件成功、22／23 個略過、零失敗。build、vet、811 份正式 Go 的格式、固定 Insyra v0.3.4、模組校驗與最後 11 項治理／31 個事件通過，見[工程驗收](../../evidence/OPS-03/operational-temp-paths-20261015/verification.json)。兩份歷史證據 Go 程式保留原始位元組，格式範圍修正見[紀錄](../../evidence/OPS-03/operational-temp-paths-20261015/format-scope-correction.json)。README 的訓練、接續及推論已實跑，必要快照與輸入原文已保存且指紋一致，見[保存核對](../../evidence/OPS-03/operational-temp-paths-20261015/tutorial-preservation.json)。自有暫存已清除，其他五個工作目錄、815 份來源、18 個模型及固定訓練輸入保持，見[清理](../../evidence/OPS-03/operational-temp-paths-20261015/cleanup.json)。沒有執行完整 OPS-10 或真實資料匯入。剩下拆分提交與遠端核對。
