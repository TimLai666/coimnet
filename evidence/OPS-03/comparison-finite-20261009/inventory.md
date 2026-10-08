# Ticket 50 變更清單

核心只修改兩處既有判斷並新增回歸測試。表列本票實際檔案；初版失敗、修正前對照與最終回條均保留。實作 933ba4c 已推送且遠端一致，交付回條見 delivery.json。

| 檔案 | 變更摘要 |
|---|---|
| [AGENTS.md](/Users/timlai/Developer/coimnet/AGENTS.md) | 修改：移除已修正 NaN 項目，記錄比較設定共用指標的 P2 後續 |
| [ENG.md](/Users/timlai/Developer/coimnet/ENG.md) | 修改：保存兩個比較入口的數值驗證與相容性契約 |
| [delivery-status.md](/Users/timlai/Developer/coimnet/delivery-status.md) | 修改：更新 ticket 50、驗收與下一個可驗證成果 |
| [docs/requirements-status.json](/Users/timlai/Developer/coimnet/docs/requirements-status.json) | 修改：補充 OPS-03／LRN-08／TSK-12 的驗證證據，狀態保持 |
| [docs/tickets/50-comparison-finite-validation.md](/Users/timlai/Developer/coimnet/docs/tickets/50-comparison-finite-validation.md) | 新增：完整範圍、共用契約、固定五項完成條件 |
| [evidence/OPS-03/comparison-finite-20261009/README.md](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/README.md) | 新增：證據索引、修正說明與重現方式 |
| [evidence/OPS-03/comparison-finite-20261009/artifact-format.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/artifact-format.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/artifact-format.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/artifact-format.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/baseline.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/baseline.json) | 新增：來源、測試或受保護原件的 SHA-256 與比對結果 |
| [evidence/OPS-03/comparison-finite-20261009/build.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/build.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/build.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/build.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/check.py](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/check.py) | 新增：本機命令回條或合法 SDK／CLI 重現程式 |
| [evidence/OPS-03/comparison-finite-20261009/cli-after.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/cli-after.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/cli-after.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/cli-after.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/cli-before-reviewed.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/cli-before-reviewed.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/cli-before-reviewed.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/cli-before-reviewed.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/cli-before.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/cli-before.json) | 新增：保留的初版驗證輸出；最終驗收以 README 指定回條為準 |
| [evidence/OPS-03/comparison-finite-20261009/cli-before.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/cli-before.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/cli-comparison.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/cli-comparison.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/cli-report-after.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/cli-report-after.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/cli-report-before.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/cli-report-before.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/contract-discovery.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/contract-discovery.json) | 新增：共用契約搜尋或入口文件內容指紋 |
| [evidence/OPS-03/comparison-finite-20261009/contract-inventory.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/contract-inventory.json) | 新增：共用契約搜尋或入口文件內容指紋 |
| [evidence/OPS-03/comparison-finite-20261009/delegation.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/delegation.json) | 新增：免費模型核對、逾時與 Luna max 替代派工實際結果 |
| [evidence/OPS-03/comparison-finite-20261009/entry-documents.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/entry-documents.json) | 新增：共用契約搜尋或入口文件內容指紋 |
| [evidence/OPS-03/comparison-finite-20261009/environment.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/environment.json) | 新增：實際 Go、平台與執行環境 |
| [evidence/OPS-03/comparison-finite-20261009/evidence-audit.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/evidence-audit.json) | 新增：Root 再核對命令結果、數量、來源與原件保存 |
| [evidence/OPS-03/comparison-finite-20261009/fixed.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/fixed.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/fixed.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/fixed.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/format.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/format.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/format.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/format.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/general.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/general.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/general.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/general.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/governance-final.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/governance-final.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/governance-final.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/governance-final.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/governance-precommit-reviewed.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/governance-precommit-reviewed.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/governance-precommit-reviewed.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/governance-precommit-reviewed.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/governance-precommit.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/governance-precommit.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/governance-precommit.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/governance-precommit.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/governance-preliminary.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/governance-preliminary.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/governance-preliminary.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/governance-preliminary.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/initial-valid-probe.go.txt](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/initial-valid-probe.go.txt) | 新增：可獨立執行的人工驗證程式或保留的修正前來源 |
| [evidence/OPS-03/comparison-finite-20261009/isolated-baseline-initial-error.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/isolated-baseline-initial-error.json) | 新增：保留 Python 版本不支援參數的初次錯誤 |
| [evidence/OPS-03/comparison-finite-20261009/mod-verify.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/mod-verify.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/mod-verify.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/mod-verify.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/models.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/models.json) | 新增：免費模型核對、逾時與 Luna max 替代派工實際結果 |
| [evidence/OPS-03/comparison-finite-20261009/opencode-result.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/opencode-result.json) | 新增：免費模型核對、逾時與 Luna max 替代派工實際結果 |
| [evidence/OPS-03/comparison-finite-20261009/ownership-followup.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ownership-followup.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/ownership-followup.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ownership-followup.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/ownership-probe.go.txt](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ownership-probe.go.txt) | 新增：可獨立執行的人工驗證程式或保留的修正前來源 |
| [evidence/OPS-03/comparison-finite-20261009/ownership-ready.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ownership-ready.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/ownership-ready.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ownership-ready.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/preservation.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/preservation.json) | 新增：來源、測試或受保護原件的 SHA-256 與比對結果 |
| [evidence/OPS-03/comparison-finite-20261009/priority-after.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/priority-after.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/priority-after.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/priority-after.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/priority-before.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/priority-before.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/priority-before.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/priority-before.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/priority-probe.go.txt](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/priority-probe.go.txt) | 新增：可獨立執行的人工驗證程式或保留的修正前來源 |
| [evidence/OPS-03/comparison-finite-20261009/priority-ready.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/priority-ready.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/priority-ready.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/priority-ready.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/probe-correction.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/probe-correction.json) | 新增：初版驗證程式修正與最終證據指定 |
| [evidence/OPS-03/comparison-finite-20261009/race-progress.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/race-progress.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/race.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/race.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/race.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/race.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/ready-after-a.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ready-after-a.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/ready-after-a.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ready-after-a.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/ready-after-b.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ready-after-b.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/ready-after-b.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ready-after-b.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/ready-baseline.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ready-baseline.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/ready-before.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ready-before.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/ready-before.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ready-before.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/ready-comparison.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/ready-comparison.json) | 新增：完整公開報告、既有錯誤或另票設定共用問題的實際證據 |
| [evidence/OPS-03/comparison-finite-20261009/red-reviewed.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/red-reviewed.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/red-reviewed.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/red-reviewed.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/red.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/red.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/red.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/red.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/root-review.md](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/root-review.md) | 新增：Root 的完整實作與契約審查 |
| [evidence/OPS-03/comparison-finite-20261009/scoped.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/scoped.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/scoped.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/scoped.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/sdk-comparison.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/sdk-comparison.json) | 新增：保留的初版驗證輸出；最終驗收以 README 指定回條為準 |
| [evidence/OPS-03/comparison-finite-20261009/sdk-isolated.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/sdk-isolated.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/sdk-isolated.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/sdk-isolated.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/sdk-ready.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/sdk-ready.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/sdk-ready.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/sdk-ready.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/source-freeze.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/source-freeze.json) | 新增：來源、測試或受保護原件的 SHA-256 與比對結果 |
| [evidence/OPS-03/comparison-finite-20261009/test-freeze.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/test-freeze.json) | 新增：來源、測試或受保護原件的 SHA-256 與比對結果 |
| [evidence/OPS-03/comparison-finite-20261009/tidy.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/tidy.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/tidy.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/tidy.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/valid-after-a.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-after-a.json) | 新增：保留的初版驗證輸出；最終驗收以 README 指定回條為準 |
| [evidence/OPS-03/comparison-finite-20261009/valid-after-a.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-after-a.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/valid-after-b.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-after-b.json) | 新增：保留的初版驗證輸出；最終驗收以 README 指定回條為準 |
| [evidence/OPS-03/comparison-finite-20261009/valid-after-b.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-after-b.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/valid-before.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-before.json) | 新增：保留的初版驗證輸出；最終驗收以 README 指定回條為準 |
| [evidence/OPS-03/comparison-finite-20261009/valid-before.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-before.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/valid-isolated-after-a.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-isolated-after-a.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/valid-isolated-after-a.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-isolated-after-a.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/valid-isolated-after-b.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-isolated-after-b.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/valid-isolated-after-b.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-isolated-after-b.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/valid-isolated-baseline.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-isolated-baseline.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/valid-isolated-before.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-isolated-before.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/valid-isolated-before.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-isolated-before.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/valid-isolated-comparison.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-isolated-comparison.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/valid-probe-before-format.go.txt](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-probe-before-format.go.txt) | 新增：可獨立執行的人工驗證程式或保留的修正前來源 |
| [evidence/OPS-03/comparison-finite-20261009/valid-probe.go.txt](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/valid-probe.go.txt) | 新增：可獨立執行的人工驗證程式或保留的修正前來源 |
| [evidence/OPS-03/comparison-finite-20261009/verification.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/verification.json) | 新增：工程驗收、命令、環境、輸入指紋及科學界線 |
| [evidence/OPS-03/comparison-finite-20261009/verify-cli.py](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/verify-cli.py) | 新增：本機命令回條或合法 SDK／CLI 重現程式 |
| [evidence/OPS-03/comparison-finite-20261009/verify-sdk.py](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/verify-sdk.py) | 新增：本機命令回條或合法 SDK／CLI 重現程式 |
| [evidence/OPS-03/comparison-finite-20261009/vet.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/vet.json) | 新增：對應命令、觀察結果、退出碼與精確輸出 |
| [evidence/OPS-03/comparison-finite-20261009/vet.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/vet.log) | 新增：對應命令的文字輸出；精確原文保存在同名 JSON |
| [evidence/OPS-03/comparison-finite-20261009/worker-prompt.txt](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/worker-prompt.txt) | 新增：指定檔案責任、公開契約與驗收的派工原文 |
| [experiment/attribution.go](/Users/timlai/Developer/coimnet/experiment/attribution.go) | 修改：歸因比較只接受嚴格介於 0 與 1 的數值 |
| [experiment/comparison_finite_test.go](/Users/timlai/Developer/coimnet/experiment/comparison_finite_test.go) | 新增：先失敗回歸、14 組非法設定與 10 個合法控制 |
| [experiment/continual.go](/Users/timlai/Developer/coimnet/experiment/continual.go) | 修改：連續任務比較只接受嚴格介於 0 與 1 的數值 |
| [evidence/OPS-03/comparison-finite-20261009/inventory.md](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/inventory.md) | 新增：逐檔變更清單 |
| [evidence/OPS-03/comparison-finite-20261009/precommit-review.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/precommit-review.json) | 新增：完整變更內容的提交前指紋 |
| [evidence/OPS-03/comparison-finite-20261009/delivery.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/delivery.json) | 新增：實際提交、推送與遠端讀回證據 |
| [evidence/OPS-03/comparison-finite-20261009/delivery-governance.json](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/delivery-governance.json) | 新增：完成交付紀錄後的八個治理／索引檢查回條 |
| [evidence/OPS-03/comparison-finite-20261009/delivery-governance.log](/Users/timlai/Developer/coimnet/evidence/OPS-03/comparison-finite-20261009/delivery-governance.log) | 新增：交付紀錄治理檢查的文字輸出 |
