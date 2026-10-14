# NAT-06 ticket 93 candidate 修正版唯讀審查

Skills：investigate, diff-inspector

## Problem

本輪 candidate 修正 `l2Norm` 的平方中間溢位、部分平方下溢與接近 `MaxFloat64` 的捨入邊界，同時保留原本正常值的按索引位元結果，並在真正不可回報時沿用 `Run` 的晚期有限值拒絕。Root 另修正新邊界測試的 `math/big` 參考初始化，本審查確認該修正沒有改變數值期待或正式來源。

## Done

已唯讀檢查三個 tracked candidate diff、兩份新增測試全文、NAT-06 frozen manifests、Root 的參考初始化裁決、ticket 93、`ENG.md` 與必要的可塑性驗證路徑。現行 candidate 檔案與 `source-final-v2.json`／`tests-frozen-final-v2.json` 指紋一致。未修改來源、測試、證據或 Git。

## CONFIRMED

- **[P2][已修正] `simulate/plastic_norm_boundary_test.go:61-64`：首輪測試失敗是參考初始化錯誤，不是 production norm 輸出錯誤。** 舊寫法把 53-bit `big.NewFloat(1)` 傳給 `SetMantExp`，會把 receiver 的 256-bit precision 複製成 53-bit。現行寫法先 `SetPrec(256).SetInt64(1)`，再以 receiver self alias 呼叫 `SetMantExp`，保留 256-bit。`reference-initialization-adjudication.md:3-5` 明確記錄首輪唯一的 96 pass／1 fail 與修正界線；現行檔案 SHA256 也與 `tests-frozen-final-v2.json` 及 `source-final-v2.json` 相符。`green-v2.json`／`green-v2.log` 的 97 pass、0 fail，以及用原始來源 overlay 的 `red-v3.json`／`red-v3.log` 失敗，支持這個裁決。

- **[P2] `simulate/plasticity.go:264-277`：部分平方下溢的 fallback 門檻已涵蓋。** 觸發例 `x=2^-537`、`y=2^-538` 中，`x*x` 是最小次正規值而 `y*y` 是零。獨立 Python `Decimal.from_float` 參考得到 norm `0x1.1e3779b97f4a8p-537`，不是舊路徑的 `0x1p-537`。candidate 以 `v != 0 && square < smallestNormal` 計數，且在 `sum < tinySquares*smallestNormal` 時走 scaled／補償加總。`TestNativePlasticNormAdditionalBoundaries` 的 `partial_underflow` 與 frozen `green-v2.log` 均通過。這只建立了代表性案例的正確性，完整門檻限制另列 NEEDS INVESTIGATION。

- **[P2] `simulate/plasticity.go:265-277`、`simulate/plastic_norm_boundary_test.go:43-54`：正常位元相容路徑仍保留。** 沒有平方下溢或溢位且加總有限時，函式直接回傳原本按索引的 `sum` 開根號。`[1] + 1024*[2^-27]` 的獨立 256-bit 參考是 `1 + 128` ULP，而原按索引結果是 `1`，現行 `TestPlasticNormLegacyNormalReduction` 固定此舊位元；`[.1,.2,1e-200]` 也由 `TestPlasticNormReductionIndependentReference` 固定極小尾項不切換正常路徑。這不是一般加總精度政策，也沒有混入 ticket 89 或 pre/post 時點。

- **[P2] `simulate/plasticity.go:298-324`、`simulate/plastic_norm_boundary_test.go:57-87`：最高端 overflow guard 的數學界線一致。** 對 binary64 最大有限值 `M = 2^1024 - 2^971`，round-to-nearest-even 的 overflow midpoint 是 `H = 2^1024 - 2^970`，所以比較 `sum(v²) >= H²` 會把 tie 與上方值判成 `+Inf`，低於 midpoint 的值仍轉成有限值。獨立 Python `Decimal` 計算確認 `M` 加 `ldexp(M,-27)` 低於 `H²` 且 norm round 為 `M`，加 `ldexp(M,-26)` 在 `H²` 以上且 round 為 `+Inf`；直接把 `H` 轉 binary64 也得到 `+Inf`，確認 tie 行為。production 使用 self alias 建立 `limit`，與修正後測試的 256-bit self alias 一致。

- **[P2] `simulate/plasticity.go:302-310`：4260-bit exact square capacity 在目前 64-bit `int` 前提下足夠。** binary64 有限輸入的平方有效指數範圍為 `-2148..2047`，需要 `4196` 個有效位；最多 `2^63-1` 個 slice 項的加總 carry 以 64 位保留，因此 `2*(1024+1074)+64 = 4260` 足以讓每個平方與非負 partial sum 不被 `big.Float` 截斷。`H²` 本身只有約 108 個有效位，這個 precision 也足以保留 midpoint 比較。這個結論依目前 Go 的 64-bit `int`、IEEE binary64 與 `math/big.Float` 預設 round-to-nearest 前提，非對假設性更寬 `int` 平台的宣稱。

- **[P2] `simulate/runner.go:659-667,741-756`、`plasticity/plasticity.go:282-327`：有限 validated input 與晚期原子性契約相容。** `Run` 的快速狀態由 `clonePlasticState` 取得；`RestorePlasticState` 經 `ValidateState` 拒絕非有限值，`StepWith` 每步也拒絕非有限 eligibility／fast change。norm 檢查仍在 `r.state`、`r.plasticState`、measurement 提交前，真正的 `plastic l2` 拒絕回傳空報告，故 candidate 沒有把非有限輸入政策擴到未驗證狀態，也沒有讓晚期拒絕提交部分神經／快速狀態。`TestNativePlasticNormOverflowDoesNotCommit`、原有改寫的 `TestPlasticL2OverflowIsRefused` 與 `green-v2.log` 支持三種 native core／rule 組合的流程結果。

## NEEDS INVESTIGATION

- **`tinySquares` 門檻的全域誤差證明仍缺。** 現有案例證明部分下溢會 fallback，且當 `sum >= tinySquares*smallestNormal` 時每個次正規平方的單項捨入誤差相對於該下界不超過約 `2^-53`。但尚未有針對所有 slice 長度、指數分布及按索引加總誤差的形式界線或 property test，能證明 fallback 被跳過時一定保留「正確四捨五入位元」而不只是測試容許的相對誤差。若本票契約只要求 plan 所述的正常舊位元與極端值可回報，現有 frozen 案例足夠支持候選方向；若要宣稱所有有限輸入的正確捨入，還缺這份證據。

- **4260-bit capacity 的平台與 runtime 證據仍有限。** 本審查用獨立算術驗證位數範圍，未執行 Go 或 `math/big`。缺少的是在允許的目標 Go／平台上對 `SetFloat64`、self-alias `SetMantExp`、`Mul`、`Add`、`Sqrt` 與 `Float64` 的實際 rounding 行為確認；目前只能採用 Root 已保存的 `green-v2`／`reference-initialization-adjudication.md` 證據，不能把本審查說成新一輪 runtime 驗證。

- **最高端 fallback 的效能尚未量測。** `norm >= MaxFloat64/2` 時會多做 scaled pass、4260-bit `big.Float` exact pass 與 square accumulation。這是為避免 double rounding 的必要稀有路徑，但若合法快速狀態經常接近上界，需有 benchmark 才能量化額外成本。正常有限範圍的第一條路徑仍是一趟按索引平方加總。

- **`l2Norm` 的直接非有限輸入不在本輪驗證範圍。** 函式註解限定 validated finite fast changes，production call sites 由 `ValidateState`／`StepWith` 維持此前提；沒有證據支持把 unexported helper 當成可接受 NaN／Inf 的獨立 API。若未來增加其他 call site，需重新檢查此 invariant。

## Scope

候選實際範圍是 `simulate/plasticity.go` 的 l2 計算、`simulate/runner.go` 的拒絕註解、既有 `simulate/plasticity_test.go` 的錯誤 overflow fixture，以及新增的兩份 norm 測試。沒有新增公開欄位、序列化版本、依賴、化學層、ticket 89 或 pre/post 政策。

## Compatibility

正常平方可表示且加總有限的值維持舊按索引結果。只在平方中間值溢位／下溢或接近上界時改走穩定／exact 路徑，故合法 `plastic_max` 沒有被縮小。報告 schema、`checkFinite` 的錯誤契約與成功時的 state 提交位置均未變。

## Performance

正常路徑仍是單次 O(n) 加總。下溢或溢位時需要 scale／補償 pass；最高端 finite／overflow 另外需要 O(n) exact square pass 與固定約 4260-bit `big.Float` 暫存。未執行 benchmark，不能宣稱額外成本可接受於高頻上界工作負載。

## Verified

- 唯讀檢查 `git diff -- simulate/plasticity.go simulate/runner.go simulate/plasticity_test.go` 及兩份新增測試全文。
- 確認現行 `simulate/plastic_norm_boundary_test.go:61-64` 與 `boundary-frozen-v2.go.txt` 完全相同，且現行五個 candidate/test SHA 與 `source-final-v2.json`／`tests-frozen-final-v2.json` 指紋一致。
- 讀取 frozen `green-v2.json`：候選來源 overlay 執行為 exit 0、97 pass events、0 fail events、package pass；讀取 `red-v3.json`：原始來源 overlay 為預期 exit 1，故 baseline RED 與修正 GREEN 的來源界線存在。這些是 Root 保存的實際執行證據，本審查沒有重新執行 Go。
- 以不寫檔 Python stdlib `math`／`decimal` 做獨立計算。`Decimal.from_float` 取得精確 binary64 操作數，precision 5000；未使用原 `l2Norm` 作為預期值。確認 partial subnormal、`M`/`H` midpoint、`-27`／`-26` upper cases 與 4260-bit 位數算術。
- 讀取時未發現本次 candidate／frozen 檔案在同一輪內漂移。最終 SHA 如下。

## Changed

| 檔案 | 變更摘要 |
| --- | --- |
| `runs/.tmp/native-plastic-norm-20261014/review/fix-review.md` | 新增本輪 candidate 數值、狀態契約與參考初始化唯讀審查。 |

實際由本審查新增的檔案只有上列報告。candidate 的來源、測試及證據檔案均未修改。

## Actions

| 動作 | 對象 | 結果 |
| --- | --- | --- |
| 唯讀 diff／來源核對 | candidate 三個 tracked 檔、兩份新增測試、NAT-06 manifests、ticket 93、`ENG.md` | 完成。 |
| 獨立算術 | Python stdlib `math`／`decimal` | 完成，未寫檔。 |
| Go／網路／服務 | 全部 | 依任務限制未執行。 |
| Git 寫入 | repository | 未執行 commit、stash、reset、checkout、merge 或其他寫入。 |

## 讀取 SHA256／HEAD

HEAD：`9dbc7aeda009c173c56dfd4b9e3446a402733bb7`

| 檔案 | SHA256 |
| --- | --- |
| `evidence/NAT-06/native-plastic-norm-20261014/plan.md` | `f59559dc62447417d514e933e6dfae7060afc653becce9403025930d213770d9` |
| `evidence/NAT-06/native-plastic-norm-20261014/original-plasticity.go.txt` | `7396988824a604b4fb126470d12675e9dcf196496ce3ae308090a44db9073413` |
| `evidence/NAT-06/native-plastic-norm-20261014/original-runner.go.txt` | `2fe1741d7f55939ae66a67bca5d10d5c03756bec996e05f8e16a91693502450e` |
| `evidence/NAT-06/native-plastic-norm-20261014/original-plasticity_test.go.txt` | `049c3c97058eb1dcb83ef0a52b00ef9ab07a32aef7889ab0799b5db6b6c88c8c` |
| `docs/tickets/93-native-plastic-norm-representability.md` | `e6a5f66b56e9e02ed95934c7fe459c1329e68812eae56f1f25403b61489fd63b` |
| `ENG.md` | `6f1268d51f2961a873caf266d7a86d14bf344922e5ae9882486cef59a434ec62` |
| `simulate/plasticity.go` | `516c55e1915d91fb5ce6ffd166cb5d1c639050c54c6912c2e2ccdb5a4f360bf8` |
| `simulate/runner.go` | `22b2f0be883a8797540df3b502ebc0178a312d99e34a767874a4e33232660dba` |
| `simulate/plasticity_test.go` | `29ca3dba68dd1a477e0334a94a74d33564af516c417ad6219b66c6f7f25f17e1` |
| `simulate/plastic_norm_test.go` | `143916a0404afbc24c0edbfd678a27ae2137edd82622d21bb84896a90cded06c` |
| `simulate/plastic_norm_boundary_test.go` | `38a33636bcfd2df60532cbfa1eeda09e83dd476b5956eeccf4b90a5028d53882` |
| `evidence/NAT-06/native-plastic-norm-20261014/source-final-v2.json` | `5191c0e6c58dd8943a40d139d1a6fdc22ae36fd38652cd5c0b57a23a34f25378` |
| `evidence/NAT-06/native-plastic-norm-20261014/tests-frozen-final-v2.json` | `af2500e71342dd6ada8a4d0df2f208d864d0e05c0a827a968c1d2f45460b97db` |
| `evidence/NAT-06/native-plastic-norm-20261014/reference-initialization-adjudication.md` | `6aaeababe0187940849b584d9b9c8f1bebba5c84ef4fb7f597605ca1c1dc152c` |
| `evidence/NAT-06/native-plastic-norm-20261014/green-v2.json` | `53ad3ac1a12b68085a149fa666fe723d47c82b607442a1cc492e07f252c6f09b` |
| `evidence/NAT-06/native-plastic-norm-20261014/green-v2.log` | `0593d718b5ec8188fbbe43ae8334296e6dcb0b8438e94b0bef031a72fa9c5be5` |
| `evidence/NAT-06/native-plastic-norm-20261014/red-v3.json` | `0c72012fa47de8f9aacdb101c409eb5685ac924e2a5415e7c9f76d43d0ea99db` |
| `evidence/NAT-06/native-plastic-norm-20261014/red-v3.log` | `930f16d9b582c2508b2351b30bf9ff07fc42caf2840f639c188c9f2f5b1adbca` |

## 未驗證項目

- 本審查未執行 Go、`go test`、build、race、vet 或其他服務；候選 GREEN／原始來源 RED 均引用 Root 已保存的 frozen logs。
- 未做跨平台／不同 Go runtime 的 `math/big` rounding 實測，也未做高 norm 工作負載 benchmark。
- 未對 `tinySquares` 門檻做所有指數與 slice 長度的 property／形式驗證；上列 NEEDS INVESTIGATION 仍保留。
- 未以 unvalidated NaN／Inf 直接呼叫 `l2Norm`，因該 helper 的文件前提是 validated finite state。

## Notes

本報告確認的是目前候選與 Root 修正版測試在已保存證據及獨立算術下相容，不宣稱 zero bug。若 Root 要把「所有有限輸入的正確捨入」列為更強契約，需先補上 `tinySquares` 的完整誤差證明；若契約維持 NAT-06 plan 的代表性 fallback、正常舊位元與真正不可表示拒絕，現有候選沒有本審查可確認的 residual correctness defect。

## Next

Root 可依目前 frozen v2 繼續正式驗證；在擴張到全域正確捨入或高頻上界工作負載前，補做 `tinySquares` property／誤差證明與最高端 benchmark。
