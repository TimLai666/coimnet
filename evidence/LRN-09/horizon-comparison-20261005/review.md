# 六步任務期限對照審查

審查發現一項 P2 證據核對缺口，已以失敗測試重現並修正。既有訓練報告未修改；研究結果仍為期限組一個種子通過、另外兩個未通過。

## 分工與實際工具

宿主為 Codex desktop，macOS 27.0.1、darwin/arm64。本輪依使用者指定的 agent-delegation 新規則使用外部 CLI，測試與實作分派給不同執行者，主 agent 審查實際檔案與重新驗證。

| 工作 | 工具與設定 | 實際結果 |
| --- | --- | --- |
| 測試第一派 | OpenCode，`opencode/big-pickle --agent build --format json` | 沒有建立測試檔；文字聲稱改動不作完成證據，主 agent 拒收。 |
| 測試第二派 | OpenCode，`opencode/space-bunny-free --agent build --format json` | 建立測試；持續改寫行數時由主 agent 中止，退出碼 130。主 agent 整理獨立失敗案例、拷貝檢查，親自確認空骨架全部紅燈。 |
| 實作第一派 | OpenCode，`opencode/space-bunny-free --agent build --format json` | 沒有填入骨架；不作完成證據。 |
| 實作第二派 | OpenCode，`opencode/muse-spark-1.3-contributor-free --agent build --format json` | 填入 helper，沒有修改測試；主 agent 審查後刪除重複數值檢查，補最佳化器步數核對，再重跑。 |
| 審查第一派 | Antigravity，`agy --model claude-opus-4-6-thinking --mode plan --print-timeout 5m --output-format json` | 退出碼 3，`RESOURCE_EXHAUSTED`（429）；`Individual quota reached. ... Resets in 138h45m15s.`，未完成審查。 |
| 審查備援 | Claude CLI，`claude -p --model opus --permission-mode plan --allowedTools 'Read,Grep,Glob,Bash' --disallowedTools 'Write,Edit,Agent,Task' --output-format json` | 退出碼 0。實際 modelUsage 為 `claude-opus-5-5`，不是前一個工具指定的 4.6。沒有再分派 agent。 |

上述工具沒有額外指定不支援的 reasoning effort。CLI 成功退出不等於檔案已完成，主 agent 以 diff 與實際驗證判斷。

## 確認缺陷與修正

原 Python 核對只比較兩組各種子第一回合的最後一步。這三回合使用相同初始模型、環境與取樣串流，整份原收集紀錄都應相同。審查者指出，改動早期 `value` 或 `log_prob` 並重算目標及損失，會被錯誤接受。主 agent 獨立重現兩種竄改，再先新增六個回歸案例。

- [first-collection-red.log](first-collection-red.log)：三個種子乘兩個欄位，共六個案例在修正前失敗。
- [first-collection-green.log](first-collection-green.log)：恢復期限組的原最後一步後，比較完整 rollout（含初始狀態與 policy version），六個案例全部通過。
- [final-verifier-tests.log](final-verifier-tests.log)：完整四項測試，包括原十六種與新增六種竄改，以及手算與完整儲存報告核對。

另一項待查事項是新 Python 核對程式沒有列入 Go 來源清單。交付的 [verification.json](verification.json) 將三個新 Python 程式與其測試、日誌、報告納入 artifact SHA-256，並由 [check_delivery.py](check_delivery.py) 逐檔核對。Go 來源清單仍只代表 Go、shell 與模組來源，沒有宣稱涵蓋所有 Python。

審查者原先聲稱所有 Python 已列入 source 清單，與清單內容不符；主 agent 已更正。審查者認為期限組後續神經數值只靠雙程序重現支撐也不完整：Go 的 `checkPPOTrainingPolicy` 在每次更新前重播完整原回合，核對神經輸出、機率、價值與後續值；Python 不重建神經前向與 PCG，這個限制仍保留。

## 主 agent 核對

完整審查三個新 Go 測試檔、獨立 Python 核對與測試、文件及證據範圍。手算終止目標可區分真正結束與逾時後續值；原始回合、觀察及神經狀態不共用可變切片。成功計分讀原環境旗標，不使用訓練 Done。錯誤與取消不回傳半套成果。原組全部 600 更新及 39 評估條件與既有來源相同，新組沒有事後調參。

判準維持全部三個種子通過才算整體通過。seed 2 的固定最大分數行動全部成功，但取樣左側只成功 7／20，仍明確記為未通過。seed 3 的左右、提示清除／反轉、前後改善與隨機控制全部通過。兩次相同種子的執行只證明重現，不增加獨立訓練樣本。

## 審查副作用

備援審查被要求唯讀，但曾寫入 `/tmp/x_src`，因此不宣稱審查全程唯讀。主 agent 確認它的 646 行完全等於來源清單中的路徑，SHA-256 為 `14bb5f01520f1dec5560c111165dd832ad89201f4de6d9016a6036ef9ddcc558`，才依內容相符條件刪除。最初較窄的清理條件未通過，該次沒有刪除。儲存庫內沒有因此改動其他檔案。

## 減法與界線

沒有新增公開選項、訓練器或模型架構，沒有參數搜尋。沿用既有收集、更新、環境重播與提示判準；修正只補原本可核對的第一回合一致性。既有格式索引缺漏、TSK-11 真實任務與 OPS-05 平台缺口不在本對照的驗收範圍。
