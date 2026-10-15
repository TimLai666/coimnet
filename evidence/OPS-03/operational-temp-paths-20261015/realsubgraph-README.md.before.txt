# 真實子圖與人工脈衝訓練範例

這個範例用 MaleCNS v1.0 官方註記的 `class == "ALIN"` 選取子圖，再把選入連線接到 CoImNet 的連續核心。所有符合條件的節點及兩端都在子圖內的連線都會保留，沒有額外的 Traced 條件。目前這份來源選出 24 個節點、110 條連線。

程式示範現有 `connectome`、`dynamics` 與 `learning` API 的串接。使用者的其他模型可在自己的專案採用相同 API。此命令的 `real-subgraph` profile 限定為這個 ALIN 範例，會核對 `source_version=v1.0` 與 manifest 列出的三份官方來源 SHA-256。

## 建構與執行

先依根目錄 [README](../../README.md) 的資料下載說明，將 [manifest](malecns-alin.json) 列出的三份官方檔案放在 `data/malecns-v1.0/`。從 repo 根目錄執行：

```sh
go build -o bin/coimnet ./cmd/coimnet
run_dir=$(mktemp -d)
./bin/coimnet data import \
  --manifest examples/realsubgraph/malecns-alin.json \
  --max-memory-bytes 2147483648 --sort-buffer-bytes 536870912 \
  --out-store "$run_dir/alin.coimgraph" > "$run_dir/import.json"

go run ./examples/realsubgraph \
  --store "$run_dir/alin.coimgraph" \
  --expected-predicate-hash 4565f8edcd638de884b6ecbf90b6300d478772ef4b5ff8407b69fb976490d063 \
  --profile real-subgraph --updates 20 > "$run_dir/probe.json"
```

匯入會核對三份原件的指紋，保存圖檔時拒絕覆寫。後續執行範例只需要這份子圖，無須重新掃描原始 weights。圖檔保留的原件路徑只在另行使用 `StreamRawSegments` 時才需要。

選取 hash 與圖檔的報告不符時，範例會拒絕訓練。其他子圖可在使用者專案調整選取及來源條件，再重新匯入並核對報告。`--profile fixture` 供離線人工測試使用，輸出會保留該標記。

## 訓練設定與結果意義

這是固定四步人工訊號：第一步輸入正或負 0.25，後三步為零，答案分別為正或負 0.2。兩個樣本交替更新，只訓練核心連線權重。輸入與讀出映射、偏置及時間常數保持固定。

核心使用 tanh、DT=0.1、tau=1 及零延遲，均為數值假設。每條初始權重是原始非負 weight 除以該目標節點的輸入 weight 加總，再乘 0.1；分母為零時初始化為零。這個初始化沒有從傳導物質推斷興奮或抑制，也沒有為學習加入符號約束。

JSON 報告保存選取條件、來源指紋、外部神經元 ID 對照、模型設定、參數指紋、逐次更新及前後損失。兩個人工樣本同時用於更新與前後比較，沒有保留測試集。損失下降只用來觀察這次短訓練，不能當成泛化能力或果蠅行為成績。

程式只讀圖檔並輸出報告，不保存模型參數。原始圖與學習參數由不同物件持有。

## 範圍與錯誤

範例限制為 1–2,000 節點、1–100,000 條邊、1–1,000 次更新。讀取圖檔上限為 64 MiB、footer 1 MiB、帳面記憶體 128 MiB。帳面限制不等於程序 RSS，也不涵蓋整個訓練器。

缺失或負 weight、重複 pair、空圖、零邊、超限、錯誤來源或選取條件、損壞檔案、取消及輸出失敗都會返回錯誤，程式不會自動裁切圖。使用 `go run ./examples/realsubgraph --help` 查閱命令。

實際選取、獨立稽核與 Mac／Ubuntu 執行證據見 [驗證紀錄](../../evidence/real-subgraph-20260914/verification.json)，耗時與記憶體見 [資源說明](../../docs/resources.md)。每台主機內重跑一致，兩平台最後的參數指紋不同，未承諾跨平台逐位元相同。
