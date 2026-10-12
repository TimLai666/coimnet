# 受體位置來源與 MaleCNS 類型映射範例

這個範例將 Fendl、Vieira 與 Borst（2020）觀察到的 Rdl 受體樹突定位，透過明示的細胞類型推論對應 MaleCNS 節點，再比較指定受體區域與舊神經元區域的反應。它不訓練模型，也不是受體反應的定量生物重現。

```sh
go run ./examples/receptorlocation --help
go run ./examples/receptorlocation --store data/malecns-v1.0/graph-v1.coimgraph
```

來源及 GraphStore 只讀取。報告輸出至標準輸出，包含原始觀察、保存物讀回回條、推論映射、未知反應的受體宣告，以及工程數值對照。GraphStore 限制為 1 GiB，圖資料的帳面記憶體限制為 2 GiB，不等於程序峰值記憶體保證。缺少來源欄位、損壞的 GraphStore、不符的命名空間或沒有對應節點皆拒絕。

## 資料與授權

- 原研究：[Conditional protein tagging methods reveal highly specific subcellular distribution of ion channels in motion-sensing neurons](https://doi.org/10.7554/eLife.62953)。作者：Sandra Fendl、Renee Marie Vieira、Alexander Borst。文章依 Creative Commons Attribution（CC BY）授權。
- 原始表：[Figure 3—source data 1](https://cdn.elifesciences.org/articles/62953/elife-62953-fig3-data1-v3.xlsx)。本目錄 `testdata/elife-62953-fig3-data1-v3.xlsx` 保留下載位元組，SHA-256 為 `39e207baafd868e44c4fdac8ecea327c2456cda3191fdec49cd4b734dc62a622`。
- `testdata/fendl-rdl.csv` 逐列轉錄工作表的 A–E 欄與原始列號。E 欄是 Rdl puncta 數量，T4 共 18 筆，T5 共 10 筆。F、G 欄未有欄名，因此不解讀、不轉成距離或濃度。表內標題寫 Figure 4E，但來源下載頁對應 Figure 3，原檔不修改。
- CSV 的列號與樣本 ID 只識別原研究觀察，不能當成 MaleCNS body ID。

## 觀察、推論與工程設定

原研究以 UAS-Rdl::GFP 觀察 T4/T5 樹突上的受體蛋白，仍可能受額外表現量影響。這不是每個 MaleCNS body 的直接量測。範例只把 `NodeRecord.Type` 的 T4a–T4d、T5a–T5d 對應到研究的 T4、T5 類型，標記 `cell_type_inferred`，`T4_unclear` 保留未映射。原研究樣本與 body ID 沒有逐細胞身分對照。

`Receptor.Location.Region=1` 是示範用的化學區域映射。樹突位置來自觀察，區域數字來自工程宣告，兩者分開記錄。後續效果仍由純量神經元整合，不能視為多區室膜模型。

產生的受體宣告保持 `Status=unknown`，`Kd=N=0`。計數不參與反應強度。只有對照另外宣告 `Kd=N=1`、`AllowAssumedCoefficients=true`、`BetaScale=-0.2`、區域濃度 `[0, 1]`，用來驗證位置會影響程式運算。它不表示模型符合真實濃度、受體動力學或行為表現。
