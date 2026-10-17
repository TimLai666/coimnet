# 141 — 合成音訊在配置前拒絕無法表示的樣本尺寸

Epic：音訊資料形狀與錯誤處理

User Story：研究者的合成音訊設定若超過可表示的長度，會取得錯誤，而不是產生被截短的音訊或觸發程式 panic。

Blocked by：138 的凍結來源驗收與交付；139／140 的同型態報告修正先完成。

Status：completed

相關需求：TSK-02 的合成資料 helper 與失敗處理。這次沒有推翻既有正常語音辨識或模型品質證據。

## Root 決策（2026-10-17）：已確認根因與骨架

SynthConfig 的 SampleRate*GapMS 在除以 1000 之前溢位，Validate 沒有檢查衍生的負間隔。Synthesize 再用它配置總長度。Root 的公開探測在 64-bit 平台重現：1000 Hz、DurationMS=1、兩個字元，GapMS=MaxInt 時錯誤回傳 1 個樣本，GapMS=MaxInt-1 時配置零長並越界 panic。獨立大整數期望分別為 MaxInt+2、MaxInt+1 個樣本，必須在配置前拒絕。正常 GapMS=2 的 4 樣本控制通過。

後續先補失敗回歸，再保留正常截斷算式，對可能溢位的乘法與最終尺寸使用安全計算。不能因中間乘積溢位，就拒絕最終可表示的整數樣本數。總樣本數與 float64 配置位元組需在建立切片前檢查。Generate 的相關衍生尺寸須核對同型態風險，只有確認的同根因才納入，不改辨識器、特徵或學習規則。

## 固定完成條件

- [x] 公開重現錯誤長度／panic，與獨立整數參考及正常控制核對。
- [x] Root 凍結失敗回歸，釐清每字／間隔／總長度與 Generate 的適用檢查。
- [x] 最小修正與專項一般／競態通過，正常音訊及合法空集合行為保持。
- [x] 格式、build、完整一般／競態、vet、固定模組及治理通過。
- [x] 保存必要證據、清除自有暫存、提交推送並核對遠端。

## 減法審查

先修正既有尺寸計算，不新增音訊格式、任意固定長度上限或新資料處理層。正常算式與隨機串流保持，避免為防溢位重寫聲音合成。

## 證據

[公開探測與獨立參考](../../evidence/TSK-02/synth-shape-audit-20261017/verification.json)。初次公開探測的 2,993 bytes 暫存已清除，該階段 857 份正式來源保持。Root 已凍結三個父測試、十個子案例，原版一般／競態各 10 fail／3 pass；八種錯誤情境重現，普通合成、單字元未使用間隔與合法空集合控制通過，見[失敗回歸驗收](../../evidence/TSK-02/synth-shape-audit-20261017/shape-red-verification.json)及[邊界核對](../../evidence/TSK-02/synth-shape-audit-20261017/boundary-review.json)。隔離候選的一般／競態各 13 pass，29,121 bytes 正常音訊 payload 的逐位元指紋在各建置內與原版相同，見[候選驗收](../../evidence/TSK-02/synth-shape-audit-20261017/shape-candidate-verification.json)。17,082 bytes 本次隔離暫存已清除，四份必要輸入已保存，見[清理](../../evidence/TSK-02/synth-shape-audit-20261017/shape-candidate-cleanup.json)。報告階段已交付，原候選與凍結回歸已原樣採用，以 860 份最終來源完成完整工程驗證，見[採用紀錄](../../evidence/TSK-02/synth-shape-adoption-20261018/adoption.json)。正式一般／競態各 13 pass，正常音訊及合法空集合保持，見[正式專項驗收](../../evidence/TSK-02/synth-shape-adoption-20261018/focused-verification.json)。格式、build 與完整一般 8,658 pass／23 skip、58 套件已通過，原有 8,668 個終端事件保持，只新增 13 pass，見[Root 核對](../../evidence/TSK-02/synth-shape-adoption-20261018/unit-root-review.json)。完整競態 8,657 pass／24 skip、58 套件及 vet、固定模組、31 項治理與九個工程步驟已通過，見[正式驗收](../../evidence/TSK-02/synth-shape-adoption-20261018/verification.json)。最後 31 項治理通過，1,576,147,099 bytes 本票暫存已清除，860 份來源、18 份模型、6 份資料、32 份其他工作及 15,046 份原始模組保持，見[清理](../../evidence/TSK-02/synth-shape-adoption-20261018/cleanup.json)。修正 `c875fbf` 與主要證據 `5f0e30a` 已正常推送且遠端讀回完全相同，860 份提交來源與驗收指紋相符，見[交付](../../evidence/TSK-02/synth-shape-adoption-20261018/delivery.json)。固定進度 5／5。
