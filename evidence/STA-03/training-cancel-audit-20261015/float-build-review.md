# Full-window cancellation：跨建置浮點差異審查

## Problem

`evidence/STA-03/training-cancel-audit-20261015/child-resume-environment-diagnostic.json` 在 LIF 案例的 `Adam second[7]` 觀察到普通建置 `9.812042275199087e-06`、`-race` 建置 `9.812042275199089e-06`。兩個 `GOMAXPROCS` 設定都重現同一差異，但同一建置內的恢復結果仍精確一致。本審查只判斷數值差異是否能由目前來源與實際產生的機器碼確認，不能把 FMA 當成未證實的根因。

基準是 `2ff6864f854cd3129ac0ac17065bdf54f73047c3`。兩個 binary 都依 `source-before.json` 的 810 份來源建置，來源清單包含 808 份 Go 與 `go.mod`、`go.sum`。

## Done

已唯讀核對 `learning/trainer.go` 的 Adam 更新式、Go 版本與環境、兩個既有 Mach-O binary 的符號及更新區段，並對照 same-build 恢復證據。本輪沒有重新建置、執行 Go 測試、修改探針、使用額外編譯快取或修改正式程式。

## CONFIRMED

### 1. 差異是可重現的跨建置觀察，不是取消後同一建置的保存／載入失敗

- 普通建置在 `GOMAXPROCS` 未設定及 `GOMAXPROCS=2` 各重複三次，JSON 都相同。
- `-race` 建置在兩種 `GOMAXPROCS` 設定各重複三次，均只有 `.snapshot.optimizer.state.second[7]` 差異：
  - 普通建置基準：`9.812042275199087e-06`
  - `-race` 建置：`9.812042275199089e-06`
- `child-resume-same-build-diagnostic.json` 顯示兩邊各自的 `second_expected` 與 `second_resumed` 完全相同：普通建置都是 `9.812042275199087e-06`，`-race` 建置都是 `9.812042275199089e-06`。
- `resume-check-correction.json` 已指出先前失敗是把普通建置的 snapshot／control 拿來比對 race binary，屬跨建置比較；同一建置的新程序恢復檢查仍通過。

因此目前有證據支持「建置模式改變了該次成功更新產生的 float64 值」，沒有證據支持「取消後已保存的 state 被同一建置的載入流程改壞」。跨建置載入的專門相容性測試仍未做。

### 2. Adam second 的來源更新式確實沒有額外的中間精度邊界

`/Users/timlai/Developer/coimnet/learning/trainer.go:529-535` 位於 `(*Trainer).stepWithGradient`（函式起點 `/Users/timlai/Developer/coimnet/learning/trainer.go:395`）：

```go
d := mean[i] * scale
state.First[i] = tr.options.Beta1*state.First[i] + (1-tr.options.Beta1)*d
state.Second[i] = tr.options.Beta2*state.Second[i] + (1-tr.options.Beta2)*d*d
m := state.First[i] / (1 - math.Pow(tr.options.Beta1, float64(state.Steps[i])))
vhat := state.Second[i] / (1 - math.Pow(tr.options.Beta2, float64(state.Steps[i])))
p[i] = v*(1-rate*tr.options.WeightDecay) - rate*m/(math.Sqrt(vhat)+tr.options.Epsilon)
```

`d`、`Beta2`、`state.Second[i]` 及相關中間值走 float64；這一行沒有明確要求先把 `(1-Beta2)*d`、`d*d` 或乘加結果各自四捨五入後再進入下一個運算。Go 規格允許實作把多個浮點運算合併成 fused operation，結果可不同於逐項取整後的結果，除非程式以明確轉換等方式建立需要的精度邊界。[Go 語言規格：Floating-point operators](https://go.dev/ref/spec#Floating-point_operators)

### 3. 實際機器碼在兩個 binary 都使用同樣形狀的 FMA，但這不能確認 FMA 是差異根因

`go tool objdump` 在目前 `/usr/local/go` 安裝中不可用，實際回覆為 `go: no such tool "objdump"`。因此只以 `/usr/bin/objdump`（Apple LLVM 21.0.0）唯讀反組譯，沒有 Go source-line mapping，以下是依 symbol range 與資料流對照來源式的機器碼位址，不是宣稱的 Go 行號。

普通 binary：`runs/.tmp/training-cancel-full-window-20261015/full-window-cancel`，SHA-256 `1317bebf4cd2a15ef0e9447a2d6ceb42aa389d0b17b10c22f845d24f8dbebaab`。`learning.(*Trainer).stepWithGradient` symbol 從 `0x1005c2360` 到下一個 symbol `0x1005c45f0` 前。其 second 更新區段為：

```text
0x1005c34e4  ldr   d5, [x3,#0xc0]
0x1005c34e8  ldr   d6, [x11,x6,lsl#3]
0x1005c34ec  fsub  d8, d7, d5
0x1005c34f0  fmul  d8, d8, d3
0x1005c34f4  fmul  d3, d8, d3
0x1005c34f8  fmadd d3, d5, d6, d3
0x1005c34fc  str   d3, [x11,x6,lsl#3]
```

`-race` binary：`runs/.tmp/training-cancel-full-window-20261015/full-window-cancel-race`，SHA-256 `d220aef7afa664a94a4b8d53e402fcb562f2b1df918d1109c81793e12199eff0`。`learning.(*Trainer).stepWithGradient` symbol 從 `0x10096f290` 到下一個 symbol `0x1009727f0` 前。其對應區段為：

```text
0x100971318  ldr   d0, [x3,#0xc0]
0x10097131c  fmov  d1, #1.0
0x100971320  fsub  d0, d1, d0
0x100971324  ldr   d1, [sp,#0x3d0]
0x100971328  fmul  d0, d0, d1
0x10097132c  fmul  d0, d0, d1
0x100971330  ldr   d1, [sp,#0x430]
0x100971334  ldr   d2, [sp,#0x438]
0x100971338  fmadd d0, d2, d1, d0
0x100971348  str   d0, [sp,#0x438]
0x100971354  bl    runtime.racewrite
0x100971360  ldr   d0, [sp,#0x438]
0x100971364  str   d0, [x2,x1,lsl#3]
```

兩個區段都可見兩次 `fmul` 形成 `(1-Beta2)*d*d`，再以 `fmadd` 形成 `Beta2*oldSecond + product`。race 版本另有 stack spill 及 `runtime.raceread`／`runtime.racewrite` 周邊呼叫，證明 `-race` 造成不同的程式碼與記憶體存取儀器化；Go 的 race detector 文件也明確說明 `-race` 會建立 race-enabled binary 並帶來額外執行負擔。[Go Data Race Detector](https://go.dev/doc/articles/race_detector)

所以目前能確認的是「FMA 在兩個版本的這段機器碼都存在，且 race 版有額外儀器化與暫存器配置差異」。不能由這段 assembly 確認「FMA 只在其中一個版本啟用」或「某個已知的重排正是 `second[7]` 差異來源」。差異也可能在較早的 `d`／gradient 計算、編譯器因 `-race` 改變的 spill／排程或其他未映射的運算路徑；本輪沒有中間值證據。

### 4. 工具鏈與建置旗標事實已固定

- `go version`：`go version go1.26.5 darwin/arm64`
- `go env GOROOT`：`/usr/local/go`
- `GOOS=darwin`、`GOARCH=arm64`、`CGO_ENABLED=1`
- 普通建置是 `go build -o .../full-window-cancel .../full-window-cancel.go`。
- race 建置是 `go build -race -o .../full-window-cancel-race .../full-window-cancel.go`。
- 兩份建置 metadata 都記錄 `formal_sources_checked: 810`，且正式來源未修改。

## NEEDS INVESTIGATION

### Missing fact：差異最早在哪個中間值出現

目前只有最終 `state.Second[7]` 的跨建置差異。沒有普通／race 兩邊的 `mean[7]`、`scale`、`d`、`Beta2`、`oldSecond`、`(1-Beta2)*d*d` 及最後乘加各自的 bit pattern，因此不能區分：

1. `d` 或 gradient 在 Adam 前已經不同。
2. Adam second 的兩次乘法或乘加取整方式不同。
3. 只有 race 的 spill／儀器化造成後續浮點路徑差異。

最小查證是另開一次只讀來源的診斷版本，對 parameter 7 在兩個建置各記錄上述中間值及 `math.Float64bits`，找出第一個不同的值，再用獨立的逐步取整參考計算判斷是否與 fused operation 相符。這不應在本輪修改正式來源、探針或測試。

### Missing fact：專案是否要求普通與 `-race` 跨建置 bit-for-bit 相同

目前找到的 same-build cancellation／resume 證據要求同一建置內 snapshot、report 及恢復結果一致；沒有看到本輪證據把普通與 `-race` 的 optimizer 浮點狀態定義為跨旗標 bit-for-bit 契約。若專案契約只要求同建置可恢復，這個一 ULP 跨建置差異不能單獨判為取消 bug。若另有跨建置重現性契約，需先引用該契約再設計驗證。

### Missing fact：對正常模型品質的長期影響

`second[7]` 的差異非常小，但 Adam 後續步驟會讀取它，理論上可能使長期參數軌跡不同。本輪沒有 loss、參數軌跡、收斂或模型指標的普通／race 對照，不能宣稱品質無影響，也不能宣稱已造成品質退化。

### Tooling limitation：沒有 Go objdump 的 source-line 證據

目前 Go 安裝缺少 `go tool objdump`，Apple LLVM fallback 也沒有把這兩個 binary 的指令標回 `trainer.go` 行號。若需要編譯器層級根因，應在不改正式來源的條件下取得同一 Go toolchain 的 compiler／linker diagnostics 或可提供 DWARF 行號的反組譯結果；本輪不自行重建 binary。

## 影響判定

- **保存／載入：** 已確認普通與 race 各自的 same-build resume 完全一致。沒有確認取消流程遺失 Adam、模型參數或累積窗。跨建置產生的 snapshot 可能自然帶著各自計算出的 bit pattern，但跨建置載入相容性未單獨驗證。
- **取消提交：** 本差異出現在成功更新的數值觀察，不能由它推導取消後有部分 commit。既有 correction 已把跨建置 control 誤比對分開，取消後同建置恢復仍 exact。
- **正常模型品質：** 影響未量測。現有證據只支持一個 Adam second float64 值的一 ULP 跨建置差異，不足以判定品質問題。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `/Users/timlai/Developer/coimnet/runs/.tmp/training-cancel-full-window-20261015/float-build-review.md` | 新增本次跨建置浮點與 assembly 審查報告。 |

## Actions

| 動作 | 對象 | 結果 |
| --- | --- | --- |
| 唯讀讀取 | 既有 evidence、來源、binary | 完成，未修改。 |
| `go version`／`go env GOROOT` | 本機 Go 1.26.5 | 完成。 |
| `go tool objdump` | 兩個既有 binary | 工具不可用：`go: no such tool "objdump"`，未重建。 |
| `/usr/bin/objdump` fallback | 兩個既有 binary | 完成唯讀反組譯，未修改 binary。 |

## Verified

- 已核對 `/Users/timlai/Developer/coimnet/learning/trainer.go:529-535` 的實際更新式。
- 已核對兩個 binary 的 `stepWithGradient` symbol range、Adam second 區段及 `fmadd`／race instrumentation 指令。
- 已核對 ordinary／race build metadata、Go 版本、source manifest 810 份及 binary SHA-256。
- 已核對 `child-resume-environment-diagnostic.json` 與 `child-resume-same-build-diagnostic.json`，分清跨建置差異與 same-build exact resume。
- 本輪未執行 Go build、Go test、race test 或任何探針；未使用額外編譯 cache。

## Notes

`fmadd` 是目前合理的語言契約候選，但不是已確認根因。實際 assembly 反而顯示兩個建置都採用了相同的 second 更新 FMA 形狀，所以不能用「race 版才做 FMA」解釋觀察。若沒有更強的跨建置 bit-for-bit 契約，Go 規格允許的浮點融合及 `-race` 造成的程式碼差異足以使這個 observation 保持 NEEDS INVESTIGATION，而不能升格為 confirmed bug。

## Next

1. 先確認專案是否要求普通／`-race` 跨旗標 bit-for-bit optimizer state 相同。
2. 若需要定位，做不改正式程式的最小中間值診斷，從 `d`、old second、Beta2、乘積到 commit value 比對兩個 binary 的第一個分歧點。
3. 若第一個分歧確在 Adam 乘加，再以獨立逐步取整參考計算與可取得的 compiler diagnostics 驗證 FMA／重排；在證據完成前不改更新式。

Skills：software-engineering-guidelines、investigate、human-writing、project-memory
