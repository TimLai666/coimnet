# 95 — 使用者可以安全產生 OCR 人工樣本

**Epic:** 模型輸入契約稽核
**User Story:** 樣本數或字長衍生的儲存大小不能表示時，OCR 產生器與 fixture 在建立資料或訓練前回傳錯誤，保留正常資料與模型格式。
**Blocked by:** 無，不依賴 ticket 89 的增益決策
**Status:** in_progress

- [x] Root 固定樣本容器、文字長度與影像位元組的表示契約及正常／空批次控制。
- [x] 凍結公開失敗回歸，確認 Generate 與 RunOCRFixture 的實際崩潰，不用大記憶體案例推測。
- [x] 最小共用尺寸驗證，公開入口、正常渲染／資料／模型接續與完整工程檢查通過。
- [x] 同步 TSK-01 證據及本輪暫存清理，保留原件。
- [ ] 拆分提交修正與回歸證據，推送、遠端及來源讀回相符。

Root 已以 Generate 公開入口確認 int64 的三個 panic：n=1,minLen=0,maxLen=MaxInt 觸發 IntN 非正數；n=MaxInt,minLen=maxLen=1 的樣本容器被 makeslice 拒絕；n=1,minLen=maxLen=MaxInt 的 rune 容器被 makeslice 拒絕。正常 n=2 與 n=0 的寬範圍控制成功，見[原始觀察](../../evidence/TSK-01/glyph-contract-20261014/probe-observed.json)。

Root 骨架：新增具名的共用尺寸驗證，讓 Generate、Render 與 FixtureConfig.Validate 先檢查宣告與衍生位元組能以 int 表示。樣本元素大小按目前平台計算，文字長度須容許寬度 9L−1、8 列 float64 像素、rune 與 Box 陣列。n=0 保留既有空集合，基本負值／倒置範圍與選項驗證保持。這不是 RSS 保證或任意數量上限，不改正常亂數流、渲染、模型格式或化學增益政策。

反相字距的實際值已確認，但 Options 與 Render 的公開說明對反相範圍不一致，仍屬預期語意待釐清。它沒有混入本票的尺寸修正，也沒有被當成真實辨識率偏低的原因。

減法審查：在共用尺寸入口拒絕已確認的不可表示宣告，避免每個 seed 各自失敗；不新增資源排程器或任意硬上限。

完整一般／race 各 58 套件通過，測試事件為 5,681／5,680 pass，22／23 skip，沒有 fail。gofmt、build、vet、固定相依性及 CLI 80+40 與連續 120 更新的新程序接續相同。28 組完整正常 JSON 位元組及三種實際舊 OCR 快照的新程序載入、辨識與下一次參數更新保持，見[驗收](../../evidence/TSK-01/glyph-contract-20261014/verification.json)及[相容性](../../evidence/TSK-01/glyph-contract-20261014/compatibility-v3.json)。

最後 11 項治理、31 個事件通過。本輪 9,782 個檔案、41,797 個快取連結及 9,239 個目錄，共 1,225,230,613 bytes 已清除，原有目錄及連結指向的原始快取保持，見[清理](../../evidence/TSK-01/glyph-contract-20261014/cleanup.json)。
