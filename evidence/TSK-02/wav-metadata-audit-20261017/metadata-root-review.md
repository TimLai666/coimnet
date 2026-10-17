# WAV 欄位與尺寸候選審查

公開原版的五種 metadata 情境均錯誤成功寫入，一般／競態各6fail／5pass，正常單聲道、雙聲道、最大合法block align空檔與既有路徑保留控制通過。另以新私有計算格子的未檢查骨架驗證整數尺寸邊界，一般／競態各7fail／4pass。這個骨架只存在overlay，不宣稱是既有正式函式。

候選沿用原本basic驗證順序。之後才以私有wavOutputBytes檢查格式及native容量，通過後才配置byte buffer與開檔。聲道數仍來自len(Samples)，不是Signal.Channels。正值stride至少為2，因此block align界線也涵蓋聲道欄位，byte rate界線也涵蓋sample rate欄位。payload界線取uint32 RIFF容量減36與native MaxInt減44的較小值，在乘法前以除法核對frames，所以不會先溢位。

Root移除候選中已由呼叫端保證的basic重複檢查，以及由前置界線已保證的重複檢查。最終helper26行。原worker來源與簡化差異均保留，測試完全沒改。header byte rate改用已受界線約束的uint64運算，避免32位元native int中間值溢位。讀取、解析、pcm16及樣本寫入迴圈保持。

候選一般／競態各22pass，單聲道／雙聲道完整golden bytes、合法空檔、既有檔保留及純大整數尺寸參考均通過。沒有配置多GB檔案，也沒有執行32位元Go。11,606bytes隔離工作已清除，三份必要輸入保留，860份正式來源、原模型、資料與其他工作保持。正式WAV檔還沒採用候選，不能把這份隔離結果當成完整工程驗收。

減法審查：只加入一個可單獨驗證的私有尺寸計算，不增加音訊格式、讀取修復、任意實務上限或通用驗證層。正式採用等目前合成音訊凍結來源的驗收與交付完成。
