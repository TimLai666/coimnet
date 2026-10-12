# GPU episode 訓練的裝置更新整合

## 原始需求與範圍

OPS-05 與主規格 §17.4 要求實際稀疏前向／反向、裝置更新與恢復、CPU／獨立參考比對、初始化／編譯／傳輸／暖機／穩態量測及啟動前記憶體檢查。常駐全神經狀態是條件性效能方向，GPU 全圖及第二平台不冒充本票成果。

新增 opt-in float32 裝置 AdamW episode 模式。既有 NewGPUTrainer 的 CPU AdamW 和所有 CPU 路徑保持。編碼／讀出仍沿用 Insyra，稀疏核心仍沿用已實測 adapter。新模式的參數、動量和計數在裝置保持，成功更新後讀回完整候選以維持既有不回錯的 Snapshot 契約和下一次核心前向；目前仍有主機鏡像與稀疏算子的參數傳輸，不宣稱全模型常駐或加速。

## 入口與資料

- backend/webgpu：`StepChecked(ctx context.Context, gradient []float32, mask []bool, rate float32, accept func(AdamWSnapshot) error) (AdamWMetrics, error)`。既有 Step 的正學習率契約保持。StepChecked 允許零學習率，仍更新動量及個別計數，供已存在的排程使用。accept 必須非 nil，收到完全獨立的候選。禁止在 callback 再呼叫同一 optimizer 的鎖定方法。
- learning：`NewGPUDeviceTrainer(ctx context.Context, c Config, p Parameters, o Options, maxOptimizerBytes uint64) (*Trainer, error)`。
- learning：`RestoreGPUDeviceTrainer(ctx context.Context, s TrainingSnapshot, maxOptimizerBytes uint64) (*Trainer, error)`。
- learning：`(*Trainer).GPUAdamWMetrics() (webgpu.AdamWMetrics, bool)`。CPU 與舊 GPUTrainer 回 false。

獨立格子：checked optimizer 在新的 device_adam_checked.go；trainer 建構／恢復／checked-update helper 在新的 gpu_device_trainer.go。既有 execute 在 bank swap 前呼叫候選檢查，Snapshot helper 可讀指定 bank。Trainer 只在既有 AdamW 位置選新模式，不複製損失、累積、裁切或排程流程。

## 狀態與原子性

梯度 → 候選 bank → 所有 chunk flags → 完整候選讀回 → host forward 驗證／更新 norm → 最後 ctx 檢查 → bank swap → 無其他可失敗操作的 host mirror 提交。

callback error、候選 tau 不可用、非有限 forward、取消、讀回或裝置錯誤：bank 不換，host parameters／moments／steps／updates／accumulator 不換。不能先 Step 提交再驗證，不能以還原舊快照／重開裝置作正常 rollback。錯誤後下一次合法更新應與未執行失敗更新的參考相同。

## 支援與精度

第一版沿用 GPU episode 的純量、零延遲 continuous profile。Masks、loss-scale、truncation、accumulation、clip、schedule（包含合法零 rate）可用。Ranges 非 nil、fixed signs 非零與 Sharing 非 nil 在開啟裝置前明確拒絕 ErrGPUTrainerUnsupported，不忽略 projection 或同參數共用。persistent neural state／LIF／mixed／vector／positive delay／recompute 保持明確拒絕。

建構參數與 kernel optimizer 設定明示量化到 float32，拒絕非有限結果、正 epsilon 轉為零、beta 轉為 1、超量预算。snapshot 的實際 kernel options 記成可精確表示的 float64；StepResult.LearningRate 記實際 float32 rate。Restore 不再偷偷量化已訓練狀態：完整參數／動量須可精確 round-trip float32，個別計數須在 uint32，與更新數關係仍依既有 RestoreTrainer 驗證。拒絕只返回錯誤，不退 CPU。

## 錯誤與邊界的驗收歸屬

| 操作／情境 | 行為 | 驗證 |
| --- | --- | --- |
| nil／取消 ctx、nil callback、錯 gradient／mask、負／非有限 rate | 具識別性的錯誤，完全不提交 | checked public regression |
| callback 回錯或取消、跨 chunk 非有限候選 | 保留全部 bank 狀態，後續可重用 | 獨立 Snapshot 與 next-step reference |
| zero rate、部分 freeze、weight decay | unfrozen 動量／計數更新，frozen 全部位元保持 | 獨立 float32 Adam reference |
| unsupported profile／projection／sharing | 啟動前 ErrGPUTrainerUnsupported | constructors／restore public calls |
| 不可表示的 restored state／options／budget | 啟動前拒絕，不重初始化 | corruption cases |
| accumulation 部分窗 | 只改 accumulator，不 dispatch optimizer | metrics、snapshot 與 resume |
| 再次 Close／Close 後訓練 | Close 冪等，後續返回閉合錯誤，鏡像快照可讀 | public lifecycle tests |
| 构造部分成功後 optimizer 打開失敗 | 釋放已開 GPU core，不留下可用半模型 | bounded failure + subsequent create |

## 三條完整使用流程

1. 同一固定稀疏 fixture：CPU、舊 GPUTrainer 與新裝置模式的前向／反向／4次更新對照，另逐值驗證 float32 Adam reference。確認下一次前向使用已更新參數，量测实际device、bytes及所有host phase，報告精度誤差。
2. partial accumulation 保存 → 新程序 RestoreGPUDeviceTrainer → 完成窗與後續更新。完整 parameters／moments／counts／updates／accumulator／predict 與不中斷GPU參考逐位一致；實際使用 checkpoint training bundle，沒有 child hardware skip。
3. late candidate rejection／callback cancel → snapshot 全部不變 → 下一次合法 Step 等於無失敗更新的参照。完成一般／race／vet／module／index／governance，保存 RED、GREEN、環境與來源指紋，清除本票owned暫存。

## 實作與檢查責任

主 agent 寫入口骨架，測試 agent 只改 device_adam_checked_test.go 及 gpu_device_trainer_test.go。主 agent 審測試並在空骨架實機跑 RED 後，分開派 checked optimizer 和 trainer 實作。實作 agent 不得修改任何測試。最終主 agent 完整 diff、實機與全套檢查。沒有通過全部要求前，不修改 OPS-05 passed 狀態。
