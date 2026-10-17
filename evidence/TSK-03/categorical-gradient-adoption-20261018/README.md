# 分類梯度正式採用

原本以目標類別的機率減去 1，極小尾端機率會使可表示的負導數變為零。修正將目標導數改為其他類別質量的負值，保留尾數與指數直到權重及平均組合完成。九份入口保留各自的尺度、blank 排除與更新責任，蒸餾標籤導數另加回教師 KL。

[隔離驗收](../categorical-gradient-audit-20261018/isolated-comprehensive-verification.json)的一般／競態各 1,316 pass／14 skip，原 1,190 個結果保持，各新增 140 pass。[採用](adoption.json)包含九份既有呼叫端、一份共用函式及 17 份凍結回歸，固定來源共 895 份。[舊模型驗收](old-models-root-verification.json)確認兩種文字模型設定與三種 OCR 模式的一般／競態載入及下一次訓練輸出逐位元組相同。

ticket147 目前為 4／5。[完整工程驗收](full-engineering-root-verification.json)確認 session96150 已結束且 exit0，九項工程步驟全部通過。一般 8,917 pass／23 skip、競態 8,916 pass／24 skip，各 59 套件通過，原 8,800 個結果保持，各新增 140 pass。兩次文件驗收失敗及修正原因均保留，沒有放寬測試。1,880,270,157 bytes 自有暫存已[清除](cleanup.json)，原模型、資料、其他工作及原始模組均由 Root [核對保持](cleanup-root-verification.json)。提交交付待完成。

[清理控制](cleanup-guard-v3-root-verification.json)的 10 個情境及必要回條缺漏時的拒絕均通過。清理前已[封存](work-preservation.json)149 個必要非快取檔案。OCR 與蒸餾純標籤的回報誤差被算成零另有[實證](../../TSK-01/reported-loss-audit-20261018/README.md)，本票導數修正沒有處理這兩項回報缺陷。
