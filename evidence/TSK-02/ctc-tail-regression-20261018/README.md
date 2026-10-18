# CTC 原版失敗回歸驗收

Ticket150 完成 2／5。四份測試與驗收條件已凍結，914 份正式 Go／模組來源沒有修改。尚未製作隔離候選，TSK-01／TSK-02 保持 specified。

## 驗收結果

Root 親自執行共用 CTC、OCR 與語音三個套件，再解析完整日誌並與先前同源完整工程逐項比較。

| 建置 | 新增 pass | 新增 fail | 保持的既有 pass |
| --- | ---: | ---: | ---: |
| 一般 | 49 | 35 | 495 |
| 競態 | 49 | 35 | 495 |

28 個參考子案例失敗，包含分數差 16 的案例、極小值與半個最小正值附近。35 fail 另含公開訓練四個子案例及三個父測試。兩種建置的完整測試結果相同，沒有編譯失敗或競態警告。

OCR 的 columns／aligned-columns 及語音 TrainUtterance，在一／兩時間點 `[-40,0]` 的四個控制案例都回傳 loss=-0，目標 Readout 係數 0→0。兩個等分正常控制回傳 ln(2)，係數均為 0→0.0099999998000000027。這些是人工數值 fixture，尚未量測辨識率或長期模型品質。

Root 核對見 [original-all-root-verification-v2.json](original-all-root-verification-v2.json)。原始命令與環境在 root-all-original-unit.json、root-all-original-race.json、command-environment.json 及 environment-go.json，日誌保持原件。

## 固定輸入與精度

[tests-frozen-all.json](tests-frozen-all.json) 固定四份測試、84 個新增事件、來源與獨立參考指紋。38 個小路徑案例來自[獨立枚舉參考](../ctc-tail-audit-20261018/ctc-small-path-reference-v2.json)，誤差及每個導數最多相差四個相鄰 float64 值。參考非零時要求非零且符號相同，真正捨入為零時要求零。這是凍結案例的驗收，沒有新增普遍的公開精度承諾。

37 個 CTC 錯誤處理及正常控制事件、兩個公開正常子案例均通過。涵蓋非法值、形狀、target、空 target、不可對齊策略及原有數值拒絕。輸入不變、報告形狀與清空錯誤結果也在測試內。

公開 learning.Parameters 沒有獨立 ReadoutBias，fixture 使用已有的 Readout 係數，藉正隱藏活動產生精確 logits。Root 已修正原派工的欄位假設。誤差與更新分別用非致命檢查，避免第一項失敗後漏看第二項。

## 分工與歷史輸出

宿主：Codex。opencode models 本次列出七個模型 ID；opencode/big-pickle 的 --agent plan 可用性詢問在 55 秒逾時，未取得 AVAILABLE。該詢問禁止工具且沒有傳送專案來源，沒有完成測試工作。實際測試分工為 Codex 內建 gpt-5.6-luna，reasoning effort=max，依專案的免費模型失敗後備規則執行。

Root 撰寫共用 CTC 測試，測試 agent 只修改兩份公開入口 .go.txt。Root 完整讀取後要求補上獨立更新觀察及真正更新的正常控制，最後親自凍結並執行一般／競態。agent-public-* 是保留的編譯、早期與修訂日誌，不能用早期輸出取代最終 Root 驗收。

## 下一個驗證與減法審查

下一項由不同實作者製作隔離候選，不得修改凍結測試。保留公開介面、模型格式、依賴與不可對齊政策。候選通過後才進行正式採用、舊模型接續、完整工程及效能核對。

共用一份 CTC 修正，不新增 OCR／語音設定。原 914 份來源未變，沿用先前同源完整工程，不為整理證據重跑整套訓練。必要的測試 overlay 已封存，21,531 個自有暫存項目共 1,158,914,368 bytes 已清除，見 [cleanup.json](cleanup.json)。18 份原模型、6 份必要資料、32 份其他工作、15,046 份原始模組及兩個舊暫存目錄保持。下一階段的候選須另建專案內自有暫存，不能沿用已刪除目錄的 inode。
