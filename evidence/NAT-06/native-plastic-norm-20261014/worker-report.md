# NAT-06 ticket 93 數值契約唯讀審查

Skills：investigate

## Problem

目前 `l2Norm` 先逐項平方再依索引加總。合法有限快速狀態在平方中間步驟可能溢位或下溢，造成合法執行被拒絕或非零幅度被回報成零。審查目標是確認根因、既有失敗測試的數值期待，以及 Root 計畫對穩定計算、正常位元相容與真正不可表示幅度的邊界是否足夠。

## CONFIRMED

- **[P2] [CONFIRMED] `simulate/plasticity.go:260-267`：`l2Norm` 的中間平方會把可表示幅度誤判成非有限或零。**
  觸發條件是快速狀態含 `1e200` 或 `1e-200` 等極端有限值。`v*v` 對 `1e200` 變成 `+Inf`，對 `1e-200` 變成 `0`，而後續 `math.Sqrt(sum)` 只能傳遞錯誤。獨立 Python stdlib `decimal` 參考以精確 binary64 輸入計算，分別得到 `1e200` 與 `1e-200`，故兩者都是可回報的有限非零幅度。這正是 ticket 93 第 16 行與 Root plan 第 13 行記載的六個公開流程觀察，根因可由目前函式直接重現於算術層，不是核心或可塑性規則本身的拒絕。

- **[P2] [CONFIRMED] `simulate/runner.go:665,742-750`：報告幅度錯誤會在 Run 的晚期拒絕合法流程。**
  `Run` 在建構報告前後呼叫 `l2Norm`，再由既有 `checkFinite` 拒絕 `plastic l2`。`simulate/report.go:138-144` 的檢查本身符合「報告不可含非有限值」的契約，錯誤在幅度函式把中間結果當成最終幅度。`simulate/runner.go:755-756` 才提交神經與快速狀態，所以現有失敗路徑的「回傳空報告且不提交」行為仍可保留。

- **[P2] [CONFIRMED] `simulate/plasticity_test.go:714-737`（原件 `evidence/NAT-06/native-plastic-norm-20261014/original-plasticity_test.go.txt` 同行）：`plastic_max = 1e200`、兩條邊的拒絕期待不符合數值上界。**
  `plasticity/plasticity.go:304-305` 驗證快速值不超過 `plastic_max`，`plasticity/plasticity.go:412-415` 也在每步將值夾在 `[-plastic_max,+plastic_max]`。因此兩條邊的快速狀態幅度必定不超過 `sqrt(2)*1e200 = 1.414213562373095e200`，這是有限 binary64 值。原測試以 `plastic l2` 錯誤作為成功條件，捕捉的是平方中間值溢位，不是真正的幅度不可表示。Root plan 第 19 行保留真正不可表示拒絕、改以有限報告驗證這個判斷，數值依據成立。

## Root 計畫判斷與 NEEDS INVESTIGATION

- **[NEEDS INVESTIGATION] `evidence/NAT-06/native-plastic-norm-20261014/plan.md:13,25`：fallback 觸發條件必須辨識「部分」平方下溢。**
  反例取 `x = 2^-537`、`y = 2^-538`。binary64 中 `x*x` 是最小次正規值、`y*y` 已下溢為零；舊路徑回傳 `0x1p-537`，但以 180 位十進位精度、精確 binary64 輸入的獨立參考為 `0x1.1e3779b97f4a8p-537`。若只在最後 `sum == 0` 時 fallback，這個 `sum` 非零，錯誤會保留。缺少的證據是 Root 實作的逐項偵測規則；需用此案例確認 `v != 0 && v*v == 0` 之類的條件，或等價的穩定前置檢查。

- **[NEEDS INVESTIGATION] `evidence/NAT-06/native-plastic-norm-20261014/plan.md:13,25`：最高端捨入需要獨立的 overflow guard，單純 scaled/Kahan 不足以證明真正不可表示。**
  令 `M = 0x1.fffffffffffffp+1023`，`y26 = ldexp(M,-26) = 0x1.fffffffffffffp+997`，`y27 = ldexp(M,-27) = 0x1.fffffffffffffp+996`。180 位十進位參考以 `sqrt(M*M+y*y)` 與 binary64 overflow 閾值比較，得到 `[M,y26]` 已超出可回報有限幅度，而 `[M,y27]` 應回報有限 `M`。對 `[M,y26]`，普通 binary64 scaled 路徑的中間值是 `1 + 2^-52`，`sqrt` 捨入成 `1`，再乘回 `M` 會得到有限 `M`，因而可能把真正不可表示的幅度誤放行；Kahan 對只有兩項的平方和也不能消除此雙重捨入問題。缺少的證據是 Root 實際 Go 實作在 overflow 邊界的行為與 guard；本輪禁止執行 Go，故不能把 Python 的 IEEE 計算直接宣稱為 Go 驗證結果。這一對案例必須先納入獨立參考與公開 Run 驗收，再決定最高端的拒絕條件。

- **[NEEDS INVESTIGATION] `evidence/NAT-06/native-plastic-norm-20261014/plan.md:13,25`：正常有限範圍的「歷史位元相容」與「Kahan 改善累積誤差」尚未以測試凍結。**
  反例為 `[1] + 1024*[2^-27]`。每個 `2^-27` 的平方 `2^-54` 都是正常有限值，沒有平方溢位或下溢；舊按索引加總回 `1.0`，而 Kahan／180 位參考回 `0x1.0000000000080p+0`，差距為 `2.842170943040401e-14`。因此 scaled/Kahan 若全域使用會改變目前正常位元，若只在溢位／下溢 fallback 則會保留此歷史近似。缺少的證據是 Root 要凍結哪一項為本票契約；依 plan 第 13 行「正常範圍維持原本位元結果」，目前較一致的做法是只在已辨識的失敗模式 fallback，並把此案例作為相容性測試，而不是未授權擴大為一般精度政策。

- **[NEEDS INVESTIGATION] `evidence/NAT-06/native-plastic-norm-20261014/plan.md:17,25`：獨立參考的精度與 overflow 比較規則仍需明確保存。**
  `math/big` 256-bit 或本輪使用的 180 位十進位精度足以計算上述小型反例，但「轉成 float64」本身可能在最高端先遇到 overflow，不能只以轉換結果判斷真實值是否落在有限 rounding 閾值內。缺少的證據是公開測試採用的精確 overflow 閾值、rounding mode 與 Go 端比較方式。沒有這份規則前，不應以單次 `math.Hypot` 或單次 `float64(big)` 當唯一判定。

### 對 scaled/Kahan fallback 的結論

scaled 計算搭配補償加總適合作為已確認平方溢位／下溢路徑的主要候選，Kahan 應放在 fallback 內以免改變 plan 第 13 行要求的正常位元。它仍需逐項記錄平方下溢，並在 `M` 附近加入可由獨立參考驗證的最高端 overflow guard。沒有這兩項證據前，不能宣稱「scaled/Kahan + 最後 `checkFinite`」已涵蓋真正不可表示與有限邊界。

## Root 建議測試

- 直接幅度回歸：空列／全零、符號不變、`[1e200,0]`、`[1e-200,0]`、`[2^-537,2^-538]`，並用獨立參考比對非零結果。
- 正常位元相容：`[1,2^-27]` 及 `[1]+1024*[2^-27]`。前者確認極小尾項不被錯誤放大；後者須依 plan 第 13 行凍結舊位元結果，或在改變契約前先取得 Root 裁決。
- 最高端四格：`[M,1]` 應為有限 `M`；`[M,y27]` 應為有限 `M`；`[M,y26]` 應被 Run 以 `plastic l2` 拒絕；`[M,M]` 也應被拒絕。以真正報告 overflow 為條件，不以平方中間值作條件。
- 既有大閘門流程：兩條邊、`plastic_max = 1e200` 應成功並產生有限報告，且神經／快速狀態在成功時提交。另用真實不可表示的兩邊 frozen state 驗證空報告、輸入不變、`State` 與 `PlasticState` 不變。
- 公開流程維持 plan 第 17 行的 continuous/hebbian、LIF/hebbian、LIF/stdp 與 Build → RestorePlasticState → Run；本輪只審查數值，不把 ticket 89 或 pre/post 時點納入。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `runs/.tmp/native-plastic-norm-20261014/review/report.md` | 新增本次唯讀數值與契約審查報告。 |

## Actions

| 動作 | 對象 | 結果 |
| --- | --- | --- |
| 唯讀來源核對 | `plan.md`、ticket 93、`original-*.go.txt`、`simulate`、`plasticity`、`ENG.md` | 完成；未修改來源、測試、證據或舊成果。 |
| Git 狀態查詢 | CoImNet worktree | 初始 `git status --short` 僅見未追蹤的 `evidence/NAT-06/native-plastic-norm-20261014/`；`git diff --stat` 為空。未執行任何 Git 寫入。 |
| Go、網路、服務 | 全部 | 依本任務限制未執行。 |

## Verified

- 確認目前 `simulate/plasticity.go` 與 `original-plasticity.go.txt` SHA256 相同，`simulate/runner.go` 與 `original-runner.go.txt` SHA256 相同，現行測試與 `original-plasticity_test.go.txt` SHA256 相同，故審查基準沒有在讀取期間漂移。
- 確認 `Run` 的幅度檢查位於狀態提交前，故修正幅度計算時可保留既有晚期原子性拒絕。
- 以 Python stdlib `math` 與 `decimal` 做不寫檔獨立數值計算。Decimal 使用 `Decimal.from_float` 取得精確 binary64 操作數，精度 110～180 位十進位，未使用原 `l2Norm` 作為預期值。

## 讀取 SHA256／HEAD

HEAD（依 Root 指定）：`9dbc7aeda009c173c56dfd4b9e3446a402733bb7`

| 檔案 | SHA256 |
| --- | --- |
| `evidence/NAT-06/native-plastic-norm-20261014/plan.md` | `f59559dc62447417d514e933e6dfae7060afc653becce9403025930d213770d9` |
| `evidence/NAT-06/native-plastic-norm-20261014/original-plasticity.go.txt` | `7396988824a604b4fb126470d12675e9dcf196496ce3ae308090a44db9073413` |
| `evidence/NAT-06/native-plastic-norm-20261014/original-plasticity_test.go.txt` | `049c3c97058eb1dcb83ef0a52b00ef9ab07a32aef7889ab0799b5db6b6c88c8c` |
| `evidence/NAT-06/native-plastic-norm-20261014/original-runner.go.txt` | `2fe1741d7f55939ae66a67bca5d10d5c03756bec996e05f8e16a91693502450e` |
| `docs/tickets/93-native-plastic-norm-representability.md` | `e6a5f66b56e9e02ed95934c7fe459c1329e68812eae56f1f25403b61489fd63b` |
| `simulate/plasticity.go` | `7396988824a604b4fb126470d12675e9dcf196496ce3ae308090a44db9073413` |
| `simulate/plasticity_test.go` | `049c3c97058eb1dcb83ef0a52b00ef9ab07a32aef7889ab0799b5db6b6c88c8c` |
| `simulate/runner.go` | `2fe1741d7f55939ae66a67bca5d10d5c03756bec996e05f8e16a91693502450e` |
| `simulate/report.go` | `68c633d0d496c7024a18b7bad89f528edbaffeea0eee2a0c505f70eecc5d78bd` |
| `plasticity/plasticity.go` | `ab73f4e5e448b8299fbd46aa2cf9ac0f81085298b5b0a8c8b865c2e15ae3ebb0` |
| `ENG.md` | `6f1268d51f2961a873caf266d7a86d14bf344922e5ae9882486cef59a434ec62` |

## 未驗證項目與 Notes

- 未執行 Go、`math/big`、測試、build、race、vet、服務或網路，因此 Go runtime／平台對最高端 overflow guard 的實際結果仍未驗證。
- 未讀取 `data`、`models`、`checkpoints`、私人憑證或其他 repository，也未碰 ticket 89、pre/post 時點政策。
- 初始檢查曾列出本輪 `runs/.tmp/native-plastic-norm-20261014` 的檔名以確認 review 目錄；未讀取該目錄下快取或資料內容，後續僅限定讀取 review 子目錄。唯一新增檔案仍是本報告。

## Next

Root 先把部分下溢與 `M` 邊界四格納入永久測試，再依結果實作最小 fallback；在此之前不要把原 `TestPlasticL2OverflowIsRefused` 的錯誤期待直接視為契約。
