# Ticket 95 Root 審查

公開 OCR 產生器在不可表示的字長範圍、樣本容器或影像大小下會 panic。RunOCRFixture 原本先接受同樣配置，之後才在 Generate 崩潰。這是訓練中斷缺陷，沒有證據指出它造成正常大小資料的辨識率偏低。

Root 親自讀取完整正式 diff、兩份新增永久回歸、舊來源快照與 native 審查全文，並執行 RED、GREEN、28 組正常 JSON 對照、三種舊 OCR 模式的新程序載入及下一次更新。原件覆蓋檔與 baseline 25fbc296 的 Git blob 位元組相同，見 baseline-overlay-proof.json。

修正只在兩個正式檔案加入共用算術驗證。Sample 元素大小依平台型別，影像位元組為 64*(9L−1)，同一界線也限制較小的 rune、UTF-8 與 Box 配置並保證 inclusive IntN 的範圍能表示。零樣本保留未使用的寬字長範圍。原有非法宣告與選項、亂數、字形及模型格式保持。

四個新增頂層回歸共 46 個通過事件包含正常／空集合、非法宣告、FixtureConfig.Validate 與 RunOCRFixture 入口、呼叫者設定保持，以及 math/big 直接乘法的獨立 byte 參考。原始公開回歸有 11 個失敗事件；新增 API 的空骨架另有 26 個失敗事件。後者明確使用舊來源加 proposed validator skeleton，不冒充舊版本曾有這個 API。

完成判定只使用 green-current.json、source-final.json 與 tests-frozen-v2.json。20-pass 的 green.json 及較早 manifests 為過程原件。manifest-adjudication.json 以原 SHA 重建早期 46-pass 命令的正確 manifest，原始收據另存，沒有改寫執行結果。兩份 native 審查均未執行 Go，Root 的實跑才是驗收依據。

OpenCode big-pickle 的 int64 溢位判斷與公開實跑矛盾，已拒絕採用，見 opencode-adjudication.json。native gpt-5.6-luna/max 審查沒有確認新缺陷，Root 重算其尺寸推論與各收據 SHA。這些審查不構成所有 bug 已排除的證明。

尺寸驗證只承諾 int 可表示性。Go runtime 的 maxAlloc 與實體記憶體不足不在本票修正保證內。Render 在這項檢查前會轉換 rune，超大可配置輸入的資源語意沒有由本票驗證。Invert 字距維持既有行為，Options 與 Render 說明的反相範圍仍待釐清。ticket 89 的中性化學增益政策尚待使用者相容性決策，正式 effects.go 指紋保持。

減法審查：把各入口需要的同一算術驗證集中，避免每個 seed 重複失敗。不新增任意 RAM 上限、資源排程、保存版本或生物機制，也不把這項輸入拒絕修正當成模型品質改善。

完整一般／race 各 58 套件通過，測試 pass 5,681／5,680，skip 22／23，沒有 fail。最後 11 項治理、31 個事件通過，CLI 80+40 與連續 120 更新接續相同。本輪暫存清理的內容／身分／開啟檔案雙重檢查與保留結果有 cleanup.json，沒有刪除原始資料或模型。完整結果以 verification.json 及各實跑收據為準。

修正 0ac2565 與回歸／證據 e492041 已正常推送並讀回一致。Root 逐項檢查主要提交的全部 804 份來源，SHA256 與驗收來源相同，見 committed-source.json 與 delivery.json。ticket 95 的五項成果均有證據，這不消除待決策的 MOD-04 缺陷。
