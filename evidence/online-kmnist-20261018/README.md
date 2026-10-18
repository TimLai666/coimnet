# KMNIST 部署後邊用邊學對決（2026-10-18）

問題：模型訓練完、部署後遇到沒看過的資料，一邊用一邊學，CoImNet 會不會比一般分類器好？

## 設定

- 資料：官方 KMNIST。每類依檔案順序取圖：預先訓練用第 0–599 張，正式資料流用第 600–799 張（10 類輪流，共 2,000 張），挑參數用的資料流用第 800–899 張（1,000 張），測試用官方測試集每類前 200 張（2,000 張）。兩邊的資料流與測試集雜湊相同（`stream_split`、`test_split`）。
- 情境：
  - **新類別**：預先訓練只看標籤 0–4，資料流 10 類混合，一半是沒看過的類別。
  - **圖片反相**：預先訓練看全部 10 類，資料流與「反相測試」的像素都改成 1 − 原值。
- 流程：資料流每張圖先預測、記錄對錯，之後才告訴模型答案並讓它學（prequential 正確率）。資料流結束後再測一次測試集。
- 模型：CoImNet 單字元 OCR（28 輸入節點、24 隱藏、可訓練 encoder，23,568 參數）；PyTorch MLP 784-28-24-10（tanh，22,926 參數）。兩邊都是一次一張、學習率 0.002、梯度裁切 1、固定順序訓練 20 輪，並保留最佳化器狀態接著學。
- 學習方式：
  - **凍結**：完全不更新。
  - **逐筆梯度**：每張圖做一次梯度更新（CoImNet `Recognizer.TrainLine`，PyTorch Adam 一步）。
  - **局部可塑性**（只有 CoImNet）：`learning.OnlineLearner` 只開可塑性，核心所有邊用 hebbian_rate，答對給 +g、答錯給 −g 的閘門，不改基礎參數。g 在挑參數資料流上從 0.01／0.03／0.1／0.3 挑 prequential 最高者：新類別 0.01、反相 0.1（`tune/`）。
- 每種組合 3 個初始值（CoImNet 種子 20260924／1／2，PyTorch 0／1／2），單執行緒，雲端 VM（Intel Xeon）。

## 結果（3 個初始值的平均 ± 標準差）

新類別：

| 學習方式 | 資料流正確率 | 學完後測試 | 舊類別 | 新類別 | 每次更新 |
| --- | ---: | ---: | ---: | ---: | ---: |
| CoImNet 凍結 | 45.7 ± 0.5% | 42.1 ± 0.2% | 84.2% | 0.0% | – |
| PyTorch 凍結 | 45.2 ± 0.1% | 41.8 ± 0.4% | 83.5% | 0.0% | – |
| **CoImNet 逐筆梯度** | **65.0 ± 2.0%** | **57.9 ± 1.1%** | 67.2% | **48.6%** | 3.87 ms |
| PyTorch 逐筆 SGD | 61.4 ± 0.7% | 52.5 ± 1.0% | 64.0% | 40.9% | 0.61 ms |
| CoImNet 局部可塑性 | 43.7 ± 0.3% | 38.4 ± 0.5% | 76.7% | 0.0% | 0.52 ms |

圖片反相：

| 學習方式 | 資料流正確率 | 學完後反相測試 | 學完後原圖測試 | 每次更新 |
| --- | ---: | ---: | ---: | ---: |
| CoImNet 凍結 | 5.1 ± 1.4% | 5.9% | 71.8% | – |
| PyTorch 凍結 | 2.9 ± 1.5% | 4.2% | 70.3% | – |
| CoImNet 逐筆梯度 | 10.0 ± 2.5% | 13.3 ± 2.4% | 60.0 ± 1.9% | 3.69 ms |
| **PyTorch 逐筆 SGD** | **12.2 ± 1.5%** | **16.0 ± 4.4%** | 56.4 ± 3.5% | 0.62 ms |
| CoImNet 局部可塑性 | 5.7 ± 0.3% | 7.4% | 55.7 ± 4.2% | 0.59 ms |

- 新類別情境 CoImNet 逐筆梯度三個初始值的測試結果（56.7／58.9／58.0%）都高於 PyTorch 三個（52.95／53.15／51.3%）。這是梯度更新的比較，PyTorch 也做得到；差距可能來自模型結構（遞迴核心兩步）、損失或最佳化器細節，本次沒有拆開歸因。
- 圖片反相情境兩邊都學不太起來（2,000 張後仍只有 1–2 成），PyTorch 略好但差距在標準差內。
- 局部可塑性兩個情境都沒有幫助：新類別完全學不到（讀出層固定，可塑性只改核心邊），還讓舊類別從 84% 掉到 77%。
- 成本：CoImNet 逐筆梯度每次更新約 3.8 ms，PyTorch 約 0.6 ms。可塑性路徑的預測要 2.9 ms，比 Recognizer 的 0.08 ms 慢約 35 倍，`learning.Individual` 的前向沒有走推論快取。程序 RSS 峰值 CoImNet 143–328 MiB、PyTorch 374–384 MiB，兩邊都把整份訓練集放在記憶體。
- 未涵蓋：其他資料集、更長的資料流、防遺忘方法（回放、EWC）、可塑性規則的其他參數（衰減、上限、只開部分邊）、GPU。

## 重跑

```sh
D=data/TSK-11/kmnist O=runs/.tmp/online
go build -o $O/onlineocr ./examples/onlineocr
for s in new-classes domain; do
  $O/onlineocr pretrain --data-dir $D --scenario $s --model-out $O/coimnet-$s          # 另加 --seed 1、--seed 2
  python3 scripts/bench/kmnist_online.py pretrain --data-dir $D --scenario $s --out $O/mlp-$s.pt   # --seed 1、2
  $O/onlineocr run --data-dir $D --scenario $s --model $O/coimnet-$s --learner frozen|gradient --out R.json
  $O/onlineocr run --data-dir $D --scenario $s --model $O/coimnet-$s --learner plastic --gate G [--stream tune] --out R.json
  python3 scripts/bench/kmnist_online.py run --data-dir $D --scenario $s --model $O/mlp-$s.pt --learner frozen|sgd --out R.json
done
```

`domain` 情境種子 20260924 的預先訓練與 `evidence/bench-kmnist-20261018` 用 `examples/realocr` 訓練的模型 `arrays.bin` 逐位元組相同。`models.sha256` 記錄全部 12 個模型的指紋。
