# Ticket 101 LIF 跨建置浮點契約審查

審查範圍是 ticket 101、本輪稽核計畫、主規格 9.4、`ENG.md`、現行 LIF／Network 實作與測試，以及既有浮點證據。這份報告只做唯讀查證；本輪沒有執行 `go build`、`go test`、`go test -race` 或 `go vet`，也沒有修改正式來源、測試、共享文件或 Git。讀到的本機工具鏈是 `go1.26.5 darwin/arm64`。

## Confirmed

1. 主規格 9.4 明示的是：單執行緒參考模式在相同硬體、編譯器與版本上應可重現；跨 CPU／GPU 與不同加總順序只承諾通過數值容差，不保證位元完全一致。這不是「普通建置與 `-race` 建置必須逐位相同」的文字契約。依據：`docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:428-432`。
2. CPU 參考路徑使用 `float64`、固定邊順序與同步更新。LIF 的 `Workers=0/1` 是單執行緒參考模式，平行模式按每個 target 的宣告邊順序累加，原始程式宣告其結果應與參考模式逐位相同。這是同一套執行語意內的邊順序／Workers 契約，不是跨編譯建置契約。依據：`ENG.md:59-60`、`dynamics/lif.go:33-37`、`dynamics/lif.go:424-432`。
3. ticket 11 的 LIF 驗收要求同一輸入兩次 `Forward` 位元相同，但現行 `TestLIFForwardIsDeterministicAndBinary` 在每一次被編譯的建置內比較兩次呼叫；它沒有把普通與 `-race` 的輸出互相比較。依據：`docs/tickets/11-lif-core.md:98-109`、`dynamics/lif_test.go:154-184`。
4. Go 1.26.5 語義允許實作把多個浮點運算合併成一次 fused operation，結果可不同於逐條指令各自取整；`x*y+z` 是明示的 FMA 合法例子。顯式轉換到目標浮點型別會要求該次取整，阻止省略該取整的融合。依據：本機官方規格 `/usr/local/go/doc/go_spec.html:5037-5054`、`/usr/local/go/doc/go_spec.html:5057-5074`。
5. 現行 LIF 前向的膜電位式仍直接寫成 `lambda[i]*tr.voltage[t][i] + alpha[i]*drive[i]`；這提供了 Go 合法融合差異的候選位置，但只指出可能機制，尚未證明它是本輪 `Core.Inputs[4][2]` 差異的因果第一個運算。依據：`dynamics/lif.go:381-405`。
6. 既有稽核證據確認共同初始快照相同，第一步 public core input gradient 的 row 4／node 2 相差一個 float64 間距，差異出現在 Adam 前；同一份證據仍標記 `root_cause_confirmed: false`。依據：`evidence/OPS-05/gpu-cancel-audit-20261015/core-input-gradient-comparison.json:2-20,31-36`。

## Needs investigation

- 目前沒有足夠證據把普通／`-race` 的一個 ULP 差異判為 LIF 演算法缺陷。若差異只在跨建置比較出現，且每個建置各自的重複執行都逐位穩定，且獨立算術能以合法的 fused 與兩次取整結果涵蓋，則較符合「編譯器允許的建置差異」。這仍不等於模型品質沒有影響。
- 現行跨核心測試已把這類 compiler-rounding 行為當成建置條件處理：普通建置要求 exact，`-race` 對 LIF 膜電位衍生值使用 `1e-16` 相對上限，其他結構值仍要求 exact。這是既有測試／工程取捨與歷史觀察，不是主規格新增的普通／`-race` 逐位契約，也不是 ticket 101 已完成的根因證明。依據：`dynamics/mixed_test.go:963-982,1015-1103`、`dynamics/mixed_contraction_default_test.go:1-12`、`dynamics/mixed_contraction_race_test.go:1-13`、`ENG.md:172-174`。
- `learning/lif_test.go:204-260` 的 public Network 對 direct LIF `Backward` 比較採 `1e-7 + 1e-4*abs(want)` 容差，不是 raw-bit equality。Network 還有 Insyra float32 邊界，核心本身為 float64；若沒有固定相同的編碼後輸入、讀出上游與反向邊界，直接比較 public Network 與 LIF 會混入預期的精度轉換。依據：`README.md:320`、`learning/network.go:258-261,331-355,455-483`、`learning/lif_test.go:148-167,204-260`。
- 歷史 COR-05 記錄了單一 `go1.26.5 darwin/arm64` 環境的普通／`-race` LIF 膜電位衍生值差異與 FMA 解釋，但它是另一票的 fixture 完成紀錄，不能取代目前 ticket 101 的同輸入、同上游與完整梯度證據。依據：`evidence/COR-05/verification.json:12-15,23-28,42-52`、`docs/tickets/25-mixed-types-vector-nodes-shared-parameters-and-recompute.md:160-172`。
- 是否把「普通與 `-race` 跨建置也必須逐位相同」提升為新契約，仍需使用者決策。主規格目前只給同建置重現與跨建置／順序容差；若選擇更強契約，將涉及明示取整邊界或固定不可內聯計算的相容性與效能取捨，本報告不替使用者改契約。

## 契約逐項表

| 項目 | 目前明示內容 | 裁決 | 主要依據 |
|---|---|---|---|
| 同一單執行緒建置重複執行 | 相同硬體、編譯器與版本的單執行緒參考模式應可重現 | **Confirmed**。每個建置都要能各自驗證，不能用一次成功推論另一建置 | `docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:428-432` |
| 普通建置對 `-race` 建置逐位相同 | 規格沒有這項 pairwise 要求；`ENG` 要求兩種建置各自執行同核心關閉／中性逐位對照，並沿用 compiler-rounding 契約 | **未明示為跨建置逐位契約**；需決策才可提高門檻 | `ENG.md:172-174`、`docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:430-432` |
| LIF Workers 與固定邊順序 | 每個 target 依宣告邊順序累加，平行模式應與單執行緒結果逐位相同 | **Confirmed**，但範圍是同一數值語意／建置內的 Workers 比較 | `dynamics/lif.go:33-37,424-432`、`dynamics/workers_lif_test.go:119-232,234-271` |
| 兩次相同 LIF `Forward` | ticket 11 要求相同輸入兩次位元相同；現行測試用同一次建置的兩次呼叫驗證 | **Confirmed as same-build test intent**，未驗證 ordinary-vs-race pair | `docs/tickets/11-lif-core.md:98-109`、`dynamics/lif_test.go:154-184` |
| Public Network 與 public LIF 完整梯度 | ticket 101 計畫要求同一建置內 Network 梯度與 LIF 核心梯度精確相同；既有 LIF 測試仍採容差 | **Ticket 101 待驗收**，不能以舊測試宣稱完成 | `docs/tickets/101-lif-build-numerics.md:8-13`、`evidence/LRN-01/lif-float-audit-20261015/plan.md:5-10`、`learning/lif_test.go:204-260` |
| FMA／融合 | Go 實作可融合浮點運算，結果可與逐步取整不同；顯式浮點轉換可固定該次取整 | **語義允許**；單靠二進位不同不能判缺陷 | `/usr/local/go/doc/go_spec.html:5049-5074` |
| 跨 CPU／GPU／加總順序 | 只承諾數值容差，不保證位元完全一致 | **Confirmed**；不可把 cross-platform tolerance 當作同建置 nondeterminism 的證據 | `docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:428-432` |
| 硬放電導數驗證 | 硬 spike 不可用有限差分當正確答案，應核對宣告的替代梯度 | **Confirmed**；本票的差異診斷不可改用硬事件有限差分 | `docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:369-380`、`docs/tickets/11-lif-core.md:47-55` |
| 一個 ULP 對模型品質的意義 | 規格與既有證據只描述數值／建置差異，沒有品質免責 | **未驗證**；需獨立任務對照後才能談品質影響 | `evidence/OPS-05/gpu-cancel-audit-20261015/core-input-gradient-comparison.json:6-20,31-36`、`docs/tickets/101-lif-build-numerics.md:15-17` |

## same-build 可重現的最小驗證

1. 固定來源版本、`go version`、GOOS／GOARCH、CPU、普通或 `-race` 狀態、build tags／`-gcflags`／CGO 與相關環境；固定 LIF config、節點／邊／delay 順序、參數、初始狀態、原始輸入、編碼後 float32 輸入、讀出參數與完整 upstream。每份輸入與輸出保存 shape、數值與 `math.Float64bits` 指紋。
2. 在不插入觀察程式的正式控制下，各建置分別以相同 binary 執行至少兩個新程序，逐位比較完整 public LIF `Forward`（outputs、spikes、voltages、final state）與 `Backward`（weights、bias、log_tau、theta_raw、inputs、initial）。同一建置重複不一致才足以直接指向 deterministic defect 或未固定輸入／狀態。
3. 在每個建置內，使用計畫指定的相同編碼／讀出邊界，逐位比對 public Network 與 direct public LIF 的完整核心輸出及梯度。若先比的是 Network 的 end-to-end 梯度，必須先證明讀出／編碼的 float32 邊界輸入與 upstream 完全相同；不可把邊界轉換差異誤算成 LIF 核心差異。
4. 只有在第 2、3 項各自通過後，才做 ordinary 對 `-race` 的跨建置診斷。記錄第一個 public divergence 的欄位、step、node、兩個 bit pattern、ULP／絕對差與事件是否改變。再用未改動控制取得相同 operands、運算順序與 bits，獨立比較 fused 單次取整與顯式兩次取整；私有 trace 只能是 test-only、方法完成後讀取，不能把 logging 插進正式運算而改變 codegen。
5. 要判定「品質影響」，另以固定 seed／資料／門檻做普通與 `-race` 的任務級 paired run，保存 loss、梯度、更新、事件與任務指標。核心一個 ULP 的觀察本身不支持「無品質影響」或「一定有品質影響」任何一方。

## 哪些事實足以作何種裁決

### 可確認為缺陷

- 同一個固定 binary、同一工具鏈與同一輸入，在新程序重跑仍出現 raw-bit 不一致，且能排除未保存的隨機、共享可變狀態、競態或探針改變 codegen。
- 同一建置內，public Network 與 direct public LIF 在已對齊的 float32 boundary、相同 upstream 下未能符合計畫要求的完整梯度逐位相同，並且差異可在核心／橋接邊界重現。
- 最早可重現的運算不符合宣告方程式、固定邊順序或 Go 允許的取整結果，且獨立算術已排除合法融合與邊界轉換。

### 可判定為正常的建置差異

- 差異只出現在 ordinary 對 `-race` 或其他明確不同 build configuration；每個建置各自重複逐位穩定。
- 第一個差異的 operands／順序相同，結果恰落在 Go 允許的融合單次取整與逐步取整選項，且沒有事件、狀態契約或固定任務門檻的實際違反。
- 此裁決只能稱為「目前契約允許的數值差異」，不能延伸成品質無影響，也不能取代 ticket 101 要保存的因果證據。

### 仍需調查或使用者決策

- 目前舊證據只把差異定位到 public core input gradient，沒有找到 causal first machine operation；因此目前最精確的狀態仍是 `root_cause_confirmed: false`。
- 若要跨 ordinary／`-race` 強制逐位相同，需先確認是否接受新增的取整／不可內聯邊界、效能與既有編譯器行為相容性；在確認前不得直接改公式或放寬／收緊驗收。

## 精確未驗證範圍

- 沒有執行本輪 ticket 101 計畫所列的普通／`-race` public Network、public LIF Forward／Backward、完整編碼／讀出與完整梯度對照。
- 沒有執行任何 Go build、測試、race 測試或 vet；因此不能把目前工作樹狀態說成 ticket 101 已通過。
- 沒有逐項取得 LIF Forward、反向累加、float32 encode／readout bridge 更早步驟的 operands、取整 bits 或機器碼；Go 規格只證明融合合法，不證明本次差異確實由融合造成。
- 沒有做第二個 CPU／OS／Go 版本，也沒有做全圖或任務級品質對照。
- `evidence/COR-05/verification.json` 與 `docs/tickets/25-mixed-types-vector-nodes-shared-parameters-and-recompute.md` 的完成敘述是歷史 fixture／單一環境證據，不能替代本輪目前來源與探針結果。

## 建議最小檢查

先由 root 依 `evidence/LRN-01/lif-float-audit-20261015/plan.md:5-10` 完成三組隔離：同一建置的 fresh-process exact replay、同一建置的 Network↔LIF boundary／完整梯度 exact compare、最後才做 ordinary↔`-race` first-divergence 與獨立取整對照。若前兩組通過而第三組只呈現合法 FMA／兩次取整差異，保留差異證據並以「契約允許、品質未判定」結案；若要改成跨建置逐位契約，先取得使用者對相容性與效能取捨的決策。
