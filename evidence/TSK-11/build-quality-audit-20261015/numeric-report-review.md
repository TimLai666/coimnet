# Numeric report review

## 審查範圍與方法

只以 AST 從 `evidence/TSK-11/build-quality-audit-20261015/analyze.py.txt` 抽出 `numeric_arrays`、`array_comparison`、`score_pair`，在記憶體內用 `math.hypot`、`math.nextafter`、`struct.pack` 與獨立手算比對。沒有執行完整分析、Go 程式、訓練、下載或讀取模型快照。

## CONFIRMED

### C-01 `difference_l2` 對有限差值會下溢或溢位

- **位置**：`analyze.py.txt:83-86`。實作先算 `diff = [v - u ...]`，再以 `math.sqrt(math.fsum(v * v ...))` 求 L2。
- **輸入與實際結果**：
  - `a={"p":[0.0]}`、`b={"p":[1e200]}` 都是有限值。回傳 `max_absolute_difference=1e200`、`different_float64_bits=1`，但 `difference_l2=inf`。
  - `a={"p":[0.0]}`、`b={"p":[math.nextafter(0.0, 1.0)]}`，差值為最小正次正規數 `5e-324`。回傳 `max_absolute_difference=5e-324`、`different_float64_bits=1`，但 `difference_l2=0.0`。
- **獨立預期**：單一元素的 L2 應等於該元素差值。`math.hypot(1e200)` 為 `1e200`，`math.hypot(5e-324)` 為 `5e-324`。
- **影響**：距離報告會把有限的非零差異錯報成無限大或零，會扭曲數值差異排序、閾值判斷及跨建置比較。
- **實際模型影響**：本輪未讀取模型數值，尚未量測 23568 個參數或 optimizer 陣列是否含有這些極端差值。

### C-02 `numeric_arrays` 會靜默捨棄混合陣列中的數值

- **位置**：`analyze.py.txt:65-74`。列表只要含有非 `int`／`float` 元素，就遞迴處理；列表中的直接數值元素不會再被收集。
- **輸入與實際結果**：
  - `a={"p":[1.0,[2.0]]}`、`b={"p":[9.0,[2.0]]}`。兩邊都只收集 `('.p[1]', [2.0])`，`array_comparison` 回傳零差異。
  - `a={"p":[1.0,"x"]}`、`b={"p":[9.0,"x"]}`。兩邊都收集不到任何陣列，回傳 `{}`。
- **獨立預期**：第一個案例至少有 `1.0` 對 `9.0` 的一項差異；第二個案例也不應把數值差異靜默當成沒有可比較資料。若混合型態不屬合法輸入，函式應拒絕，而不是回傳相等結果。
- **影響**：不符合純數值陣列形狀的資料可能漏報差異，且不會觸發形狀或型別錯誤。
- **實際模型影響**：`protocol.json` 沒有提供參數 JSON 的陣列同質性契約，本輪未讀模型快照，尚不能判定實際 23568 個參數是否會走到此分支。

### C-03 路徑字串未跳脫，會讓不同陣列互相覆寫

- **位置**：`analyze.py.txt:67-74` 產生路徑，`analyze.py.txt:76-77` 以 `dict(numeric_arrays(...))` 收集結果。
- **輸入與實際結果**：
  - `a={"a.b":[1.0],"a":{"b":[2.0]},"filler":[0.0]*23566}`。
  - `b={"a.b":[1.0],"a":{"b":[3.0]},"filler":[0.0]*23566}`。
  - 兩邊 `sum(len(v) for _,v in numeric_arrays(...))` 都是固定的 `23568`，但兩個不同陣列都產生 `.a.b` 路徑；轉成 `dict` 後只保留後者，`array_comparison` 回傳 `.a.b` 的 `different_float64_bits=0`、距離皆為零。
- **獨立預期**：巢狀 `2.0` 對 `3.0` 應被報為一項 bit 差異及距離 `1.0`。
- **影響**：即使 23568 項總數檢查通過，路徑衝突仍可使一整個葉陣列不被比較。`120000` 更新狀態的長度與計數檢查不能補足這個覆蓋問題。
- **實際模型影響**：尚缺參數物件的實際 key 集合及 key 字元限制；目前不能判定模型 schema 是否允許 `.`、`[` 或 `]`。

### C-04 `score_pair` 對長度不符輸入會靜默截斷

- **位置**：`analyze.py.txt:140-142`。所有配對統計都使用 `zip`，沒有檢查 `len(pa) == len(pb) == len(truth)`。
- **輸入與實際結果**：`truth=[0,1,2]`、`a={"predictions":[0,1],"correct":2,"accuracy":1.0,"mean_cross_entropy":0.0}`、`b={"predictions":[0,2],"correct":1,"accuracy":0.5,"mean_cross_entropy":1.0}`。回傳 `b_minus_a_accuracy_percentage_points=-33.333333333333336`，並只計算前兩筆的 discordance。
- **獨立預期**：這組資料缺少第三筆預測，應先拒絕，不能產生百分點與分類轉移統計。若只比較已存在的兩筆，百分點分母也應是 2，而不是 `len(truth)==3`。
- **影響**：函式一旦被重用、驗證閘門被繞過，會產生看似合法但分母和配對數不一致的品質報告。
- **目前呼叫路徑限制**：主流程在 `analyze.py.txt:119-122` 先以 `verify_score` 檢查 predictions 長度，再於 `analyze.py.txt:153` 呼叫 `score_pair`。因此本缺陷的實際模型影響尚未在本輪觀察到，僅確認一般函式契約沒有自我防護。

## 固定 23516／23568／120000 契約檢查結果

程式碼本身有明確的每個 arm 閘門：`analyze.py.txt:130` 檢查 `updates == 120000`，`analyze.py.txt:133` 檢查 nominal trainable parameters `23516`，`analyze.py.txt:134` 檢查所有 parameter leaf 長度總和 `23568`，`analyze.py.txt:135` 檢查三個 optimizer 陣列各為 `23568`，`analyze.py.txt:136` 檢查 `steps` 中 `120000` 有 `23516` 筆、`0` 有 `52` 筆。這些數量在純函式審查中沒有發現算術 off-by-one。

但 `23568` 的涵蓋範圍依賴 `numeric_arrays` 的路徑與型別處理。C-02 的漏收集和 C-03 的路徑覆寫都能讓總數檢查與實際逐元素比較脫鉤；因此「總數正確」不能單獨證明每個參數都被比較。

## 已驗證且未發現缺陷的控制項

- 一般有限值相同時回傳零差異。
- `0.1` 對 `0.10000000000000002` 會回報 1 個 float64 bit 差異及 `1.3877787807814457e-17` 的距離。
- `-0.0` 對 `0.0` 會回報 1 個 float64 bit 差異，但數值距離為零。這符合 bit-level comparison 的既定語意。
- 長度不同的純數值 leaf，例如 `[1.0]` 對 `[1.0,2.0]`，會以 `AssertionError` 拒絕。

## NEEDS INVESTIGATION

- **數字型別語意**：`decode_snapshot` 在 `analyze.py.txt:61-63` 將一般 JSON 整數保留為 Python `int`，`numeric_arrays` 又接受 `int`。`2**53` 對 `2**53+1` 的 `struct.pack('>d', ...)` bit 相同，但 `max_absolute_difference` 為 `1`；需確認比較契約要採 JSON 整數精確值，還是先轉成 float64。若允許超過 float64 範圍的 Python 整數，`10**400` 會在 `math.isfinite` 直接 `OverflowError`，不是明確的契約拒絕。
- **實際 schema**：需確認 23568 個參數陣列是否保證純 `int`／`float` leaf，且所有 dict key 不含會與路徑語法衝突的字元。這些事實不在 `protocol.json` 中。
- **極端數值分布**：需從已產生的 ordinary／race 快照量測參數、optimizer `first`／`second` 與 `steps` 的最小非零差值及最大差值，才能判定 C-01 是否已影響本次模型報告。
- **score 長度保證**：需確認所有品質報告消費者都只能透過已執行 `verify_score` 的主流程呼叫 `score_pair`，否則 C-04 應改成函式內明確拒絕。

## 未驗證項目

- 沒有執行主分析或讀取任何模型／私人資料，因此沒有本次 ordinary／race 實際數值差異的重新量測結果。
- 沒有驗證下游是否以 `difference_l2` 作為門檻或排序依據。
- 沒有修改測試、框架、訓練程式或資料，也沒有 commit、stash 或 reset。
