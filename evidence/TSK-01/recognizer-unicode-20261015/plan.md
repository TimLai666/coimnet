# Recognizer Unicode 建構與保存一致性

## 已確認的問題與邊界

OCR 與 ASR 的公開 RecognizerConfig.Validate／NewRecognizer 接受 -1、U+D800、U+DFFF、U+110000、MaxInt32。兩者 Save 的 validateOCRConfig／validateASRConfig 已明確拒絕這些不是 Unicode scalar 的值。Root 用公開 API 實測十個非法案例，全部建立成功而存檔失敗，沒有寫出 bundle。八個合法案例可以保存，且在新程序恢復後完整訓練快照及設定指紋相同，見 public-reproduction-verification.json。這是 P2 建構驗證缺口，不表示合法 alphabet 的現有模型有品質缺陷。

## 修正骨架

公開簽名及資料格式保持。OCR／ASR RecognizerConfig.Validate 在 alphabet 迴圈共用既有 utf8.ValidRune 規則，讓 NewRecognizer 提早回錯並回傳 nil；OCR NewRecognizerWith 經同一入口拒絕。Save／Load 仍採相同規則，重複的私人驗證改為呼叫公開 Validate，不增加新的限制或自動改寫設定。合法值、初始化、訓練算式、快照欄位與已保存模型的接受集合保持。

正式來源目前固定供 tickets 104／106 驗收。先在 Go overlay 寫失敗回歸與候選，正式採用要等 104 固定分析及 106 來源驗收完成，不能改輸入鎖定或重啟既有程序。

## 檔案責任與測試結果要求

來源候選僅 tasks/ocr/recognizer.go、persistence.go 與 tasks/asr/recognizer.go、persistence.go。測試 agent 只寫 runs/.tmp/recognizer-unicode-20261015/tests/ 內的四份檔案，不得改正式來源或候選實作。

1. OCR recognizer_unicode_test.go，package ocr_test：TestRecognizerAlphabetUnicodeScalar。非法字碼逐一放在第一格與合法 A 後方，檢查公開 Validate、NewRecognizer 與 NewRecognizerWith 都拒絕，建構失敗必須回 nil。With 的 Parameters 從相同 alphabet 長度的合法設定建構，避免以無關 shape 錯誤通過。非法案例為 -1、U+D800、U+DFFF、U+110000、MaxInt32。
2. ASR recognizer_unicode_test.go，package asr_test：同名測試檢查 Validate 與 NewRecognizer。有效小型前端用 16000 Hz、Window 16、Hop 8、Mels 2、FMin 0、FMax 8000、Floor 1e-10、Hidden 2、LearningRate .01、Seed 7。
3. 兩份測試的合法控制：A、NUL、U+D7FF、U+E000、U+FFFD、U+10FFFF。每個單獨 alphabet 都須 Validate／建構成功；不因字碼值大而配置對應大小陣列。Root 的公開保存／新程序恢復控制另外保持。
4. ASR 原 TestNewStreamRejectsInvalidAlphabetRune 目前用接受壞設定的 NewRecognizer 建立 fixture。不能因提早拒絕而拿掉 streaming 的防護測試。private stream_validation_test.go 僅移除該完整函式，其餘原文逐位保持；private stream_unicode_internal_test.go 改用 package asr 的白箱 fixture，先以合法 alphabet 建立 recognizer，再只把 config.Alphabet 改為含 U+D800 的相同長度值，呼叫 NewStream 仍須拒絕。函式名保持 TestNewStreamRejectsInvalidAlphabetRune。

Root 審查凍結四份測試，再以 overlay 在原來源執行一般／競態失敗回歸；合法控制與 streaming guard 須通過。Root 寫獨立候選，不得由實作人員更改凍結測試，候選一般／競態須全部通過。採用後完成格式、build、全套一般／race、vet、固定相依性與治理檢查。保留八份舊合法快照的新程序接受與指紋，以及十八個原模型的指紋。

## 本票不改動

CTC 對損壞 UTF-8 字串轉成 U+FFFD 的行為尚未釐清支援契約，不列為已確認缺陷。learning.rows 的尾端非整除疑慮沒有公開 OCR 可達路徑，不列為確認缺陷。Options 的兩條所有權路徑已由 Root 查證有複製與重設防護，不修改正常演算法。

## 交付

只有工程驗收與真實公開重現對應完成後才恢復受影響需求 passed。保存來源、命令、環境、指紋及結果，清除自有暫存，依既有授權提交／推送並讀回。既有 104、106、舊暫存、資料、模型與共享快取保留到各自完成。
