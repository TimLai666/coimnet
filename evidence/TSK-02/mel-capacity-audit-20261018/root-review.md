# Root 查核

2026-10-18，ticket145 公開重現階段。

- Root 核對 darwin/arm64 Go1.26.5 的 runtime/slice.go:102–117，負長度與 uintptr 乘法溢位於 mallocgc 前 panic。probe 每個極端呼叫先以 big.Int 證明其中一項條件，沒有要求配置仍可表示的巨量緩衝。
- 正式 frontend.go SHA 3827e4f9bf629304c6a97b1426d86deaf9b596dd8cd108d1c9bbbdabc7bbb0fa 與固定 864 份來源相符。MelFilterbank:108–125 沒有先檢查衍生長度與 bytes。LogMel:162–167 先呼叫該配置入口。
- 原程序48329已 exit0，原始 log 的六項壞尺寸均 panic，三項正常／STFT 未使用 Mels 控制成功。Root 逐項解析並核對 log SHA、完整正常 report、輸出 bits 與版本。
- 唯讀 Luna/max mel_capacity_audit_145 的 caller 盤點與目前 recognizer.go:36–66、113–152、198–202，recognizer_dimensions.go:13–63、persistence.go:55–125 及 stream.go:216–219 相符。極端 Mels 會先被模型維度拒絕，Window 不在那些維度公式內。沒有執行模型恢復，不把靜態路徑當成恢復重現。
- 不直接採用 worker 將檢查放進 FrontEndConfig.Validate 的建議。為保留原驗證契約與模型格式，修正骨架限於 MelFilterbank 配置前檢查，LogMel 自然傳遞 error。STFT 的 validateFraming 與未使用 Mels 的行為保持。
- Worker 所說 edges bytes 檢查同時涵蓋外層 bank 不充分：外層元素是 slice header，sizeof([]float64(nil)) 大於 float64，須獨立檢查 Mels×header bytes。每列與 freq 共用 bins×8 邊界。不能只補最先重現的 edges。
- 不增加任意記憶體額度或聚合所有獨立 row 的 int 乘積限制，也不把可表示當成實體記憶體保證。尚無正式修正、失敗回歸或完整驗收結果。

## 減法審查

沿用同一個配置入口回傳錯誤，不增加恢復層、模型版本或重複的外層設定驗證。先保存最小公開重現，正在執行 ticket144 的正式來源保持固定。

## Root 決策（2026-10-18）：失敗回歸與隔離候選

- 凍結兩個公開父測試與一個私有尺寸父測試。Root 把 LogMel 失敗案固定為16個零樣本，並修正 shape 失敗訊息本身可能越界的診斷。worker 原稿保留，最終公開測試 SHA 01214baf8eb82cd5a4e37a0bcc42081dfed532f5b0f11e8cebeda3f9fa27c2ef，尺寸測試 SHA 66584399b51d01e02542fca5c6c5e6868736ae2d4de1228613d21cfdf9832039。
- Root 以60位 Decimal 的 HTK 中點公式獨立核對9個正常權重參考，最大差2.78e-16，小於固定1e-14。32／64位元邊界只做Python整數算術，不宣稱32位元實跑。
- 公開原版一般／競態各7 fail、4 pass，六個子案例均為原配置 panic。未填 guard 的私有骨架各4 fail、4 pass，獨立大整數 oracle 能分辨三種缺少的容量檢查。全部 red 結束後才交付候選實作。
- 候選 SHA 74a250c05544f0bcfb703f7520f43fe13fe9b00318b25c88593b83080e220e1a。Root 審查完整 diff，八個其他函式逐位元組不變，正常濾波算式僅提前取得同一個 bins。
- 候選一般／競態各19 pass，整個音訊套件各100 pass，原81個終端事件全部保持，只新增凍結19 pass。Root重新讀取全部原始日誌與SHA確認，未只採信runner摘要。
- 原版與候選每個建置的三項正常完整record相同，包含輸出float64 bits、FrontEndVersion與完整report，SHA e82cb8a6aca0f4f2821698715f55b6872846b000f5e813a22d89f47b7f2ebfb3。候選六個壞尺寸回傳error、nil output與零report，不再panic。
- 私有尺寸檢查不配置音訊或濾波陣列；錯誤分支fmt.Errorf仍可能配置小型字串，未宣稱zero-allocation。
- 34,128 bytes 隔離工作已清除，864份來源、18模型、6資料、32其他工作、15,046原始模組與兩個既有暫存根保持。ticket144快取仍供原程序45356使用。
- 目前只完成隔離階段，尚未正式採用145，也未完成其完整工程驗收與交付。正式來源指紋及腳本的事後Root錨點另存root-input-fingerprints.json，不把後補錨點冒充成過去命令已檢查的輸入。

## Root 決策（2026-10-18）：原版人工存檔跨程序恢復

原版先保存正常與巨大Window的兩個3-node人工存檔，再由原版／候選的一般／競態四個新程序讀取同一批六份檔案。全部exit0，完整訓練快照SHA dfc7b7f273b32fff4b9c1c576f0ba8e7bfa820399cfabed309969220b74bba93與兩份個別task config保持。正常Features的完整record保持，原版巨大Window會panic，候選回error、nil輸出與零報告，四種狀態的完整snapshot前後都相同。

Root重新解析全部原始JSON與receipt SHA，Luna/max獨立審查確認上述範圍沒有具體誤判。這只支持兩個小型人工存檔的相容性，沒有驗證全部模型、模型推論或訓練品質。實跑為Go1.26.5 darwin/arm64。

六份原始人工存檔共3,405 bytes已封存為1,114 bytes，逐成員SHA核對，僅供恢復測試，不作模型產品。22,051 bytes本次工作已清除，864份正式來源與原模型、資料、原始模組、其他工作均保持。移除沒有保存原檔的格式化前worker hash欄位，只保留實際執行來源a01e8a37...，並明示Root gofmt；沒有改動實跑程式、日誌或結果。
