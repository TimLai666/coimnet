# Root 審查：文字辨識字碼建構與保存

OCR／ASR 的公開設定驗證接受非 Unicode scalar，既有保存驗證則拒絕，根因是同一規則僅存在於 persistence 的私人 wrapper。公開重現十個非法案例全部建立成功、保存失敗，與真實訓練品質無關。

隔離候選把現有 utf8.ValidRune 規則移到兩個公開 Validate，兩個保存 wrapper 沿用同一入口。公開簽名、合法模型初始化、訓練及保存格式保持。合法 U+FFFD、NUL、代理區前後端點及 MaxRune 沒有被拒絕。ASR streaming 原有拒絕檢查保留，白箱 fixture 先建立合法模型再只改同長度 alphabet，避免新增建構檢查遮蔽 streaming 防護。原外部測試檔只移除被移動的完整函式，其餘逐位相同。

四份測試先審查及凍結。原來源一般／競態各有 22 個失敗事件與 13 個通過事件：20 個非法案例及兩個父測試失敗，12 個合法案例與 streaming 防護通過。候選一般／競態各 35 個事件全部通過，凍結測試保持。公開候選另拒絕十個非法設定，八個合法設定及初始訓練快照指紋與原來源相同。八份舊保存物的一般及競態新程序載入、八份新保存物的新程序載入都保持相同設定／快照指紋，合計 24 個載入控制。

最初兩次 Red 因 overlay 尚未建立而失敗，沒有執行測試，不列入回歸證據。第一次公開候選因 runner 缺少目標父目錄，合法保存控制失敗，不列入相容性通過證據。原始輸出保留，分類收據分別為 regression-setup-failure-classification.json 與 candidate-public-setup-failure.json。正式接受的是 v2 Red／Green 及 candidate-public-compatibility.json 的實際事件。

兩個完整套件的隔離一般／競態測試另各通過 353 個事件。原有 319 個事件及其結果全部保留，新增字碼回歸 34 個事件通過，包含既有訓練後保存、新程序接續與下一次更新的控制。來源、候選與凍結測試指紋保持，見 candidate-full-packages-verification.json。

Root 接受縮小 streaming fixture 的模型尺寸。移除 NewStream 字碼防護的獨立負向對照，在一般／競態都確實讓同一個凍結測試失敗，證明此測試能抓到防護消失，而非被新增的建構驗證擋住，見 stream-guard-negative-verification.json。正常候選與正式來源沒有套用該移除。

815 份正式 Go／module、18 份原模型及既有長程執行保持。小型舊初始化快照共 13,726 bytes 已逐位保存，僅供恢復回歸。正式採用等待 ticket 104 固定分析與 ticket 106 工程驗收結束，再以當時來源重新完整驗證。TSK-01／TSK-02 保持 failed；TSK-02 另須等待 ticket 108 的尺寸缺陷驗收。最新狀態以需求追蹤檔為準，隔離通過不能當作正式修正完成。

減法審查：移除兩個保存 wrapper 的重複字碼迴圈；不新增自動替換無效字碼、保存格式或跨任務共用抽象。損壞 UTF-8 字串的正規化仍是未釐清契約，不在本次修正。
