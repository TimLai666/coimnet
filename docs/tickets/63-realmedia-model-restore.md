# 63 — 使用者可以在新程序讀回影音模型並生成相同內容

Epic：真實任務模型驗收

User Story：使用者可以保存真實影音範例的完整模型、編碼與解碼設定，讀回後不重新訓練，生成相同 PNG／WAV 與時間線。

Blocked by：59 的通用 task training bundle 實作。

Status：功能、完整驗證與暫存清理通過，提交交付待完成。

原始依據：主規格 §13.1／15.3 與 TSK-11。沿用既有 realmedia 模型、授權來源、資料分割與輸出工具，不將同程序新建 Network 當作模型讀回，不要求保留集品質改善。

## 使用流程

來源與輸出設定核對 → 既有訓練 → task bundle 保存完整 config／optimizer → 新程序 Load → 驗證來源與 encoder／decoder／scale／clock → 同模型獨立推論 → PNG／WAV／timeline 與評估。

## 驗收

- [x] bundle 綁定來源 manifest 指紋、時鐘、編碼／解碼／縮放規則、特徵數、seed 與完整訓練狀態。未知、缺失、null、錯形狀與不相符的設定均拒絕，不重初始化。
- [x] 保存恢復後完整參數、動量、次數與預測相同，下一次更新與不中斷路徑逐位一致；子程序實際讀取 bundle。
- [x] CLI --model-input／--model-output 互斥且說明完整。讀回模式 epochs 和 this_run_updates 為零，累積 updates 保持，不為了通過核心變動檢查而重訓。
- [x] 同真實短片和已訓練模型的新程序生成 PNG／WAV／timeline 與數值評分相同，來源與模型不變。MP4 封裝 metadata 不作逐位保證。
- [x] 路徑不得重疊原始資料或輸出目錄，不覆寫既有成果。失敗不留下 partial report，已完成 bundle 不因後續輸出失敗而被刪，錯誤明示部分成功路徑。
- [ ] 完整相關測試及專案驗證通過，來源、命令、日誌與品質負面結果保存，清除不再必要的暫存後交付。

## 檢查責任

範例專用配置和同一組 fit／predict／score／preview 消費者作為驗證入口，不新增通用任務抽象。Root 骨架、獨立 RED、另一 worker 實作、Root 新程序實測與完整審查。

## 驗收證據

2026-10-12：evidence/TSK-11/media-bundle-20261012/verification.json。最終整合命令、日誌與來源指紋見 [91 項驗收](../../evidence/goal-91-20261012/verification.json)。[清理回條](../../evidence/goal-91-20261012/cleanup.json)已保存，提交交付完成後才勾選最後一項。
