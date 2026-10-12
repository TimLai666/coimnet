# 受體位置與一般化學訓練驗收

本證據對應 [ticket 65](../../../docs/tickets/65-receptor-location-and-chemical-training.md)。受體位置決定讀取的區域濃度，一般訓練的前向與反向涵蓋神經活動、來源釋放、濃度、受體反應及神經效果。最終工程證據是 [verification.json](verification.json)。本票已完成 5／5，實作 `9ed31a5` 已推送並核對遠端，見 [delivery.json](delivery.json)。本輪暫存已清除，見 [cleanup.json](cleanup.json)。

## 已驗證的能力

- CPU 純量連續、LIF 與混合核心的一般化學訓練。神經活動對來源釋放的回授會進入訓練梯度，控制器、受體與化學傳輸係數在此路徑固定。
- CPU 純量連續／LIF 化學分段重算、新程序接續、部分累積梯度與最佳化器。重播使用私有來源視窗與化學狀態，沒有重複提交持續狀態。
- 可選位置與來源狀態的舊格式相容、嚴格欄位／數值檢查、深複製、取消與錯誤原子性。未支援的 GPU、向量、PPO、混合重算及保存狀態重算組合會在更新前拒絕。
- 完整一般與 race 各 58 個套件、格式、build、vet、相依性、CLI 新程序接續位元組比對通過。最後治理／索引／隱私 11 項通過。772 份 Go／script／module 檔案及兩份 evidence Go 程式的指紋吻合。

## 實測來源與限制

[Fendl 等人 2020 年研究](https://elifesciences.org/articles/62953) 的 Rdl 是 GABA 受體。範例保存 CC BY 原始表、28 筆 T4／T5 樹突觀察與來源指紋。MaleCNS 全圖讀取包含 165,122 個節點、25,563,197 條邊；13,580 個節點依細胞類型對應，4 個不明類型排除。這是類型推論，沒有逐節點的受體量測。

`Location.Region` 影響濃度選擇。`Compartment` 保存原研究部位，沒有各部位獨立的膜電位或細胞內訊號。來源的單通道釋放率目前進入各區域。反應係數是明示的工程設定，觀察數量不轉成受體密度或反應強度。此票沒有量測生物定量正確性、完整圖化學訓練成本或 OCR 品質改善。

## 重現

實際命令、離線快取前提與 Go 環境見 `verification.json`，原始 exit code 與起訖時間見 `full/full-verify-v5.receipt.json`。完整命令使用 `GOFLAGS='-p=2 -timeout=60m'` 執行 `bash scripts/verify.sh`，延長既有套件的累計時間上限，沒有減少測試或斷言。`full/validation.log.gz` 保存完整輸出；`full/artifacts.json` 同時列出壓縮檔與原文指紋。

實測定位範例與原表在 `source/` 及 `examples/receptorlocation/`。完整圖原件不進 Git；重現時須提供相同指紋的圖檔。範例只讀圖檔，不驗證網路磁碟保存耐久性。

## 證據組織

| 路徑 | 內容 |
| --- | --- |
| `location/`、`source/` | 位置 RED／GREEN、實測原表與完整圖來源映射 |
| `full/`、`governance/`、`target-flow/` | 最終來源的完整驗收、治理及答案僅進損失檢查 |
| `extreme-gradient-candidate/`、`fresh-process-candidate/` | 非零梯度下溢修正與跨程序接續的 RED／GREEN |
| `training-candidate/`、`candidate-source-audit.json` | 歷史候選與最終來源的逐檔差異 |
| `race-profile/`、`suite-history/`、`early-history/` | 中斷／失敗、配置與編譯差異的調查紀錄；不當成已通過的最終驗收 |
| `review.md` | Root 審查、範圍與分工工具限制 |
| `cleanup.json`、`delivery.json` | 完成後的本輪暫存清理與實際交付回條 |

減法審查：沿用既有 runtime、核心與最佳化器；保留關閉機制的簡單參考，先以同一真實任務比較開關效果，再決定需要哪些額外生理細節。
