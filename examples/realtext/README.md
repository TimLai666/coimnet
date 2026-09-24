# 真實英文文字生成範例

這個範例讀取使用者提供的 `coimnet-textgen-corpus/v1` manifest，依 `source` 做完整來源切分，從每份文件取固定小視窗，執行一次可重現的 byte-level text generation 訓練，並輸出訓練前後的 holdout NLL/perplexity、next-token accuracy 與獨立推論結果。

範例只讀取 `--manifest` 指定的資料，不下載資料、不寫入資料、不保存模型或二進位檔。成功時只在 stdout 輸出一份縮排 JSON，失敗時寫 stderr 且不輸出部分報告。

```sh
go run ./examples/realtext --manifest /Users/timlai/Developer/coimnet-data/TSK-11/gutenberg/manifest.json
```

manifest 必須是 `coimnet-textgen-corpus/v1`，`scope.kind` 必須是 `real`，每份文件須提供相對於 manifest 的 `path` 或內嵌 `text`，以及完整的 `license.holder`、`license.terms`、`license.source`。相對路徑由 `tasks/textgen.ReadCorpus` 解析，目錄逃逸與格式錯誤會被拒絕。

報告的 `input.manifest_sha256` 使用 `ReadCorpus` 同一次解析所讀取的 manifest 原始 bytes 指紋；它不是範例另外讀取檔案後自行推測的相等性。`documents[].raw_sha256` 則是 `ReadCorpus` 回傳文件文字的 UTF-8 bytes 指紋。

固定設定如下：

- 來源切分比例 `0.34`、seed `7`，完整 source 不跨 train/test。
- 每份文件取官方 Project Gutenberg START/END marker 之間的固定 8 KiB；沒有 marker 時使用文件開頭並在報告標示 fallback。
- byte vocabulary，model embed/hidden/settle `16/32/3`，learning rate `0.01`，model seed `7`。
- 訓練 3 epochs，window `128` tokens；留出評估使用同一 vocabulary hash。
- next-token accuracy 取留出資料第一份文件的前 1,024 bytes。
- 獨立推論使用固定 prompt、96 tokens、temperature `0.8`、top-k `20`、top-p `0.95`、seed `19`。

## Gutenberg 驗收重現

這個範例使用的外部資料與報告位於 `/Users/timlai/Developer/coimnet-data/TSK-11/gutenberg/`，不放進 repository。來源與授權頁在 `sources.json`，可重現證據在 `evidence/verification.json`。

已驗證的 manifest 指紋是 `6943685a940b7b08d1c64af5d143d91c16bc50746a8ce38f2ada1dd7cf3cd45f`。三份原始檔案指紋如下：

- Alice's Adventures in Wonderland：`01b38ea4c710a84bc18d0bd41271a5a1a92b94e97b2812f4dece97d4a694725e`
- Pride and Prejudice：`3f6bb9d6f78e0293b56acd4714dd68cb7d6d1d293402031ce9d5a216bcaf9d75`
- The Time Machine：`2892e919000e17c83e1dac51b30f4675db50536b644d7579fe8a89bb399a9bdc`

三本書都來自 Project Gutenberg 官方目錄與純文字 URL；`sources.json` 同時記錄 [Project Gutenberg 授權頁](https://www.gutenberg.org/policy/license.html) 與各目錄頁的 `Public domain in the USA.` 聲明。這個聲明只支持美國範圍，沒有延伸成其他司法管轄區的公版判定。

在 `Mac15,12`、Darwin arm64、Go 1.26.5、8 logical CPUs、16 GiB 上，固定 8 KiB 視窗的已核實小型執行完成 390 updates。留出 Gutenberg 1342 的 perplexity 為 `260.18470432283067 -> 14.175226774453042`，前 1,024 bytes 的 next-token accuracy 為 `1/1024 -> 306/1024`。第二次執行移除時間欄位並排序 JSON 後，報告指紋仍為 `37dfb120902935fe746113dd75b56e78ff26e86b64f9d3871a216c70d6fc6539`。

repo 內範例本身以相同 manifest 連續執行兩次，兩份完整 JSON 都是 `bee4ddaeb22318e3ccae0292b291231f9336155fd4a9b271d86d5b89c19d1bf2`。可用以下命令重現：

```sh
go build -o /tmp/coimnet-realtext-example ./examples/realtext
/tmp/coimnet-realtext-example \
  --manifest /Users/timlai/Developer/coimnet-data/TSK-11/gutenberg/manifest.json \
  > /tmp/coimnet-realtext-report.json
```

上述數值是固定小視窗的工程驗收觀察，不代表整本書訓練、英文流暢度、知識、收斂或泛化。訓練後獨立推論仍是片段化文字，README 不把它當品質通過。報告不從 Insyra 或其他 backend log 推論 GPU／accelerator 訓練，也不宣稱 GPU 加速。

目前 repository 的 `examples run textgen --corpus` 會把 task metric 綁在合成中文主詞動詞文法，對 Gutenberg 英文資料在訓練前回報 unsupported subject or verb。這個範例直接使用 `tasks/textgen` 的 `ReadCorpus`、`SplitBySource`、`Examples`、`EvaluateHoldout`、`Model.Step`、`Logits` 與 `Generate`，以英文適用的 source-heldout 指標替代該 task metric；缺口詳見外部 `evidence/runtextgen-api-gap.json`。

本範例的離線測試只在暫存目錄建立小型嚴格 manifest 與文字檔，不依賴網路或外部資料：

```sh
go test -count=1 ./examples/realtext
```
