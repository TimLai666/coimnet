# 145 — 音訊濾波器先檢查衍生配置尺寸

Epic：音訊尺寸與錯誤處理

User Story：公開音訊特徵入口遇到無法表示的濾波器緩衝尺寸時回傳錯誤，避免在配置時 panic。

Blocked by：無，ticket144 已完成固定來源工程驗收與交付。

Status：done

相關需求：TSK-02 音訊前處理與失敗處理。

## Root 決策（2026-10-18）：根因與修正骨架

公開 MelFilterbank 與 LogMel 在 Mels=MaxInt、MaxInt-2，以及 Window=1<<(nativeBits-2)、Mels=1 三種設定均重現 makeslice panic。基本 Validate 全部接受，edges 的加法或 float64 bytes、freq 的 bytes 超出表示範圍。每項呼叫先以獨立 big.Int 證明負長度或 uintptr 乘積溢位，並查核本機 runtime 在 mallocgc 前拒絕。三項正常 bank／LogMel／STFT 未使用 Mels 的控制成功。

修正位置限於 MelFilterbank 的既有 Validate 之後、首次配置之前。先檢查 Mels+2 的加法與 float64 bytes、bins=Window/2+1 的 float64 bytes，以及外層 bank 的 Mels×sizeof(slice header)，各自必須可由原生 int 表示。每列與 freq 共用 bins 邊界。LogMel 經既有 error 分支回傳 nil、零報告與 error。

為安全檢查外層 bank 與各個 byte 邊界，於同檔抽出不配置音訊陣列的私有 melFilterbankBins(FrontEndConfig) (int, error)。公開入口在基本驗證後使用回傳 bins，正常算式保持。先保留無檢查骨架並以獨立 big.Int oracle 驗證失敗，再填入容量檢查。只在 helper 測試巨大但仍可表示的尺寸，公開壞尺寸測試必須先證明 uintptr 溢位或負長度，不嘗試配置實體記憶體。

保留基本 FrontEndConfig.Validate、STFT 只讀 Window／Hop 的契約、正常運算順序、FrontEndVersion 與模型保存格式。NewRecognizer 已拒絕極端 Mels，但 Window 不在其維度公式內；Features 與 stream step 呼叫 LogMel，故應由共同配置入口處理。恢復路徑只做靜態查核，未執行恢復，不宣稱已重現。

不設任意記憶體上限，也不把可表示當成實體記憶體足夠。不因本次錯誤推論模型權重或任務品質已受影響。ticket144 的正式 864 份來源保持固定，先保存重現再做隔離失敗回歸與候選。

## 固定完成條件

- [x] 公開重現三種尺寸在兩個入口的 panic，核對獨立算術與三項正常控制。
- [x] Root 凍結先失敗的回歸，隔離候選一般／競態及正常輸出對照通過。
- [x] 正式採用最小修正，專項一般／競態與既有控制通過。
- [x] 格式、build、完整一般／競態、vet、固定模組與治理通過。
- [x] 保存證據、清除自有暫存、提交推送並核對遠端。

## 減法審查

沿用既有配置入口的 error 傳遞，不新增模型版本、重複驗證層或任意容量限制。

## 證據

[公開重現與 Root 核對](../../evidence/TSK-02/mel-capacity-audit-20261018/verification.json)。六項壞尺寸均 panic，三項正常控制成功，正常輸出 bits 與完整報告保留供候選比較。[Root 審查](../../evidence/TSK-02/mel-capacity-audit-20261018/root-review.md)記錄 worker 建議的採用與不採用理由。3,295 bytes 本次探測暫存已清除，864 份正式來源、18 份模型、6 份必要資料、32 份其他工作與 15,046 份原始模組檔保持，見[清理](../../evidence/TSK-02/mel-capacity-audit-20261018/cleanup.json)。原始公開重現階段為 1／5，正式修正與完整工程驗收仍待處理。

## Root 決策（2026-10-18）：隔離候選通過

公開原版一般／競態各7 fail、4 pass，無檢查尺寸骨架各4 fail、4 pass，候選各19 pass。整個音訊套件各100 pass，原81個結果保持，只新增19 pass，見[候選驗收](../../evidence/TSK-02/mel-capacity-audit-20261018/candidate-verification.json)。每個建置三項正常輸出的位元與完整報告保持，基本Validate與STFT契約不變。34,128 bytes 隔離暫存已清除，原來源、模型、資料、模組與其他工作保持，見[隔離清理](../../evidence/TSK-02/mel-capacity-audit-20261018/regression-cleanup.json)。目前2／5，ticket144 已交付，正式採用、完整工程與交付待接續。

## Root 決策（2026-10-18）：舊存檔相容性對照

兩個小型人工存檔由原版保存，再由原版／候選的一般／競態四個新程序載入同一份檔案，完整訓練快照、task config與正常Features保持。巨大Window由panic改為error，snapshot未變，見[跨程序驗證](../../evidence/TSK-02/mel-capacity-audit-20261018/recognizer-compatibility-verification.json)。此證據只涵蓋這兩個人工存檔，不外推全部模型或訓練品質。3,405 bytes必要輸入已封存供恢復測試，22,051 bytes工作暫存已清除，見[清理](../../evidence/TSK-02/mel-capacity-audit-20261018/recognizer-cleanup.json)。這份對照屬隔離候選階段，正式採用與完整驗收另列。

## Root 決策（2026-10-18）：正式採用

ticket144 已完成交付。原候選與兩份凍結回歸已逐位元組採用，其他 863 份既有來源保持，最終 866 份來源固定，見[採用紀錄](../../evidence/TSK-02/mel-capacity-adoption-20261018/adoption.json)。正式音訊套件一般／競態各 100 pass，隔離候選全部 100 個結果與原有 81 個結果保持，只新增 19 pass，見[專項驗收](../../evidence/TSK-02/mel-capacity-adoption-20261018/focused-verification.json)及[Root 核對](../../evidence/TSK-02/mel-capacity-adoption-20261018/focused-root-review.json)。格式、build 與完整一般 8,709 pass／23 skip、58 套件通過，原有 8,713 個結果保持，只新增 19 pass。正常音訊、九份報告與中性增益控制保持，見[Root 核對](../../evidence/TSK-02/mel-capacity-adoption-20261018/unit-root-review.json)。首輪格式索引舊行號已修正，31 項治理重查通過，失敗紀錄另存。完整競態 8,708 pass／24 skip、58 套件通過，原有 8,713 個結果保持，只新增 19 pass。其餘九項工程步驟均已通過，見[正式驗收](../../evidence/TSK-02/mel-capacity-adoption-20261018/verification.json)與[Root 核對](../../evidence/TSK-02/mel-capacity-adoption-20261018/full-engineering-root-review.json)。最後 31 項治理通過，1,648,123,020 bytes 本票快取與暫存已清除，原模型、資料、模組與其他工作保持，見[清理](../../evidence/TSK-02/mel-capacity-adoption-20261018/cleanup.json)。修正 `647a4e0` 與主要證據 `5fd79b5` 已正常推送，遠端讀回完全相同，866 份提交來源與驗收指紋相符，見[交付](../../evidence/TSK-02/mel-capacity-adoption-20261018/delivery.json)。本票 5／5。
