# 本輪逐檔變更

| 檔案 | 變更摘要 |
| --- | --- |
| [AGENTS.md](../../../AGENTS.md) | 移除兩項已解決提醒，記錄鞏固拒絕報告的 P2 計數缺口 |
| [ENG.md](../../../ENG.md) | 保存有限候選、時鐘生命週期、前置契約與相容性決策 |
| [delivery-status.md](../../../delivery-status.md) | 更新本輪進度、限制及交付證據 |
| [docs/individual-state.md](../../../docs/individual-state.md) | 校正化學訓練、重設與網路資料夾保存入口 |
| [docs/model-and-mechanisms.md](../../../docs/model-and-mechanisms.md) | 依 CPU 小圖證據修正局部機制狀態 |
| [docs/requirements-status.json](../../../docs/requirements-status.json) | 附加七項相關需求的實際稽核證據，不改需求定義 |
| [docs/tickets/67-individual-chemical-report-finite.md](../../../docs/tickets/67-individual-chemical-report-finite.md) | Ticket 67：研究者可以安全取得持續化學推進的有限報告 |
| [docs/tickets/68-consolidation-after-optimizer-reset.md](../../../docs/tickets/68-consolidation-after-optimizer-reset.md) | Ticket 68：研究者可以在重設最佳化器後繼續鞏固新經驗 |
| [docs/tickets/69-mechanism-preflight-validation.md](../../../docs/tickets/69-mechanism-preflight-validation.md) | Ticket 69：研究者可以在建模前拒絕不合法的機制宣告 |
| [docs/tickets/70-ocr-pixel-range-validation.md](../../../docs/tickets/70-ocr-pixel-range-validation.md) | Ticket 70：使用者可以在 OCR 輸入時拒絕超界像素 |
| [docs/tickets/71-asr-finite-frontend.md](../../../docs/tickets/71-asr-finite-frontend.md) | Ticket 71：使用者可以在語音前處理時拒絕無效數值 |
| [docs/tickets/72-consolidation-finite-commit.md](../../../docs/tickets/72-consolidation-finite-commit.md) | Ticket 72：鞏固記憶只提交有限狀態與可保存報告 |
| [docs/tickets/73-individual-episode-clock-overflow.md](../../../docs/tickets/73-individual-episode-clock-overflow.md) | Ticket 73：訓練與鞏固回合時鐘不回繞 |
| [examples/realocr/main.go](../../../examples/realocr/main.go) | 範例輸入驗證沿用有限 [0,1] 契約 |
| [learning/chemistry_report_finite_test.go](../../../learning/chemistry_report_finite_test.go) | 化學報告溢位、八種核心／來源／閘門組合與失敗原子性回歸 |
| [learning/consolidation.go](../../../learning/consolidation.go) | 先檢查有限候選及穩定範數，再原子提交記憶 |
| [learning/consolidation_finite_test.go](../../../learning/consolidation_finite_test.go) | 鞏固大值、穩定範數與拒絕後完整狀態回歸 |
| [learning/consolidation_reset_test.go](../../../learning/consolidation_reset_test.go) | 已／未寫入重設、新回合、取消與保存恢復回歸 |
| [learning/episode_clock_test.go](../../../learning/episode_clock_test.go) | 最大回合拒絕、合法邊界與舊標記快照相容回歸 |
| [learning/individual.go](../../../learning/individual.go) | 拒絕化學報告與回合溢位，重設時保留鞏固去重語意 |
| [learning/network.go](../../../learning/network.go) | 共用建構入口拒絕向量非零固定符號 |
| [learning/trainer.go](../../../learning/trainer.go) | 移除共用入口已負責的重複固定符號判斷 |
| [learning/vector_sign_validation_test.go](../../../learning/vector_sign_validation_test.go) | 向量固定符號拒絕與純量／全零／缺省合法回歸 |
| [modulation/config.go](../../../modulation/config.go) | 拒絕來源宣告通道以外的非零 timeline／replay |
| [modulation/config_test.go](../../../modulation/config_test.go) | 修正成功 fixture 到既有單通道契約，不放寬斷言 |
| [modulation/source_channel_validation_test.go](../../../modulation/source_channel_validation_test.go) | 錯位 timeline／replay 在 Build／Validate 拒絕及合法來源回歸 |
| [tasks/asr/audio/frontend.go](../../../tasks/asr/audio/frontend.go) | 拒絕非法 Floor、非有限樣本與溢位特徵 |
| [tasks/asr/audio/frontend_finite_test.go](../../../tasks/asr/audio/frontend_finite_test.go) | 非有限音訊、運算溢位、Floor 及合法特徵回歸 |
| [tasks/ocr/input_profile.go](../../../tasks/ocr/input_profile.go) | 三種圖片模式驗證有限 [0,1] 像素 |
| [tasks/ocr/input_range_test.go](../../../tasks/ocr/input_range_test.go) | 所有公開圖片入口的非法像素拒絕、0／1 邊界與訓練快照不變回歸 |
| [tasks/ocr/page.go](../../../tasks/ocr/page.go) | 頁面分行驗證有限 [0,1] 像素 |
| [tasks/ocr/recognizer.go](../../../tasks/ocr/recognizer.go) | Columns 入口驗證有限 [0,1] 像素 |

新增證據的逐檔清單與 SHA-256 見 [artifact-manifest.json](artifact-manifest.json)。該索引排除自身，避免循環指紋。刪除的暫存逐檔清單見 [cleanup-inventory.json.gz](cleanup-inventory.json.gz)。
