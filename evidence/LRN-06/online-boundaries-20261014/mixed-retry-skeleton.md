# 混合回饋重試修正骨架

根因：Update 在梯度（含間隔 skip／未滿累積窗）完成之後才執行塑性階段；塑性失敗保留 feedback，但沒有保存已完成的梯度階段，retry 再從頭執行。

## 格子與共用契約

- Update(ctx, now) (UpdateReport,error)：仍依收到順序處理到期 feedback。已完成部分不回滾，回報只計本次實際套用與消耗。
- apply(ctx, fb, report) error：對同一 action/source 保存梯度階段是否完成。階段失敗不提交完成狀態；合法 skip、部分累積或成功更新都只前進一次。塑性失敗保留 feedback 及階段，retry 只跑塑性。完全完成後移除該筆內部階段記錄。
- 責任：正式來源限 learning/online.go；沒有公開 API、保存 schema、規則公式或機制新增。OnlineLearner 未提供 checkpoint，私人階段記錄與既有 queue 同生命週期。

## 預期結果與驗收

1. UpdateEvery=1，AccumulateSteps=1/2，reward gate 首次失敗：第一次 gradient 保留、Consumed=0；retry GradientSteps=0、PlasticSteps=1、Consumed=1，Optimizer 與基礎參數保持第一次成功結果。
2. UpdateEvery=2，第二筆 gradient 後 plastic 失敗：retry 不改間隔，第三筆仍是合法 skip。
3. UpdateEvery=2，第一筆 skip 後 plastic 失敗：retry 不提前變成 gradient。
4. gate 回傳非有限值令 AdvanceGated 拒絕：與 gate 本身 error 同樣只重試 plastic。
5. 純 gradient 失敗：既有 ticket84 refusal 與 retry 行為保持。純 plastic、Evaluate、延後 feedback、多來源及正常混合結果保持。
6. 所有條件先在原來源重現。Root 凍結測試後由不同角色實作，完整工程及治理驗證、Root 審查、清理、交付。

## 減法審查

建議保留最小的私人階段標記，並在消耗後立即移除。不要複製整個 Individual 或引入全交易回滾。
