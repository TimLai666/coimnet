# Root 新參考計算修正

首輪 candidate green.json 是 96 pass、1 fail。唯一失敗為 TestPlasticNormRoundingThreshold 的參考前置檢查 exponent=-26，不是正式 norm 輸出檢查。/usr/local/go/src/math/big/float.go:319-329 的 SetMantExp 先 z.Copy(mant)，將 big.NewFloat(1) 的 53-bit 精度複製至原先 256-bit receiver，故參考中點相減捨入不正確。

只修正初始化為 SetPrec(256).SetInt64(1) 後 self alias SetMantExp，保持精度。沒有改閾值、預期結果、容許誤差或正式 candidate。原件 boundary-frozen.go.txt、tests-frozen-v2.json 與 red-v2/green 原始結果保留。Production 已使用 self alias，沒有同一問題。新參考及凍結回歸仍要在原來源 overlay 先確認失敗，再執行正式來源 GREEN。
