# 62 — 使用者可以載入圖片、辨識整頁並比較編碼器訓練

Epic：完整 OCR 任務

User Story：使用者可以把 PNG／JPEG 送入同一行模型，按閱讀順序取得頁面文字，保存模型後繼續推論或訓練。

Blocked by：59 的完整 OCR 模型保存恢復實作。

Status：功能、完整驗證與暫存清理通過，提交交付待完成。

原始依據：主規格 §13.2。沿用既有 CTC、行模型及單欄水平分行，不引入外部辨識器或新依賴，不宣稱任意版面或自然文字品質。

## 使用流程

有來源的 PNG／JPEG → 有上限的讀取與解碼 → 灰階、墨色與縮放 metadata → 單行模型，或分行／裁切／縮放 → 同一模型逐行辨識 → Bounds／ReadingOrder／Text → 保存完整模型 → 新程序讀回。

固定編碼器仍為預設。可訓練模式與固定模式具有相同 seed、初始參數、核心與讀出容量、資料和更新預算，報告編碼器容量與空輸出比例，不要求可訓練模式一定勝出。

## 驗收

- [x] PNG／JPEG、alpha／灰階／RGB 解碼及 SHA-256、原始／輸出尺寸、座標、縮放與方向可核對。只支援 stored-pixel-order、水平由左至右，不宣稱 EXIF／ICC 轉換。
- [x] 缺失、破損、不支援格式、nil／取消 context、encoded／pixel 上限及縮放溢位返回零結果與錯誤，配置前檢查容量。
- [x] 多行頁面逐塊文字與直接同一模型辨識裁切內容相同，Bounds 和閱讀順序保留，空白頁面回空非 nil Blocks，模型和來源影像隔離。
- [x] 圖片缺像素、非有限值、非法 threshold／background 或尺寸溢位不 panic。修正既有 SegmentLines／Stack／Crop 的相關公共邊界。
- [x] 固定 encoder 不更新，可訓練 encoder 有完整梯度。保存恢復 mode、optimizer 與下一步一致，舊缺省 false 的 v1 模型仍能載入。
- [x] 空輸出計數與比例只計完全空字串，保留原 CER 規則。真實 KMNIST 1,000 train／200 test、1 epoch 固定配對比較兩模式，保存新程序推論、容量與品質結果。
- [ ] realocr --encoder fixed|trainable 的說明、report、模型設定比對完整，未知模式不輸出半套成果。完整相關檢查與專案驗證通過，證據保存後清除本票暫存。

## 檢查責任

圖片與頁面公共入口、Recognizer 的 TrainLine／Evaluate／Save／Load 及 realocr CLI 為驗證入口。Root 骨架 → 獨立測試 RED → 分開填實作 → Root 完整審查與驗證。

## 驗收證據

2026-10-12：evidence/TSK-11/ocr-raster-encoder-20261012/verification.json。最終整合命令、日誌與來源指紋見 [91 項驗收](../../evidence/goal-91-20261012/verification.json)。[清理回條](../../evidence/goal-91-20261012/cleanup.json)已保存，提交交付完成後才勾選最後一項。
