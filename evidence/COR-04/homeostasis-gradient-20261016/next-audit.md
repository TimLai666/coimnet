# 下一個可驗證候選

活動依賴化學與慢速活動穩定並用的完整獨立梯度仍有驗證缺口，不是已確認缺陷。

本票只補固定外部閾值效果與全 LIF 的獨立參考。`learning/chemical_mixed_reference_test.go` 目前拒絕 enabled homeostasis，不能用已有化學 dual reference 宣稱這個組合完成獨立驗證。

下一個最小成果是固定一個有化學活動回授、非零 upstream 與 nominal h 的小圖，以既定停止梯度核對完整訓練更新。先建立手算或獨立 forward tangent 與錯誤反例，再考慮是否有正式實作需要修正。保持既有合法設定與正常模型。
