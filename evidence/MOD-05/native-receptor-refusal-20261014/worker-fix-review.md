# Root 修正唯讀契約審查

## CONFIRMED

- **沒有確認到新的具體漏洞。** `git diff -- simulate/plasticity.go` 顯示 Root 只改了契約註解與 `validatePlasticity` 的共用拒絕，沒有改公式、JSON tag、schema 版本、runner 狀態或合法參數。正式檔案目前 SHA256 為 `7396988824a604b4fb126470d12675e9dcf196496ce3ae308090a44db9073413`。
- **四個原生入口沒有可見繞過。** `simulate/protocol.go:190-200` 的 `DecodeProtocol` 會呼叫 `Protocol.Validate`；`simulate/runner.go:104-114` 的 `Build` 先走 `buildPreflight`，`simulate/runner.go:119-146` 與 `154-169` 兩處都呼叫 `Protocol.Validate`。`simulate` 正式碼唯一的 `Runner` 建構在 `runner.go:214`，位於 `BuildVariant` 已通過驗證之後。`validatePlasticity` 的新拒絕在 `simulate/plasticity.go:119-121`，因此 gate、window、both、index 0/100 以及 Frozen 都經同一檢查。
- **拒絕矩陣已由回條涵蓋。** `simulate/receptor_binding_test.go:16-45` 建立 continuous/hebbian、LIF/hebbian、LIF/stdp 三組；`56-110` 枚舉三種引用、index 0/100、Frozen false/true 及 Validate/DecodeProtocol/Build/BuildVariant，共 144 個公開拒絕。`green.json:1` 的回條為 exit 0、168 個 test pass、0 fail；讀取 `green.log` 的 pass 事件可分出 144 個拒絕子測試、14 個合法控制子測試及 7 個錯誤優先序子測試，另有三個父測試。RED 回條 `red.json:1` 則記錄修正前 145 fail、23 pass，符合失敗先凍結的證據。
- **輸入、錯誤優先序與拒絕文字符合契約。** `validatePlasticity` 先於 `plasticity.New` 完成結構、通道、注入、尺度與核心規則檢查，再於 `simulate/plasticity.go:116-121` 先完成既有 Rule 數值驗證、才拒絕非 nil 受體。`plasticity/plasticity.go:148-225` 保持既有數值檢查順序；新錯誤同時含 `receptor`、`chemistry`、`native`。`receptor_binding_test.go:160-188` 的 7 個控制案例保留負 index、非法 window、edges、channel、injection、core 的較早錯誤，且確認 caller JSON 未被改動。拒絕分支只讀取 pointer，不會把 `plasticity.New` 的副本或輸入改回 caller。
- **合法原生模型結果沒有觀察到變化。** `receptor_binding_test.go:112-158` 的 14 個控制涵蓋兩個核心、三個合法 rule、Frozen false/true、刺激 gate 0/0.3、無 plasticity block，並檢查 Protocol JSON 與 hash、學習狀態及報告。`normal-old.log`、`normal-fixed.log`、`normal-fresh.log` 各讀到相同的 14 個 `NATIVE_CASE`；三份 canonical payload SHA256 都是 `cbbbe9b41954dd47ed3fbb30b46a1eece92e7a850cb12eb6f9c5bd6b9b909bf1`，兩兩相等。payload 也保留 `coimnet-simulate-protocol/v1` 與 `coimnet-simulate-run/v1`。
- **原生公式仍是刺激 gate 與固定衰退。** `simulate/runner.go:761-778` 仍以 `stimulus[r.plasticGate]` 呼叫 `plasticity.Model.Step`，沒有呼叫 `StepWith` 的化學覆寫。`plasticity/plasticity.go:330-363,381-419` 的 `StepWith` 化學入口仍存在，表示 Root 沒有刪掉共用化學能力，只拒絕沒有 chemistry binding 的 native 宣告。
- **learning 化學路徑的正式來源未被這個修正改動。** `learning/individual.go:316-357,747-811` 仍以 `checkReceptorRule`、受體 occupancy、`GateFor`/`WindowFor` 及 `StepWith` 處理合法化學引用；`learning/chemistry.go:291-316` 仍在替換 chemistry 前檢查既有 rule。這些檔案 SHA256 與修正前一致。這是 live source 的契約確認，不把本輪 native 回條冒充 learning package 的執行結果。

## NEEDS INVESTIGATION

- 本輪唯讀審查沒有執行 Go；`green.json`、`normal-*.json` 與日誌是既有 Root 回條，不是本 worker 的執行。沒有 `go test ./...`、race、vet 或獨立 learning chemistry 執行回條，不能把它們宣稱已驗證。
- `ticket 89` 的 gain policy、`ticket 91` 指標所有權及 pre/post learning 時點均未納入判斷，依 plan 保持原狀。

## 唯一修改檔

- `/Users/timlai/Developer/coimnet/runs/.tmp/native-receptor-refusal-20261014/review/fix-review.md`（本檔新增；沒有修改 `report.md`、正式來源或測試。）

## 讀取 SHA256／HEAD

- HEAD：`e2dabd1f5f1c296ccbb1983564dcefbfc3368a5b` (`docs: close receptor ownership verification and delivery`)
- `simulate/plasticity.go`: `7396988824a604b4fb126470d12675e9dcf196496ce3ae308090a44db9073413`
- `simulate/receptor_binding_test.go`: `429c11dfd1abd4f1b7850d6a1e4176fdd0e0f012634cc1c983f1f05298cfdd29`
- `simulate/protocol.go`: `3f5e374b3811ff69bf90dd9b3f8548f511f4f973360e05114ab9ed3f0b037d3d`
- `simulate/runner.go`: `2fe1741d7f55939ae66a67bca5d10d5c03756bec996e05f8e16a91693502450e`
- `plasticity/plasticity.go`: `ab73f4e5e448b8299fbd46aa2cf9ac0f81085298b5b0a8c8b865c2e15ae3ebb0`
- `learning/individual.go`: `531d3ebba6829853a2b8a3eefaf81f744668a281c4afd926170898b5c88486d9`
- `learning/chemistry.go`: `4e6164cbafd843c56c357558e9b8a580598b607daeccffbfc74a714052a52032`
- `docs/tickets/92-native-receptor-declaration-refusal.md`: `7b644ff24a35e83873687ba31145cd908991f545d36fdc978be9255897d72439`
- `evidence/MOD-05/native-receptor-refusal-20261014/plan.md`: `fc228536f2ee55e9b5cf7bf11fdd119faa52739347a718e85480875cb9a0a8d3`
- `red.json`: `6c9b8ed3e0a6363b3d899b6f4228a3668d79c7c53a66e9cfb2028a3937f6f71d`
- `green.json`: `3ab008459f41f9dff2bf8f09908fd3fe65a9f18023ca312a8401357d9bdca65c`
- `normal-old.json`: `bfc73d42120520f742e3b8382097a5af6d75df41b78364d5bc75be5617c47ac5`
- `normal-fixed.json`: `426308206bb76e2103f49506208a5c8c0756d8d957f5fdefbff2e7acb29db64a`
- `normal-fresh.json`: `61837ace6fae5ed5fcc617c60f983bf5a5b9b40c7be2ee77c401ca32663279c8`
- `red.log`: `dbcbf155f525016858471bce33a1249f2ace08226f0c1ccfb1d9ae50038e4c61`
- `green.log`: `031b0466db9badc6cf1fd82c8959007f100b042f1ac5a357b9a40ab8e9f932f5`
- `normal-old.log`: `801b76c9fe3557fbdf0c9846caafe2d517b0aee06b0653cf81d08e4c8ab16d24`
- `normal-fixed.log`: `588609b0674e2289e0b8d3766611fa32993bdcc32ce9416af13456d4c46c23f`
- `normal-fresh.log`: `03b3a0a4ea8a943133fbad9f6cc843c5f6f3fb39ca80c7c6ebb0e0ec61c46de9`
- `source-red.json`: `cd6d6e7f7881687e54339395901a7c49b718e8c005d531a1efee0d7113cd8ea6`
- `source-final.json`: `881c46454b5422a14c7d69abc75802e7183c520cb4e014118f8bd7f4ea73fe6d`

未確認到具體修正漏洞；尚未執行的完整工程與 learning package 驗證維持待查，不視為通過。

Skills：software-engineering-guidelines、diff-inspector
