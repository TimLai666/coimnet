# LIF float arithmetic review

日期：2026-10-15

## Problem

`float-probe-v2` 的第一個共同初始案例，在一般建置與 `-race` 建置的公開 core reverse 結果出現一個 ULP 差異：`g.Core.Inputs[4][2]` 是一般 `3fd3de0b7fb2bee0`、race `3fd3de0b7fb2bee1`，公開累加後的 `g.Core.Bias[2]` 是一般 `3fee65bc866308b6`、race `3fee65bc866308b7`。現有證據只把差異定位到公開 core input gradient，尚未定位到最早的機器運算。

本文件固定相同初始案例，整理 `LIF` 的 forward/backward 資料流、可重現的編碼與上游 f32 計算，以及可區分逐項取整和 FMA 的獨立參考。所有數字都是記憶體內的純量重算；沒有改正式方法，也沒有執行 Go build/test。

## 固定案例與精度邊界

案例直接取自 `evidence/OPS-05/gpu-cancel-audit-20261015/float-probe-v2.go.txt:27-46`：

```text
nodes=3
sources=[0,1,1]  targets=[1,1,2]  delays=[0,1,0]
dt=1  tau_syn=1  theta_min=0.05  theta_max=1  v_reset=-0.5
refractory_steps=1  surrogate=fast_sigmoid(scale=2)
weights=[0.65,0.25,0.7]  bias=[0,0,0]  log_tau=[0,0,0]
theta_raw=[-1,-1,-1]  encoder=[1,0,0]  readout=[1]
input=[[.7],[.8],[.7],[.8],[.7],[.8]]  target=[.35]
```

`learning.tensor` 將輸入與 encoder 從 float64 驗證後轉成 float32，再由 `doubles` 轉回 float64 core input，見 `learning/network.go:685-711`。因此 encoder 輸出的兩個非零值是：

```text
.7 -> float32 0.69999998807907104  bits32=3f333333
     widened float64 0.69999998807907104 bits64=3fe6666660000000
.8 -> float32 0.80000001192092896  bits32=3f4ccccd
     widened float64 0.80000001192092896 bits64=3fe99999a0000000
```

這個 encoder MatMul 是 `6*1*3=18` 個 f32 MAC，readout 是 `1*1*1=1` 個 f32 MAC。Insyra 的 device floor 是 `16_777_216` MAC（`nn/device_matmul.go:10-16`），所以這個案例走 CPU `matMul2DWithWorkers`，內層是 `float32 sum += a*b`（`nn/kernels.go:1078-1117`）。LIF core 接到的 `coreInputs` 是上述 widened float64，不是原始 `.7/.8` 的二進位 float64。

`learning.forwardSegmentState` 的順序是輸入 tensor、encoder tensor、encoder MatMul、`rows(doubles(z.Data()))`、LIF forward、選最後一列 readout，再建立 f32 readout MatMul，見 `learning/network.go:561-582`、`621-663`。`lifCore.forward` 只呼叫 `LIF.Forward` 並回傳 synaptic trace，沒有額外轉換或算術（`learning/core.go:183-195`）。

## forwardReader 與延遲取樣

`forwardReader` 的宣告順序是 `output, sources, delays, t, history, startSteps`（`dynamics/continuous.go:158-166`）。無 history 時，`at` 先以 `past=0` 建立 prehistory，再於 `delay < t` 時用 `past=t-delay`，回傳 `output[past][source]`（`dynamics/continuous.go:168-186`）。LIF 每一步在 `dynamics/lif.go:363-370` 以同一個 reader 呼叫 `accumulateSerial`；edge 順序固定是 `e0,e1,e2`（`dynamics/continuous.go:208-214`）。

本案例的 `past` 索引如下，列的順序是 `[e0,e1,e2]`：

```text
t=0 [0,0,0]   t=1 [1,0,1]   t=2 [2,1,2]
t=3 [3,2,3]   t=4 [4,3,4]   t=5 [5,4,5]
```

## 可重現的 forward trace

以下表格採用一個「端點吻合」的 FMA 候選 `R2`，只為了讓中間 trace 有一組可核對的數值：

1. `theta_base = fma(span, s, ThetaMin)`。
2. `synapse = fma(kappa, syn_before, spike)`。
3. `cand = fma(alpha, drive, lambda*voltage)`，其中 `lambda*voltage` 先各自取整。

這是候選參考，不是對 Go 編譯器實際指令的宣告。`R2` 的 `g.Core.Inputs[4][2]` 與 `g.Core.Bias[2]` bits 恰好等於一般建置的公開結果；下節會把 `R0/R1/R2` 分開比較。

在這組設定中，獨立重算的常數為：

```text
kappa=lambda = 0.36787944117144233  bits64=3fd78b56362cef38
alpha        = 0.63212055882855770  bits64=3fe43a54e4e98864
theta(R2)    = 0.30549435030149535  bits64=3fd38d382cea1799
theta slope  = 0.18678133657940774  bits64=3fc7e8736a0e4cff
```

陣列元素順序都是 node `[0,1,2]`。`spike`、`voltage`、`syn`、`refract` 是 step 結束後的值；`drive`、`cand`、`u`、`dspike`、`reset` 是該 step 的 trace。

```text
t=0
 drive  [0.69999998807907104, 0, 0]
 cand   [0.44248438364452608, 0, 0]
 u      [0.13699003334303073, -0.30549435030149535, -0.30549435030149535]
 dspike [0.61613337466698415, 0.38531419733924532, 0.38531419733924532]
 spike  [1, 0, 0]  reset [0, 1, 1]
 voltage[-0.5, 0, 0]  syn [1, 0, 0]  refract [1, 0, 0]

t=1
 drive  [0.80000001192092896, 0.65000000000000002, 0]
 cand   [0.32175673401258925, 0.41087836323856247, 0]
 u      [0.016262383711093897, 0.10538401293706712, -0.30549435030149535]
 dspike [0, 0.68214721902723052, 0.38531419733924532]
 spike  [0, 1, 0]  reset [0, 0, 1]
 voltage[-0.5, -0.5, 0]  syn [0.36787944117144233, 1, 0]  refract [0, 1, 0]

t=2
 drive  [0.69999998807907104, 0.23912163676143752, 0.69999999999999996]
 cand   [0.25854466305880491, -0.032786017928081904, 0.44248439117999033]
 u      [-0.046949687242690441, -0.33828036822957724, 0.13699004087849498]
 dspike [0.83569008976755343, 0, 0.61613336008951602]
 spike  [0, 0, 1]  reset [1, 0, 0]
 voltage[0.25854466305880490, -0.5, -0.5]  syn [0.13533528323661270, 0.36787944117144233, 1]  refract [0, 0, 1]

t=3
 drive  [0.80000001192092896, 0.33796793410379827, 0.25751560882000962]
 cand   [0.60080972076224237, 0.029696758786104949, -0.021158810031340430]
 u      [0.29531537046074702, -0.27579759151539041, -0.32665316033283576]
 dspike [0.39524033346707022, 0.41537767842326973, 0]
 spike  [1, 0, 0]  reset [0, 1, 0]
 voltage[-0.5, 0.029696758786104950, -0.5]  syn [1.0497870683678638, 0.13533528323661270, 0.36787944117144233]  refract [1, 0, 0]

t=4
 drive  [0.69999998807907104, 0.77433145473197207, 0.094734698265628886]
 cand   [0.25854466305880491, 0.50039565891053961, -0.12405597017759705]
 u      [-0.046949687242690441, 0.19490130860904425, -0.42955032047909242]
 dspike [0, 0.51771857797624921, 0.28933048750689067]
 spike  [0, 1, 0]  reset [0, 0, 1]
 voltage[-0.5, -0.5, -0.12405597017759705]  syn [0.38619508006017650, 1.0497870683678638, 0.13533528323661270]  refract [0, 1, 0]

t=5
 drive  [0.80000001192092896, 0.28486062284826796, 0.73485094785750460]
 cand   [0.32175673401258925, -0.0038734644826230197, 0.41887675083246556]
 u      [0.016262383711093897, -0.30936781478411840, 0.11338240053097021]
 dspike [0.93799180492585832, 0, 0.66447307399002287]
 spike  [1, 0, 1]  reset [0, 0, 0]
 voltage[-0.5, -0.5, -0.5]  syn [1.1420732302356982, 0.38619508006017650, 1.0497870683678638]  refract [1, 0, 1]
```

不應期分支是必要的：`dspike` 與 `reset` 在 `tr.refract[t][i] > 0` 時保持零，膜電位固定為 `VReset`，見 `dynamics/lif.go:391-405`。把不應期神經元誤當成一般 event 會改變 t=2、t=4 的 syn trace，不能拿來和公開結果比對。

## readout、MSE 與 core upstream

`R2` 的最後 synaptic trace node 2 是 `1.0497870683678638`，bits64=`3ff0cbed86667585`。它經 `tensor` 轉為 prediction float32 `1.0497870445251465`, bits32=`3f865f6c`。target 是 `0.3499999940395355`, bits32=`3eb33333`。

Insyra 的 MSE VJP 是 `scale*(prediction-target)`，其中 `scale=upstream*(2/N)`，而 N=1，見 `nn/autodiff.go:1052-1064`。因此本案例的 f32 上游是：

```text
prediction-target = 0.6997870206832886  bits32=3f33253e
scale             = 2                  bits32=40000000
dL/dprediction    = 1.3995740413665771 bits32=3fb3253e
```

readout 是 `[1]`，所以 readout VJP 的 `dh` 也是上述 f32 值；MatMul VJP 由 `dA=MatMul(upstream,B^T)`、`dB=MatMul(A^T,upstream)` 實作（`nn/autodiff_attention.go:42-68`）。`lossGradientReverse` 將 `dh` widened 回 float64，放到最後一列 node 2，即 `up[5][2]=1.3995740413665771`，其他 `up` 元素為零（`learning/network.go:379-430`）。encoder VJP 要到 core backward 完成後才開始（`learning/network.go:455-483`），因此它不能改寫已產生的 `g.Core.Bias[2]`。

## backward 公式與完整 reverse trace

本案例 `window=0`、沒有 adaptation/homeostasis、`smooth=false`，所以所有 reverse step 的 `keep=true`。實際順序是：

```text
dspike = gx[t+1][i]
gx[t][i] += kappa * gx[t+1][i]
du     = dspike * tr.dspike[t][i]
dcand  = gv[i] * tr.reset[t][i] + du
Input  = alpha * dcand
Bias  += Input
prevV  = lambda * dcand
```

來源是 `dynamics/lif.go:535-587`。完成三個 node 後，才依 `e0,e1,e2` 用 `past=max(0,t-delay)` 做 `g.Weights += driveGrad*tr.syn[past][source]` 與 `gx[past][source] += driveGrad*weight`（`dynamics/lif.go:589-603`）。以下 `gx[t]` 已包含 carry 和當步三條 edge 的 additions；數值同樣是 `R2` 候選。

```text
t=5
 gx[t+1] [0, 0, 1.3995740413665771]
 gv      [0, 0, 0]
 reset   [0, 0, 0]
 dspike  [0.93799180492585832, 0, 0.66447307399002287]
 du      [0, 0, 0.92997926554348898]
 dcand   [0, 0, 0.92997926554348898]
 Inputs  [0, 0, 0.58785901303432186]
 gx[t] after carry+edges [0, 0.41150130912402527, 0.51487451621599356]

t=4
 gx[t+1] [0, 0.41150130912402527, 0.51487451621599356]
 gv      [0, 0, 0.34212025250916711]
 reset   [0, 0, 1]
 dspike  [0, 0.51771857797624921, 0.28933048750689067]
 du      [0, 0.21304187259505530, 0.14896889478164790]
 dcand   [0, 0.21304187259505530, 0.49108914729081499]
 Inputs  [0, 0.13466814755866874, 0.31042754622010982]
 gx[t] after carry+edges [0.087534295913134677, 0.36868215399594023, 0.18941174929895643]

t=3
 gx[t+1] [0.087534295913134677, 0.36868215399594023, 0.18941174929895643]
 gv      [0, 0.078373725036386566, 0.18066160107070514]
 reset   [0, 1, 0]
 dspike  [0.39524033346707022, 0.41537767842326973, 0]
 du      [0.034597084306512552, 0.15314233720292406, 0]
 dcand   [0.034597084306512552, 0.23151606223931062, 0]
 Inputs  [0.021869528265671435, 0.14634606264050018, 0]
 gx[t] after carry+edges [0.12732700858018475, 0.16929762167157733, 0.069680688483405431]

t=2
 gx[t+1] [0.12732700858018475, 0.16929762167157733, 0.069680688483405431]
 gv      [0.012727556040841114, 0.085169999598810456, 0]
 reset   [1, 0, 0]
 dspike  [0.83569008976755343, 0, 0.61613336008951602]
 du      [0.10640591923020865, 0, 0.042932596728631429]
 dcand   [0.11913347527104975, 0, 0.042932596728631429]
 Inputs  [0.075306718963524130, 0, 0.027138577036063607]
 gx[t] after carry+edges [0.046840988762509812, 0.11786463403756370, 0.025634092739716549]

t=1
 gx[t+1] [0.046840988762509812, 0.11786463403756370, 0.025634092739716549]
 gv      [0.043826756307525631, 0, 0.015794019692567822]
 reset   [0, 0, 1]
 dspike  [0, 0.68214721902723052, 0.38531419733924532]
 du      [0, 0.080401032330386341, 0.0098771798685236591]
 dcand   [0, 0.080401032330386341, 0.025671199561091481]
 Inputs  [0, 0.050823145487076747, 0.016227293012356571]
 gx[t] after carry+edges [0.050266881336469810, 0.054719080812265092, 0.0094302557120238516]

t=0
 gx[t+1] [0.050266881336469810, 0.054719080812265092, 0.0094302557120238516]
 gv      [0, 0.029577886843309593, 0.0094439065487349101]
 reset   [0, 1, 1]
 dspike  [0.61613337466698415, 0.38531419733924532, 0.38531419733924532]
 du      [0.030971103231823987, 0.021084038702319223, 0.0036336114103823038]
 dcand   [0.030971103231823987, 0.050661925545628816, 0.013077517959117213]
 Inputs  [0.019577471082437528, 0.032024444687233672, 0.0082665679604076725]
 gx[t] after carry+edges [0.039308041262193602, 0.046628519986494046, 0.0034691972014431367]
```

在 `R2` 中，node 2 的 `Inputs` 逆序累加是：

```text
t=5  0.58785901303432186
t=4  0.31042754622010982 bits64=3fd3de0b7fb2bee0
t=3  0
t=2  0.027138577036063607
t=1  0.016227293012356571
t=0  0.0082665679604076725
sum  0.9499189972632596 bits64=3fee65bc866308b6
```

`g.Bias` 的公開 probe 正是用相同的逆序 `reconstructedBias += g.Core.Inputs[t][2]` 重新驗證，見 `evidence/OPS-05/gpu-cancel-audit-20261015/float-probe-v2.go.txt:49-57`。

## rounded reference 與 FMA 候選

逐項取整代表 `round(round(a*b)+c)`；FMA 代表 `round(a*b+c)`。在這個案例，單看公開 endpoint 的一致性，得到下列三個獨立純量候選：

| 候選 | theta base | synapse recurrence | cand | `g.Core.Inputs[4][2]` | `g.Core.Bias[2]` |
|---|---|---|---|---|---|
| `R0` separated | `round(span*s)+ThetaMin` | `round(kappa*syn)+spike` | 兩乘積分別取整後相加 | `3fd3de0b7fb2bede` | `3fee65bc866308b1` |
| `R1` race witness | `fma(span,s,ThetaMin)` | `fma(kappa,syn,spike)` | 兩乘積分別取整後相加 | `3fd3de0b7fb2bee1` | `3fee65bc866308b7` |
| `R2` ordinary witness | `fma(span,s,ThetaMin)` | `fma(kappa,syn,spike)` | `fma(alpha,drive,lambda*voltage)` | `3fd3de0b7fb2bee0` | `3fee65bc866308b6` |

`R1` 與 `R2` 分別吻合現有 race 與一般建置的兩個公開 bits，但這仍不是根因證明：不同的 FMA 位置可能偶然得到同一個 endpoint。特別是 `cand=lambda*voltage + alpha*drive`（`dynamics/lif.go:383`）有兩個乘積，既可候選 `fma(lambda,voltage,alpha*drive)`，也可候選 `fma(alpha,drive,lambda*voltage)`；兩者都必須以實際 operands 比對，不能從結果反推未觀察的 operand。

需要檢查的候選位置如下：

| 來源 | 分離運算 | FMA 候選 | 目前判定 |
|---|---|---|---|
| `dynamics/lif.go:329` | `round(span*s)+ThetaMin` | `fma(span,s,ThetaMin)` | Needs investigation |
| `dynamics/lif.go:383` | `round(lambda*V)+round(alpha*drive)` | 任一乘積與另一項融合 | Needs investigation；R2 用第二乘積融合可吻合一般 bits |
| `dynamics/lif.go:405` | `round(kappa*syn)+spike` | `fma(kappa,syn,spike)` | Needs investigation；R1/R2 用此可吻合公開 bits |
| `dynamics/continuous.go:212` | `drive += round(weight*at)` | `fma(weight,at,drive)` | Needs investigation；本 endpoint witness 不足以判定 |
| `dynamics/lif.go:559` | `gx += round(kappa*gx_next)` | `fma(kappa,gx_next,gx)` | Needs investigation |
| `dynamics/lif.go:600,602` | gradient product 後加總 | 對累加器做 FMA | Needs investigation |

## 能證明最早運算的獨立參考

要從「候選可吻合」提升到「最早運算已證明」，需要兩份互相獨立的資料：

1. 對同一組已捕獲 operands，以 exact-rational/IEEE-754 rounding 參考逐項計算 `round(a*b)+c`，再以同一組 operands 計算 `fma(a,b,c)`。只在兩者不同時，輸出 bits 才能區分 FMA 與兩次取整。
2. 從普通與 race 的共同初始 snapshot，依 forward 到 reverse 的資料流直接捕獲每個候選邊界的輸入與輸出 bits。第一個 ordinary/race output bits 不同的邊界，才是目前可證明的最早差異；公開 probe 目前只捕獲到 `g.Core.Inputs[4][2]`，所以仍只能稱為「最早觀察到的差異」。

最小捕獲集合如下：

```text
Encoder/f32 boundary:
  x, encoder, each f32 MAC accumulator, z, widened coreInputs

Forward:
  lambda, alpha, theta base/slope
  for each edge: drive_before, weight, delayed syn, drive_after
  for each node: voltage[t], drive, lambda*voltage, alpha*drive,
                 cand, theta, u, abs(u), den, den*den, dspike,
                 refractory flag, spike, reset, voltage[t+1], syn_before,
                 kappa*syn_before, syn_after

Backward:
  gx[t+1], gx[t] before/after carry, gv, reset, dspike, du, dcand,
  alpha, Input, Bias before/after
  for each edge: past, driveGrad, delayed syn, weight, weight-gradient,
                 gx source before/after

Readout/MSE:
  h, readout, prediction, target, difference, MSE scale, dL/dprediction,
  readout dh, core upstream
```

若某一組 operands 的 `round(mul)+add` 與 `fma` 相同，該組不能區分兩種實作，必須等下一個對兩者敏感的邊界；不能以 endpoint 倒推出中間未捕獲值。若 trace 已證明 operands 與 output，最後再用編譯器指令層或受控的 no-FMA/FMA build 交叉核對，才可把數值一致性提升為 lowering 證據。

## Confirmed

- `forwardReader` 欄位宣告、prehistory、delay 索引與 declared edge order 已由 `dynamics/continuous.go:158-214` 核對。
- LIF forward 的 drive、candidate、refractory、event、synaptic recurrence 順序已由 `dynamics/lif.go:359-418` 核對。
- LIF backward 的 carry、surrogate、reset、input/bias 累加與 edge reverse 順序已由 `dynamics/lif.go:535-603` 核對。
- encoder/readout/MSE 的 f32 邊界與 core upstream 位置已由 `learning/network.go:267-329`、`358-483`、`516-704` 和 Insyra v0.3.4 source 核對。
- 現有公開證據確認 step 1 的第一個觀察差異是 core input gradient row 4、node 2，step 2 相同；`root_cause_confirmed=false`，見 `evidence/OPS-05/gpu-cancel-audit-20261015/core-input-gradient-comparison.json:6-32`。
- `R1/R2` 純量候選分別重現 race/ordinary 的公開 `g.Core.Inputs[4][2]` 與 `g.Core.Bias[2]` bits，這是一致性證據，不是因果證明。

## Needs investigation

- 尚未知道普通與 race 建置是否在 `theta_base`、`cand`、`synapse` 或 backward accumulator 使用不同 FMA lowering。
- 尚未捕獲 Go `math.Exp`、`math.Expm1` 及所有 trace 常數的實際 bits；獨立 libm 的相同十進位輸出不能替代 bits 證據。
- 尚未有私有 trace overlay 的 `cand`、`syn`、`gx` 與每個候選邊界 operands，因此不能宣稱最早機器運算或 root cause 已確認。
- Insyra 小型 MatMul 依 source 應走 CPU f32 path，但實際執行時仍應以必要的 runtime trace 確認沒有外部 device hook；這不改變目前 core input gradient 已先於 encoder VJP 的資料流判斷。
