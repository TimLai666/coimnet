# 126 — 研究者可以獨立核對持續化學來源的生效時點

**Epic:** 持續機制與保存驗證
**User Story:** 研究者在暖機、化學凍結及存檔恢復後，可以確認 timeline 與延遲回饋在原定神經模型步數生效。
**Blocked by:** 21、65、125（已交付）
**Status:** in_progress

- [x] 當前契約、獨立時間表、診斷骨架與 RED 完成。
- [x] 一般／競態矩陣、48 次新程序、原子性及隔離錯誤對照通過。
- [x] 正式來源身分、工程與治理驗證通過。
- [ ] 審查、必要證據保存、暫存清理、提交與遠端讀回完成。

## Root 決策｜2026-10-16

目前是已確認的證據缺口，尚未確認正式運算缺陷。ticket125 的來源不讀絕對時間，本票以具時點差異的 timeline／feedback controller 補足持續入口，保留暖機、兩種時鐘及凍結旗標不保存的原契約。固定矩陣、手算、責任、失敗與範圍見[事前計畫](../../evidence/MOD-02/persistent-source-clock-20261016/plan.md)。

一般／競態各 48 案例、1,920 條不同分段／恢復路徑、48 次新程序及 48 次原子性拒絕通過。controller 視窗在逐列基準另與公開 neural history 獨立比對，凍結列保持。兩種刻意時鐘錯誤在兩個建置均成功編譯並被行為檢查拒絕。840 份正式來源與 ticket125 逐位一致，已重新雜湊及重算其完整一般／競態測試事件，7,915／7,914 pass、23／24 skip。本輪重新執行診斷一般／競態、全框架 build／vet、固定依賴及治理，沒有重跑未變動正式來源的完整一般／競態。見[驗收](../../evidence/MOD-02/persistent-source-clock-20261016/verification.json)。

後續線索：FreezeChemistry 與 Intervene 並用的對照旗標、OfferFeedback 舊註解見[唯讀盤點](../../evidence/MOD-02/persistent-source-clock-20261016/readonly-findings.md)，前者尚未重現，不列正式缺陷。
