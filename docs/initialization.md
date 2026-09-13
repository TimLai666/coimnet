# 工作區初始化紀錄

日期：2026-09-13。範圍：建立本機開發入口、保存交接包、鎖定指定依賴、建立需求追蹤。這次初始化不是 W01 全部完成，也不是框架功能驗收。

本頁保存初始化當時的紀錄。後續實作、驗證與下一項工作請看 [開發進度](../delivery-status.md)。

## 採用的設定

| 項目 | 設定與依據 |
| --- | --- |
| 工作區 | `/Users/timlai/Developer/coimnet`，既有 Git 儲存庫 |
| Go module | `github.com/TimLai666/coimnet`，依既有 origin URL |
| Go 最低版本 | `1.25.12`，依指定 Insyra 提交的 module metadata |
| Insyra | `v0.3.2`，Go 查詢確認對應 `1f1cdb949c51a10be104965cf4cf30f8f0aaa648` |
| 本機環境 | `go1.26.5 darwin/arm64`，`CGO_ENABLED=1` |
| 授權檔 | 保留既有 MIT `LICENSE`，沒有更改授權文字 |
| 交接文件 | `docs/handoff/` 保留原始 8 個檔案，日期為 2026-09-11 |
| 需求追蹤 | `requirements-status.json` 收錄 85 個需求編號，初始全部為 `specified` |

初始化時 `go.mod` 與 `go.sum` 先鎖定指定 Insyra 版本，當時尚無 Go 功能套件。後續整合已由 `go mod tidy` 補齊實際使用的傳遞依賴，完整驗證見 [CPU 參考日誌](../evidence/cpu-reference-20260913/macos-v2/validation.log)。

## 本輪查證與重現

在儲存庫根目錄執行依賴查詢：

```sh
go version
go env GOVERSION GOOS GOARCH CGO_ENABLED
go list -m -json github.com/HazelnutParadise/insyra@1f1cdb949c51a10be104965cf4cf30f8f0aaa648
go mod download -json github.com/HazelnutParadise/insyra@v0.3.2
```

實際查詢取得 `v0.3.2`、上述完整提交、`go 1.25.12` 與兩個 module 校驗值。下載結果只證明套件已取得，沒有證明 Insyra 編譯、梯度整合或模型訓練成功。

在 `docs/handoff/` 執行：

```sh
shasum -a 256 -c SHA256SUMS.txt
```

來源包的 7 個列管檔案皆通過 SHA-256 校驗。`SHA256SUMS.txt` 本身為第 8 個檔案，複製後與來源逐位元組比對。

## 接續實作

完整目標依主規格第 20 章的 W01–W14 與 85 項需求推進。目前 CPU 核心、Insyra 完整梯度接合與獨立序列續訓已有實作，資料取得與匯入依 [近期 tickets](tickets/) 接續。

`requirements-status.json` 的 `definition` 指向原始需求檔，更新每個編號的狀態與證據即可，不另抄一套驗收條件。合法狀態沿用原始需求檔的 `allowed_statuses`。沒有執行的測試不可標成通過。

減法審查：功能套件隨實作建立，不預建整棵空目錄、不增加假 CLI。初始化不下載全腦資料或啟動訓練，以免把環境準備擴大成完整研究工作。
