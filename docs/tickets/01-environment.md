# 01 — 開發者可以核對環境與 Insyra 運算能力

Epic：環境與依賴

User Story：開發者可以核對環境與 Insyra 運算能力

Blocked by：無

Status：done（CPU 參考階段）

## 交付

編譯指定 Insyra 並提供實際 API/梯度能力證據，命令顯示可用與缺少能力。

## 驗收

- [x] `cli.Doctor(context.Context)` 從真實環境與 build info 取值，未知不當成零或支援。
- [x] Insyra 使用公開 `nn` API 完成非假輸出的運算探測，失敗保留原錯誤。
- [x] 命令 help、JSON schema、未知命令與取消測試。

## 本階段證據

- `GOCACHE=/private/tmp/coimnet-doctor-gocache go test doctor.go doctor_gpu.go doctor_memory_darwin.go doctor_test.go`：通過。
- 整合後的完整建置、測試、race 與 vet 已在 macOS 通過，日誌見 [CPU 參考驗證](../../evidence/cpu-reference-20260913/macos/validation.log)。
- Doctor probe 實際執行 `nn.NewTensor`、`Tape.Param`、`Tape.MatMul`、`Tape.MSELoss`、`Tape.Backward`、`Tape.Grad`，輸出 `[23]`、loss `[484]`、權重梯度 `[88 132]`。
- JSON 根欄位為 `schema_version: coimnet-doctor/v1`。GPU 硬體探測與 CoImNet core backend 能力分開報告；Insyra 矩陣能力為 `status=available_in_dependency`、`execution=not_probed`，不宣稱為 sparse recurrent GPU。
- Insyra 自訂 tape/VJP 與 optimizer state 序列化缺口追蹤於 [#375](https://github.com/HazelnutParadise/insyra/issues/375) 與 [#376](https://github.com/HazelnutParadise/insyra/issues/376)。目前 bridge 使用 `Reshape+MatMul` 內積，核心反向維持明示向量梯度。

`doctor` JSON 已由真實 CLI 執行並重新解析。Mac 記憶體使用 `unix.SysctlUint64`，另以系統 `sysctl -n hw.memsize` 比對，修正將二進位值當字串導致截斷的問題。GPU 後端與 Windows 實機驗收依完整需求另外追蹤。
