# LIF float causal review

## 判定範圍

本審查只讀取 `evidence/LRN-01/lif-float-audit-20261015` 指定證據、
`dynamics/lif.go`、`dynamics/continuous.go` 與 `learning/network.go`。本輪沒有執行
Go build/test，也沒有把 endpoint 相等當成機器碼證據。十六進位字串是觀察到的
IEEE-754 binary64 bits。

## Confirmed

### 1. encoder、MSE/readout VJP 與 LIF core 的精度邊界

- 公開 probe 先以 `float32` 建立交替的 `.7/.8` 輸入與 `[1,0,0]` encoder，經
  Insyra `MatMul` 後把 `z.Data()` 逐項拓寬為 core 的 `float64` 輸入。這個流程見
  `evidence/LRN-01/lif-float-audit-20261015/public-core.go.txt:40-60`。兩個公開 log
  的 encoded bits 完全相同，只有 `3fe6666660000000` 與 `3fe99999a0000000` 交替，
  且 `core_upstream_f32_bits` 都是 `3fb3253e`
  (`public-normal.log:1`, `public-race.log:1`)。
- 正常 Network 路徑的 `tensor` 會在 `learning/network.go:685-704` 將輸入、參數與
  upstream 轉成 finite `float32`，`doubles` 在 `learning/network.go:706-711` 再拓寬
  成 core 使用的 `float64`。encoder MatMul 在 `learning/network.go:561-581`，最後
  readout MatMul 在 `learning/network.go:638-663`。
- MSE loss 與其 VJP 由 Insyra tape 執行：`learning/network.go:283-320`。readout
  reverse、core upstream、encoder reverse 的順序是
  `learning/network.go:379-437`、`learning/network.go:455-483`。公開 probe 以同一
  個 core upstream 呼叫 `LIF.Backward`，再用 `Network.LossGradient`，並以完整
  core gradient 做 `reflect.DeepEqual`，見
  `evidence/LRN-01/lif-float-audit-20261015/public-core.go.txt:65-95`。
- 因此本案例可確認：encoder/readout/MSE 邊界沒有在兩個 build 之間先造成不同的
  core upstream。`public-comparison.json:2` 也確認每個 build 內直接 LIF gradient
  與 Network core gradient 相等。這不等於已獨立驗證 Insyra 每個反向指令的機器碼
  rounding。

### 2. LIF forward 的來源順序與 trace 索引

- `LIF.Forward` 先由 `math.Exp`／`math.Expm1` 產生 lambda、alpha，位置是
  `dynamics/lif.go:315-324`；每步先複製輸入，經 `forwardReader` 取得延遲 synaptic
  trace，再做 edge 累加，位置是 `dynamics/lif.go:359-371` 與
  `dynamics/continuous.go:158-185,208-213`。
- bias 在 `dynamics/lif.go:381-382` 加入 drive，候選值在
  `dynamics/lif.go:383` 定義為 `lambda*voltage + alpha*drive`。候選之後才進入
  refractory/event/reset 分支 `dynamics/lif.go:391-404`，狀態在
  `dynamics/lif.go:416-419` 寫回。
- `LIFTrace.Voltages()` 回傳 `voltage[1:]`，所以公開 `voltages_bits[4][2]` 是內部
  `voltage[5][2]`，依 `dynamics/lif.go:474-480` 解讀。這與
  `trace-comparison.json:44-46` 的 `/voltage/5/2` 是同一個差異。

### 3. candidate 的融合方式已由 operand table 與原執行檔共同支持

`complete-reference.py.txt:7-12` 的 `fma` 是 Fraction 精確乘加後再做 nearest-even
binary64 轉換。它對每個 slot 同時計算 separated、`fused_alpha` 與 `fused_lambda`
候選，公式位於 `complete-reference.py.txt:109-118`。在共同 operand 下，關鍵 slot
如下：

| slot | lambda | previous voltage | alpha | drive | ordinary actual / variant | race actual / variant | refractory before |
|---|---|---|---|---|---|---|---:|
| `cand[1][0]` | `3fd78b56362cef38` | `bfe0000000000000` | `3fe43a54e4e98864` | `3fe99999a0000000` | `3fd497a98e768493` = fused-alpha | `3fd497a98e768492` = separated = fused-lambda | 1 |
| `cand[2][1]` | `3fd78b56362cef38` | `bfe0000000000000` | `3fe43a54e4e98864` | `3fce9b89acd403c9` | `bfa0c9543588cc02` = fused-alpha | `bfa0c9543588cc00` = separated = fused-lambda | 1 |
| `cand[4][2]` | `3fd78b56362cef38` | `bfe0000000000000` | `3fe43a54e4e98864` | `3fb840887ed8e8ea` | `bfbfc221cec94cf7` = fused-alpha | `bfbfc221cec94cf6` = separated = fused-lambda | 0 |
| `cand[5][0]` | `3fd78b56362cef38` | `bfe0000000000000` | `3fe43a54e4e98864` | `3fe99999a0000000` | `3fd497a98e768493` = fused-alpha | `3fd497a98e768492` = separated = fused-lambda | 0 |
| `cand[5][1]` | `3fd78b56362cef38` | `bfe0000000000000` | `3fe43a54e4e98864` | `3fd23b280cc34bcc` | `bf6fbb3e68cd7bc4` = fused-alpha | `bf6fbb3e68cd7bc0` = separated = fused-lambda | 1 |
| `cand[5][2]` | `3fd78b56362cef38` | `bfbfc221cec94cf7` / `bfbfc221cec94cf6` | `3fe43a54e4e98864` | `3fe783e6228f7153` | `3fdacee06e7854e0` = fused-alpha | `3fdacee06e7854e1` = separated = fused-lambda | 0 |

The ordinary records are in `complete-reference.json:46-56,98-108,189-199,202-238`; the
race records are in `complete-reference.json:529-539,581-591,672-682,685-721`. The last
row has a different previous voltage because it is downstream of `cand[4][2]`.
Every one of the 18 recorded candidates and the reconstructed forward trace matched its
build's observed trace (`complete-reference.json:241-242,724-725`). This is stronger than
matching only the final gradient, but the script's `expected` choice is explicitly selected
by `mode` at `complete-reference.py.txt:103-117`; it is a controlled arithmetic check, not
a blind discovery of the machine instruction.

The source-PC-labelled disassembly provides the independent operation-shape evidence:

- ordinary has a separate `fmul` followed by `fmadd`, with the candidate store at
  `10054b640`, all labelled `lif.go:383`, in
  `forward-source-assembly-normal.txt:1399-1417`;
- race has a separate `fmul`, then `fmadd`, and stores the candidate at
  `1008b7f1c`, also labelled `lif.go:383`, in
  `forward-source-assembly-race.txt:1929-1995`;
- the ordinary sequence is `fmul d1,d2,d1` then `fmadd d0,d2,d0,d1`
  (`forward-source-assembly-normal.txt:1413-1417`), which agrees with the
  fused-alpha bits in the table; the race sequence is `fmul d0,d0,d1` then
  `fmadd d0,d2,d1,d0` (`forward-source-assembly-race.txt:1987-1995`), which agrees
  with fused-lambda bits.

The two disassemblies were generated from the recorded public and race binaries with
successful `objdump --line-numbers`; the binary/output hashes and command are recorded in
`forward-assembly-normal.json:1-10` and `forward-assembly-race.json:1-10`. The PC-to-source
helper uses `gosym.PCToLine` (`pc_lines.go.txt:14-34`). This confirms the candidate source
block and its fused operation shape. It does not by itself observe a dynamic `(PC,t,node)`
pair or prove which register is a logical lambda/alpha/drive slot; the operand table is what
disambiguates the two FMA variants.

### 4. `cand[1][0]` 是最早差異，但被 refractory 捨棄

`trace-comparison.json:2-7` 顯示最早 forward candidate 差異是
`cand[1][0]`: ordinary `3fd497a98e768493`、race `3fd497a98e768492`。兩邊 operand 完全
相同，且 `complete-reference.json:46-56,529-539` 都記錄 `refractory_before: 1`。
來源分支在 `dynamics/lif.go:392-396` 會直接寫入 `VReset`、遞減 refractory，並註明
這一步 drive 不進膜電位且不帶 gradient。實際 trace 也相符：

- `dspike[1][0]` 兩邊都是零，`trace-values-normal.json:73-83`、
  `trace-values-race.json:73-83`；
- `reset[1][0]` 兩邊都是零，`trace-values-normal.json:110-120`、
  `trace-values-race.json:110-120`；
- 後續 `voltage[2][0]` 兩邊都是 `bfe0000000000000`，
  `trace-values-normal.json:221-240`、`trace-values-race.json:221-240`。

因此這一位不能被當成第一個 gradient-causing difference。`cand[2][1]` 也是同樣的
refractory discard，見 `complete-reference.json:98-108,581-591`。

### 5. `cand[4][2]` 是第一個傳到 gradient 的 forward 差異

`cand[4][2]` 的 lambda、前一電位、alpha、drive 完全相同，只有 rounding schedule
不同：ordinary 是 fused-alpha `bfbfc221cec94cf7`，race 是 separated/fused-lambda
`bfbfc221cec94cf6`，且兩邊 `refractory_before: 0`
(`complete-reference.json:189-199,672-682`)。它造成：

- forward surrogate derivative `dspike[4][2]` ordinary
  `3fd284640564fb33`、race `3fd284640564fb34`，見
  `trace-comparison.json:33-37`；
- 內部 `voltage[5][2]` ordinary `bfbfc221cec94cf7`、race
  `bfbfc221cec94cf6`，見 `trace-comparison.json:44-46`。這正是公開回傳
  `voltages_bits[4][2]` 的 row，見 `public-comparison.json:35-37`；
- reverse 記錄依反向 traversal 最先在 `(t=4,node=2)` 分叉。兩邊 reverse upstream
  `dspike_bits` 都是 `3fe079da1f1620e2`，`gv_bits` 都是
  `3fd5e54c57f4df64`，但 ordinary 的 `du/dcand/input/bias` 是
  `3fc31169a99aae41 / 3fdf6e012cc23684 / 3fd3de0b7fb2bee0 /
  3fecbec3744de48f`，race 是
  `3fc31169a99aae42 / 3fdf6e012cc23685 / 3fd3de0b7fb2bee1 /
  3fecbec3744de490`。來源見 `complete-reference.json:356-363,839-846`。

後續 reverse 差異是這一位沿 recurrent/edge reverse 傳播的結果，例如 `(t=3,node=1)`
的 `du/dcand` 由 ordinary `...43d/...45f` 變成 race `...43e/...460`
(`complete-reference.json:376-383,859-866`)，再累加到 node 2 bias。公開結果最後只差
`Bias[2]` 與 `Inputs[4][2]` 一個 binary64 ulp，且 Network core gradient 與直接 LIF
gradient 在各 build 內仍完全相等，見 `public-comparison.json:2-33`。

## 全部已觀察的 forward trace 差異

`trace-comparison.json:2-47` 是完整 diff，沒有其他 trace slot 差異：

| trace slot | ordinary | race | 因果分類 |
|---|---|---|---|
| `cand[1][0]` | `3fd497a98e768493` | `3fd497a98e768492` | 最早差異，refractory discard |
| `cand[2][1]` | `bfa0c9543588cc02` | `bfa0c9543588cc00` | refractory discard |
| `cand[4][2]` | `bfbfc221cec94cf7` | `bfbfc221cec94cf6` | 第一個 active、傳到 gradient |
| `cand[5][0]` | `3fd497a98e768493` | `3fd497a98e768492` | active candidate 但最後 upstream node 0 為零 |
| `cand[5][1]` | `bf6fbb3e68cd7bc4` | `bf6fbb3e68cd7bc0` | refractory discard |
| `cand[5][2]` | `3fdacee06e7854e0` | `3fdacee06e7854e1` | downstream candidate，前一電位已不同 |
| `dspike[4][2]` | `3fd284640564fb33` | `3fd284640564fb34` | `cand[4][2]` 的 surrogate 後果 |
| `dspike[5][0]` | `3fee040763c25285` | `3fee040763c25288` | 最後一步且 upstream node 0 為零 |
| `voltage[5][2]` | `bfbfc221cec94cf7` | `bfbfc221cec94cf6` | `cand[4][2]` 的 state 後果 |

`trace-values-normal.json:1-3` 與 `trace-values-race.json:1-3` 都標示 public control
values 相等；其餘共用的 alpha、lambda、drive 與 state bits 可在
`trace-values-normal.json:4-71,105-109,221-255` 及 race 對應行核對。

## Backward 順序與獨立參考的邊界

原始 `LIF.Backward` 的逐點順序是：取得 `gx[t+1]`、回傳 synaptic upstream、算
`du`、算 `dcand`，再寫 theta、input、bias、log-tau、前一電位，最後處理 edge
gradient 與 `gx[past]`。程式行號是 `dynamics/lif.go:535-587` 與 `dynamics/lif.go:589-605`。
`complete-reference.py.txt:74-97` 以同一資料流重建，並以
`complete-reference.py.txt:119-121` 比對每個 build 的完整 gradient；兩個 case 的
`gradient_differences` 都是空物件，見 `complete-reference.json:303-304,786-787`。

這個 independent reference 的強度與限制必須分開看：

1. 它對 candidate 與 reverse 的乘加使用 Fraction 精確結果再轉 binary64，避免把
   Python 自己先 round 的結果當真值。
2. 它仍讀取每個 build 的 `trace-values-*` 作為 alpha、lambda、theta slope、drive、
   voltage、syn、spike、reset、dspike 等 state，並讀取公開 fixture 的 encoded input
   與 upstream (`complete-reference.py.txt:103-108`)。所以它是「對已觀察 state 的
   獨立 arithmetic replay」，不是重新證明 encoder、`Exp/Expm1`、event 或 trace
   storage 的最早來源。
3. reverse 的 `dspike` 變數是 `gx[t+1]`，不是 forward trace 的 surrogate derivative；
   這個區別明列於 `complete-reference.py.txt:77-82`。它成功重現每個 build 的
   reverse bits，足以說明 `cand[4][2]` 改變如何進入 `du → dcand → input/bias`，但不
   足以單獨排除每一個 backward FMA/rounded order 的差異。
4. backward disassembly 在兩個 build 都可見對應 `lif.go:559,565,576,584,600,602`
   的 FMA/乘法，例：ordinary `backward-source-assembly-normal.txt:636-730,750-767,927-968`，
   race `backward-source-assembly-race.txt:846-1026,1332-1342,1542-1602`。來源行
   註解與靜態 PC 可指向函式／source line，但沒有這份證據能把一個 runtime PC 與
   `(t,node)`、實際 registers、destination slot 同時綁定。因此不能宣稱 backward
   每一個 FMA 的 machine-level order 已完全證明。

## Needs investigation

1. **動態 PC 到 logical slot 的直接證據仍缺。** `pc_lines.go.txt` 只把靜態 PC 轉成
   source line；forward assembly 的 `[base,index,lsl #3]` store 可看出 node stride，
   但目前沒有每次執行的 `(PC,t,node,destination offset)` trace。candidate-level 因果
   已由相同 operands、三種 rounded reference 與完整 trace replay 支持，machine-level
   的 slot binding 仍不是直接觀察。
2. **backward 每一步的融合順序尚未逐位定案。** 目前可確認第一個 reverse divergence
   發生在 forward `cand[4][2]` 已分叉之後，不能由 endpoint gradient 反推出所有
   backward FMA。要區分 backward 的 fused／separated reference，至少要在每個
   source-PC 與 `(t,node)` 綁定下捕獲 operand/result bits。
3. **完整 encoder/readout 參考尚未獨立於實作。** 公開 probe 的 direct LIF 與 Network
   equality 已確認資料流契約，但 complete-reference 並未重建 Insyra 的 float32
   MatMul、MSE VJP 或 encoder VJP。若要把「最早運算」延伸到 LIF 之前，需另捕獲
   `x32`、encoder、encoded32、readout input/weight、prediction、target、MSE delta、
   readout VJP 與 core seed 的每個結果 bits。
4. **品質影響未量測。** evidence 的 `public-comparison.json:40-41` 仍標示
   `root_cause_confirmed:false`、`model_quality_measured:false`。本報告只把 candidate
   rounding schedule 與本人工案例的 gradient 差異縮小到可驗證的 causal chain，不把
   一個 ulp 的案例差異外推成模型品質結論。

## 最小可獨立驗證 operand 集合

- **Forward candidate：** binary/build hash、source PC、outer step `t`、inner node `i`、
  candidate destination base/offset、lambda、前一 voltage、alpha、edge 累加及 bias
  後的 drive、separated result、fused-alpha result、fused-lambda result、實際
  candidate，以及 refractory counter。若要證明 drive 不是更早來源，還要逐 edge
  捕獲 weight、delayed syn value、累加前後 drive。
- **LIF reverse：** source PC、`t,i`、`gx[t+1][i]`、forward surrogate derivative、
  `gv[i]`、reset、`du`、`dcand`、alpha、input result、bias accumulator、lambda、
  `(voltage-drive)`、log-tau product、`prevV`，以及 edge weight/syn/gradient 與其
  destination slot。這組資料才能區分最早是 forward state、reverse FMA，或 accumulator
  順序。
- **learning boundary：** raw input 的 float32 bits、encoder tensor bits、encoded
  float32 bits、拓寬後 core input、readout input/weight、prediction、target、delta、
  MSE upstream、core upstream、core gradient 與 encoder/readout gradient。只比較最後
  的 `Bias[2]` 或 `Inputs[4][2]` 不足以證明最早運算。

## Rounded-reference 判別規則

對固定 operands，Fraction exact product/addition 後 nearest-even 的三個候選已足以在
本案例區分：ordinary `cand[4][2]` 只能對上 fused-alpha，race 只能對上
separated/fused-lambda。這個判別必須和原執行檔的 source-PC 指令序列及 dynamic slot
operand 同時成立；單獨重寫 Go/Python 公式，或只看到 final endpoint 相等，都不能當成
machine-code truth。`complete-reference.py.txt:10-12,111-118` 與兩份 source assembly
區塊共同滿足前半個條件，dynamic PC/slot overlay 仍是待補證據。

## 參考來源索引

- Forward contract／state index：`dynamics/lif.go:315-324,359-419,474-480`。
- Forward reader／declared edge order：`dynamics/continuous.go:150-185,208-213`。
- Backward contract／operation order：`dynamics/lif.go:492-499,535-605`。
- Float32 encoder/readout/MSE VJP：`learning/network.go:258-320,379-483,561-711`。
- Candidate arithmetic replay：`complete-reference.py.txt:7-12,20-61,63-100,102-121`。
- Observed forward diff：`trace-comparison.json:2-50`。
- Observed public gradient diff：`public-comparison.json:2-41`。
