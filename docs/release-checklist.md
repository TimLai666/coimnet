# 發布前確認清單（主規格 22.4）

狀態：blocked_permission（等使用者選定程式授權與發布授權）

| 文件 | 由誰確認 |
|---|---|
| [ ] 程式授權已由使用者選定（規格建議 MIT，但不代替使用者做法律選擇） | 使用者 |
| [ ] 遠端儲存庫與發布授權已取得 | 使用者 |
| [ ] 不含金鑰（參照 scripts/check-target-flow.sh 的唯讀稽核風格，與未來的 scan-commit.sh） | |
| [ ] 不含使用者資料與大型來源檔（`.gitignore` 排除 `/data/`、`/models/`、`/checkpoints/`、`/runs/`、`/.env`、`/.env.*`、`/bin/`、`/dist/`、`/coverage.out`、`*.test`；`data/` 不進 git） | |
| [ ] 不含未授權模型 | |
| [ ] 不宣稱未完成的能力（依 docs/requirements-status.json 的 passed 清單陳述；GOV-05 目前為 specified） | |
| [ ] 相依授權盤點已更新（docs/licenses.md 的產生日期為本次提交前） | |