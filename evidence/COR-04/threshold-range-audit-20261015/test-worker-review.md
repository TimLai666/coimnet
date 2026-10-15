# LIF threshold-range red test review

## Scope

新增的永久回歸測試只使用公開 LIF／Mixed 建構、前向、反向、重算與持續狀態入口。耦合測試沿用既有的非 nil 中性 sequence helper：`dynamics/lif_coupled_test.go:14,182` 與 `dynamics/mixed_coupled_test.go:14,174`。

極端 fixture 在 `dynamics/lif_threshold_range_test.go:10-26` 宣告 `a = 2^1023`、`ThetaMin=-a`、`ThetaMax=a`、`VReset=-MaxFloat64`、`ThetaRaw=0`、`DT=ln(2)`、`TauSyn=1`，並使用一個零邊節點。獨立二進位手算在 `dynamics/lif_threshold_range_test.go:67-72`，第二列電位位元核對在 `dynamics/lif_threshold_range_test.go:87-93`：

- `theta(0) = (-2^1023 + 2^1023) / 2 = 0`。
- `dtheta/draw = (2^1023 - (-2^1023)) / 4 = 2^1022`。
- `u = 0` 時 fast-sigmoid 替代導數為 1，因此一列上游輸出 1 的 `theta_raw` 梯度為 `-2^1022`。
- `lambda = alpha = 1/2`，所以第一列 spike/output 為 1，第二列輸出為 1/2；輸出列均依公開 `Outputs`／`Spikes` 的「每個輸入步驟、不含 prehistory」約定核對。
- 第二列電壓獨立使用 binary64 位元 `0xffdfffffffffffff`，即 `-0x1.fffffffffffffp+1022 = -MaxFloat64 * 1/2`，不是 `-2^1023`。

## 覆蓋項目

| 項目 | 測試位置 | 獨立核對 |
| --- | --- | --- |
| 普通 LIF Forward／Backward | `dynamics/lif_threshold_range_test.go:74-101` | 兩列輸出、spike、獨立 binary64 reset 後電位及 `-2^1022` 閾值梯度 |
| 非 nil 中性 ModulationSequence 耦合 Forward／Backward | `dynamics/lif_threshold_range_test.go:103-127` | 中性 gain/offset/threshold、輸出、spike、閾值梯度與 reverse call |
| ForwardRecompute／BackwardRecompute | `dynamics/lif_threshold_range_test.go:129-148` | segment=1 下兩列輸出及同一閾值梯度 |
| NewState／連續兩次 Advance | `dynamics/lif_threshold_range_test.go:150-181` | 第一列 1、第二列 1/2，步數 1→2，最後 ValidateState |
| 全 LIF Mixed Forward／Backward | `dynamics/lif_threshold_range_test.go:183-208` | 單 LIF 節點輸出、spike、閾值梯度 |
| 含一個連續節點的 Mixed Forward／Backward | `dynamics/lif_threshold_range_test.go:210-235` | tanh 連續輸出 0 與 LIF 輸出 1，僅對 LIF output 給上游 1 |
| Mixed 非 nil 中性耦合 Forward／Backward | `dynamics/lif_threshold_range_test.go:237-268` | 中性耦合輸出、spike、閾值梯度與 reverse call |
| `ThetaRaw=±1000` 飽和端點 | `dynamics/lif_threshold_range_test.go:270-326` | 小範圍 `[-2,2]` 下端點 spike/reset 與正常 `[0,2]` 控制，依手算閾值梯度 |
| 巨大溢位跨度的 `ThetaRaw=±1000` | `dynamics/lif_threshold_range_test.go:328-366` | `exp(-1000)=0` 使上下端分別為 `-a/+a`，輸出／spike／電位及零梯度；舊 span 來源在 Forward 失敗 |
| 非法 bounds 與不可表示輸出 | `dynamics/lif_threshold_range_test.go:368-441` | LIF／Mixed 非法 bounds 拒絕；輸入與 bias 相加成 `+Inf` 時 Forward／Recompute／Advance 回錯且不發布部分結果或改變 state，before 使用深複本 |

目前正式程式中，對應的五個閾值變換位置仍先計算 `ThetaMax-ThetaMin`：普通 LIF `dynamics/lif.go:325-332`、耦合 LIF `dynamics/lif_coupled.go:91-99`、分段重算共用 `dynamics/lif_segment.go:91-99`、持續狀態 `dynamics/lif_state.go:377-383`，以及 Mixed 共用 `dynamics/mixed.go:325-340`（Mixed 耦合先呼叫同一 `baseThreshold`，`dynamics/mixed_coupled.go:65-67`）。這是測試所針對的候選根因，不把候選當成已修正結論。

## 實際驗證

使用指定的 red 指令：

```text
GOCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/lif-threshold-range-20261015/go-cache GOMODCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/lif-threshold-range-20261015/go-mod-cache GOTMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/lif-threshold-range-20261015/go-tmp TMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/lif-threshold-range-20261015/go-tmp GOPROXY=file:///Users/timlai/go/pkg/mod/cache/download GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=2 go test -p=2 -count=1 ./dynamics -run '^TestThresholdRange'
```

結果：命令以 exit 1 結束，失敗集中在預期的極端合法範圍案例，共 9 個子案例：普通 LIF、耦合 LIF、分段重算、持續狀態、全 LIF Mixed、含連續節點 Mixed、Mixed 耦合，以及巨大溢位跨度的 raw=-1000／raw=+1000。每個失敗均發生在正式入口回報 `theta_base[0] is not representable`，不是測試編譯錯誤或 initial/input 形狀錯誤。小範圍飽和、正常範圍、非法 bounds、不可表示輸出控制均未列為失敗。

## 尚未驗證

- 尚未修改正式程式，因此極端合法範圍尚未有綠結果。
- 尚未執行完整 suite、race、vet、build 或訓練流程，符合本輪 red 階段限制。
- 尚未以本案例推論任何既有 OCR 或其他模型的實際影響。
