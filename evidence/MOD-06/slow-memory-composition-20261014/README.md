# 慢速記憶組合驗證

慢速記憶寫入前會核對共同有效權重，避免鞏固成功後個體無法繼續運算。正常公式、固定符號下限、兩次掃描與模型格式保持。正式修正、完整一般／race、合法逐位相容及 CLI 接續通過，本輪暫存已清除。交付紀錄完成後以 delivery.json 為準。

檢查範圍是三種 CPU 人工核心及數值／保存行為，沒有量測真實任務準確率或生物校準。完整一般與 race 各 58 套件通過，race 的既有大型模型包測試因記憶體負擔跳過，一般已實測。其他跳過項目完整列在 suite-summaries.json。

主要結果見 [Root 審查](root-review.md)、[驗證](verification.json)、[清理](cleanup.json)。指標共用與恢復缺少交叉檢查的三組診斷觀察到原缺陷，尚未修復，不計為通過的機制。ticket 89 的增益相容性決策保持 pending。

## 重現

所有新快取與暫存放專案的 runs/.tmp 內。正常回歸命令為：

```sh
go test -count=1 -run '^TestConsolidate(RejectsUnrepresentableEffectiveComposition|FiniteCompositionMatchesIndependentReference|FixedSignFloorRetainsFiniteIntegration|CompositionUsesSparseCoreEdgeIndex)$' ./learning
go test -count=1 -run '^TestEdgeMetadataUsesEnabledPosition$' ./plasticity
```

完整命令、環境及 SHA256 在各 JSON receipt，最終 Go／模組來源在 source-final.json。重現 RED 時用 baseline-consolidation.go.txt 作舊來源並採 overlay-old-source.json 映射。映射中的本機路徑須改到重現者專案的 runs/.tmp，不覆寫正式來源。compatibility-probe.go.txt 與兩份 compatibility overlay 用於三個獨立程序的 54 組逐位比較。

public-probe-v4.go.txt 是原始六案例探針，前三份較早來源只記錄診斷建構錯誤。所有 .go.txt 是保存的來源，先複製到自己的 runs/.tmp 中恢復 .go 副檔名再執行。Rule 所有權／恢復的 .go.txt 診斷以測試私有 compositionIndividual helper 建立個體，使用對應 overlay 及一次只選該診斷的命令。診斷預期並捕捉舊 bug，exit 0 不表示缺陷已修復。

大型日誌以 gzip 保存，receipt 的 log_sha256 指原始解壓後位元組。cleanup-inventory.json.gz 記錄刪除前兩次內容／指紋核對及連結目標觀察，沒有執行期間的歷史寫入保證。CLI 快照是人工 delayed 恢復證據，不是發布模型。
