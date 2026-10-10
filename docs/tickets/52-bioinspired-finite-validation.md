# Ticket 52：研究者可以在生物啟發協定執行前拒絕非法數值

**Epic:** 生物啟發範例的可驗證設定
**User Story:** 合法設定與報告保持，非法數值在執行及雜湊前回錯
**Blocked by:** 無
**Status:** in_progress

## Root 決策｜2026-10-10：共用契約與驗收

入口沿用原協定、種子、訓練回合、協定欄位與來源登錄的驗證順序。NPF 只補 Suppressed 與 Tolerance 的非有限值拒絕。Ecdysone、定量命名、取消與 nil context 的行為保持。Luna max 只寫新測試；Root 凍結後自行填入兩個短判斷。

- [x] Root 審查先失敗測試並凍結，包含公開 Validate／Run、有限邊界及錯誤優先順序。
- [x] NPF Suppressed 嚴格介於 0 與 1，Tolerance 嚴格正且有限，沿用原錯誤位置與文字。
- [x] 合法 NPF／Ecdysone 完整報告與修正前及兩個新程序逐位元組相同。
- [x] gofmt、build、完整一般與 race、vet、相依性及治理通過。
- [ ] Root 審查完整 diff、需求證據、提交推送與遠端核對完成。

## 驗證證據

八個新增回歸與九個既有控制通過，修正前有三個新增回歸會失敗。七份完整合法報告在修正前與兩個新程序逐位元組相同。完整一般與 race 各 57 套件、格式、build、vet、相依性及八個治理／索引專項通過，見 [MOD-09 驗證](../../evidence/MOD-09/finite-config-20261010/verification.json)。

## 減法審查

Won't List：不改模型、訓練目標、依賴或歷史證據；不順手修正其他待辦。沿用現有資料夾與忽略規則，不另建資料管理系統。
