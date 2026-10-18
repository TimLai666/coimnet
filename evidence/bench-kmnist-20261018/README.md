# KMNIST 跨框架量測（2026-10-18）

同一台雲端 VM（Intel Xeon、4 邏輯 CPU、linux/amd64），單執行緒，各自在新程序載入模型後逐張推論 2,000 張平衡選取的 KMNIST 測試圖（split 雜湊相同）。訓練資料為官方訓練集平衡選取的 6,000 張、20 輪、一次一張、學習率 0.002。

| 模型 | 正確率 | 參數量 | 程序 RSS 峰值 | p50 延遲 | p99 延遲 |
| --- | ---: | ---: | ---: | ---: | ---: |
| CoImNet 單字元模式，28 輸入節點、24 隱藏、可訓練 encoder | 71.25% | 23,568 | 57.8 MiB | 0.155 ms | 0.480 ms |
| PyTorch MLP 784-28-24-10（tanh、Adam） | 71.75% | 22,926 | 242.9 MiB | 0.018 ms | 0.041 ms |

判定：CoImNet 沒有勝出。正確率低 0.5 個百分點、p50 延遲約 8.5 倍，只有程序記憶體較少（PyTorch 的 RSS 主要是 Python 加 PyTorch 執行環境本身）。

- CoImNet 載入模型後多出的約 20 MiB 是 Go 垃圾回收的預設餘裕（GOGC=100），不是模型本身：同一量測設 `GOGC=25` 時峰值 41.1 MiB、載入前 39.8 MiB，p50 0.164 ms。
- CoImNet 參數量含 52 個固定的時間常數，可訓練參數 23,516。
- 只在雲端 VM 量過，Mac 與 ubuntu-1 尚未量測；VM 有其他工作同時執行，延遲數值只用於兩者相對比較。

## 推論優化後（同日）

`learning.Network` 快取由 encoder／readout 參數轉成的 float32 張量（來源值逐位元相同才沿用），推論路徑不再建立自動微分紀錄。同一模型重量：正確率不變 71.25%，p50 0.155 → 0.084 ms，p99 0.480 → 0.179 ms，峰值 57.9 MiB（`coimnet-h24-cached.json`、`compare-cached.json`）。仍慢 PyTorch 約 4.6 倍，剩下的時間約一半在 Insyra 的 float32 矩陣乘法（逐值純量迴圈）。改前改後以 1,000／200 張、2 輪訓練的 realocr 報告比對，除程式指紋外逐位元組相同。

## 重跑

```sh
D=data/TSK-11/kmnist O=runs/.tmp/bench
go run ./examples/realocr --data-dir $D --train-count 6000 --test-count 2000 --epochs 20 --hidden 24 \
  --input-profile single-character --encoder trainable --model-output $O/coimnet-h24-te --report $O/realocr.json
python3 scripts/bench/kmnist_mlp.py train --data-dir $D --out $O/mlp-28-24.pt
go build -o $O/benchocr ./examples/benchocr
$O/benchocr --data-dir $D --model $O/coimnet-h24-te --out $O/coimnet.json
python3 scripts/bench/kmnist_mlp.py measure --data-dir $D --model $O/mlp-28-24.pt --out $O/mlp.json
go run ./cmd/coimnet bench compare --candidate $O/coimnet.json --baseline $O/mlp.json
```

`inputs.sha256` 記錄兩個模型與資料檔的指紋；來源為提交 8728c56 加上本次未提交的量測程式（隨本證據一起提交）。
